package ui

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/generate"
	"github.com/jiripisa/tuipe/internal/ghost"
	"github.com/jiripisa/tuipe/internal/incident"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/recommend"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui/roundview"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const (
	tickEvery        = 100 * time.Millisecond
	sampleEvery      = time.Second // throughput sample spacing, in active time
	maxSamples       = 30
	paceFallbackMs   = 400.0 // pace ghost speed for characters without samples
	focusRepeats     = 3     // times a drill shows each of its focus characters
	hintsOffChapter  = 12    // the chapter that tests without hints
	runIDLen         = 7
	dateLayout       = "2006-01-02"
	resumeKindLesson = "lesson"
)

// roundControl lets a controller take over how a round ends. A round without
// one ends in the results screen.
type roundControl struct {
	stop  func(*roundScreen) bool // ends the round early, before the text is done
	done  func(a *App, res play.Result, now time.Time) Screen
	leave func(*App) Screen // where the pause menu's Leave goes
}

func (c *roundControl) shouldStop(r *roundScreen) bool { return c != nil && c.stop(r) }

// tickMsg drives the clock of the round with the given generation.
type tickMsg struct{ gen int }

func tickCmd(gen int) tea.Cmd {
	return tea.Tick(tickEvery, func(time.Time) tea.Msg { return tickMsg{gen} })
}

// stats returns the character statistics of the active layout.
func (a *App) stats() *stats.Stats {
	p := a.Store.Profile()
	s := p.Stats[a.Layout.ID]
	if s == nil {
		s = stats.New()
		p.Stats[a.Layout.ID] = s
	}
	return s
}

// SpecFor builds the text, targets and ghost of a recommended round.
func (a *App) SpecFor(p recommend.Pick) (play.Spec, error) {
	switch p.Kind {
	case recommend.Lesson:
		return a.lessonSpec(p.LessonID)
	case recommend.Code:
		return a.codeSpec(p.SnippetID)
	case recommend.Incident:
		inc, ok := a.incidentByID(p.IncidentID)
		if !ok {
			return play.Spec{}, fmt.Errorf("unknown incident %q", p.IncidentID)
		}
		return a.incidentSpec(inc, incident.SEV3)
	}
	return a.drillSpec(p), nil
}

func (a *App) lessonSpec(id string) (play.Spec, error) {
	for _, ch := range a.Curr.Chapters() {
		for _, l := range ch.Lessons {
			if l.ID != id {
				continue
			}
			txt, err := a.Curr.LessonText(id)
			if err != nil {
				return play.Spec{}, err
			}
			spec := play.Spec{
				Kind: play.KindLesson, Title: l.Title, LessonID: id, Text: txt,
				Targets:  a.Curr.Targets(id),
				HintsOff: ch.Number == hintsOffChapter,
			}
			return a.withReplay(spec)
		}
	}
	return play.Spec{}, fmt.Errorf("unknown lesson %q", id)
}

func (a *App) codeSpec(id string) (play.Spec, error) {
	i := slices.IndexFunc(a.Lib.Snippets, func(s content.Snippet) bool { return s.ID == id })
	if i < 0 {
		return play.Spec{}, fmt.Errorf("unknown snippet %q", id)
	}
	sn := a.Lib.Snippets[i]
	spec := play.Spec{
		Kind: play.KindCode, Title: sn.ID, Lang: sn.Lang, Text: sn.Text,
		Targets: a.chapterTargets(),
	}
	return a.withReplay(spec)
}

// drillSpec generates the text and races a pace ghost computed from the user's timing.
func (a *App) drillSpec(p recommend.Pick) play.Spec {
	cur := a.Curr.CurrentChapter(a.Store.Profile())
	txt := generate.Cover(a.Lib, generate.Input{
		Focus: p.Focus, Allowed: a.Curr.AllowedAt(cur.ID), Seed: p.Seed,
	}, p.Focus, focusRepeats)
	return play.Spec{
		Kind: play.KindDrill, Title: text.DrillTitle, Text: txt,
		Targets:     a.chapterTargets(),
		Ghost:       ghost.Pace(engine.Prepare(txt), a.stats(), paceFallbackMs),
		GhostIsPace: true,
	}
}

// chapterTargets are the targets of the last lesson of the current chapter,
// which generated and code rounds are measured against.
func (a *App) chapterTargets() score.Targets {
	cur := a.Curr.CurrentChapter(a.Store.Profile())
	if n := len(cur.Lessons); n > 0 {
		return a.Curr.Targets(cur.Lessons[n-1].ID)
	}
	return score.Targets{}
}

// withReplay attaches the stored best attempt on a fixed text as the ghost.
func (a *App) withReplay(spec play.Spec) (play.Spec, error) {
	g, err := a.Store.Ghost(engine.Prepare(spec.Text).Hash)
	if err != nil {
		a.SaveErr(err)
		return spec, nil // a damaged ghost never blocks the round
	}
	if g != nil {
		spec.Ghost = ghost.Replay(g.TimesMs)
	}
	return spec, nil
}

// recommended picks the next round for the quick run.
func (a *App) recommended() recommend.Pick {
	seed := uint64(a.Now().UnixNano())
	return recommend.Next(recommend.State{
		Curr: a.Curr, Lib: a.Lib, Profile: a.Store.Profile(), Stats: a.stats(),
		RunCount: a.runCadence(), Rand: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)), Now: a.Now(),
	})
}

// quickRun is the next-round function of the quick run: the recommended round.
func quickRun(a *App) Screen { return NewRoundScreen(a, a.recommended(), quickRun) }

// roundScreen is one round being typed.
type roundScreen struct {
	spec  play.Spec
	next  func(*App) Screen
	story *story        // set for an incident
	ctl   *roundControl // set when a controller such as the placement test owns the round's end
	sess  *play.Session
	gen   int
	alive bool // a tick is in flight
	now   time.Time

	menu      pauseMenu
	resumeSet bool // Profile.Resume names this lesson
	left      bool // Leave has run
	samples   []float64
	sampled   int
}

// NewRoundScreen builds the screen for a recommended round. When the round
// cannot be built it returns the home screen, with a warning on the status line.
func NewRoundScreen(a *App, pick recommend.Pick, next func(*App) Screen) Screen {
	if pick.Kind == recommend.Incident {
		inc, ok := a.incidentByID(pick.IncidentID)
		if !ok {
			a.SaveErr(fmt.Errorf("unknown incident %q", pick.IncidentID))
			return NewHomeScreen(a)
		}
		return NewSeverityScreen(a, inc, next, nil) // SEV3 is selected: Enter starts a quick run
	}
	spec, err := a.SpecFor(pick)
	if err != nil {
		a.SaveErr(err)
		return NewHomeScreen(a)
	}
	return newRound(a, spec, next)
}

// newRound builds the screen for a round whose spec is already made.
func newRound(a *App, spec play.Spec, next func(*App) Screen) Screen {
	r := &roundScreen{spec: spec, next: next}
	r.begin(a)
	return r
}

// begin starts a fresh session and invalidates ticks of the previous one.
func (r *roundScreen) begin(a *App) {
	a.tickGen++
	r.gen = a.tickGen
	r.alive = false
	r.now = a.Now()
	r.sess = play.New(r.spec, a.Layout, a.stats(), a.focusEvents())
	r.menu = pauseMenu{}
	r.resumeSet = false
	r.samples, r.sampled = nil, 0
}

// Init starts the tick of the round once.
func (r *roundScreen) Init(*App) tea.Cmd {
	if r.alive {
		return nil
	}
	r.alive = true
	return tickCmd(r.gen)
}

// Leave saves the lesson to continue when the app quits mid-round.
func (r *roundScreen) Leave(a *App) {
	if !r.left {
		r.left = true
		r.abandon(a, r.now)
	}
}

func (r *roundScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	now := a.Now() // one reading per update
	r.now = now
	switch m := msg.(type) {
	case tickMsg:
		if m.gen != r.gen {
			return r, nil
		}
		r.sess.Tick(now)
		r.sample(now)
		if r.sess.Done() {
			return r.finish(a, now), nil
		}
		return r, tickCmd(r.gen)
	case tea.WindowSizeMsg:
		if a.tooSmall() { // below 40 x 8 the round waits for more room
			r.sess.Pause(now)
		}
	case tea.FocusMsg:
		r.sess.Focus(true, now)
	case tea.BlurMsg:
		r.sess.Focus(false, now)
	case tea.KeyPressMsg:
		return r.key(m, a, now)
	}
	return r, nil
}

func (r *roundScreen) key(m tea.KeyPressMsg, a *App, now time.Time) (Screen, tea.Cmd) {
	if r.sess.Frame(now).ManualPause {
		switch r.menu.key(m) {
		case pauseResume:
			r.sess.Resume(now)
			r.menu = pauseMenu{}
		case pauseRestart:
			return r, r.restart(a, now)
		case pauseLeave:
			r.abandon(a, now)
			if r.ctl != nil {
				return r.ctl.leave(a), nil
			}
			return NewHomeScreen(a), nil
		}
		return r, nil
	}
	switch {
	case m.Code == tea.KeyEscape:
		r.sess.Pause(now)
		return r, nil
	case m.Mod.Contains(tea.ModCtrl) && m.Code == 'r':
		return r, r.restart(a, now)
	case m.Mod.Contains(tea.ModCtrl) || m.Mod.Contains(tea.ModMeta):
		return r, nil // a shortcut, never typed input
	}
	alt := m.Mod.Contains(tea.ModAlt)
	switch {
	case m.Code == tea.KeyEnter:
		r.sess.Key(play.Input{Rune: '\n', Alt: alt}, now)
	case m.Text == "" && alt && unicode.IsPrint(m.Code):
		// The terminal decoder drops Text for Alt chords.
		r.sess.Key(play.Input{Rune: m.Code, Alt: true}, now)
	default:
		for _, c := range m.Text {
			r.sess.Key(play.Input{Rune: c, Alt: alt}, now)
		}
	}
	r.rememberLesson(a, now)
	r.sample(now)
	if r.sess.Done() || r.ctl.shouldStop(r) {
		return r.finish(a, now), nil
	}
	return r, nil
}

// restart records the keys so far as practice and starts the same text again.
// It is not a failure: nothing is added to the run history.
func (r *roundScreen) restart(a *App, now time.Time) tea.Cmd {
	r.feedStats(a, now)
	r.countPartial(a, now)
	r.begin(a)
	return r.Init(a)
}

// rememberLesson writes Profile.Resume at the first keystroke of a lesson, so
// a closed window or a killed process still offers to continue it.
func (r *roundScreen) rememberLesson(a *App, now time.Time) {
	if r.resumeSet || (r.spec.LessonID == "" && a.chain == nil) {
		return
	}
	if sprintResume(a.Store.Profile().Resume) && (a.chain == nil || a.chain.state == nil) {
		return // a Sprint left earlier outranks the lesson of this round
	}
	if _, log := r.sess.PartialLog(); len(log) == 0 {
		return
	}
	r.resumeSet = true
	if a.chain != nil { // a Sprint continues as a whole, not as this lesson
		a.chain.started(a)
		return
	}
	a.Store.Profile().Resume = &store.Resume{
		Kind: resumeKindLesson, LessonID: r.spec.LessonID, Day: now.Local().Format(dateLayout),
	}
	a.SaveErr(a.Store.SaveProfile())
}

// abandon feeds the keys so far into the statistics. It adds no run and keeps
// Resume as rememberLesson set it.
func (r *roundScreen) abandon(a *App, now time.Time) {
	if r.sess.Done() {
		return
	}
	r.feedStats(a, now)
	r.countPartial(a, now)
	a.SaveErr(a.Store.SaveProfile())
}

// countPartial gives the chain the active time of keys that did not become a run.
func (r *roundScreen) countPartial(a *App, now time.Time) {
	if _, log := r.sess.PartialLog(); len(log) > 0 {
		a.chain.partial(a, r.sess.Result(now).Metrics.Duration)
	}
}

func (r *roundScreen) feedStats(a *App, now time.Time) {
	t, log := r.sess.PartialLog()
	if len(log) > 0 {
		a.stats().Record(t, log, now)
	}
}

// finish turns a finished session into the results screen.
func (r *roundScreen) finish(a *App, now time.Time) Screen {
	if r.ctl != nil {
		return r.ctl.done(a, r.sess.Result(now), now)
	}
	if r.story != nil {
		return NewIncidentResultsScreen(a, r.sess.Result(now), r.spec, r.story, r.next)
	}
	return NewResultsScreen(a, r.sess.Result(now), r.spec, r.next)
}

// sample adds one throughput sample per second of active time, so a pause adds none.
func (r *roundScreen) sample(now time.Time) {
	active := r.sess.Result(now).Metrics.Duration
	if r.sampled >= int(active/sampleEvery) {
		return
	}
	wpm := r.sess.Frame(now).WPM
	for r.sampled < int(active/sampleEvery) {
		r.sampled++
		r.samples = append(r.samples, wpm)
	}
	if len(r.samples) > maxSamples {
		r.samples = r.samples[len(r.samples)-maxSamples:]
	}
}

func (r *roundScreen) View(a *App, w, h int) string {
	f := r.sess.Frame(r.now)
	v := roundview.View{
		Frame: f, Title: r.spec.Title, Lang: r.spec.Lang, Layout: a.Layout,
		Stars: a.Store.Profile().Stars[r.spec.LessonID], Throughput: r.samples,
	}
	active := r.sess.Result(r.now).Metrics.Duration
	if r.story != nil {
		v.Prompt, v.Lang, v.RightInfo = r.story.view(f.Pos, active)
	}
	if label := a.chain.label(active); label != "" {
		v.RightInfo = strings.TrimSpace(label + "  " + v.RightInfo)
	}
	view := roundview.Render(a.Style, v, w, h)
	if f.ManualPause {
		return r.menu.overlay(a.Style, view, w, h)
	}
	return view
}

// runCadence counts the runs that set the rhythm of incidents. Placement runs
// are no part of it.
func (a *App) runCadence() int {
	runs, _ := a.Store.Runs() // a damaged history only affects the incident rhythm
	n := 0
	for _, r := range runs {
		if r.Mode != "placement" {
			n++
		}
	}
	return n
}
