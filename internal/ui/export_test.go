package ui

import tea "charm.land/bubbletea/v2"

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
