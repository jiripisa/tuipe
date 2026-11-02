package ui_test

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/course"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const firstTeachLesson = "ch01-01"

// openTeach puts the Výuka of a lesson on screen; leaving it opens the career list.
func openTeach(t *testing.T, a *ui.App, id string) {
	t.Helper()
	ui.SetScreen(a, ui.NewTeachScreen(a, id, func(a *ui.App) ui.Screen { return ui.NewCareerScreen(a) }))
	wantScreen(t, a, "teach")
}

// typeLine types the rest of the current line of Výuka.
func typeLine(a *ui.App, clk *clock) {
	for _, r := range ui.TeachKeys(a) {
		clk.advance(200 * time.Millisecond)
		a.Update(key(r))
	}
}

// lessonWhere returns the first lesson of the course that fits.
func lessonWhere(t *testing.T, a *ui.App, ok func(course.Lesson) bool) course.Lesson {
	t.Helper()
	for _, l := range a.Course.Lessons() {
		if ok(l) {
			return l
		}
	}
	t.Fatal("no such lesson in the course")
	return course.Lesson{}
}

func TestTeachWalksThroughTheLines(t *testing.T) {
	a, _, clk := playApp(t)
	openTeach(t, a, firstTeachLesson)
	tu, err := a.Course.Tutorial(firstTeachLesson, a.Now().UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if ui.TeachStep(a) != -1 {
		t.Fatalf("step = %d, want the explanation first", ui.TeachStep(a))
	}
	showsAll(t, a, tu.Explain[0], fmt.Sprintf("krok 0/%d", len(tu.Lines)), text.HintTeachExplain)
	a.Update(key('f')) // nothing is typed before Enter
	if ui.TeachStep(a) != -1 {
		t.Fatal("a letter left the explanation")
	}
	a.Update(enter)
	for i, line := range tu.Lines {
		if ui.TeachStep(a) != i || ui.TeachLine(a) != line {
			t.Fatalf("step %d line %q, want %d %q", ui.TeachStep(a), ui.TeachLine(a), i, line)
		}
		showsAll(t, a, fmt.Sprintf("krok %d/%d", i+1, len(tu.Lines)), text.HintTeach)
		for _, done := range tu.Lines[:i] {
			showsAll(t, a, done) // finished lines stay
		}
		typeLine(a, clk)
	}
	wantScreen(t, a, "career")
}

func TestTeachWrongKeyDoesNotAdvance(t *testing.T) {
	a, _, _ := playApp(t)
	openTeach(t, a, firstTeachLesson)
	a.Update(enter)
	before := screen(a)
	bad := style.ByID(a.Style.ID).Bad
	if strings.Contains(a.View().Content, colourParam(bad)) {
		t.Fatal("precondition: the bad colour shows before any mistake")
	}
	a.Update(key('x'))
	if ui.TeachPos(a) != 0 || ui.TeachStep(a) != 0 {
		t.Fatalf("pos %d step %d after a wrong key", ui.TeachPos(a), ui.TeachStep(a))
	}
	if screen(a) != before {
		t.Errorf("the text changed after a wrong key:\n%s", screen(a))
	}
	if !strings.Contains(a.View().Content, colourParam(bad)) {
		t.Error("a wrong key has no visible response")
	}
	a.Update(key('f'))
	if ui.TeachPos(a) != 1 {
		t.Errorf("pos = %d after the right key", ui.TeachPos(a))
	}
}

// colourParam is the colour as a true-colour foreground or background writes it.
func colourParam(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("2;%d;%d;%d", r>>8, g>>8, b>>8)
}

func TestTeachSetsTheMarkOnlyAtTheEnd(t *testing.T) {
	a, st, clk := playApp(t)
	openTeach(t, a, firstTeachLesson)
	n := len(mustTutorial(t, a, firstTeachLesson).Lines)
	a.Update(enter)
	for i := range n {
		if st.Profile().Taught[firstTeachLesson] {
			t.Fatalf("marked before line %d of %d was done", i+1, n)
		}
		typeLine(a, clk)
	}
	if !st.Profile().Taught[firstTeachLesson] {
		t.Fatal("not marked after the last line")
	}
	wantScreen(t, a, "career")
}

func mustTutorial(t *testing.T, a *ui.App, id string) course.Tutorial {
	t.Helper()
	tu, err := a.Course.Tutorial(id, a.Now().UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	return tu
}

func TestTeachEscSetsNoMark(t *testing.T) {
	for _, mid := range []bool{false, true} {
		a, st, clk := playApp(t)
		openTeach(t, a, firstTeachLesson)
		if mid {
			a.Update(enter)
			typeLine(a, clk)
			a.Update(key('j'))
		}
		a.Update(esc)
		wantScreen(t, a, "career")
		if len(st.Profile().Taught) != 0 {
			t.Errorf("mid=%v: Taught = %v after Esc", mid, st.Profile().Taught)
		}
	}
}

func TestCtrlCInTeachSetsNoMark(t *testing.T) {
	a, st, clk := playApp(t)
	openTeach(t, a, firstTeachLesson)
	a.Update(enter)
	for range len(mustTutorial(t, a, firstTeachLesson).Lines) - 1 {
		typeLine(a, clk)
	}
	_, cmd := a.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+C did not quit")
	}
	a.Shutdown()
	if len(st.Profile().Taught) != 0 {
		t.Errorf("Taught = %v after Ctrl+C on the last line", st.Profile().Taught)
	}
}

func TestTeachRecordsNoStatsAndNoRun(t *testing.T) {
	a, st, clk := playApp(t)
	openTeach(t, a, firstTeachLesson)
	a.Update(key('x'))
	a.Update(enter)
	a.Update(key('x'))
	for range len(mustTutorial(t, a, firstTeachLesson).Lines) {
		typeLine(a, clk)
	}
	a.Shutdown()
	if s := st.Profile().Stats[a.Layout.ID]; s != nil && s.Typed() != 0 {
		t.Errorf("%d keystrokes reached the statistics", s.Typed())
	}
	runs, err := st.Runs()
	if err != nil || len(runs) != 0 {
		t.Errorf("runs = %v, %v", runs, err)
	}
}

func TestTeachHighlightsTheKeys(t *testing.T) {
	a, _, _ := playApp(t)
	keyOf := func(r rune) string {
		route, ok := a.Layout.Route(r)
		if !ok {
			t.Fatalf("no route for %q", r)
		}
		return route[0].Key
	}
	openTeach(t, a, firstTeachLesson)
	for _, id := range mustTutorial(t, a, firstTeachLesson).Keys {
		if !slices.Contains(ui.TeachHighlights(a), id) {
			t.Errorf("explanation does not mark %s: %v", id, ui.TeachHighlights(a))
		}
	}
	a.Update(enter)
	if !slices.Contains(ui.TeachHighlights(a), keyOf('f')) {
		t.Errorf("the keys of the line are not marked: %v", ui.TeachHighlights(a))
	}

	// A capital letter shows which Shift to hold, left or right.
	l := lessonWhere(t, a, func(l course.Lesson) bool {
		return l.Lang == "" && !l.Review && slices.ContainsFunc(l.Focus, unicode.IsUpper)
	})
	openTeach(t, a, l.ID)
	a.Update(enter)
	for _, r := range ui.TeachKeys(a) {
		if !unicode.IsUpper(r) {
			a.Update(key(r))
			continue
		}
		hi := ui.TeachHighlights(a)
		if !slices.Contains(hi, style.ShiftLeft) && !slices.Contains(hi, style.ShiftRight) {
			t.Fatalf("%q needs Shift, marked: %v", r, hi)
		}
		return
	}
	t.Fatal("the lesson never asks for a capital")
}

func TestTeachKeepsTheLineAtTinySize(t *testing.T) {
	for _, st := range style.All() {
		for _, explain := range []bool{true, false} {
			a, _, clk := playApp(t)
			a.Style = st
			a.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
			openTeach(t, a, firstTeachLesson)
			tu := mustTutorial(t, a, firstTeachLesson)
			if explain {
				showsAll(t, a, tu.Explain[0], text.HintTeachExplain)
				continue
			}
			a.Update(enter)
			typeLine(a, clk) // line 1 done, line 2 current
			showsAll(t, a, tu.Lines[1], text.HintTeach)
			if lines := strings.Split(screen(a), "\n"); len(lines) != 8 {
				t.Errorf("%s: %d lines at 40x8", st.ID, len(lines))
			}
		}
	}
}

func TestTeachDropsRowsInOrder(t *testing.T) {
	a, _, clk := playApp(t)
	openTeach(t, a, firstTeachLesson)
	tu := mustTutorial(t, a, firstTeachLesson)
	a.Update(enter)
	typeLine(a, clk)
	typeLine(a, clk)
	// The window shrinks: first the finished lines go, then the drawing, then the explanation.
	var last [3]bool
	for h := 40; h >= 8; h-- {
		a.Update(tea.WindowSizeMsg{Width: 60, Height: h})
		v := screen(a)
		if !strings.Contains(v, tu.Lines[2]) {
			t.Fatalf("h=%d: the line being typed is gone:\n%s", h, v)
		}
		got := [3]bool{strings.Contains(v, tu.Lines[1]), strings.Contains(v, "⇧"), strings.Contains(v, tu.Explain[0])}
		for i := range got {
			if got[i] && !last[i] && h < 40 {
				t.Errorf("h=%d: part %d came back", h, i)
			}
		}
		if (got[0] && !got[1]) || (got[1] && !got[2]) {
			t.Errorf("h=%d: finished %v, drawing %v, explanation %v: dropped out of order", h, got[0], got[1], got[2])
		}
		last = got
	}
	if last[0] || last[1] {
		t.Errorf("at 60x8 the finished lines or the drawing are still there: %v", last)
	}
}

func TestTeachOnACodeLesson(t *testing.T) {
	a, st, clk := playApp(t)
	l := lessonWhere(t, a, func(l course.Lesson) bool { return l.Lang != "" })
	openTeach(t, a, l.ID)
	tu := mustTutorial(t, a, l.ID)
	showsAll(t, a, strings.Fields(tu.Explain[0])[0])
	a.Update(enter)
	if !strings.Contains(tu.Lines[0], "\n") {
		t.Skip("the snippet has a single line")
	}
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	typeLine(a, clk)
	wantScreen(t, a, "career")
	if !st.Profile().Taught[l.ID] {
		t.Error("a code lesson is not marked after its snippet")
	}
}

func TestTeachWrapsALongReviewLine(t *testing.T) {
	a, _, _ := playApp(t)
	l := lessonWhere(t, a, func(l course.Lesson) bool { return l.Review && l.Lang == "" })
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	openTeach(t, a, l.ID)
	a.Update(enter)
	line := ui.TeachLine(a)
	if len([]rune(line)) <= 36 {
		t.Fatalf("precondition: line %q is short", line)
	}
	showsAll(t, a, strings.Fields(line)...)
	for _, row := range strings.Split(screen(a), "\n") {
		if w := len([]rune(row)); w != 60 {
			t.Fatalf("row width %d: %q", w, row)
		}
	}
}

func TestTeachIgnoresOptionAsAltOnTheWrongKey(t *testing.T) {
	a, _, _ := playApp(t)
	openTeach(t, a, firstTeachLesson)
	a.Update(enter)
	a.Update(tea.KeyPressMsg{Code: 'x', Text: "x", Mod: tea.ModAlt})
	if ui.TeachPos(a) != 0 {
		t.Fatalf("pos = %d after Alt+x", ui.TeachPos(a))
	}
	wantText(t, a, "Option je Alt")
	a.Update(key('f'))
	if ui.TeachPos(a) != 1 || strings.Contains(screen(a), "Option je Alt") {
		t.Errorf("pos %d, warning still up:\n%s", ui.TeachPos(a), screen(a))
	}
	a.Update(tea.KeyPressMsg{Code: ' ', Text: " ", Mod: tea.ModAlt}) // expected character with Alt: counts
	if ui.TeachPos(a) != 2 {
		t.Errorf("pos = %d after the expected key with Alt", ui.TeachPos(a))
	}
}

func TestCourseIsRebuiltWhenTheLayoutChanges(t *testing.T) {
	a, st, _ := playApp(t)
	writeUserLayout(t, st, czechCopy(t))
	before := a.Course
	openSettings(t, a)
	onRow(a, rowLayout)
	a.Update(enter)
	if a.Layout.ID != "user-test" {
		t.Fatalf("layout = %s", a.Layout.ID)
	}
	if a.Course == nil || a.Course == before {
		t.Error("the course was not rebuilt for the new layout")
	}
}
