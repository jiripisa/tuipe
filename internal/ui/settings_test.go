package ui_test

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/course"
	"github.com/jiripisa/tuipe/internal/hint"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const (
	homeSettings   = 2 // downs from the first home entry to Nastavení
	rowStyle       = 0
	rowLayout      = 1
	rowUpdateCheck = 2
	rowRetake      = 3
)

func openSettings(t *testing.T, a *ui.App) {
	t.Helper()
	openFromHome(t, a, homeSettings, "settings")
}

// onRow moves the selection from the first row to row n.
func onRow(a *ui.App, n int) {
	for range n {
		a.Update(down)
	}
}

func TestSettingsChangeStyleLayoutUpdateCheck(t *testing.T) {
	a, st, _ := playApp(t)
	writeUserLayout(t, st, czechCopy(t))
	openSettings(t, a)
	showsAll(t, a, text.SettingsTitle, text.SettingsStyle, text.SettingsLayout, text.SettingsUpdateCheck, text.SettingsRetake)

	// Vzhled: Enter and the arrows change it, and Ctrl+T agrees with the row.
	a.Update(enter)
	if a.Style.ID != style.Velin {
		t.Fatalf("style after Enter = %s, want velin", a.Style.ID)
	}
	a.Update(right)
	if a.Style.ID != style.Fosfor {
		t.Fatalf("style after Right = %s, want fosfor", a.Style.ID)
	}
	a.Update(left)
	if a.Style.ID != style.Velin {
		t.Fatalf("style after Left = %s, want velin", a.Style.ID)
	}

	// Rozložení.
	onRow(a, rowLayout)
	a.Update(enter)
	if a.Layout.ID != "user-test" {
		t.Fatalf("layout after Enter = %s, want user-test", a.Layout.ID)
	}

	// Kontrola aktualizací is on by default.
	onRow(a, 1)
	a.Update(enter)
	if st.Config().UpdateCheck {
		t.Fatal("update check still on after Enter")
	}

	if err := st.SaveProfile(); err != nil {
		t.Fatal(err)
	}
	b, again := reopen(t, st)
	cfg := again.Config()
	if cfg.Style != "velin" || cfg.Layout != "user-test" || cfg.UpdateCheck {
		t.Fatalf("config after reopening = %+v, want velin, user-test, update check off", cfg)
	}
	if b.Style.ID != style.Velin || b.Layout.ID != "user-test" {
		t.Errorf("a new app starts with %s and %s", b.Style.ID, b.Layout.ID)
	}

	// Right turns the check on again, and that persists too.
	b.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	openSettings(t, b)
	onRow(b, rowUpdateCheck)
	b.Update(right)
	if _, third := reopen(t, again); !third.Config().UpdateCheck {
		t.Error("update check off after turning it on again")
	}
}

func TestSettingsEscGoesHomeAndRowsWrap(t *testing.T) {
	a, _, _ := playApp(t)
	openSettings(t, a)
	a.Update(tea.KeyPressMsg{Code: tea.KeyUp}) // wraps to the last row
	a.Update(enter)
	if ui.ScreenName(a) != "placement" {
		t.Fatalf("Up from the first row then Enter: %s, want the placement test", ui.ScreenName(a))
	}
	ui.SetScreen(a, ui.NewSettingsScreen(a))
	a.Update(esc)
	if ui.ScreenName(a) != "home" {
		t.Errorf("Esc: %s, want home", ui.ScreenName(a))
	}
}

func TestLayoutChangeTakesEffectAtOnce(t *testing.T) {
	a, st, clk := playApp(t)
	writeUserLayout(t, st, czechCopy(t))
	before := a.Curr
	oldID := a.Layout.ID
	openSettings(t, a)
	onRow(a, rowLayout)
	a.Update(enter)
	if a.Layout.ID != "user-test" || a.Curr == before {
		t.Fatalf("layout %s, curriculum rebuilt %v: the change must reach the session", a.Layout.ID, a.Curr != before)
	}
	showsAll(t, a, text.SettingsLayoutWarn)

	// Statistics go to the new layout and leave the old one alone.
	a.Update(esc)
	startLesson(t, a)
	typeKeys(a, clk, 5)
	a.Update(esc)
	a.Update(down)
	a.Update(down)
	a.Update(enter) // leave: the keys so far reach the stats
	if n := len(st.Profile().Stats["user-test"].Chars); n == 0 {
		t.Error("the round after the change did not feed the new layout's stats")
	}
	if old := st.Profile().Stats[oldID]; old != nil && len(old.Chars) != 0 {
		t.Errorf("the old layout's stats changed: %v", old.Chars)
	}
}

func TestUserLayoutFromConfigDirIsListed(t *testing.T) {
	a, st, _ := playApp(t)
	writeUserLayout(t, st, czechCopy(t))
	openSettings(t, a)
	showsAll(t, a, "macOS Czech QWERTY", "Testovací rozložení")
	onRow(a, rowLayout)
	a.Update(enter)
	if a.Layout.ID != "user-test" {
		t.Fatalf("layout = %s, want the user's", a.Layout.ID)
	}
	a.Update(enter) // wraps back to the built-in one
	if a.Layout.ID != "macos-czech-qwerty" {
		t.Fatalf("layout after the second Enter = %s, want it to wrap", a.Layout.ID)
	}
	a.Update(left) // and Left goes back to the user's
	if a.Layout.ID != "user-test" {
		t.Fatalf("layout after Left = %s, want the user's", a.Layout.ID)
	}
	if _, again := reopen(t, st); again.Config().Layout != "user-test" {
		t.Errorf("saved layout = %q", again.Config().Layout)
	}
}

func TestBrokenUserLayoutIsReportedNotFatal(t *testing.T) {
	a, st, _ := playApp(t)
	writeUserLayout(t, st, "this is [[[ not a layout")
	if err := os.WriteFile(filepath.Join(st.ConfigDir(), "layouts", "good.toml"), []byte(czechCopy(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	openSettings(t, a)
	showsAll(t, a, fmt.Sprintf(text.SettingsBrokenLayout, "user.toml"), "Testovací rozložení")
	onRow(a, rowLayout)
	a.Update(enter)
	if a.Layout.ID != "user-test" {
		t.Errorf("the good layout is not usable beside the broken one: %s", a.Layout.ID)
	}
}

func TestRetakePlacementFromSettings(t *testing.T) {
	a, st, _ := playApp(t)
	done := a.Curr.Chapters()[0].ID
	st.Profile().Completed[done] = true
	openSettings(t, a)
	onRow(a, rowRetake)
	a.Update(enter)
	wantScreen(t, a, "placement")
	showsAll(t, a, text.PlacementTitle)
	a.Update(esc) // give up the test: the job offer follows
	showsAll(t, a, text.OfferTitle)
	a.Update(enter)
	wantScreen(t, a, "home")
	if !st.Profile().Completed[done] {
		t.Error("retaking the placement lowered the rank")
	}
}

func TestActiveLayoutStaysVisibleInALongList(t *testing.T) {
	a, st, _ := playApp(t)
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	writeUserLayout(t, st, czechCopy(t))
	for i := range 12 {
		body := strings.NewReplacer(`id = "user-test"`, fmt.Sprintf(`id = "user-%02d"`, i),
			`name = "Testovací rozložení"`, fmt.Sprintf(`name = "Rozložení č. %02d"`, i)).Replace(czechCopy(t))
		if err := os.WriteFile(filepath.Join(st.ConfigDir(), "layouts", fmt.Sprintf("l%02d.toml", i)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	openSettings(t, a)
	onRow(a, rowLayout)
	for range 9 {
		a.Update(enter)
		if !strings.Contains(screen(a), "• "+a.Layout.Name) {
			t.Fatalf("the active layout %q is not marked in view:\n%s", a.Layout.Name, screen(a))
		}
	}
}

func TestActiveLayoutMissingFromTheListIsSaid(t *testing.T) {
	a, st, _ := playApp(t)
	writeUserLayout(t, st, czechCopy(t))
	openSettings(t, a)
	a.Layout = &layout.Layout{ID: "gone", Name: "Zmizelé"} // a file removed while the app runs
	showsAll(t, a, fmt.Sprintf(text.SettingsLayoutMissing, "Zmizelé"))
	onRow(a, rowLayout)
	a.Update(enter)
	if a.Layout.ID != "macos-czech-qwerty" {
		t.Errorf("Enter from an unlisted layout gave %s, want the first one", a.Layout.ID)
	}
}

// fitCase reaches a screen through the app.
type fitCase struct {
	name  string
	reach func(t *testing.T) *ui.App
}

func playReach(f func(t *testing.T, a *ui.App, st *store.Store, clk *clock)) func(t *testing.T) *ui.App {
	return func(t *testing.T) *ui.App {
		a, st, clk := playApp(t)
		f(t, a, st, clk)
		return a
	}
}

func firstRunReach(f func(t *testing.T, a *ui.App)) func(t *testing.T) *ui.App {
	return func(t *testing.T) *ui.App {
		a, _, _ := firstRunApp(t)
		f(t, a)
		return a
	}
}

func statsTab(n int) func(t *testing.T) *ui.App {
	return playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
		id, _ := lessonOf(t, a, 1)
		for i := range 40 {
			addRun(t, st, store.Run{ID: fmt.Sprintf("r%05d", i), At: daysAgo(clk, i/3), Mode: "lesson", LessonID: id,
				WPM: float64(30 + i), Accuracy: 0.95, Stars: i % 4})
		}
		chars := map[rune]stats.Entry{}
		for i, r := range "asdfjklqwerty" {
			chars[r] = stats.Entry{LatencyMs: float64(1000 - 50*i), Samples: 10}
		}
		setStats(a, chars)
		openStats(t, a)
		for range n {
			a.Update(tab)
		}
	})
}

var fitCases = []fitCase{
	{"home-dense", playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
		denseHome(t, a, st, clk)
		_, title := longestLesson(a)
		wantText(t, a, title[:20])
		wantText(t, a, "Je tu novější verze")
		wantText(t, a, text.HomeBudgetSpent)
	})},
	{"results-baseline-paused", playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
		id, _ := longestLesson(a)
		for _, pause := range []bool{false, true} {
			ui.SetScreen(a, lessonRound(t, a, id, nil))
			a.Update(ui.TickMsg(a))
			for i, r := range ui.RoundKeys(a) {
				clk.advance(200 * time.Millisecond)
				if pause && i == 5 {
					a.Update(esc)
					clk.advance(time.Minute)
					a.Update(esc)
				}
				a.Update(key(r))
			}
			wantScreen(t, a, "results")
		}
		wantText(t, a, text.ResultsOld)
		wantText(t, a, text.ResultsPaused)
	})},
	{"career", playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) { openFromHome(t, a, homeCareer, "career") })},
	{"lessons", playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
		first := a.Course.Lessons()[0]
		st.Profile().Stars[first.ID] = 2
		st.Profile().Taught[a.Course.Lessons()[1].ID] = true
		st.Profile().Recommended = a.Course.Lessons()[3].ID
		a.Banner = append(a.Banner, "Soubor byl odložen stranou.")
		a.UpdateNotice = "Je tu novější verze"
		ui.SetScreen(a, ui.NewLessonsScreen(a))
	})},
	{"lesson", playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
		id := a.Course.Lessons()[0].ID
		st.Profile().Stars[id], st.Profile().Taught[id] = 2, true
		addRun(t, st, store.Run{ID: "r1", At: daysAgo(clk, 1), Mode: store.ModeTest, LessonID: id, WPM: 20, Stars: 2})
		ui.SetScreen(a, ui.NewLessonScreen(a, id))
	})},
	{"lesson-review", playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
		ls := lessonWhere(t, a, func(l course.Lesson) bool { return l.Review && len(l.Focus) > 15 })
		ui.SetScreen(a, ui.NewLessonScreen(a, ls.ID))
	})},
	{"lesson-code", playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
		ls := lessonWhere(t, a, func(l course.Lesson) bool { return l.Lang != "" })
		ui.SetScreen(a, ui.NewLessonScreen(a, ls.ID))
	})},
	{"stats-gitlog", statsTab(0)},
	{"stats-heatmap", statsTab(1)},
	{"stats-trends", statsTab(2)},
	{"stats-records", statsTab(3)},
	{"pause", playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
		id, _ := longestLesson(a)
		ui.SetScreen(a, lessonRound(t, a, id, nil))
		a.Update(ui.TickMsg(a))
		typeKeys(a, clk, 3)
		a.Update(esc)
		wantText(t, a, text.PauseTitle)
	})},
	{"job-offer-no-change", playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
		longestRankAndChapter(a)
		ui.SetScreen(a, ui.NewPlacementScreen(a, nil))
		a.Update(esc)
		wantText(t, a, text.OfferTitle)
	})},
	{"job-offer-placed", playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
		ui.SetScreen(a, ui.NewPlacementScreen(a, nil))
		for range 40 {
			if ui.ScreenName(a) == "placement" && strings.Contains(screen(a), text.OfferTitle) {
				wantText(t, a, "před nástupem")
				return
			}
			switch ui.ScreenName(a) {
			case "placement":
				clk.advance(time.Second)
				a.Update(enter)
			case "round":
				typeKeys(a, clk, -1)
			}
		}
		t.Fatalf("the placement test did not reach its offer:\n%s", screen(a))
	})},
	{"settings", func(t *testing.T) *ui.App {
		a, st, _ := playApp(t)
		writeUserLayout(t, st, "broken [[[")
		for i := range 8 { // many layouts and a second broken file: the longest state
			body := strings.Replace(czechCopy(t), `id = "user-test"`, fmt.Sprintf(`id = "user-%d"`, i), 1)
			if err := os.WriteFile(filepath.Join(st.ConfigDir(), "layouts", fmt.Sprintf("good%d.toml", i)), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(st.ConfigDir(), "layouts", "worse.toml"), []byte("[[["), 0o644); err != nil {
			t.Fatal(err)
		}
		openSettings(t, a)
		onRow(a, rowLayout)
		for range 5 {
			a.Update(enter) // the stats warning shows and the active layout is deep in the list
		}
		onRow(a, 2) // and the retake note
		return a
	}},
	{"teach-explain", playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
		openTeach(t, a, firstTeachLesson)
		wantText(t, a, text.HintTeachExplain)
	})},
	{"teach-line", playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
		openTeach(t, a, firstTeachLesson)
		a.Update(enter)
		typeLine(a, clk)
		typeLine(a, clk)
		a.Update(key('x')) // a mistake shows too
		wantText(t, a, text.HintTeach)
	})},
	{"teach-code", playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
		openTeach(t, a, lessonWhere(t, a, func(l course.Lesson) bool { return l.Lang != "" }).ID)
		a.Update(enter)
		for _, r := range ui.TeachKeys(a)[:3] {
			a.Update(key(r))
		}
	})},
	{"teach-review", playReach(func(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
		openTeach(t, a, lessonWhere(t, a, func(l course.Lesson) bool { return l.Review && l.Lang == "" }).ID)
		a.Update(enter)
	})},
	{"first-run-style", firstRunReach(func(t *testing.T, a *ui.App) {})},
	{"first-run-check", firstRunReach(func(t *testing.T, a *ui.App) { a.Update(enter) })},
	{"first-run-wrong-char", firstRunReach(func(t *testing.T, a *ui.App) {
		a.Update(enter)
		a.Update(key('x'))
		showsAll(t, a, text.FirstRunCheckWrongHint)
	})},
	{"first-run-picker", firstRunReach(func(t *testing.T, a *ui.App) {
		a.Update(enter)
		a.Update(key('x'))
		a.Update(enter)
		showsAll(t, a, text.FirstRunPickTitle)
	})},
	{"first-run-option", firstRunReach(func(t *testing.T, a *ui.App) {
		a.Update(enter)
		a.Update(key('ě'))
		a.Update(tea.KeyPressMsg{Code: 'q', Mod: tea.ModAlt})
		showsAll(t, a, text.FirstRunOptionTitle)
	})},
	{"first-run-no-layout", firstRunReach(func(t *testing.T, a *ui.App) {
		t.Cleanup(ui.SetGOOS("linux"))
		a.Update(enter)
		showsAll(t, a, "docs/layouts.md")
	})},
	{"first-run-fork", firstRunReach(func(t *testing.T, a *ui.App) { passChecks(t, a) })},
	{"first-run-hands", firstRunReach(func(t *testing.T, a *ui.App) {
		passChecks(t, a)
		a.Update(enter)
		showsAll(t, a, "Domovská pozice")
	})},
}

// denseHome fills the home screen: the top rank, a long uptime with the
// forgiven day spent, the longest lesson to continue, a
// stand-in update notice and a save warning.
func denseHome(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
	t.Helper()
	p := st.Profile()
	for _, ch := range a.Curr.Chapters() {
		p.Completed[ch.ID] = true
	}
	for i := range 12 {
		if i == 3 { // one missed day, forgiven
			continue
		}
		addRun(t, st, store.Run{ID: fmt.Sprintf("u%05d", i), At: daysAgo(clk, i), Mode: "drill", WPM: 40, Accuracy: 0.95})
	}
	id, _ := longestLesson(a)
	p.Resume = &store.Resume{Kind: "lesson", LessonID: id}
	a.UpdateNotice = "Je tu novější verze 1.2.3. Nainstalujete ji příkazem tuipe upgrade."
	ui.SetScreen(a, ui.NewHomeScreen(a))
	a.SaveErr(errors.New("open /Users/někdo/.local/share/tuipe/profile.json: permission denied"))
}

func longestLesson(a *ui.App) (id, title string) {
	for _, ch := range a.Curr.Chapters() {
		for _, l := range ch.Lessons {
			if utf8.RuneCountInString(l.Title) > utf8.RuneCountInString(title) {
				id, title = l.ID, l.Title
			}
		}
	}
	return id, title
}

// longestHints are the n characters of cand whose typing hints read longest.
func longestHints(a *ui.App, cand string, n int) []rune {
	rs := []rune(cand)
	slices.SortStableFunc(rs, func(x, y rune) int {
		return cmp.Compare(utf8.RuneCountInString(hint.Text(a.Layout, y)), utf8.RuneCountInString(hint.Text(a.Layout, x)))
	})
	return rs[:n]
}

// longestRankAndChapter completes chapters up to where the rank and the next
// chapter title read longest together.
func longestRankAndChapter(a *ui.App) {
	p := a.Store.Profile()
	best, bestAt := 0, 0
	for k := 0; k <= len(a.Curr.Chapters()); k++ {
		for i, ch := range a.Curr.Chapters() {
			p.Completed[ch.ID] = i < k
		}
		n := utf8.RuneCountInString(a.Curr.RankOf(p).String() + a.Curr.CurrentChapter(p).Title)
		if n > best {
			best, bestAt = n, k
		}
	}
	for i, ch := range a.Curr.Chapters() {
		p.Completed[ch.ID] = i < bestAt
	}
}

func TestAllSharedScreensFitInAllStyles(t *testing.T) {
	sizes := [][2]int{{60, 18}, {80, 24}, {80, 45}}
	for _, c := range fitCases {
		t.Run(c.name, func(t *testing.T) {
			// Installed before the screen is reached, so the renders of the setup are
			// checked too. The hook is a package variable: no test in internal/ui or
			// internal/ui/style may call t.Parallel (none does).
			var where string
			var clips []string
			where = c.name + " (setup)"
			style.ClipHook = func(what string) { clips = append(clips, where+": "+what) }
			t.Cleanup(func() { style.ClipHook = nil })
			a := c.reach(t)
			for _, st := range style.All() {
				for _, sz := range sizes {
					w, h := sz[0], sz[1]
					a.Style = st
					a.Update(tea.WindowSizeMsg{Width: w, Height: h})
					// The screen's own view, before the app pads or cuts it.
					where = fmt.Sprintf("%s %s %dx%d", c.name, st.ID, w, h)
					lines := strings.Split(ui.RawView(a), "\n")
					if len(lines) != h {
						t.Errorf("%s %dx%d: %d lines, want %d:\n%s", st.ID, w, h, len(lines), h, ansi.Strip(strings.Join(lines, "\n")))
						continue
					}
					for i, l := range lines {
						if got := ansi.StringWidth(l); got != w {
							t.Errorf("%s %dx%d line %d: width %d, want %d: %q", st.ID, w, h, i, got, w, ansi.Strip(l))
							break
						}
					}
					// The window as the app draws it, with a save warning if one is up.
					full := strings.Split(a.View().Content, "\n")
					if len(full) != h {
						t.Errorf("%s %dx%d: the app draws %d lines", st.ID, w, h, len(full))
					}
					for i, l := range full {
						if got := ansi.StringWidth(l); got != w {
							t.Errorf("%s %dx%d app line %d: width %d: %q", st.ID, w, h, i, got, ansi.Strip(l))
							break
						}
					}
				}
			}
			for _, c := range clips {
				t.Errorf("content was clipped, %s", c)
			}
		})
	}
}
