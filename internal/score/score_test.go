package score_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/score"
)

func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.01 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func eq[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestScoreValuesFromSpec(t *testing.T) {
	near(t, score.Value(60, 0.98), 55.34)
	near(t, score.Value(60, 0.90), 39.37)
}

func TestAccuracyStars(t *testing.T) {
	tg := score.Targets{Speed: [3]float64{20, 30, 40}}
	for _, tc := range []struct {
		acc  float64
		want int
	}{{1, 3}, {0.999, 2}, {0.99, 2}, {0.989, 1}, {0.97, 1}, {0.969, 0}} {
		if got := score.GradeOf(10, tc.acc, tg).Accuracy; got != tc.want {
			t.Errorf("accuracy %v: %d stars, want %d", tc.acc, got, tc.want)
		}
	}
}

// The accuracy is shown to one decimal, so a Test shown at 99,0 % or 97,0 %
// earns the stars of that line.
func TestAccuracyStarsGradeTheShownPercent(t *testing.T) {
	tg := score.Targets{Speed: [3]float64{20, 30, 40}}
	for _, tc := range []struct {
		right, reached int
		want           int
	}{{189, 191, 2}, {160, 165, 1}, {494, 500, 1}, {484, 500, 0}} {
		if got := score.GradeOf(10, float64(tc.right)/float64(tc.reached), tg).Accuracy; got != tc.want {
			t.Errorf("%d/%d right: %d stars, want %d", tc.right, tc.reached, got, tc.want)
		}
	}
}

func TestSpeedStarsIgnoreAccuracy(t *testing.T) {
	tg := score.Targets{Speed: [3]float64{20, 30, 40}}
	for _, tc := range []struct {
		wpm  float64
		want int
	}{{19.9, 0}, {20, 1}, {30, 2}, {39, 2}, {40, 3}, {90, 3}} {
		for _, acc := range []float64{0.5, 1} {
			if got := score.GradeOf(tc.wpm, acc, tg).Speed; got != tc.want {
				t.Errorf("%v wpm at accuracy %v: %d stars, want %d", tc.wpm, acc, got, tc.want)
			}
		}
	}
}

func TestBetterPrefersAccuracy(t *testing.T) {
	g := func(a, s int) score.Grade { return score.Grade{Accuracy: a, Speed: s} }
	if !g(2, 1).Better(g(1, 3)) {
		t.Error("two accuracy stars should beat one, whatever the speed")
	}
	if !g(2, 2).Better(g(2, 1)) {
		t.Error("on equal accuracy more speed stars win")
	}
	if g(2, 2).Better(g(2, 2)) {
		t.Error("an equal grade is not better")
	}
	if g(1, 3).Better(g(2, 0)) {
		t.Error("fewer accuracy stars must not win")
	}
}

func TestGradeTotal(t *testing.T) {
	eq(t, score.Grade{Accuracy: 2, Speed: 3}.Total(), 5)
	eq(t, score.Grade{}.Total(), 0)
}

func TestAccuracyCountsFirstTryOnly(t *testing.T) {
	txt := engine.Prepare("ab")
	log := []engine.Keystroke{
		{Pos: 0, Typed: 'x', Correct: false},
		{Pos: 0, Typed: 'a', Correct: true},
		{Pos: 1, Typed: 'b', Correct: true},
	}
	m := score.Compute(txt, log, time.Minute)
	eq(t, m.Chars, 2)
	near(t, m.Accuracy, 0.5)
}

func TestAutoCellsAreNotCounted(t *testing.T) {
	txt := engine.Prepare("a\n  b")
	log := []engine.Keystroke{
		{Pos: 0, Typed: 'a', Correct: true},
		{Pos: 1, Typed: '\n', Correct: true},
		{Pos: 4, Typed: 'b', Correct: true},
	}
	m := score.Compute(txt, log, time.Minute)
	eq(t, m.Chars, 3)
	near(t, m.Accuracy, 1)
}

func TestWPMIsCorrectCharsOverFiveOverMinutes(t *testing.T) {
	txt := engine.Prepare(strings.Repeat("a", 50))
	var log []engine.Keystroke
	for i := 0; i < 50; i++ {
		log = append(log, engine.Keystroke{Pos: i, Typed: 'a', Correct: true})
	}
	m := score.Compute(txt, log, 60*time.Second)
	near(t, m.WPM, 10)
	near(t, m.Score, 10)
	eq(t, m.Duration, 60*time.Second)
}

func TestEmptyInputGivesZeroMetrics(t *testing.T) {
	m := score.Compute(engine.Prepare(""), nil, 0)
	eq(t, m.WPM, 0.0)
	eq(t, m.Accuracy, 0.0)
	eq(t, m.Score, 0.0)
}

func TestPartialLogCountsOnlyCharactersReached(t *testing.T) {
	// "abcde": a right, b wrong then right, c wrong (still pending at the cursor).
	txt := engine.Prepare("abcde")
	log := []engine.Keystroke{
		{Pos: 0, Typed: 'a', Correct: true},
		{Pos: 1, Typed: 'x', Correct: false},
		{Pos: 1, Typed: 'b', Correct: true},
		{Pos: 2, Typed: 'y', Correct: false},
	}
	m := score.Compute(txt, log, 12*time.Second)
	eq(t, m.Chars, 2)
	near(t, m.Accuracy, 1.0/3)
	near(t, m.WPM, 2.0/5/0.2)
}

func TestPartialLogWithoutPendingWrongPress(t *testing.T) {
	txt := engine.Prepare("abcde")
	log := []engine.Keystroke{
		{Pos: 0, Typed: 'a', Correct: true},
		{Pos: 1, Typed: 'b', Correct: true},
	}
	m := score.Compute(txt, log, time.Minute)
	eq(t, m.Chars, 2)
	near(t, m.Accuracy, 1)
}

func TestEmptyLogOnNonEmptyTextGivesZeroMetrics(t *testing.T) {
	m := score.Compute(engine.Prepare("abc"), nil, 5*time.Second)
	eq(t, m.Chars, 0)
	eq(t, m.WPM, 0.0)
	eq(t, m.Accuracy, 0.0)
	eq(t, m.Score, 0.0)
}

func TestCPM(t *testing.T) {
	if got := score.CPM(40); got != 200 {
		t.Errorf("CPM(40) = %v, want 200", got)
	}
	if got := score.CPM(0); got != 0 {
		t.Errorf("CPM(0) = %v, want 0", got)
	}
}
