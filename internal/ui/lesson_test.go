package ui_test

import (
	"strings"
	"testing"

	"github.com/jiripisa/tuipe/internal/course"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

func TestLessonShowsFingers(t *testing.T) {
	a, _, _ := playApp(t)
	ui.SetScreen(a, ui.NewLessonScreen(a, firstTeachLesson))
	tu, err := a.Course.Tutorial(firstTeachLesson, 0)
	if err != nil {
		t.Fatal(err)
	}
	showsAll(t, a, tu.Explain...)
	showsAll(t, a, text.LessonPartTeach, text.LessonPartTraining, text.LessonPartTest)
}

func TestLessonShowsTheLanguageOfACodeLesson(t *testing.T) {
	a, _, _ := playApp(t)
	ls := lessonWhere(t, a, func(l course.Lesson) bool { return l.Lang != "" })
	ui.SetScreen(a, ui.NewLessonScreen(a, ls.ID))
	showsAll(t, a, ls.Title)
	if strings.Contains(screen(a), "Naučíte se") {
		t.Errorf("a code lesson lists characters:\n%s", screen(a))
	}
}

func TestLessonShowsTheCharactersOfAReviewLesson(t *testing.T) {
	a, _, _ := playApp(t)
	ls := lessonWhere(t, a, func(l course.Lesson) bool { return l.Review })
	ui.SetScreen(a, ui.NewLessonScreen(a, ls.ID))
	showsAll(t, a, text.LessonReviewFocus(ls.Focus[:3]))
}

func TestLessonEscKeepsTheRow(t *testing.T) {
	a, _, _ := playApp(t)
	id := a.Course.Lessons()[4].ID
	openLessons(a)
	downTo(t, a, id)
	a.Update(enter)
	wantScreen(t, a, "lesson")
	a.Update(esc)
	wantScreen(t, a, "lessons")
	if got := ui.LessonsSelected(a); got != id {
		t.Errorf("back on %s, want %s", got, id)
	}
}

func TestLessonMarksTaught(t *testing.T) {
	a, st, clk := playApp(t)
	ui.SetScreen(a, ui.NewLessonScreen(a, firstTeachLesson))
	if strings.Contains(screen(a), text.LessonTaught) {
		t.Fatal("the mark shows before the lesson is taught")
	}
	a.Update(enter) // Výuka is the first row
	wantScreen(t, a, "teach")
	a.Update(enter)
	for ui.ScreenName(a) == "teach" {
		typeLine(a, clk)
	}
	wantScreen(t, a, "lesson")
	if !st.Profile().Taught[firstTeachLesson] {
		t.Fatal("the lesson is not marked taught")
	}
	showsAll(t, a, text.LessonTaught)
}

func TestLessonEscFromTeachLeavesNoMark(t *testing.T) {
	a, st, _ := playApp(t)
	ui.SetScreen(a, ui.NewLessonScreen(a, firstTeachLesson))
	a.Update(enter)
	a.Update(esc)
	wantScreen(t, a, "lesson")
	if st.Profile().Taught[firstTeachLesson] {
		t.Error("Esc marked the lesson taught")
	}
}

func TestLessonShowsLastTrainingOfEachLength(t *testing.T) {
	a, st, clk := playApp(t)
	id := firstTeachLesson
	addRun(t, st, store.Run{ID: "r1", At: daysAgo(clk, 3), Mode: store.ModeTraining, Length: "short", LessonID: id, WPM: 20})
	addRun(t, st, store.Run{ID: "r2", At: daysAgo(clk, 2), Mode: store.ModeTraining, Length: "short", LessonID: id, WPM: 24})
	addRun(t, st, store.Run{ID: "r3", At: daysAgo(clk, 1), Mode: store.ModeTraining, Length: "long", LessonID: id, WPM: 30})
	addRun(t, st, store.Run{ID: "r4", At: daysAgo(clk, 1), Mode: "lesson", Length: "medium", LessonID: id, WPM: 99}) // an old mode
	ui.SetScreen(a, ui.NewLessonScreen(a, id))
	a.Update(down) // Trénink
	if got := ui.LessonScreenRow(a); got != 1 {
		t.Fatalf("row = %d, want 1", got)
	}
	last := func(wpm string) string { return text.LessonLast(wpm) }
	a.Update(left)                     // the screen starts on the medium length
	wantText(t, a, last("120 zn/min")) // the later short run
	a.Update(right)
	if strings.Contains(screen(a), "495 zn/min") {
		t.Error("an old-style run shows on the medium length")
	}
	wantText(t, a, text.LessonNoResult)
	a.Update(right)
	wantText(t, a, last("150 zn/min"))
	a.Update(right) // stays on the longest
	wantText(t, a, last("150 zn/min"))
	a.Update(left)
	a.Update(left)
	wantText(t, a, last("120 zn/min"))
}

func TestLessonShowsTestResults(t *testing.T) {
	a, st, clk := playApp(t)
	id := firstTeachLesson
	st.Profile().Stars[id] = 2
	addRun(t, st, store.Run{ID: "r1", At: daysAgo(clk, 2), Mode: store.ModeTest, LessonID: id, WPM: 30, Score: 28, Stars: 2})
	addRun(t, st, store.Run{ID: "r2", At: daysAgo(clk, 1), Mode: store.ModeTest, LessonID: id, WPM: 20, Score: 18, Stars: 1})
	ui.SetScreen(a, ui.NewLessonScreen(a, id))
	showsAll(t, a, "★★☆", text.LessonBest("150 zn/min"), text.LessonLast("100 zn/min"))
}

func TestLessonStubsTheRoundsForNow(t *testing.T) {
	a, st, _ := playApp(t)
	ui.SetScreen(a, ui.NewLessonScreen(a, firstTeachLesson))
	for range 2 {
		a.Update(down)
		a.Update(enter)
		wantScreen(t, a, "lesson")
		wantText(t, a, text.LessonNotReady)
	}
	if runs, _ := st.Runs(); len(runs) != 0 {
		t.Errorf("the stub stored %d runs", len(runs))
	}
}

func TestLessonRecordsSkipPausedBest(t *testing.T) {
	runs := []store.Run{
		{ID: "1", Mode: store.ModeTest, LessonID: "L", Score: 10},
		{ID: "2", Mode: store.ModeTest, LessonID: "L", Score: 50, Paused: true},
		{ID: "3", Mode: store.ModeTest, LessonID: "L", Score: 5},
		{ID: "4", Mode: store.ModeTraining, Length: "short", LessonID: "L", Score: 7},
		{ID: "5", Mode: store.ModeTraining, Length: "long", LessonID: "L", Score: 8},
		{ID: "6", Mode: "lesson", LessonID: "L", Score: 99},
		{ID: "7", Mode: store.ModeTest, LessonID: "M", Score: 1},
		{ID: "8", Mode: store.ModePlacement, LessonID: "L", Score: 99},
	}
	got := ui.LessonRecords(runs)
	l := got["L"]
	if b := l.Best["test"]; b == nil || b.ID != "1" {
		t.Errorf("best test = %+v, want run 1", b)
	}
	if b := l.Last["test"]; b == nil || b.ID != "3" {
		t.Errorf("last test = %+v, want run 3", b)
	}
	if b := l.Last["training/short"]; b == nil || b.ID != "4" {
		t.Errorf("last short training = %+v", b)
	}
	if b := l.Last["training/long"]; b == nil || b.ID != "5" {
		t.Errorf("last long training = %+v", b)
	}
	if len(l.Last) != 3 || len(got) != 2 {
		t.Errorf("old-style and placement runs are not ignored: %+v", got)
	}
}
