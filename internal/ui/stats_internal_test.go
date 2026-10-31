package ui

import (
	"testing"

	"github.com/jiripisa/tuipe/internal/layout"
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
