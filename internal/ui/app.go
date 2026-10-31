// Package ui is the terminal front end: the application shell and its screens.
package ui

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/curriculum"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const (
	defaultStyle  = style.Editor
	defaultLayout = "macos-czech-qwerty"

	minW, minH = 40, 8 // below this the app pauses and asks for more room
)

// Screen is one page of the application.
type Screen interface {
	Update(msg tea.Msg, a *App) (Screen, tea.Cmd)
	View(a *App, w, h int) string
}

// App is the Bubble Tea model: shared state, global keys and the current screen.
type App struct {
	Store  *store.Store
	Layout *layout.Layout
	Lib    *content.Library
	Curr   *curriculum.Curriculum
	Style  style.Style
	Now    func() time.Time
	W, H   int

	screen    Screen
	saveWarn  string
	Entries   []menuEntry // home menu entries; later screens register theirs
	focusSeen bool        // a focus message has arrived in this process
	tickGen   int         // identifies the running round tick; older ticks are dropped
	chain     *chain      // the Standup or Sprint being played; nil outside one
	update    updateCheck // the daily look for a newer release

	// UpdateNotice is the quiet line about a newer version, shown on the home
	// screen when it is not empty.
	UpdateNotice string
}

// initer is implemented by a screen that needs a command when it appears,
// such as the round tick. Init must be safe to call on every update.
type initer interface{ Init(a *App) tea.Cmd }

// leaver is implemented by a screen with work to save when the app quits.
type leaver interface{ Leave(a *App) }

// Option configures New.
type Option func(*App)

// WithClock sets the clock the app reads. It applies from the first screen on.
func WithClock(now func() time.Time) Option { return func(a *App) { a.Now = now } }

// New builds the app from an open store and the loaded content. The style and
// layout come from the config, with defaults when it is empty. The clock
// defaults to the wall clock.
func New(st *store.Store, lib *content.Library, opts ...Option) (*App, error) {
	cfg := st.Config()
	layoutID := cfg.Layout
	if layoutID == "" {
		layoutID = defaultLayout
	}
	all, err := loadLayouts(st)
	if err != nil {
		return nil, err
	}
	var lay, def *layout.Layout
	for _, l := range all {
		if l.ID == layoutID {
			lay = l
		}
		if l.ID == defaultLayout {
			def = l
		}
	}
	notice := ""
	if lay == nil { // a removed layout file must never stop the app from starting
		if def == nil {
			return nil, fmt.Errorf("missing built-in layout %q", defaultLayout)
		}
		lay, notice = def, fmt.Sprintf(text.UnknownLayout, layoutID)
	}
	styleID := style.ID(cfg.Style)
	if styleID == "" {
		styleID = defaultStyle
	}
	a := &App{
		Store: st, Layout: lay, Lib: lib,
		Curr:  curriculum.New(lib, lay),
		Style: style.ByID(styleID),
		// Round(0) strips the monotonic reading, so the wall clock is compared
		// and a gap while the computer slept is visible.
		Now:    func() time.Time { return time.Now().Round(0) },
		update: defaultUpdateCheck(),
	}
	for _, o := range opts {
		o(a)
	}
	a.saveWarn = notice
	a.Entries = defaultEntries()
	a.RegisterEntry(EntryStandup, openStandup)
	a.RegisterEntry(EntrySprint, openSprint)
	a.RegisterEntry(EntryCareer, func(a *App) (Screen, tea.Cmd) { return NewCareerScreen(a), nil })
	a.RegisterEntry(EntryIncident, func(a *App) (Screen, tea.Cmd) { return NewIncidentListScreen(a), nil })
	a.RegisterEntry(EntryPractice, func(a *App) (Screen, tea.Cmd) { return NewPracticeScreen(a), nil })
	a.RegisterEntry(EntryStats, func(a *App) (Screen, tea.Cmd) { return NewStatsScreen(a), nil })
	a.RegisterEntry(EntrySettings, func(a *App) (Screen, tea.Cmd) { return NewSettingsScreen(a), nil })
	a.screen = NewHomeScreen(a)
	if !st.Profile().FirstRunDone {
		a.screen = NewFirstRunScreen(a)
	}
	return a, nil
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd { return a.initUpdate() }

func (a *App) tooSmall() bool { return a.W < minW || a.H < minH }

// Update implements tea.Model. Ctrl+T and Ctrl+C are global; everything else
// goes to the screen, except keys while the window is too small.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.W, a.H = m.Width, m.Height
	case tea.KeyPressMsg:
		switch m.String() {
		case "ctrl+c":
			return a, tea.Quit
		case "ctrl+t":
			a.cycleStyle()
			return a, nil
		}
		if a.tooSmall() {
			return a, nil
		}
	case tea.FocusMsg, tea.BlurMsg:
		a.noteFocusEvents()
	case updateCheckedMsg: // global: whichever screen is on top is left alone
		return a, a.updateChecked(m)
	case UpdateAvailableMsg:
		a.showUpdate(m.Version)
		return a, nil
	}
	next, cmd := a.screen.Update(msg, a)
	a.screen = next
	if i, ok := next.(initer); ok {
		cmd = tea.Batch(cmd, i.Init(a))
	}
	return a, cmd
}

// terminalKey identifies the terminal, because focus reporting is a property
// of the terminal and not of the user.
func terminalKey() string {
	return os.Getenv("TERM_PROGRAM") + "|tmux=" + strconv.FormatBool(os.Getenv("TMUX") != "")
}

// noteFocusEvents remembers that this terminal reports focus changes, so later
// rounds do not pause on idle.
func (a *App) noteFocusEvents() {
	a.focusSeen = true
	if p := a.Store.Profile(); !p.FocusTerminals[terminalKey()] {
		p.FocusTerminals[terminalKey()] = true
		a.SaveErr(a.Store.SaveProfile())
	}
}

// focusEvents reads the knowledge again for a new session.
func (a *App) focusEvents() bool {
	return a.focusSeen || a.Store.Profile().FocusTerminals[terminalKey()]
}

// Shutdown saves what a screen has in flight and the profile. Run calls it
// after the program ends, however it ended.
func (a *App) Shutdown() {
	if l, ok := a.screen.(leaver); ok {
		l.Leave(a)
	}
	a.SaveErr(a.Store.SaveProfile())
}

func (a *App) cycleStyle() { a.setStyle(style.Next(a.Style.ID)) }

// setStyle switches the look at once and saves the choice.
func (a *App) setStyle(id style.ID) {
	a.Style = style.ByID(id)
	cfg := a.Store.Config()
	cfg.Style = string(a.Style.ID)
	a.SaveErr(a.Store.SaveConfig(cfg))
}

// setLayout switches the keyboard layout for the rest of the session and saves
// the choice. The curriculum is resolved against the layout, so it is rebuilt,
// and a Standup or Sprint in flight holds the old one, so it is dropped.
// Statistics are kept per layout id and need no change.
func (a *App) setLayout(l *layout.Layout) {
	a.Layout = l
	a.Curr = curriculum.New(a.Lib, l)
	a.chain = nil
	cfg := a.Store.Config()
	cfg.Layout = l.ID
	a.SaveErr(a.Store.SaveConfig(cfg))
}

// SaveErr shows a warning for a failed save, or clears it for nil. It never panics.
func (a *App) SaveErr(err error) {
	if err == nil {
		a.saveWarn = ""
		return
	}
	a.saveWarn = fmt.Sprintf(text.SaveWarning, err)
}

// View implements tea.Model.
func (a *App) View() tea.View {
	var body string
	if a.tooSmall() {
		body = centre(text.TooSmall, a.W, a.H)
	} else {
		body = a.screen.View(a, a.W, a.H)
	}
	if a.saveWarn != "" && a.H > 0 {
		lines := strings.Split(a.Style.Fill(body, a.W, a.H), "\n")
		warn := a.Style.Base().Foreground(a.Style.Bad).Render(ansi.Truncate(a.saveWarn, a.W, "…"))
		lines[len(lines)-1] = a.Style.Paint(warn, a.W)
		body = strings.Join(lines, "\n")
	}
	v := tea.NewView(a.Style.Fill(body, a.W, a.H))
	v.AltScreen = true
	v.ReportFocus = true
	v.BackgroundColor = a.Style.Bg
	return v
}

// centre places a block of text in the middle of a w × h area.
func centre(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	var out []string
	for range max(0, (h-len(lines))/2) {
		out = append(out, "")
	}
	for _, l := range lines {
		out = append(out, strings.Repeat(" ", max(0, (w-ansi.StringWidth(l))/2))+l)
	}
	return strings.Join(out, "\n")
}

// Run opens the store, runs the program and returns when the user quits.
func Run() error {
	cfgDir, dataDir, err := store.Paths()
	if err != nil {
		return err
	}
	st, err := store.Open(cfgDir, dataDir)
	switch {
	case errors.Is(err, store.ErrLocked):
		return errors.New(text.SecondInstance)
	case errors.Is(err, store.ErrNewerData):
		return errors.New(text.NewerData)
	case err != nil:
		return err
	}
	defer st.Close()
	lib, err := content.Load()
	if err != nil {
		return err
	}
	a, err := New(st, lib)
	if err != nil {
		return err
	}
	return runApp(a)
}

// runApp runs the program until it ends. A closed terminal (SIGHUP) ends it
// like a quit, and Shutdown then saves the round in flight.
func runApp(a *App, opts ...tea.ProgramOption) error {
	p := tea.NewProgram(a, opts...)
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-hup:
			p.Quit()
		case <-done:
		}
	}()
	_, err := p.Run()
	a.Shutdown()
	return err
}
