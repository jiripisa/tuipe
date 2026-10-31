package placement_test

import (
	"slices"
	"testing"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/curriculum"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/placement"
	"github.com/jiripisa/tuipe/internal/score"
)

func eq[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func setup(t *testing.T) (*curriculum.Curriculum, *content.Library) {
	t.Helper()
	lib, err := content.Load()
	if err != nil {
		t.Fatal(err)
	}
	all, err := layout.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range all {
		if l.ID == "macos-czech-qwerty" {
			return curriculum.New(lib, l), lib
		}
	}
	t.Fatal("no Czech layout")
	return nil, nil
}

func TestPlacementStagesCoverGroups(t *testing.T) {
	c, lib := setup(t)
	stages := placement.Stages(c, lib, 42)
	ids := []string{"letters", "diacritics", "digits", "shift", "option", "code"}
	chapters := [][]string{
		{"ch00", "ch01", "ch02", "ch03", "ch04"}, {"ch05"}, {"ch06"}, {"ch07"},
		{"ch08", "ch09", "ch10"}, {"ch11"},
	}
	minWPM := []float64{20, 15, 12, 12, 10, 12}
	eq(t, len(stages), 6)
	for i, st := range stages {
		eq(t, st.ID, ids[i])
		eq(t, slices.Equal(st.Chapters, chapters[i]), true)
		eq(t, st.MinWPM, minWPM[i])
		eq(t, st.MinAccuracy, 0.9)
		eq(t, st.Title != "", true)
		eq(t, st.Text != "", true)
	}
}

func TestPlacementStageTextUsesOnlyUnlockedChars(t *testing.T) {
	c, lib := setup(t)
	stages := placement.Stages(c, lib, 42)
	last := []string{"ch04", "ch05", "ch06", "ch07", "ch10", "ch11"}
	for i, st := range stages {
		allowed := c.AllowedAt(last[i])
		for _, r := range st.Text {
			if !allowed(r) {
				t.Fatalf("stage %s has %q", st.ID, r)
			}
		}
	}
	eq(t, stages[5].Text != stages[4].Text, true)
	eq(t, placement.Stages(c, lib, 42)[0].Text, stages[0].Text)
}

func TestPlacementTakesAboutThreeMinutes(t *testing.T) {
	c, lib := setup(t)
	total := 0.0
	for _, st := range placement.Stages(c, lib, 42) {
		total += float64(len([]rune(st.Text))) / 5 / st.MinWPM * 60
	}
	if total < 150 || total > 240 {
		t.Fatalf("placement takes %.0f s at the minimum speeds", total)
	}
}

func TestStageSucceeded(t *testing.T) {
	st := placement.Stage{MinWPM: 20, MinAccuracy: 0.9}
	eq(t, placement.StageSucceeded(st, score.Metrics{WPM: 20, Accuracy: 0.9}), true)
	eq(t, placement.StageSucceeded(st, score.Metrics{WPM: 19, Accuracy: 0.99}), false)
	eq(t, placement.StageSucceeded(st, score.Metrics{WPM: 40, Accuracy: 0.85}), false)
}

func TestPlacementStopsAtFirstFailure(t *testing.T) {
	c, lib := setup(t)
	stages := placement.Stages(c, lib, 1)
	out := placement.Evaluate(c, stages[:2]) // letters and diacritics passed
	eq(t, slices.Equal(out.Completed, []string{"ch00", "ch01", "ch02", "ch03", "ch04", "ch05"}), true)
	eq(t, out.StartChapter, "ch06")
	eq(t, out.Rank, curriculum.Junior)
}

func TestPlacementNoStagesPassedStartsAtZero(t *testing.T) {
	c, _ := setup(t)
	out := placement.Evaluate(c, nil)
	eq(t, out.StartChapter, "ch00")
	eq(t, len(out.Completed), 0)
	eq(t, out.Rank, curriculum.Intern)
}

func TestPlacementAllPassed(t *testing.T) {
	c, lib := setup(t)
	out := placement.Evaluate(c, placement.Stages(c, lib, 1))
	eq(t, out.StartChapter, "ch12")
	eq(t, len(out.Completed), 12)
}

func TestAbortEarly(t *testing.T) {
	eq(t, placement.AbortEarly(30, 10), true)
	eq(t, placement.AbortEarly(30, 3), false)
	eq(t, placement.AbortEarly(19, 19), false) // too few characters to judge
	eq(t, placement.AbortEarly(20, 5), false)  // exactly 25 % is not over it
	eq(t, placement.AbortEarly(20, 6), true)
}
