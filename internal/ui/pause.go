package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// pauseChoice is what the pause menu asks for after a key.
type pauseChoice int

const (
	pauseNothing pauseChoice = iota
	pauseResume
	pauseRestart
	pauseLeave
)

var pauseLabels = []string{text.PauseResume, text.PauseRestart, text.PauseLeave}

// pauseMenu is the small overlay of an Esc pause.
type pauseMenu struct{ sel int }

// key moves the selection and reports a choice. Esc resumes.
func (m *pauseMenu) key(k tea.KeyPressMsg) pauseChoice {
	switch k.String() {
	case "up":
		m.sel = (m.sel + len(pauseLabels) - 1) % len(pauseLabels)
	case "down":
		m.sel = (m.sel + 1) % len(pauseLabels)
	case "esc":
		return pauseResume
	case "enter":
		return []pauseChoice{pauseResume, pauseRestart, pauseLeave}[m.sel]
	}
	return pauseNothing
}

// overlay draws the menu in the middle of the round view.
func (m pauseMenu) overlay(s style.Style, view string, w, h int) string {
	lines := strings.Split(view, "\n")
	box := []string{s.Base().Foreground(s.Accent).Bold(true).Render(text.PauseTitle), ""}
	for i, l := range pauseLabels {
		if i == m.sel {
			box = append(box, s.Base().Foreground(s.Accent).Bold(true).Render("> "+l))
		} else {
			box = append(box, s.Base().Foreground(s.Fg).Render("  "+l))
		}
	}
	box = append(box, "", s.Base().Foreground(s.Dim).Render(text.HintPause))
	widest := 0
	for _, b := range box {
		widest = max(widest, ansi.StringWidth(b))
	}
	pad := strings.Repeat(" ", max(0, (w-widest)/2)) // one margin keeps the menu aligned
	top := max(0, (len(lines)-len(box))/2)
	for i, b := range box {
		if top+i >= len(lines) {
			break
		}
		lines[top+i] = s.Paint(pad+b, w)
	}
	return strings.Join(lines, "\n")
}
