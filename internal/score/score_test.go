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

func TestStars(t *testing.T) {
	tg := score.Targets{Speed: [3]float64{20, 30, 40}}
	m := func(w, a float64) score.Metrics { return score.Metrics{WPM: w, Accuracy: a} }
	eq(t, score.Stars(m(45, 0.91), tg, false), 0) // fast but sloppy
	eq(t, score.Stars(m(21, 0.99), tg, false), 1) // accurate but slow
	eq(t, score.Stars(m(35, 0.96), tg, false), 2)
	eq(t, score.Stars(m(45, 0.98), tg, false), 3)
	eq(t, score.Stars(m(45, 0.98), tg, true), 2) // a hint blocks three stars
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
