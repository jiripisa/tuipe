package ui

import (
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/ui/roundview"
)

const (
	tickEvery   = 100 * time.Millisecond
	sampleEvery = time.Second // throughput sample spacing, in active time
	maxSamples  = 30
	runIDLen    = 7
	dateLayout  = "2006-01-02"
)

// roundControl says how a round ends. The Trénink, the Test and the placement
// test each bring their own.
type roundControl struct {
	stop  func(*roundScreen) bool // ends the round early, before the text is done; may be nil
	done  func(a *App, res play.Result, now time.Time) Screen
	leave func(*App) Screen // where the pause menu's Leave goes
}

func (c *roundControl) shouldStop(r *roundScreen) bool {
	return c.stop != nil && c.stop(r)
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

// roundScreen is one round being typed.
type roundScreen struct {
	spec  play.Spec
	ctl   *roundControl
	sess  *play.Session
	gen   int
	alive bool // a tick is in flight
	now   time.Time

	menu    pauseMenu
	left    bool // Leave has run
	samples []float64
	sampled int
}

// begin starts a fresh session and invalidates ticks of the previous one.
func (r *roundScreen) begin(a *App) {
	a.tickGen++
	r.gen = a.tickGen
	r.alive = false
	r.now = a.Now()
	r.sess = play.New(r.spec, a.Layout, a.stats(), a.focusEvents())
	r.menu = pauseMenu{}
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

// Leave keeps the keys typed so far when the app quits mid-round.
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
			return r.ctl.leave(a), nil
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

// abandon feeds the keys so far into the statistics. It adds no run.
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

// finish hands the finished session to the round's controller.
func (r *roundScreen) finish(a *App, now time.Time) Screen {
	return r.ctl.done(a, r.sess.Result(now), now)
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
	const noStars = -1 // only a Test keeps stars
	f := r.sess.Frame(r.now)
	v := roundview.View{
		Frame: f, Title: r.spec.Title, Lang: r.spec.Lang, Layout: a.Layout,
		Stars: noStars, Throughput: r.samples,
	}
	if r.spec.Kind == play.KindTest { // a Trénink does not judge
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
