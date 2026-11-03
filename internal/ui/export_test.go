package ui

import (
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/stats"
)

// TickMsg returns the tick of the round that is on screen, or nil.
func TickMsg(a *App) tea.Msg {
	if r, ok := a.screen.(*roundScreen); ok {
		return tickMsg{r.gen}
	}
	return nil
}

// ScreenName names the screen that is shown, for tests.
func ScreenName(a *App) string {
	switch a.screen.(type) {
	case *roundScreen:
		return "round"
	case *resultsScreen:
		return "results"
	case *partResults:
		return "part-results"
	case *homeScreen:
		return "home"
	case *careerScreen:
		return "career"
	case *statsScreen:
		return "stats"
	case *firstRunScreen:
		return "first-run"
	case *placementScreen:
		return "placement"
	case *settingsScreen:
		return "settings"
	case *teachScreen:
		return "teach"
	case *lessonsScreen:
		return "lessons"
	case *lessonScreen:
		return "lesson"
	}
	return "other"
}

// SetScreen replaces the current screen.
func SetScreen(a *App, s Screen) { a.screen = s }

// RoundKeys returns the runes the user has to type in the current round.
func RoundKeys(a *App) []rune {
	r, ok := a.screen.(*roundScreen)
	if !ok {
		return nil
	}
	var keys []rune
	for _, c := range r.sess.Frame(r.now).Cells {
		if !c.Auto {
			keys = append(keys, c.R)
		}
	}
	return keys
}

// RoundPos returns the cursor position of the current round.
func RoundPos(a *App) int {
	r, ok := a.screen.(*roundScreen)
	if !ok {
		return -1
	}
	return r.sess.Frame(r.now).Pos
}

// RoundSamples returns the throughput samples of the current round.
func RoundSamples(a *App) []float64 {
	if r, ok := a.screen.(*roundScreen); ok {
		return r.samples
	}
	return nil
}

// RoundLessonID returns the lesson of the current round.
func RoundLessonID(a *App) string {
	if r, ok := a.screen.(*roundScreen); ok {
		return r.spec.LessonID
	}
	return ""
}

// RoundActive returns the active time of the current round.
func RoundActive(a *App) time.Duration {
	if r, ok := a.screen.(*roundScreen); ok {
		return r.sess.Result(r.now).Metrics.Duration
	}
	return 0
}

// RunApp runs the program like Run does, with options for input and output.
func RunApp(a *App, opts ...tea.ProgramOption) error { return runApp(a, opts...) }

// RoundHint returns the typing hint shown now in the current round, or nil.
func RoundHint(a *App) *play.HintInfo {
	if r, ok := a.screen.(*roundScreen); ok {
		return r.sess.Frame(r.now).Hint
	}
	return nil
}

// RoundSpec returns the spec of the current round.
func RoundSpec(a *App) play.Spec {
	if r, ok := a.screen.(*roundScreen); ok {
		return r.spec
	}
	return play.Spec{}
}

// RoundFrame returns the frame of the current round.
func RoundFrame(a *App) play.Frame {
	if r, ok := a.screen.(*roundScreen); ok {
		return r.sess.Frame(r.now)
	}
	return play.Frame{}
}

// SetGOOS replaces the operating system the first run checks and returns a
// function that restores it.
func SetGOOS(s string) func() {
	old := goos
	goos = s
	return func() { goos = old }
}

// RawView is the view of the screen as it draws itself, before the app pads
// or cuts it to the window.
func RawView(a *App) string { return a.screen.View(a, a.W, a.H) }

// Label is the text of a home entry.
func (e menuEntry) Label() string { return e.label }

// Centre places a block of text in the middle of a w × h area.
func Centre(s string, w, h int) string { return centre(s, w, h) }

// StatsOf returns the character statistics of the active layout.
func StatsOf(a *App) *stats.Stats { return a.stats() }

// LessonRound builds the round of a lesson, as the lesson list starts it. A nil
// next returns home when the results are left.
func LessonRound(a *App, id string, next func(*App) Screen) (Screen, error) {
	spec, err := a.lessonSpec(id)
	if err != nil {
		return nil, err
	}
	return newRound(a, spec, next), nil
}

// LessonSpec is the spec of a lesson round.
func LessonSpec(a *App, id string) (play.Spec, error) { return a.lessonSpec(id) }

// ChapterTestOutRound builds the test-out round of a chapter.
func ChapterTestOutRound(a *App, chapterID string) (Screen, error) {
	for _, ch := range a.Curr.Chapters() {
		if ch.ID != chapterID {
			continue
		}
		spec, err := a.testOutSpec(ch)
		if err != nil {
			return nil, err
		}
		return newRound(a, spec, nil), nil
	}
	return nil, fmt.Errorf("unknown chapter %q", chapterID)
}

// SnippetRound builds a round on a snippet of the library, which no lesson of
// the course has to use.
func SnippetRound(a *App, id string) (Screen, error) {
	i := slices.IndexFunc(a.Lib.Snippets, func(s content.Snippet) bool { return s.ID == id })
	if i < 0 {
		return nil, fmt.Errorf("unknown snippet %q", id)
	}
	sn := a.Lib.Snippets[i]
	spec := play.Spec{
		Kind: play.KindCode, Title: sn.ID, Lang: sn.Lang, Text: sn.Text, Targets: a.chapterTargets(),
	}
	return newRound(a, spec, nil), nil
}

// TeachStep is the line being typed in Výuka, or -1 while the explanation shows.
func TeachStep(a *App) int {
	if t, ok := a.screen.(*teachScreen); ok {
		return t.step
	}
	return -2
}

// TeachLine is the text of the line being typed in Výuka.
func TeachLine(a *App) string {
	if t, ok := a.screen.(*teachScreen); ok && t.step >= 0 && t.step < len(t.tut.Lines) {
		return t.tut.Lines[t.step]
	}
	return ""
}

// TeachPos is the cursor position in the line being typed in Výuka.
func TeachPos(a *App) int {
	if t, ok := a.screen.(*teachScreen); ok && t.sess != nil {
		return t.sess.Frame(t.now).Pos
	}
	return -1
}

// TeachKeys returns the runes still to type in the current Výuka line.
func TeachKeys(a *App) []rune {
	t, ok := a.screen.(*teachScreen)
	if !ok || t.sess == nil {
		return nil
	}
	f := t.sess.Frame(t.now)
	var keys []rune
	for _, c := range f.Cells[f.Pos:] {
		if !c.Auto {
			keys = append(keys, c.R)
		}
	}
	return keys
}

// TeachHighlights returns the key ids the keyboard of Výuka marks now.
func TeachHighlights(a *App) []string {
	if t, ok := a.screen.(*teachScreen); ok {
		return t.highlights(a)
	}
	return nil
}

// LessonRecords is lessonRecords, for tests.
var LessonRecords = lessonRecords

// LessonsSelected is the id of the lesson under the selection of the lesson list.
func LessonsSelected(a *App) string {
	if l, ok := a.screen.(*lessonsScreen); ok {
		return l.selectedID()
	}
	return ""
}

// LessonScreenID is the lesson the lesson screen shows.
func LessonScreenID(a *App) string {
	if l, ok := a.screen.(*lessonScreen); ok {
		return l.id
	}
	return ""
}

// LessonScreenRow is the selected row of the lesson screen: 0 Výuka, 1 Trénink, 2 Test.
func LessonScreenRow(a *App) int {
	if l, ok := a.screen.(*lessonScreen); ok {
		return l.row
	}
	return -1
}
