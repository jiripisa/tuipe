package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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

// The speed beside the stars is that of the Test that earned them: a faster
// run with fewer stars, though its score is higher, does not stand there.
func TestListShowsStarsAndBestSpeed(t *testing.T) {
	a, st, clk := playApp(t)
	id := a.Course.Lessons()[1].ID
	st.Profile().Stars[id] = 2
	addRun(t, st, store.Run{ID: "r1", At: daysAgo(clk, 3), Mode: store.ModeTest, LessonID: id, WPM: 40, Score: 35, Stars: 2})
	addRun(t, st, store.Run{ID: "r2", At: daysAgo(clk, 2), Mode: store.ModeTest, LessonID: id, WPM: 60, Score: 55, Stars: 1})
	addRun(t, st, store.Run{ID: "r3", At: daysAgo(clk, 1), Mode: store.ModeTest, LessonID: id, WPM: 50, Score: 45, Stars: 2})
	addRun(t, st, store.Run{ID: "r4", At: daysAgo(clk, 0), Mode: store.ModeTest, LessonID: id, WPM: 70, Score: 65, Stars: 2, Paused: true})
	openLessons(a)
	l, _ := a.Course.Lesson(id)
	line := lineWith(t, a, l.Title)
	for _, want := range []string{"★★☆", "250 zn/min"} {
		if !strings.Contains(line, want) {
			t.Errorf("row %q lacks %q", line, want)
		}
	}
	showsAll(t, a, "★ 2")

	// The lesson screen names the same run as the best.
	downTo(t, a, id)
	a.Update(enter)
	wantScreen(t, a, "lesson")
	showsAll(t, a, text.LessonBest("250 zn/min"))
}

// A paused round never counts as the best, so when only a paused Test holds
// the stars, the list shows no speed rather than that of a run with fewer.
func TestListShowsNoSpeedWhenOnlyAPausedTestHoldsTheStars(t *testing.T) {
	a, st, clk := playApp(t)
	id := a.Course.Lessons()[1].ID
	st.Profile().Stars[id] = 3
	addRun(t, st, store.Run{ID: "r1", At: daysAgo(clk, 1), Mode: store.ModeTest, LessonID: id, WPM: 60, Score: 55, Stars: 3, Paused: true})
	addRun(t, st, store.Run{ID: "r2", At: daysAgo(clk, 0), Mode: store.ModeTest, LessonID: id, WPM: 50, Score: 45, Stars: 2})
	openLessons(a)
	l, _ := a.Course.Lesson(id)
	line := lineWith(t, a, l.Title)
	if !strings.Contains(line, "★★★") || strings.Contains(line, "zn/min") {
		t.Errorf("row %q, want three stars and no speed", line)
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
	st.Profile().Stars[ls[3].ID] = 1
	openLessons(a)
	if strings.Contains(screen(a), text.LessonRecommended) {
		t.Errorf("a starred lesson is still recommended:\n%s", screen(a))
	}
}

func TestListStartsAtTheNextLesson(t *testing.T) {
	a, st, _ := playApp(t)
	ls := a.Course.Lessons()
	st.Profile().Stars[ls[0].ID], st.Profile().Stars[ls[1].ID] = 3, 1
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
		st.Profile().Stars[l.ID] = 3
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
	showsAll(t, a, a.Course.Chapters()[0].Title, a.Course.Rank(ui.GradesOf(a.Store.Profile().Stars)).String())
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
	for i, l := range ls {
		if i < len(ls)/4 {
			st.Profile().Stars[l.ID] = 3
		}
	}
	st.Profile().Stars[ls[len(ls)/4].ID] = 1
	openLessons(a)
	showsAll(t, a, a.Course.Rank(ui.GradesOf(st.Profile().Stars)).String())
	if got := a.Course.Rank(ui.GradesOf(st.Profile().Stars)).String(); got != "Junior" {
		t.Fatalf("rank = %s, want Junior for a quarter of the stars", got)
	}
	for _, ch := range a.Course.Chapters() {
		showsAll(t, a, ch.Title)
	}
	for id, stars := range map[string]string{ls[0].ID: "★★★", ls[len(ls)/4].ID: "★☆☆", ls[len(ls)-1].ID: "☆☆☆"} {
		l, _ := a.Course.Lesson(id)
		if line := lineWith(t, a, l.Title); !strings.Contains(line, stars) {
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
	if st.Profile().Stars[id] == 0 {
		t.Fatal("a Test must record its stars")
	}
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
	showsAll(t, a, "★")
}
