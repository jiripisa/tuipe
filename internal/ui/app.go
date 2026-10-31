// Package ui is the terminal front end: the application shell and its screens.
package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

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

	screen   Screen
	saveWarn string
}

// New builds the app from an open store and the loaded content. The style and
// layout come from the config, with defaults when it is empty.
func New(st *store.Store, lib *content.Library) (*App, error) {
	cfg := st.Config()
	layoutID := cfg.Layout
	if layoutID == "" {
		layoutID = defaultLayout
	}
	all, err := layout.Builtin()
	if err != nil {
		return nil, err
	}
	var lay *layout.Layout
	for _, l := range all {
		if l.ID == layoutID {
			lay = l
		}
	}
	if lay == nil {
		return nil, fmt.Errorf("unknown layout %q", layoutID)
	}
	styleID := style.ID(cfg.Style)
	if styleID == "" {
		styleID = defaultStyle
	}
	return &App{
		Store: st, Layout: lay, Lib: lib,
		Curr:  curriculum.New(lib, lay),
		Style: style.ByID(styleID),
		// Round(0) strips the monotonic reading, so the wall clock is compared
		// and a gap while the computer slept is visible.
		Now:    func() time.Time { return time.Now().Round(0) },
		screen: startScreen{},
	}, nil
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd { return nil }

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
			a.SaveErr(a.Store.SaveProfile())
			return a, tea.Quit
		case "ctrl+t":
			a.cycleStyle()
			return a, nil
		}
		if a.tooSmall() {
			return a, nil
		}
	}
	next, cmd := a.screen.Update(msg, a)
	a.screen = next
	return a, cmd
}

func (a *App) cycleStyle() {
	a.Style = style.ByID(style.Next(a.Style.ID))
	cfg := a.Store.Config()
	cfg.Style = string(a.Style.ID)
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
		warn := a.Style.Base().Foreground(a.Style.Bad).Render(a.saveWarn)
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
		out = append(out, strings.Repeat(" ", max(0, (w-len([]rune(l)))/2))+l)
	}
	return strings.Join(out, "\n")
}

// startScreen is a placeholder until the real home screen exists.
type startScreen struct{}

func (s startScreen) Update(tea.Msg, *App) (Screen, tea.Cmd) { return s, nil }

func (startScreen) View(a *App, w, h int) string {
	return style.Frame(a.Style, w, h, text.AppName, text.Placeholder, text.HintQuit)
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
	_, err = tea.NewProgram(a).Run()
	return err
}
