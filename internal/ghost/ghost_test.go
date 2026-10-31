package ghost_test

import (
	"slices"
	"testing"
	"time"

	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/ghost"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
)

func eq[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestReplayPosition(t *testing.T) {
	g := ghost.Replay([]int64{1000, 2000, 3000})
	eq(t, g.PosAt(0), 0)
	eq(t, g.PosAt(1500*time.Millisecond), 1)
	eq(t, g.PosAt(10*time.Second), 3)
}

func TestLead(t *testing.T) {
	g := ghost.Replay([]int64{1000, 2000, 3000})
	// pos counts finished cells: the ghost finished 2 cells at 2 s
	eq(t, g.Lead(2, 1500*time.Millisecond), 500*time.Millisecond)
	eq(t, g.Lead(1, 3*time.Second), -2*time.Second)
	eq(t, g.Lead(0, 5*time.Second), 0)
}

func TestFromLogCoversEveryCellAndRepeatsAutoTime(t *testing.T) {
	txt := engine.Prepare("a\n  b")
	log := []engine.Keystroke{
		{Pos: 0, Typed: 'a', Correct: true, T: 100 * time.Millisecond},
		{Pos: 1, Typed: 'x', Correct: false, T: 250 * time.Millisecond},
		{Pos: 1, Typed: '\n', Correct: true, T: 300 * time.Millisecond},
		{Pos: 4, Typed: 'b', Correct: true, T: 500 * time.Millisecond},
	}
	got := ghost.FromLog(txt, log)
	want := []int64{100, 300, 300, 300, 500}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPaceIsSlightlyFasterThanAverage(t *testing.T) {
	txt := engine.Prepare("ab")
	s := stats.New()
	s.Chars["a"] = &stats.Entry{LatencyMs: 400, Samples: 10}
	s.Chars["b"] = &stats.Entry{LatencyMs: 400, Samples: 10}
	g := ghost.Pace(txt, s, 500)
	// per char 400 × 0.97 = 388 ms
	eq(t, g.PosAt(387*time.Millisecond), 0)
	eq(t, g.PosAt(388*time.Millisecond), 1)
	eq(t, g.PosAt(776*time.Millisecond), 2)
}

func TestPaceFallbackAndAuto(t *testing.T) {
	txt := engine.Prepare("a\n  b")
	s := stats.New()
	s.Chars["a"] = &stats.Entry{LatencyMs: 400, Samples: 10}
	g := ghost.Pace(txt, s, 500)
	// a=388, "\n" unknown -> 500×0.97=485, two Auto cells repeat, b=485
	eq(t, g.PosAt(388*time.Millisecond), 1)
	eq(t, g.PosAt(873*time.Millisecond), 4)
	eq(t, g.PosAt(1358*time.Millisecond), 5)
	eq(t, g.PosAt(1357*time.Millisecond), 4)
}

func TestReplaceOnlyOnHigherScoreAndNoPause(t *testing.T) {
	old := &store.Ghost{Score: 50}
	eq(t, ghost.ShouldReplace(old, score.Metrics{Score: 49}, false, false), false)
	eq(t, ghost.ShouldReplace(old, score.Metrics{Score: 51}, true, false), false)
	eq(t, ghost.ShouldReplace(old, score.Metrics{Score: 51}, false, false), true)
	eq(t, ghost.ShouldReplace(nil, score.Metrics{Score: 1}, false, false), true)
	eq(t, ghost.ShouldReplace(nil, score.Metrics{Score: 99}, false, true), false) // generated text
}
