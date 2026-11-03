package ui

import (
	"fmt"
	"slices"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui/roundview"
)

const (
	tickEvery        = 100 * time.Millisecond
	sampleEvery      = time.Second // throughput sample spacing, in active time
	maxSamples       = 30
	hintsOffChapter  = 12 // the chapter that tests without hints
	codeTrendChapter = 11 // from here on rounds count as code in the trends
	runIDLen         = 7
	dateLayout       = "2006-01-02"
	resumeKindLesson = "lesson"
)

// roundControl lets a controller take over how a round ends. A round without
// one ends in the results screen.
type roundControl struct {
	stop  func(*roundScreen) bool // ends the round early, before the text is done; may be nil
	done  func(a *App, res play.Result, now time.Time) Screen
	leave func(*App) Screen // where the pause menu's Leave goes
}

func (c *roundControl) shouldStop(r *roundScreen) bool {
	return c != nil && c.stop != nil && c.stop(r)
}

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
				Kind: play.KindLesson, Title: l.Title, LessonID: id, Text: txt, Lang: a.lessonLang(l),
				Targets:  a.Curr.Targets(id),
				HintsOff: ch.Number == hintsOffChapter,
			}
			return spec, nil
		}
	}
	return play.Spec{}, fmt.Errorf("unknown lesson %q", id)
}

// lessonLang is the language of a lesson made of a snippet; a drill has none.
func (a *App) lessonLang(l content.LessonDef) string {
	if len(l.Snippets) == 0 {
		return ""
	}
	i := slices.IndexFunc(a.Lib.Snippets, func(s content.Snippet) bool { return s.ID == l.Snippets[0] })
	if i < 0 {
		return ""
	}
	return a.Lib.Snippets[i].Lang
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

// roundScreen is one round being typed.
type roundScreen struct {
	spec  play.Spec
	next  func(*App) Screen
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
	r.begin(a)
	return r.Init(a)
}

// rememberLesson writes Profile.Resume at the first keystroke of a lesson, so
// a closed window or a killed process still offers to continue it. A Trénink
// or a Test has nothing to continue.
func (r *roundScreen) rememberLesson(a *App, now time.Time) {
	if r.resumeSet || r.spec.LessonID == "" || isLessonPart(r.spec.Kind) {
		return
	}
	if _, log := r.sess.PartialLog(); len(log) == 0 {
		return
	}
	r.resumeSet = true
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
	a.SaveErr(a.Store.SaveProfile())
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
	const noStars = -1 // only a lesson keeps stars
	f := r.sess.Frame(r.now)
	v := roundview.View{
		Frame: f, Title: r.spec.Title, Lang: r.spec.Lang, Layout: a.Layout,
		Stars: noStars, Throughput: r.samples,
	}
	if r.spec.LessonID != "" && r.spec.Kind != play.KindTraining { // a Trénink does not judge
		v.Stars = a.Store.Profile().Stars[r.spec.LessonID]
	}
	if f.AltWarning {
		v.AltHelp = altHelp()
	}
	view := roundview.Render(a.Style, v, w, h)
	if f.ManualPause {
		return r.menu.overlay(a.Style, view, w, h)
	}
	return view
}
