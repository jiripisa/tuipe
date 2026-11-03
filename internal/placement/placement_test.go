package placement_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/course"
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
	t.Fatal("no Czech layout")
	return nil
}

func setup(t *testing.T) (*course.Course, *content.Library) {
	t.Helper()
	lib, err := content.Load()
	if err != nil {
		t.Fatal(err)
	}
	return course.New(lib, czech(t)), lib
}

func TestPlacementStagesCoverGroups(t *testing.T) {
	c, lib := setup(t)
	stages := placement.Stages(c, lib, 42)
	ids := []string{"letters", "diacritics", "digits", "shift", "option", "code"}
	chapters := [][]string{
		{"ch01", "ch02", "ch03", "ch04"}, {"ch05"}, {"ch06"}, {"ch07"},
		{"ch08", "ch09", "ch10"}, {"ch11", "ch12"},
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
	last := []string{"ch04", "ch05", "ch06", "ch07", "ch10", "ch12"}
	for i, st := range stages {
		allowed := c.ChapterAllowed(last[i])
		for _, r := range st.Text {
			if !allowed(r) {
				t.Fatalf("stage %s has %q", st.ID, r)
			}
		}
	}
	eq(t, stages[5].Text != stages[4].Text, true)
	eq(t, placement.Stages(c, lib, 42)[0].Text, stages[0].Text)
}

// holdsAll fails when text lacks a character of chars.
func holdsAll(t *testing.T, seed int64, stage, text, chars string) {
	t.Helper()
	for _, r := range chars {
		if !strings.ContainsRune(text, r) {
			t.Fatalf("seed %d: stage %s lacks %q: %q", seed, stage, r, text)
		}
	}
}

// Passing a stage moves the recommendation past its chapters, so on any seed it
// must have shown the whole group, the rare keys too.
func TestStagesCoverTheirGroupsOnEverySeed(t *testing.T) {
	c, lib := setup(t)
	const capitals = "ĚŠČŘŽÝÁÍÉÚŮÓĎŤŇ"
	for seed := int64(1); seed <= 2000; seed += 7 {
		stages := placement.Stages(c, lib, seed)
		byID := map[string]string{}
		for _, st := range stages {
			byID[st.ID] = st.Text
		}
		holdsAll(t, seed, "letters", byID["letters"], "abcdefghijklmnopqrstuvwxyz")
		holdsAll(t, seed, "diacritics", byID["diacritics"], "ěščřžýáíéúůóďťň")
		holdsAll(t, seed, "digits", byID["digits"], "0123456789")
		holdsAll(t, seed, "shift", byID["shift"], "+=%/()\"`")
		holdsAll(t, seed, "option", byID["option"], "@#${}[];'~^&*<>\\|°ˇ¨§")
		caps := 0
		for _, r := range capitals {
			if strings.ContainsRune(byID["diacritics"], r) {
				caps++
			}
		}
		if caps < 5 {
			t.Fatalf("seed %d: %d capital diacritics, want at least 5: %q", seed, caps, byID["diacritics"])
		}
	}
}

// The code stage covers chapter 11, which adds Enter, so it must ask for it.
func TestCodeStageIsTwoLinesOfCode(t *testing.T) {
	c, lib := setup(t)
	for seed := int64(1); seed <= 100; seed++ {
		text := placement.Stages(c, lib, seed)[5].Text
		first, second, _ := strings.Cut(text, "\n")
		if strings.TrimSpace(first) == "" || strings.TrimSpace(second) == "" || strings.HasPrefix(text, "//") {
			t.Fatalf("seed %d: code stage %q", seed, text)
		}
	}
}

func TestPlacementTakesAboutFourMinutes(t *testing.T) {
	c, lib := setup(t)
	for seed := int64(1); seed <= 100; seed++ {
		total := 0.0
		for _, st := range placement.Stages(c, lib, seed) {
			total += float64(len([]rune(st.Text))) / 5 / st.MinWPM * 60
		}
		if total < 180 || total > 310 {
			t.Fatalf("seed %d: placement takes %.0f s at the minimum speeds", seed, total)
		}
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
	eq(t, placement.Evaluate(c, stages[:2]), "ch06-01") // letters and diacritics passed
}

func TestPlacementFailingFirstRecommendsTheFirstLesson(t *testing.T) {
	c, _ := setup(t)
	eq(t, placement.Evaluate(c, nil), "ch01-01")
}

func TestPlacementPassingAllRecommendsTheLastLesson(t *testing.T) {
	c, lib := setup(t)
	lessons := c.Lessons()
	eq(t, placement.Evaluate(c, placement.Stages(c, lib, 1)), lessons[len(lessons)-1].ID)
}

// A chapter this layout cannot type is left out of the course, so it cannot be
// where the recommendation lands.
func TestEvaluateSkipsChaptersTheLayoutCannotType(t *testing.T) {
	_, lib := setup(t)
	lay := czech(t)
	lib.Course[5].Lessons = []content.CourseLesson{{ID: "ch06-01", Title: "x", Focus: "\u2603"}} // no route
	c := course.New(lib, lay)
	stages := placement.Stages(c, lib, 1)
	eq(t, placement.Evaluate(c, stages[:2]), "ch07-01") // ch06 would be next
}

func TestEvaluateOfAnEmptyCourseIsNothing(t *testing.T) {
	_, lib := setup(t)
	lib.Course = nil
	c := course.New(lib, czech(t))
	eq(t, placement.Evaluate(c, nil), "")
}

func TestAbortEarly(t *testing.T) {
	eq(t, placement.AbortEarly(30, 10), true)
	eq(t, placement.AbortEarly(30, 3), false)
	eq(t, placement.AbortEarly(19, 19), false) // too few characters to judge
	eq(t, placement.AbortEarly(20, 5), false)  // exactly 25 % is not over it
	eq(t, placement.AbortEarly(20, 6), true)
}

// A group with fewer capitals than a stage asks for shows all of them.
func TestStageWithFewCapitalsShowsThemAll(t *testing.T) {
	_, lib := setup(t)
	lib.Course[4].Lessons = []content.CourseLesson{{ID: "ch05-01", Title: "x", Focus: "ěĚŠČ"}}
	c := course.New(lib, czech(t))
	for seed := int64(1); seed <= 20; seed++ {
		holdsAll(t, seed, "diacritics", placement.Stages(c, lib, seed)[1].Text, "ĚŠČ")
	}
}
