package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/recommend"
	"github.com/jiripisa/tuipe/internal/streak"
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
	open  func(*App) (Screen, tea.Cmd) // set by RegisterEntry
}

func defaultEntries() []menuEntry {
	return []menuEntry{
		{EntryStandup, text.MenuStandup, nil},
		{EntrySprint, text.MenuSprint, nil},
		{EntryCareer, text.MenuCareer, nil},
		{EntryPractice, text.MenuPractice, nil},
		{EntryIncident, text.MenuIncident, nil},
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

// homeScreen offers the recommended round first, then the other screens.
type homeScreen struct {
	pick   recommend.Pick
	resume string // lesson title when a lesson can be continued
	sel    int
	up     streak.Status
}

// NewHomeScreen builds the home screen. The recommended round is selected, or
// the lesson to continue when a round was left unfinished.
func NewHomeScreen(a *App) Screen {
	a.chain = nil
	a.fillBanner()
	// A Sprint left on an earlier day ends with a Retro on the part that was done.
	if r := a.Store.Profile().Resume; sprintResume(r) && r.Day != a.today() {
		return a.endSprint(r.Sprint)
	}
	h := &homeScreen{pick: a.recommended(), up: a.uptime()}
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
	a.Banner = nil
	count := len(a.Entries) + 1
	switch k.String() {
	case "up", "k":
		h.sel = (h.sel + count - 1) % count
	case "down", "j":
		h.sel = (h.sel + 1) % count
	case "enter":
		if h.sel == 0 {
			return NewRoundScreen(a, h.pick, quickRun), nil
		}
		if open := a.Entries[h.sel-1].open; open != nil {
			return open(a)
		}
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
	lines := []string{
		dim(text.HomeRank+"  ") + st.Base().Foreground(st.Accent).Render(a.Curr.RankOf(a.Store.Profile()).String()),
		dim(text.HomeUptime+"  ") + st.Base().Foreground(st.Fg).Render(uptimeText(h.up.Days)),
	}
	if h.up.ForgivenThisWeek { // on a line of its own: beside the uptime it does not fit 60 columns
		lines = append(lines, dim("  ("+text.HomeBudgetSpent+")"))
	}
	lines = append(lines, "")
	labels := []string{ansi.Truncate(h.pickLabel(a), max(w-6, 1), "…")} // "│> " in front
	for _, e := range a.Entries {
		labels = append(labels, e.label+a.entryNote(e.id))
	}
	selLine := 0
	for i, l := range labels {
		if i == h.sel {
			selLine = len(lines)
			lines = append(lines, st.Base().Foreground(st.Accent).Bold(true).Render("> "+l))
		} else {
			lines = append(lines, st.Base().Foreground(st.Fg).Render("  "+l))
		}
		if i == 0 {
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

// entryNote is the state of an entry shown after its label: a Standup done
// today, a Sprint to continue.
func (a *App) entryNote(id entryID) string {
	switch {
	case id == EntryStandup && a.Store.Profile().StandupDay == a.today():
		return " (" + text.HomeDone + ")"
	case id == EntrySprint && a.sprintToContinue() != nil:
		return " (" + text.HomeContinue + ")"
	}
	return ""
}
