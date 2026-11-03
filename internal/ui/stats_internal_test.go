package ui

import (
	"testing"

	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

func TestCharGroup(t *testing.T) {
	all, err := layout.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	var lay *layout.Layout
	for _, l := range all {
		if l.ID == defaultLayout {
			lay = l
		}
	}
	for r, want := range map[rune]string{
		'Č': text.StatsGroupDiacritics, // Shift-typed, but a diacritic first
		'{': text.StatsGroupOption,
		'ˇ': text.StatsGroupShift, // a dead key on Shift; U+02C7 is a letter by category but no Latin one
		'A': text.StatsGroupShift,
		'5': text.StatsGroupDigits, // typed with Shift on this layout, but a digit first
		'a': text.StatsGroupLetters,
	} {
		route, ok := lay.Route(r)
		if !ok {
			t.Fatalf("no route for %q", r)
		}
		if got := charGroup(r, route); got != want {
			t.Errorf("charGroup(%q) = %q, want %q (route %v)", r, got, want, route)
		}
	}
}

func TestRunsForKeepsWhatTheFilterLetsThrough(t *testing.T) {
	runs := []store.Run{
		{ID: "t", Mode: store.ModeTest}, {ID: "k", Mode: store.ModeTraining},
		{ID: "p", Mode: store.ModePlacement}, {ID: "o", Mode: "lesson"},
	}
	ids := func(rs []store.Run) string {
		var s string
		for _, r := range rs {
			s += r.ID
		}
		return s
	}
	for filter, want := range map[historyFilter]string{filterAll: "tkpo", filterTraining: "k", filterTest: "t"} {
		if got := ids(runsFor(runs, filter)); got != want {
			t.Errorf("runsFor(%d) = %q, want %q", filter, got, want)
		}
	}
	if got := runsFor(nil, filterTest); len(got) != 0 {
		t.Errorf("no runs gave %v", got)
	}
}
