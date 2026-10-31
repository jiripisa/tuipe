package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/recommend"
	"github.com/jiripisa/tuipe/internal/store"
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
	case *homeScreen:
		return "home"
	case *careerScreen:
		return "career"
	case *practiceScreen:
		return "practice"
	case *severityScreen:
		return "severity"
	case *incidentListScreen:
		return "incident-list"
	case *incidentResultsScreen:
		return "incident-results"
	case *retroScreen:
		return "retro"
	case *sprintPickerScreen:
		return "sprint-minutes"
	case *statsScreen:
		return "stats"
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

// SprintPlan regenerates the plan of a sprint from its saved state.
func SprintPlan(a *App, s *store.SprintState) []recommend.Pick { return a.sprintPlan(s) }

// AddEntry appends a home entry that has no screen.
func AddEntry(a *App, label string) {
	a.Entries = append(a.Entries, menuEntry{id: "fake", label: label})
}
