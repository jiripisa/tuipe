package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/streak"
)

const maxStars = 3

// Finish records a finished round: statistics, the best stars of the lesson and
// the run. Stars come from a Test only. A failed save is shown on the status
// line and also returned; the run itself is always returned.
func (a *App) Finish(spec play.Spec, r play.Result) (store.Run, error) {
	now := a.Now()
	p := a.Store.Profile()
	a.stats().Record(r.Text, r.Log, now)

	stars := 0
	if spec.Kind == play.KindTest {
		stars = r.Stars
	}
	if stars > p.Stars[spec.LessonID] {
		p.Stars[spec.LessonID] = stars
	}

	hash := r.Text.Hash

	run := store.Run{
		ID: runID(now, hash), At: now, Mode: modeName(spec),
		LessonID: spec.LessonID, TextHash: hash, LayoutID: a.Layout.ID,
		WPM: r.Metrics.WPM, Accuracy: r.Metrics.Accuracy, Score: r.Metrics.Score,
		Stars: stars, Length: spec.Length, DurationMs: r.Metrics.Duration.Milliseconds(),
		Paused: r.Paused, HintUsed: r.HintUsed,
	}
	err := errors.Join(a.Store.AppendRun(run), a.Store.SaveProfile())
	a.SaveErr(err)
	return run, err
}

func modeName(spec play.Spec) string {
	switch spec.Kind {
	case play.KindTraining:
		return store.ModeTraining
	case play.KindPlacement:
		return store.ModePlacement
	}
	return store.ModeTest
}

// runID is the short hash that names a run: 7 hex digits of the time and text.
func runID(at time.Time, textHash string) string {
	sum := sha256.Sum256([]byte(at.Format(time.RFC3339Nano) + textHash))
	return hex.EncodeToString(sum[:])[:runIDLen]
}

// uptime is the streak, with days taken in local time.
func (a *App) uptime() streak.Status {
	runs, _ := a.Store.Runs() // a damaged history must not look like a failed save
	days := make([]string, len(runs))
	for i, r := range runs {
		days[i] = r.At.Local().Format(dateLayout)
	}
	return streak.Compute(days, a.Now().Local().Format(dateLayout))
}
