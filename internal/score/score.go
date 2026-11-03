// Package score turns a keystroke log into metrics and grades.
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

// minAccuracy is the accuracy for one and two accuracy stars. Three stars need
// no wrong key at all.
var minAccuracy = [2]float64{0.97, 0.99}

// Value is the score: WPM x accuracy^4, with accuracy in 0..1.
func Value(wpm, accuracy float64) float64 {
	return wpm * math.Pow(accuracy, 4)
}

// CPM is a speed in characters per minute, the unit the app shows. A word is
// five characters.
func CPM(wpm float64) float64 { return wpm * 5 }

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

// Grade is what a Test earned: up to three stars for accuracy and up to three
// for speed, each judged on its own.
type Grade struct{ Accuracy, Speed int }

// GradeOf grades a Test. Accuracy earns three stars only when no key was wrong,
// two from 99 % and one from 97 %. Speed earns a star for each target reached,
// whatever the accuracy.
func GradeOf(wpm, accuracy float64, t Targets) Grade {
	var g Grade
	switch {
	case accuracy >= 1:
		g.Accuracy = 3
	case accuracy >= minAccuracy[1]:
		g.Accuracy = 2
	case accuracy >= minAccuracy[0]:
		g.Accuracy = 1
	}
	for _, target := range t.Speed {
		if wpm < target {
			break
		}
		g.Speed++
	}
	return g
}

// Total is the number of stars in both rows.
func (g Grade) Total() int { return g.Accuracy + g.Speed }

// Better reports whether g beats o: more accuracy stars, and on equal accuracy
// more speed stars. Equal grades are not better; the caller breaks that tie.
func (g Grade) Better(o Grade) bool {
	if g.Accuracy != o.Accuracy {
		return g.Accuracy > o.Accuracy
	}
	return g.Speed > o.Speed
}
