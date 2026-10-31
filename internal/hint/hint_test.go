package hint_test

import (
	"testing"
	"time"

	"github.com/jiripisa/tuipe/internal/hint"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/stats"
)

func eq[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func czech(t *testing.T) *layout.Layout {
	t.Helper()
	all, err := layout.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range all {
		if l.ID == "macos-czech-qwerty" {
			return l
		}
	}
	t.Fatal("czech layout missing")
	return nil
}

func TestHintTiming(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	cases := []struct {
		m    stats.Mastery
		h    time.Duration
		w    int
		want bool
	}{
		{stats.NewChar, 0, 0, true},
		{stats.Learning, ms(799), 0, false}, {stats.Learning, ms(800), 0, true},
		{stats.Solid, ms(1999), 0, false}, {stats.Solid, ms(2000), 0, true},
		{stats.Automatic, ms(60000), 0, false},
		{stats.Learning, 0, 1, false}, {stats.Solid, 0, 1, false}, {stats.Automatic, 0, 1, false},
		{stats.Learning, 0, 2, true}, {stats.Solid, 0, 2, true}, {stats.Automatic, 0, 2, true},
	}
	for _, c := range cases {
		eq(t, hint.Show(hint.Input{Mastery: c.m, Hesitation: c.h, WrongAtPos: c.w}), c.want)
	}
	eq(t, hint.Show(hint.Input{Mastery: stats.NewChar, Disabled: true}), false)
}

func TestHintText(t *testing.T) {
	l := czech(t)
	eq(t, hint.Text(l, '{'), "{ → levý Option + í (pravý prsteník)")
	eq(t, hint.Text(l, 'ď'), "ď → levý Shift + ´, pak d (levý prostředník)")
	eq(t, hint.Text(l, 'a'), "a → a (levý malíček)")
	eq(t, hint.Text(l, 'A'), "A → pravý Shift + a (levý malíček)")
	eq(t, hint.Text(l, ' '), "mezera → palec")
	eq(t, hint.Text(l, '\n'), "Enter → Enter (pravý malíček)")
	eq(t, hint.Text(l, '€'), "")
}
