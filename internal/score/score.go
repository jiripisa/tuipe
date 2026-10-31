// Package score turns a keystroke log into metrics and star ratings.
package score

import (
	"math"
	"time"

	"github.com/jiripisa/tuipe/internal/engine"
)

// Metrics describes one finished round. Accuracy is in the range 0..1.
type Metrics struct {
	WPM, Accuracy, Score float64
	Duration             time.Duration
	Chars                int
}

// Targets holds the per-lesson speed thresholds in WPM for one, two and three stars.
type Targets struct{ Speed [3]float64 }

// Star accuracy thresholds.
var minAccuracy = [3]float64{0.92, 0.96, 0.98}

// Value is the score: WPM x accuracy^4, with accuracy in 0..1.
func Value(wpm, accuracy float64) float64 {
	return wpm * math.Pow(accuracy, 4)
}

// Compute derives metrics from the log, so it also works mid-round. Chars is
// the number of characters typed correctly. Accuracy is the share of reached
// characters whose first keystroke was correct, where reached means every
// position with a keystroke (a position with only wrong presses is the cursor).
// Auto cells never appear in the log and so are never counted. WPM is Chars / 5
// / minutes.
func Compute(_ engine.Text, log []engine.Keystroke, d time.Duration) Metrics {
	reached := map[int]bool{}
	correct := map[int]bool{}
	firstTry := 0
	for _, k := range log {
		if !reached[k.Pos] {
			reached[k.Pos] = true
			if k.Correct {
				firstTry++
			}
		}
		if k.Correct {
			correct[k.Pos] = true
		}
	}
	m := Metrics{Duration: d, Chars: len(correct)}
	if len(reached) > 0 {
		m.Accuracy = float64(firstTry) / float64(len(reached))
	}
	if d > 0 {
		m.WPM = float64(m.Chars) / 5 / d.Minutes()
	}
	m.Score = Value(m.WPM, m.Accuracy)
	return m
}

// Stars returns 0..3. Each star needs both its speed target and accuracy
// threshold; the third star also needs that no hint was used.
func Stars(m Metrics, t Targets, hintUsed bool) int {
	stars := 0
	for i := 0; i < 3; i++ {
		if m.WPM < t.Speed[i] || m.Accuracy < minAccuracy[i] || (i == 2 && hintUsed) {
			break
		}
		stars++
	}
	return stars
}
