package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/streak"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// entryID names a menu entry of the home screen.
type entryID string

// The menu entries, in display order.
const (
	EntryCareer   entryID = "career"
	EntryStats    entryID = "stats"
	EntrySettings entryID = "settings"
)

type menuEntry struct {
	id    entryID
	label string
	open  func(*App) (Screen, tea.Cmd) // set by RegisterEntry
}

func defaultEntries() []menuEntry {
	return []menuEntry{
		{EntryCareer, text.MenuCareer, nil},
		{EntryStats, text.MenuStats, nil},
		{EntrySettings, text.MenuSettings, nil},
	}
}

// RegisterEntry wires a home menu entry to its screen.
func (a *App) RegisterEntry(id entryID, open func(*App) (Screen, tea.Cmd)) {
	if i := slices.IndexFunc(a.Entries, func(e menuEntry) bool { return e.id == id }); i >= 0 {
		a.Entries[i].open = open
	}
}

// homeScreen offers the menu. When a lesson was left unfinished, the row to
// continue it comes first and is selected.
type homeScreen struct {
	resumeID string // the lesson that can be continued, or empty
	resume   string // its title
	sel      int
	up       streak.Status
}

// NewHomeScreen builds the home screen.
func NewHomeScreen(a *App) Screen {
	a.fillBanner()
	h := &homeScreen{up: a.uptime()}
	if r := a.Store.Profile().Resume; r != nil && r.LessonID != "" {
		if title, ok := a.lessonTitle(r.LessonID); ok {
			h.resumeID = r.LessonID
			h.resume = title
		}
	}
	return h
}

func (a *App) lessonTitle(id string) (string, bool) {
	for _, ch := range a.Curr.Chapters() {
		for _, l := range ch.Lessons {
			if l.ID == id {
				return l.Title, true
			}
		}
	}
	return "", false
}

func (h *homeScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return h, nil
	}
	a.Banner = nil
	count := len(a.Entries)
	if h.resume != "" {
		count++
	}
	switch k.String() {
	case "up", "k":
		h.sel = (h.sel + count - 1) % count
	case "down", "j":
		h.sel = (h.sel + 1) % count
	case "enter":
		i := h.sel
		if h.resume != "" {
			if i == 0 {
				return h.continueLesson(a), nil
			}
			i--
		}
		if open := a.Entries[i].open; open != nil {
			return open(a)
		}
	}
	return h, nil
}

// continueLesson plays the lesson that was left unfinished; the lesson list
// follows it.
func (h *homeScreen) continueLesson(a *App) Screen {
	spec, err := a.lessonSpec(h.resumeID)
	if err != nil {
		a.SaveErr(err)
		return h
	}
	spec.Session = sessionCareer
	return newRound(a, spec, NewCareerScreen)
}

func (h *homeScreen) View(a *App, w, hgt int) string {
	st := a.Style
	dim := func(s string) string { return st.Base().Foreground(st.Dim).Render(s) }
	lines := []string{
		dim(text.HomeRank+"  ") + st.Base().Foreground(st.Accent).Render(a.Curr.RankOf(a.Store.Profile()).String()),
		dim(text.HomeUptime+"  ") + st.Base().Foreground(st.Fg).Render(uptimeText(h.up.Days)),
	}
	if h.up.ForgivenThisWeek { // on a line of its own: beside the uptime it does not fit 60 columns
		lines = append(lines, dim("  ("+text.HomeBudgetSpent+")"))
	}
	lines = append(lines, "")
	var labels []string
	if h.resume != "" { // "│> " in front
		labels = append(labels, ansi.Truncate(fmt.Sprintf(text.HomeResume, h.resume), max(w-6, 1), "…"))
	}
	for _, e := range a.Entries {
		labels = append(labels, e.label)
	}
	selLine := 0
	for i, l := range labels {
		if i == h.sel {
			selLine = len(lines)
			lines = append(lines, st.Base().Foreground(st.Accent).Bold(true).Render("> "+l))
		} else {
			lines = append(lines, st.Base().Foreground(st.Fg).Render("  "+l))
		}
		if i == 0 && h.resume != "" {
			lines = append(lines, "")
		}
	}
	// Keep the selection on screen: scroll the body when the window is short.
	rows := hgt - 3
	banner := a.bannerLines(w, rows)
	if len(banner) > 0 {
		rows -= len(banner) + 1 // a blank row under it
	}
	notice := ""
	if a.UpdateNotice != "" { // the quiet one-line notice keeps its row under the menu
		notice = dim(ansi.Truncate(a.UpdateNotice, max(w-3, 1), "…"))
		rows--
	}
	if rows > 0 && selLine >= rows {
		lines = lines[selLine-rows+1:]
	}
	lines = lines[:min(len(lines), max(rows, 0))]
	if notice != "" {
		lines = append(lines, notice)
	}
	if len(banner) > 0 {
		lines = append(append(banner, ""), lines...)
	}
	return style.Frame(st, w, hgt, text.AppName, strings.Join(lines, "\n"), text.HintHome)
}

// bannerLines wraps the banner to the window and paints it. It takes at most
// half of the rows; a path has no spaces, so it is broken where it must be.
func (a *App) bannerLines(w, rows int) []string {
	var out []string
	for _, b := range a.Banner {
		for _, l := range strings.Split(ansi.Wrap(b, max(w-4, 10), ""), "\n") {
			out = append(out, a.Style.Base().Foreground(a.Style.Bad).Render(l))
		}
	}
	return out[:min(len(out), max(rows/2, 1))]
}

// uptimeText reads like a server uptime: "up 1 day", "up 23 days".
func uptimeText(days int) string {
	if days == 1 {
		return text.HomeUptimeDay
	}
	return fmt.Sprintf(text.HomeUptimeDays, days)
}
