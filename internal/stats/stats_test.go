package stats_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/stats"
)

func eq[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// typeText simulates typing s: every correct press follows the previous one by
// gap(ch); wrongs(ch) wrong presses precede each correct press of ch.
func typeText(s string, gap func(rune) time.Duration, wrongs func(rune) int) (engine.Text, []engine.Keystroke) {
	text := engine.Prepare(s)
	r := engine.New(text)
	at := time.Unix(1000, 0)
	for _, c := range text.Cells {
		if c.Auto {
			continue
		}
		at = at.Add(gap(c.R))
		for i := 0; i < wrongs(c.R); i++ {
			r.Press('#', at)
		}
		r.Press(c.R, at)
	}
	return text, r.Log()
}

func fastClean(s string) (engine.Text, []engine.Keystroke) {
	return typeText(s, func(rune) time.Duration { return 100 * time.Millisecond }, func(rune) int { return 0 })
}

var now = time.Unix(2000000, 0)

func TestRecordSamplesAndLatency(t *testing.T) {
	text, log := fastClean("aaa")
	s := stats.New()
	s.Record(text, log, now)
	e := s.Chars["a"]
	eq(t, e.Samples, 3)
	eq(t, e.LatencyMs, 100.0) // first char has no latency sample
	eq(t, e.ErrorRate, 0.0)
	eq(t, e.LastSeen, now)
	eq(t, s.Bigrams["aa"].Samples, 2)
}

func TestFirstCharCountsErrorButNoLatency(t *testing.T) {
	text, log := typeText("a", func(rune) time.Duration { return time.Second }, func(rune) int { return 1 })
	s := stats.New()
	s.Record(text, log, now)
	e := s.Chars["a"]
	eq(t, e.Samples, 2)
	eq(t, e.LatencyMs, 0.0)
	eq(t, e.ErrorRate > 0, true)
}

func TestRecordSkipsAutoCells(t *testing.T) {
	text, log := fastClean("a\n  b")
	s := stats.New()
	s.Record(text, log, now)
	_, ok := s.Chars[" "]
	eq(t, ok, false)
	eq(t, s.Chars["b"].Samples, 1)
	_, ok = s.Bigrams["\n b"]
	eq(t, ok, false)
}

func TestMasteryRisesWithGoodData(t *testing.T) {
	text, log := fastClean(strings.Repeat("a", 100))
	s := stats.New()
	eq(t, s.MasteryOf('a'), stats.NewChar)
	s.Record(text, log, now)
	eq(t, s.MasteryOf('a'), stats.Automatic)
}

func TestMasteryNeedsSamples(t *testing.T) {
	text, log := fastClean(strings.Repeat("a", 9))
	s := stats.New()
	s.Record(text, log, now)
	eq(t, s.MasteryOf('a'), stats.NewChar)
	text, log = fastClean(strings.Repeat("a", 5))
	s.Record(text, log, now)
	eq(t, s.MasteryOf('a'), stats.Learning)
}

func TestMasteryFallsWithErrors(t *testing.T) {
	text, log := fastClean(strings.Repeat("a", 100))
	s := stats.New()
	s.Record(text, log, now)
	text, log = typeText(strings.Repeat("a", 10),
		func(rune) time.Duration { return 100 * time.Millisecond }, func(rune) int { return 1 })
	s.Record(text, log, now)
	eq(t, s.MasteryOf('a'), stats.NewChar)
}

func TestStaleCharacterDecays(t *testing.T) {
	text, log := fastClean(strings.Repeat("a", 100))
	s := stats.New()
	s.Record(text, log, now)
	eq(t, s.MasteryOf('a'), stats.Automatic)
	text, log = fastClean("a")
	s.Record(text, log, now.Add(13*24*time.Hour))
	eq(t, s.MasteryOf('a'), stats.Automatic)
	s.Record(text, log, now.Add(13*24*time.Hour+15*24*time.Hour))
	eq(t, s.MasteryOf('a'), stats.Solid)
}

func TestSetMastery(t *testing.T) {
	s := stats.New()
	s.SetMastery('x', stats.Solid)
	eq(t, s.MasteryOf('x'), stats.Solid)
}

func TestWeakestCharsPrefersSlowAndWrong(t *testing.T) {
	gap := func(r rune) time.Duration {
		if r == 'b' {
			return 800 * time.Millisecond
		}
		return 100 * time.Millisecond
	}
	wrongs := func(r rune) int {
		if r == 'b' {
			return 1
		}
		return 0
	}
	text, log := typeText(strings.Repeat("abc", 10), gap, wrongs)
	s := stats.New()
	s.Record(text, log, now)
	got := s.WeakestChars(2, nil)
	eq(t, len(got), 2)
	eq(t, got[0], 'b')
	eq(t, s.Weakness('b') > s.Weakness('a'), true)
	got = s.WeakestChars(5, func(r rune) bool { return r != 'b' })
	eq(t, len(got), 2)
	eq(t, got[0] != 'b' && got[1] != 'b', true)
	bg := s.WeakestBigrams(1, nil)
	eq(t, len(bg), 1)
	eq(t, strings.HasSuffix(bg[0], "b") || strings.HasPrefix(bg[0], "b"), true)
}

func TestWeaknessIgnoresFewSamples(t *testing.T) {
	text, log := typeText("a", func(rune) time.Duration { return time.Second }, func(rune) int { return 3 })
	s := stats.New()
	s.Record(text, log, now)
	eq(t, s.Weakness('a'), 0.0)
	eq(t, len(s.WeakestChars(3, nil)), 0)
}

func TestAverageLatency(t *testing.T) {
	s := stats.New()
	eq(t, s.AverageLatencyMs(), 0.0)
	text, log := fastClean("aaaa")
	s.Record(text, log, now)
	eq(t, s.AverageLatencyMs(), 100.0)
}

func TestStaleDecaySticks(t *testing.T) {
	text, log := fastClean(strings.Repeat("a", 100))
	s := stats.New()
	s.Record(text, log, now)
	text, log = fastClean("aaaaa")
	s.Record(text, log, now.Add(15*24*time.Hour))
	eq(t, s.MasteryOf('a'), stats.Solid)
}

func TestErrorProneCharacterHoldsStableLevel(t *testing.T) {
	s := stats.New()
	for i := 0; i < 6; i++ {
		text, log := typeText(strings.Repeat("a", 10),
			func(rune) time.Duration { return 100 * time.Millisecond }, func(rune) int { return 1 })
		s.Record(text, log, now)
		eq(t, s.MasteryOf('a'), stats.NewChar)
	}
}

func TestWeakestNonPositiveN(t *testing.T) {
	text, log := typeText(strings.Repeat("ab", 10),
		func(rune) time.Duration { return time.Second }, func(rune) int { return 1 })
	s := stats.New()
	s.Record(text, log, now)
	eq(t, len(s.WeakestChars(0, nil)), 0)
	eq(t, len(s.WeakestChars(-1, nil)), 0)
	eq(t, len(s.WeakestBigrams(0, nil)), 0)
}

func TestAfterPauseKeystrokeGivesNoLatencySample(t *testing.T) {
	text, log := fastClean("aaaa") // 100 ms between presses
	log[2].T = log[1].T             // retroactive pause: zero gap, not a real speed
	log[2].AfterPause = true
	log[3].T = log[2].T + 100*time.Millisecond
	s := stats.New()
	s.Record(text, log, now)
	e := s.Chars["a"]
	eq(t, e.Samples, 4) // the error sample is kept
	eq(t, e.LatencyMs, 100.0)
}
