package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/recommend"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// entryID names a menu entry of the home screen.
type entryID string

// The menu entries below the recommended round, in display order.
const (
	EntryStandup  entryID = "standup"
	EntrySprint   entryID = "sprint"
	EntryCareer   entryID = "career"
	EntryPractice entryID = "practice"
	EntryIncident entryID = "incident"
	EntryStats    entryID = "stats"
	EntrySettings entryID = "settings"
)

type menuEntry struct {
	id    entryID
	label string
	open  func(*App) (Screen, tea.Cmd) // nil until the screen exists
}

var homeEntries = []menuEntry{
	{EntryStandup, text.MenuStandup, nil},
	{EntrySprint, text.MenuSprint, nil},
	{EntryCareer, text.MenuCareer, nil},
	{EntryPractice, text.MenuPractice, nil},
	{EntryIncident, text.MenuIncident, nil},
	{EntryStats, text.MenuStats, nil},
	{EntrySettings, text.MenuSettings, nil},
}

// RegisterEntry wires a home menu entry to its screen. Until an entry is
// registered it shows that it is not ready.
func RegisterEntry(id entryID, open func(*App) (Screen, tea.Cmd)) {
	if i := slices.IndexFunc(homeEntries, func(e menuEntry) bool { return e.id == id }); i >= 0 {
		homeEntries[i].open = open
	}
}

// homeScreen offers the recommended round first, then the other screens.
type homeScreen struct {
	pick   recommend.Pick
	resume string // lesson title when a lesson can be continued
	sel    int
	note   string // shown under the menu, such as "not ready"
}

// NewHomeScreen builds the home screen. The recommended round is selected, or
// the lesson to continue when a round was left unfinished.
func NewHomeScreen(a *App) Screen {
	h := &homeScreen{pick: a.recommended()}
	if r := a.Store.Profile().Resume; r != nil && r.LessonID != "" {
		if title, ok := a.lessonTitle(r.LessonID); ok {
			h.pick = recommend.Pick{Kind: recommend.Lesson, LessonID: r.LessonID}
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
	h.note = ""
	count := len(homeEntries) + 1
	switch k.String() {
	case "up", "k":
		h.sel = (h.sel + count - 1) % count
	case "down", "j":
		h.sel = (h.sel + 1) % count
	case "enter":
		if h.sel == 0 {
			return NewRoundScreen(a, h.pick, quickRun), nil
		}
		if open := homeEntries[h.sel-1].open; open != nil {
			return open(a)
		}
		h.note = text.HomeNotReady
	}
	return h, nil
}

// pickLabel describes a recommended round in one line.
func (h *homeScreen) pickLabel(a *App) string {
	if h.resume != "" {
		return fmt.Sprintf(text.HomeResume, h.resume)
	}
	switch h.pick.Kind {
	case recommend.Lesson:
		title, _ := a.lessonTitle(h.pick.LessonID)
		return text.HomeQuickRun + ": " + title
	case recommend.Code:
		return text.HomeQuickRun + ": " + fmt.Sprintf(text.PickCode, h.pick.SnippetID)
	case recommend.Incident:
		return text.HomeQuickRun + ": " + text.PickIncident
	}
	return text.HomeQuickRun + ": " + text.PickDrill
}

func (h *homeScreen) View(a *App, w, hgt int) string {
	st := a.Style
	dim := func(s string) string { return st.Base().Foreground(st.Dim).Render(s) }
	up := a.uptime()
	uptime := fmt.Sprintf(text.HomeUptimeFormat, up.Days)
	if up.ForgivenThisWeek {
		uptime += " (" + text.HomeBudgetSpent + ")"
	}
	lines := []string{
		dim(text.HomeRank+"  ") + st.Base().Foreground(st.Accent).Render(a.Curr.RankOf(a.Store.Profile()).String()),
		dim(text.HomeUptime+"  ") + st.Base().Foreground(st.Fg).Render(uptime),
		"",
	}
	labels := []string{h.pickLabel(a)}
	for _, e := range homeEntries {
		labels = append(labels, e.label)
	}
	for i, l := range labels {
		switch {
		case i == h.sel:
			lines = append(lines, st.Base().Foreground(st.Accent).Bold(true).Render("> "+l))
		default:
			lines = append(lines, st.Base().Foreground(st.Fg).Render("  "+l))
		}
		if i == 0 {
			lines = append(lines, "")
		}
	}
	if h.note != "" {
		lines = append(lines, "", dim(h.note))
	}
	return style.Frame(st, w, hgt, text.AppName, strings.Join(lines, "\n"), text.HintHome)
}
