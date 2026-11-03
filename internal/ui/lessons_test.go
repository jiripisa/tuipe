package ui_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// openLessons puts the lesson list on screen.
func openLessons(a *ui.App) {
	ui.SetScreen(a, ui.NewLessonsScreen(a))
}

// downTo moves the selection of the list to the lesson with the given id.
func downTo(t *testing.T, a *ui.App, id string) {
	t.Helper()
	for range 200 {
		if ui.LessonsSelected(a) == id {
			return
		}
		a.Update(down)
	}
	t.Fatalf("the list never selected %s", id)
}

// hasRows reports whether line shows the accuracy stars acc and, after them
// and some space, the speed stars speed.
func hasRows(line, acc, speed string) bool {
	for i := 0; ; {
		j := strings.Index(line[i:], acc)
		if j < 0 {
			return false
		}
		i += j + len(acc)
		rest := strings.TrimLeft(line[i:], " ")
		if len(rest) < len(line[i:]) && strings.HasPrefix(rest, speed) {
			return true
		}
	}
}

// lineWith returns the line of the screen that holds s.
func lineWith(t *testing.T, a *ui.App, s string) string {
	t.Helper()
	for _, l := range strings.Split(screen(a), "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	t.Fatalf("no line with %q:\n%s", s, screen(a))
	return ""
}

func TestListOpensEveryLesson(t *testing.T) {
	a, _, _ := playApp(t)
	for _, l := range a.Course.Lessons() {
		openLessons(a)
		wantScreen(t, a, "lessons")
		downTo(t, a, l.ID)
		a.Update(enter)
		wantScreen(t, a, "lesson")
		if got := ui.LessonScreenID(a); got != l.ID {
			t.Fatalf("Enter on %s opened %s", l.ID, got)
		}
	}
}

// Both rows and the speed beside them come from one Test, the best: more
// accuracy stars, then more speed stars, then the higher speed. A paused Test
// never counts.
func TestListShowsBothRowsOfTheBestTest(t *testing.T) {
	a, st, clk := playApp(t)
	id := a.Course.Lessons()[1].ID // speed targets 10, 15 and 20 wpm
	for _, r := range []store.Run{
		{ID: "r1", At: daysAgo(clk, 4), WPM: 15, Accuracy: 0.99},   // ★★ ★★
		{ID: "r2", At: daysAgo(clk, 3), WPM: 20, Accuracy: 0.98},   // ★ ★★★, faster but less accurate
		{ID: "r3", At: daysAgo(clk, 2), WPM: 16, Accuracy: 0.995},  // ★★ ★★ and faster than r1: the best
		{ID: "r4", At: daysAgo(clk, 1), WPM: 15.5, Accuracy: 0.99}, // ★★ ★★, slower than r3
		{ID: "r5", At: daysAgo(clk, 0), WPM: 30, Accuracy: 1, Paused: true},
	} {
		r.Mode, r.LessonID, r.Score = store.ModeTest, id, score.Value(r.WPM, r.Accuracy)
		addRun(t, st, r)
	}
	openLessons(a)
	l, _ := a.Course.Lesson(id)
	line := lineWith(t, a, l.Title)
	if !hasRows(line, "★★☆", "★★☆") || !strings.Contains(line, "80 zn/min") {
		t.Errorf("row %q, want ★★☆ ★★☆ and 80 zn/min", line)
	}
	showsAll(t, a, "★ 4")

	// The lesson screen names the same run as the best.
	downTo(t, a, id)
	a.Update(enter)
	wantScreen(t, a, "lesson")
	if l := lineWith(t, a, text.LessonPartTest+"  "); !hasRows(l, "★★☆", "★★☆") {
		t.Errorf("Test row %q, want ★★☆ ★★☆", l)
	}
	showsAll(t, a, text.LessonBest("80 zn/min"))
}

// Two Tests with the same stars: the faster one is the best, whichever came
// first, and the list, the lesson screen and the rank agree on it.
func TestEqualStarsFasterIsBest(t *testing.T) {
	for name, wpms := range map[string][2]float64{"faster first": {18, 16}, "faster last": {16, 18}} {
		t.Run(name, func(t *testing.T) {
			a, st, clk := playApp(t)
			id := a.Course.Lessons()[1].ID // ★★ from 15 wpm, ★★★ from 20
			for i, wpm := range wpms {
				addRun(t, st, store.Run{ID: fmt.Sprintf("r%d", i), At: daysAgo(clk, 2-i), Mode: store.ModeTest,
					LessonID: id, WPM: wpm, Accuracy: 1, Score: wpm})
			}
			openLessons(a)
			l, _ := a.Course.Lesson(id)
			if line := lineWith(t, a, l.Title); !hasRows(line, "★★★", "★★☆") || !strings.Contains(line, "90 zn/min") {
				t.Errorf("row %q, want ★★★ ★★☆ and 90 zn/min", line)
			}
			want := a.Course.Rank(map[string]score.Grade{id: {Accuracy: 3, Speed: 2}}).String()
			showsAll(t, a, want+" · ★ 5")
			downTo(t, a, id)
			a.Update(enter)
			if l := lineWith(t, a, text.LessonPartTest+"  "); !hasRows(l, "★★★", "★★☆") {
				t.Errorf("Test row %q, want ★★★ ★★☆", l)
			}
			showsAll(t, a, text.LessonBest("90 zn/min"))
		})
	}
}

// No error at all earns three accuracy stars, and accuracy comes first: a slow
// flawless Test beats a fast one with errors.
func TestFlawlessSlowTestBeatsFastWithErrors(t *testing.T) {
	a, st, clk := playApp(t)
	id := a.Course.Lessons()[1].ID
	addRun(t, st, store.Run{ID: "fast", At: daysAgo(clk, 2), Mode: store.ModeTest, LessonID: id, WPM: 30, Accuracy: 0.98, Score: 27})
	addRun(t, st, store.Run{ID: "slow", At: daysAgo(clk, 1), Mode: store.ModeTest, LessonID: id, WPM: 10, Accuracy: 1, Score: 10})
	openLessons(a)
	l, _ := a.Course.Lesson(id)
	if line := lineWith(t, a, l.Title); !hasRows(line, "★★★", "★☆☆") || !strings.Contains(line, "50 zn/min") {
		t.Errorf("row %q, want ★★★ ★☆☆ and 50 zn/min", line)
	}
	downTo(t, a, id)
	a.Update(enter)
	if l := lineWith(t, a, text.LessonPartTest+"  "); !hasRows(l, "★★★", "★☆☆") {
		t.Errorf("Test row %q, want ★★★ ★☆☆", l)
	}
	showsAll(t, a, text.LessonBest("50 zn/min"))
}

// A lesson whose only Tests paused has no stars and no best speed, and the
// list claims no result for it; it is still the next lesson.
func TestPausedTestShowsNoStars(t *testing.T) {
	a, st, clk := playApp(t)
	ls := a.Course.Lessons()
	addGraded(t, a, st, ls[0].ID, score.Grade{Accuracy: 1, Speed: 1})
	addRun(t, st, store.Run{ID: "p", At: daysAgo(clk, 1), Mode: store.ModeTest, LessonID: ls[1].ID, WPM: 30, Accuracy: 1, Score: 30, Paused: true})
	openLessons(a)
	if got := ui.LessonsSelected(a); got != ls[1].ID {
		t.Errorf("selected %s, want %s with only a paused Test", got, ls[1].ID)
	}
	line := lineWith(t, a, ls[1].Title)
	if !hasRows(line, "☆☆☆", "☆☆☆") || strings.Contains(line, "★") || strings.Contains(line, "zn/min") {
		t.Errorf("row %q, want no stars and no speed", line)
	}
	showsAll(t, a, "Intern · ★ 2")

	a.Update(enter)
	wantScreen(t, a, "lesson")
	if l := lineWith(t, a, text.LessonPartTest+"  "); !hasRows(l, "☆☆☆", "☆☆☆") {
		t.Errorf("Test row %q, want ☆☆☆ ☆☆☆", l)
	}
	showsAll(t, a, text.LessonLast("150 zn/min"))
	showsNone(t, a, "★", "nejlepší")

	// The results of a paused Test show no stars either.
	openPart(t, a, ls[1].ID, partTest)
	typeKeys(a, clk, 2)
	a.Update(esc)
	clk.advance(time.Minute)
	a.Update(esc)
	finishRound(t, a, clk, typingStep)
	showsAll(t, a, text.ResultsPaused)
	for _, label := range []string{text.PartSpeed, text.PartAccuracy} {
		if l := lineWith(t, a, label); strings.ContainsAny(l, "★☆") {
			t.Errorf("a paused Test shows stars: %q", l)
		}
	}
}

// The labels of the two rows stand once, above the columns of stars.
func TestListLabelsTheRowsOnce(t *testing.T) {
	a, st, _ := playApp(t)
	first := a.Course.Lessons()[0]
	addGraded(t, a, st, first.ID, score.Grade{Accuracy: 3, Speed: 1})
	for _, w := range []int{60, 80} {
		a.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		openLessons(a)
		s := screen(a)
		if strings.Count(s, "přesnost") != 1 || strings.Count(s, "rychlost") != 1 {
			t.Fatalf("%d columns: the labels do not stand once:\n%s", w, s)
		}
		labels, row := lineWith(t, a, "přesnost"), lineWith(t, a, first.Title)
		col := func(l, s string) int { return utf8.RuneCountInString(l[:max(strings.Index(l, s), 0)]) }
		if col(labels, "přesnost") != col(row, "★★★") || col(labels, "rychlost") != col(row, "★☆☆") {
			t.Errorf("%d columns: the labels are not above their stars:\n%s\n%s", w, labels, row)
		}
	}
}

// Two rows of stars still leave every lesson title whole in 60 columns.
func TestListKeepsTitlesWholeInNarrowWindows(t *testing.T) {
	a, _, _ := playApp(t)
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	openLessons(a)
	for _, l := range a.Course.Lessons() {
		downTo(t, a, l.ID)
		if line := lineWith(t, a, "> "); !hasRows(line, "☆☆☆", "☆☆☆") || !strings.Contains(line, l.Title) {
			t.Errorf("row %q, want both rows and the whole title %q", line, l.Title)
		}
	}
}

func TestListMarksStarted(t *testing.T) {
	a, st, clk := playApp(t)
	ls := a.Course.Lessons()
	st.Profile().Taught[ls[0].ID] = true
	addRun(t, st, store.Run{ID: "r1", At: daysAgo(clk, 1), Mode: store.ModeTraining, Length: "short", LessonID: ls[1].ID, WPM: 20})
	openLessons(a)
	for _, l := range ls[:2] {
		if line := lineWith(t, a, l.Title); !strings.Contains(line, text.LessonStarted) {
			t.Errorf("row %q lacks %q", line, text.LessonStarted)
		}
	}
	if line := lineWith(t, a, ls[2].Title); strings.Contains(line, text.LessonStarted) {
		t.Errorf("an untouched lesson is marked started: %q", line)
	}
}

func TestListMarksRecommended(t *testing.T) {
	a, st, _ := playApp(t)
	ls := a.Course.Lessons()
	st.Profile().Recommended = ls[3].ID
	openLessons(a)
	if line := lineWith(t, a, ls[3].Title); !strings.Contains(line, text.LessonRecommended) {
		t.Errorf("row %q lacks %q", line, text.LessonRecommended)
	}
	addGraded(t, a, st, ls[3].ID, score.Grade{Speed: 1})
	openLessons(a)
	if strings.Contains(screen(a), text.LessonRecommended) {
		t.Errorf("a starred lesson is still recommended:\n%s", screen(a))
	}
}

// A lesson both begun and recommended says doporučeno first, so a window of 60
// columns, which cuts the end of the row, still shows it.
func TestListKeepsRecommendedInNarrowWindows(t *testing.T) {
	a, st, _ := playApp(t)
	id := a.Course.Lessons()[0].ID
	st.Profile().Recommended = id
	st.Profile().Taught[id] = true
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	openLessons(a)
	if line := lineWith(t, a, "> "); !strings.Contains(line, text.LessonRecommended) {
		t.Errorf("row %q lacks %q", line, text.LessonRecommended)
	}
}

func TestListStartsAtTheNextLesson(t *testing.T) {
	a, st, _ := playApp(t)
	ls := a.Course.Lessons()
	addGraded(t, a, st, ls[0].ID, score.Grade{Accuracy: 3, Speed: 3})
	addGraded(t, a, st, ls[1].ID, score.Grade{Speed: 1}) // one star in either row is a start
	addGraded(t, a, st, ls[2].ID, score.Grade{})         // a Test without a star is not
	openLessons(a)
	if got := ui.LessonsSelected(a); got != ls[2].ID {
		t.Errorf("selected %s, want %s", got, ls[2].ID)
	}
	st.Profile().Recommended = ls[5].ID
	openLessons(a)
	if got := ui.LessonsSelected(a); got != ls[5].ID {
		t.Errorf("selected %s, want the recommended %s", got, ls[5].ID)
	}
}

func TestListWithAllStarredStartsAtTheTop(t *testing.T) {
	a, st, _ := playApp(t)
	for _, l := range a.Course.Lessons() {
		addGraded(t, a, st, l.ID, score.Grade{Accuracy: 1})
	}
	openLessons(a)
	if got := ui.LessonsSelected(a); got != a.Course.Lessons()[0].ID {
		t.Errorf("selected %q, want the first lesson", got)
	}
	a.Update(enter)
	wantScreen(t, a, "lesson")
}

func TestListWithStaleRecommendation(t *testing.T) {
	a, st, _ := playApp(t)
	st.Profile().Recommended = "gone-after-a-layout-change"
	openLessons(a)
	if got := ui.LessonsSelected(a); got != a.Course.Lessons()[0].ID {
		t.Errorf("selected %q, want the first lesson", got)
	}
	if strings.Contains(screen(a), text.LessonRecommended) {
		t.Errorf("a lesson that is gone is recommended:\n%s", screen(a))
	}
}

func TestListShowsBanner(t *testing.T) {
	a, _, _ := playApp(t)
	a.Banner = append(a.Banner, "Soubor byl odložen stranou.")
	a.UpdateNotice = "Je tu novější verze"
	openLessons(a)
	showsAll(t, a, "Soubor byl odložen stranou.", "Je tu novější verze")
	a.Update(down)
	if strings.Contains(screen(a), "Soubor byl odložen stranou.") {
		t.Error("the banner outlives a key press")
	}
}

func TestListHasNoRankByChapterAndNoNumbers(t *testing.T) {
	a, _, _ := playApp(t)
	openLessons(a)
	showsAll(t, a, a.Course.Chapters()[0].Title, "Intern") // nothing played yet
	if strings.Contains(screen(a), "1  "+a.Course.Chapters()[0].Title) {
		t.Errorf("a chapter heading is numbered:\n%s", screen(a))
	}
}

func TestSKeyOpensStats(t *testing.T) {
	a, _, _ := playApp(t)
	openLessons(a)
	a.Update(key('s'))
	wantScreen(t, a, "stats")
}

func TestNKeyOpensSettings(t *testing.T) {
	a, _, _ := playApp(t)
	openLessons(a)
	a.Update(key('n'))
	wantScreen(t, a, "settings")
}

func TestReturningToTheListKeepsTheRow(t *testing.T) {
	for name, press := range map[string]rune{"statistics": 's', "settings": 'n'} {
		t.Run(name, func(t *testing.T) {
			a, _, _ := playApp(t)
			id := a.Course.Lessons()[5].ID
			openLessons(a)
			downTo(t, a, id)
			a.Update(key(press))
			a.Update(esc)
			wantScreen(t, a, "lessons")
			if got := ui.LessonsSelected(a); got != id {
				t.Errorf("selected %q, want the row the user left, %s", got, id)
			}
		})
	}
}

func TestEscOnTheListDoesNothing(t *testing.T) {
	a, _, _ := playApp(t)
	openLessons(a)
	a.Update(esc)
	wantScreen(t, a, "lessons")
}

func TestListSurvivesDamagedRuns(t *testing.T) {
	a, _, _, dir := playAppAt(t)
	if err := os.Mkdir(filepath.Join(dir, "runs.jsonl"), 0o755); err != nil { // a history that cannot be read
		t.Fatal(err)
	}
	openLessons(a)
	wantScreen(t, a, "lessons")
}

func TestListKeepsTheSelectionVisible(t *testing.T) {
	a, _, _ := playApp(t)
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	openLessons(a)
	last := a.Course.Lessons()[len(a.Course.Lessons())-1]
	downTo(t, a, last.ID)
	showsAll(t, a, "> ", last.Title)
}

func TestListOffersTheCourseOnly(t *testing.T) {
	a, _, _ := playApp(t)
	showsAll(t, a, "S statistiky", "N nastavení")
	for _, gone := range []string{"Kariéra", "Sprint", "standup", "Rychlovka", "pokračuj", "test-out", "zamčeno"} {
		showsNone(t, a, gone)
	}
}

func TestListShowsChaptersStarsAndRank(t *testing.T) {
	a, st, _ := playApp(t)
	tall(a)
	ls := a.Course.Lessons()
	for _, l := range ls[:len(ls)/4] {
		addGraded(t, a, st, l.ID, score.Grade{Accuracy: 3, Speed: 3})
	}
	addGraded(t, a, st, ls[len(ls)/4].ID, score.Grade{Accuracy: 1})
	openLessons(a)
	// A quarter of all stars, six a lesson, is Junior.
	showsAll(t, a, fmt.Sprintf("Junior · ★ %d", 6*(len(ls)/4)+1))
	for _, ch := range a.Course.Chapters() {
		showsAll(t, a, ch.Title)
	}
	for id, stars := range map[string][2]string{ls[0].ID: {"★★★", "★★★"}, ls[len(ls)/4].ID: {"★☆☆", "☆☆☆"}, ls[len(ls)-1].ID: {"☆☆☆", "☆☆☆"}} {
		l, _ := a.Course.Lesson(id)
		if line := lineWith(t, a, l.Title); !hasRows(line, stars[0], stars[1]) {
			t.Errorf("row %q lacks %q", line, stars)
		}
	}
}

func TestListScrollsKeepingSelectionVisible(t *testing.T) {
	a, _, _ := playApp(t)
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	openLessons(a)
	count := func() int { return strings.Count(view(a), "> ") }
	total := len(a.Course.Lessons())
	for i := 1; i < total; i++ {
		a.Update(down)
		wantScreen(t, a, "lessons")
		if n := len(strings.Split(screen(a), "\n")); n > 18 {
			t.Fatalf("row %d: %d lines, want at most 18", i, n)
		}
		if count() != 1 {
			t.Fatalf("row %d: the selection is not on screen exactly once:\n%s", i, view(a))
		}
	}
	for i := 1; i < total; i++ {
		a.Update(up)
	}
	if got, want := ui.LessonsSelected(a), a.Course.Lessons()[0].ID; got != want {
		t.Fatalf("selected %s, want %s", got, want)
	}
	showsAll(t, a, "> ", a.Course.Lessons()[0].Title)
}

func TestTestStarsReachTheList(t *testing.T) {
	a, st, clk := playApp(t)
	tall(a)
	id := a.Course.Lessons()[0].ID
	a.Update(enter) // the list opens on the first lesson
	a.Update(down)
	a.Update(down)
	a.Update(enter) // its Test
	wantScreen(t, a, "round")
	typeKeys(a, clk, -1)
	runs, _ := st.Runs()
	if len(runs) != 1 || runs[0].Mode != store.ModeTest {
		t.Fatalf("runs = %+v, want one Test", runs)
	}
	a.Update(enter)
	wantScreen(t, a, "lesson")
	a.Update(esc)
	wantScreen(t, a, "lessons")
	if got := ui.LessonsSelected(a); got != id {
		t.Errorf("selected %s, want %s", got, id)
	}
	l, _ := a.Course.Lesson(id)
	if line := lineWith(t, a, l.Title); !hasRows(line, "★★★", "★★★") {
		t.Errorf("row %q, want the stars of the clean, fast Test", line)
	}
}
