package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/score"
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
		stars = oldStars(r.Grade)
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

// runIDLen is the length of the id that names a run in the storage. It is
// shown nowhere.
const runIDLen = 7

// runID is the short hash that names a run in the storage: 7 hex digits of the
// time and text.
func runID(at time.Time, textHash string) string {
	sum := sha256.Sum256([]byte(at.Format(time.RFC3339Nano) + textHash))
	return hex.EncodeToString(sum[:])[:runIDLen]
}

// daysInARow is the days practised in a row, with days taken in local time. A
// Test or a Trénink makes a day; the placement test and Výuka do not.
func daysInARow(runs []store.Run, now time.Time) streak.Status {
	var days []string
	for _, r := range runs {
		if r.Mode == store.ModeTest || r.Mode == store.ModeTraining {
			days = append(days, r.At.Local().Format(dateLayout))
		}
	}
	return streak.Compute(days, now.Local().Format(dateLayout))
}

// oldStars is a TEMPORARY bridge to the single star row of Profile.Stars: the
// speed stars alone, as accuracy no longer gates them. The screens move to Grade in the next change and
// this goes.
func oldStars(g score.Grade) int { return g.Speed }

// gradesOf is a TEMPORARY bridge from Profile.Stars to the grades the course
// rules take: the old number goes into both rows, so the share of all stars
// (and with it the rank) stays what it was. The screens move to Grade in the
// next change and this goes.
func gradesOf(stars map[string]int) map[string]score.Grade {
	grades := make(map[string]score.Grade, len(stars))
	for id, n := range stars {
		grades[id] = score.Grade{Accuracy: n, Speed: n}
	}
	return grades
}
