package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// The rows of the settings screen, in display order.
const (
	setStyle = iota
	setLayout
	setUpdateCheck
	setRetake
	settingRows
)

const settingLabelWidth = 24 // cells of the label column, so the values line up

// settingsScreen changes the look, the keyboard layout and the update check,
// and starts the placement test again.
type settingsScreen struct {
	sel     int
	layouts []*layout.Layout
	broken  []string // names of user layout files that could not be read
	note    string   // shown under the rows after a layout change
}

// NewSettingsScreen builds the settings screen. The layouts are read again on
// every visit, so a file added meanwhile is listed.
func NewSettingsScreen(a *App) Screen {
	s := &settingsScreen{}
	all, errs, err := loadLayoutsReport(a.Store)
	if err != nil { // the built-in layouts cannot fail once the app has started
		all = []*layout.Layout{a.Layout}
	}
	s.layouts = all
	for _, e := range errs {
		s.broken = append(s.broken, brokenName(e))
	}
	return s
}

// brokenName is the file name of a layout loading error.
func brokenName(err error) string {
	var fe *layout.FileError
	if errors.As(err, &fe) {
		return filepath.Base(fe.Path)
	}
	return err.Error()
}

func (s *settingsScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	switch k.String() {
	case "esc":
		return NewHomeScreen(a), nil
	case "up", "k":
		s.sel = (s.sel + settingRows - 1) % settingRows
	case "down", "j":
		s.sel = (s.sel + 1) % settingRows
	case "enter", "right", "l":
		return s.change(a, 1)
	case "left", "h":
		return s.change(a, -1)
	}
	return s, nil
}

// change moves the selected value on by step (or back, for -1). Enter on the
// last row retakes the placement test.
func (s *settingsScreen) change(a *App, step int) (Screen, tea.Cmd) {
	switch s.sel {
	case setStyle:
		styles := style.All()
		a.setStyle(styles[(styleIndex(a)+step+len(styles))%len(styles)].ID)
	case setLayout:
		i := s.current(a)
		if i < 0 { // the active layout is not listed: start from the first or the last
			i = map[bool]int{true: -1, false: 0}[step > 0]
		}
		if l := s.layouts[(i+step+len(s.layouts))%len(s.layouts)]; l.ID != a.Layout.ID {
			a.setLayout(l)
			s.note = text.SettingsLayoutWarn
		}
	case setUpdateCheck:
		cfg := a.Store.Config()
		cfg.UpdateCheck = !cfg.UpdateCheck
		a.SaveErr(a.Store.SaveConfig(cfg))
	case setRetake:
		if step > 0 {
			return NewPlacementScreen(a, nil), nil
		}
	}
	return s, nil
}

// current is the index of the active layout in the list, or -1 when it is not
// there.
func (s *settingsScreen) current(a *App) int {
	for i, l := range s.layouts {
		if l.ID == a.Layout.ID {
			return i
		}
	}
	return -1
}

func (s *settingsScreen) View(a *App, w, h int) string {
	st := a.Style
	fg := func(t string) string { return st.Base().Foreground(st.Fg).Render(t) }
	dim := func(t string) string { return st.Base().Foreground(st.Dim).Render(t) }
	bad := func(t string) string { return st.Base().Foreground(st.Bad).Render(t) }
	prose := func(t string, f func(string) string) []string {
		var out []string
		for _, l := range strings.Split(ansi.Wordwrap(t, max(w-4, 10), ""), "\n") {
			out = append(out, f(l))
		}
		return out
	}

	updates := text.SettingsOff
	if a.Store.Config().UpdateCheck {
		updates = text.SettingsOn
	}
	rows := [settingRows][2]string{
		setStyle:       {text.SettingsStyle, "< " + st.Name + " >"},
		setLayout:      {text.SettingsLayout, "< " + a.Layout.Name + " >"},
		setUpdateCheck: {text.SettingsUpdateCheck, "< " + updates + " >"},
		setRetake:      {text.SettingsRetake, ""},
	}
	body := []string{""}
	for i, r := range rows {
		label := r[0] + strings.Repeat(" ", max(0, settingLabelWidth-ansi.StringWidth(r[0])))
		if i == s.sel {
			body = append(body, st.Base().Foreground(st.Accent).Bold(true).Render("> "+label+r[1]))
		} else {
			body = append(body, fg("  "+label+r[1]))
		}
	}
	if s.sel == setRetake {
		body = append(body, "", dim("  "+text.SettingsRetakeNote))
	}
	if s.note != "" {
		body = append(body, "")
		body = append(body, prose(s.note, dim)...)
	}
	if len(s.broken) > 0 {
		body = append(body, "")
		body = append(body, prose(fmt.Sprintf(text.SettingsBrokenLayout, strings.Join(s.broken, ", ")), bad)...)
	}
	cur := s.current(a)
	if cur < 0 {
		body = append(body, "")
		body = append(body, prose(fmt.Sprintf(text.SettingsLayoutMissing, a.Layout.Name), bad)...)
	}
	body = append(body, "", dim(text.SettingsLayoutList))

	// The list takes the rows that are left and scrolls to keep the active
	// layout in view.
	room := max(1, h-3-len(body)) // body rows of the frame: all but borders and footer
	top := 0
	if len(s.layouts) > room {
		top = min(max(cur-room/2, 0), len(s.layouts)-room)
	}
	for i := top; i < min(top+room, len(s.layouts)); i++ {
		mark := "  "
		if i == cur {
			mark = "• "
		}
		body = append(body, fg(mark+s.layouts[i].Name))
	}
	return style.Frame(st, w, h, text.AppName+" · "+text.SettingsTitle, " "+strings.Join(body, "\n "), text.HintSettings)
}
