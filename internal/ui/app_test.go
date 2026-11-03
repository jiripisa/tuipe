package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

func newApp(t *testing.T) (*ui.App, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "cfg"), filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	lib, err := content.Load()
	if err != nil {
		t.Fatal(err)
	}
	st.Profile().FirstRunDone = true // these tests start on the lesson list
	a, err := ui.New(st, lib, ui.WithClock(newClock().now))
	if err != nil {
		t.Fatal(err)
	}
	return a, st
}

func TestNewDefaults(t *testing.T) {
	a, _ := newApp(t)
	if a.Style.ID != "editor" || a.Layout.ID != "macos-czech-qwerty" || a.Course == nil || a.Now == nil {
		t.Errorf("unexpected defaults: style %q layout %q", a.Style.ID, a.Layout.ID)
	}
	if strings.Contains(a.Now().String(), "m=") {
		t.Error("Now must not carry a monotonic reading")
	}
}

func TestCtrlTCyclesStyleAndPersists(t *testing.T) {
	a, st := newApp(t)
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(80, 24))
	tm.Send(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
	if got := st.Config().Style; got != "velin" {
		t.Errorf("config style = %q, want velin", got)
	}
	if a.Style.ID != "velin" {
		t.Errorf("app style = %q, want velin", a.Style.ID)
	}
}

func TestCtrlCQuits(t *testing.T) {
	a, _ := newApp(t)
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(80, 24))
	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

func TestViewIsAltScreenWithFocusReporting(t *testing.T) {
	a, _ := newApp(t)
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	v := a.View()
	if !v.AltScreen || !v.ReportFocus || v.BackgroundColor == nil {
		t.Errorf("view: alt %v focus %v bg %v", v.AltScreen, v.ReportFocus, v.BackgroundColor)
	}
	lines := strings.Split(v.Content, "\n")
	if len(lines) != 24 {
		t.Errorf("lines = %d, want 24", len(lines))
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w != 80 {
			t.Fatalf("line width %d, want 80", w)
		}
	}
	if !strings.Contains(ansi.Strip(v.Content), text.AppName) {
		t.Error("start screen does not show the app name")
	}
}

func TestWindowTooSmallPauses(t *testing.T) {
	a, _ := newApp(t)
	a.Update(tea.WindowSizeMsg{Width: 39, Height: 7})
	plain := ansi.Strip(a.View().Content)
	if !strings.Contains(plain, "víc místa") {
		t.Errorf("no 'more room' message:\n%s", plain)
	}
	// Keys are not delivered to screens; Ctrl+T is still a global key.
	before := a.Style.ID
	a.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if a.Style.ID != before {
		t.Error("style changed by a plain key")
	}
	a.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	if strings.Contains(ansi.Strip(a.View().Content), "víc místa") {
		t.Error("40 x 8 must not show the message")
	}
}

func TestSaveErrShowsWarning(t *testing.T) {
	a, _ := newApp(t)
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.SaveErr(os.ErrPermission)
	if !strings.Contains(ansi.Strip(a.View().Content), "Uložení se nepovedlo") {
		t.Error("save warning not shown")
	}
	a.SaveErr(nil)
	if strings.Contains(ansi.Strip(a.View().Content), "Uložení se nepovedlo") {
		t.Error("warning not cleared")
	}
}

func useTempDirs(t *testing.T) (cfg, data string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "cfg"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	cfg, data, err := store.Paths()
	if err != nil {
		t.Fatal(err)
	}
	return cfg, data
}

func TestSecondInstanceMessage(t *testing.T) {
	cfg, data := useTempDirs(t)
	st, err := store.Open(cfg, data)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	err = ui.Run()
	if err == nil || !strings.Contains(err.Error(), "už běží") {
		t.Errorf("Run error = %v, want the Czech second-instance message", err)
	}
}

func TestNewerDataMessage(t *testing.T) {
	_, data := useTempDirs(t)
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "profile.json"), []byte(`{"Schema": 999}`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ui.Run()
	if err == nil || !strings.Contains(err.Error(), "tuipe upgrade") {
		t.Errorf("Run error = %v, want the upgrade message", err)
	}
}

func TestCentreMeasuresCellsNotRunes(t *testing.T) {
	// Four wide characters take eight cells, so the margin of a 20 cell line is 6.
	got := ui.Centre("漢字漢字", 20, 1)
	if lead := len(got) - len(strings.TrimLeft(got, " ")); lead != 6 {
		t.Errorf("margin = %d, want 6: %q", lead, got)
	}
}

func TestNewReadsTheInjectedClockFromTheFirstScreen(t *testing.T) {
	a, _ := newApp(t)
	if got := a.Now(); !got.Equal(newClock().now()) {
		t.Errorf("Now() = %v, want the injected clock", got)
	}
	st := a.Store
	b, err := ui.New(st, a.Lib)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(b.Now()); d < 0 || d > time.Minute {
		t.Errorf("default clock is off by %v", d)
	}
}

// startWith opens an app on data prepared by prepare, on a wide window.
func startWith(t *testing.T, prepare func(cfgDir, dataDir string)) (*ui.App, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	cfg, data := filepath.Join(dir, "cfg"), filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	prepare(cfg, data)
	st, err := store.Open(cfg, data)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	lib, err := content.Load()
	if err != nil {
		t.Fatal(err)
	}
	st.Profile().FirstRunDone = true
	a, err := ui.New(st, lib, ui.WithClock(newClock().now))
	if err != nil {
		t.Fatal(err)
	}
	a.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	return a, st, data
}

func corruptProfile(t *testing.T) func(cfg, data string) {
	return func(_, data string) {
		if err := os.WriteFile(filepath.Join(data, "profile.json"), []byte("garbage{{"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCorruptedProfileIsToldOnTheListWithThePathOfTheFile(t *testing.T) {
	a, _, data := startWith(t, corruptProfile(t))
	wantScreen(t, a, "lessons")
	wantText(t, a, "Profil byl poškozený")
	wantText(t, a, filepath.Join(data, "profile.json.corrupt-1"))
}

func TestBannerStaysThroughASaveAndGoesWithAKey(t *testing.T) {
	a, _, _ := startWith(t, corruptProfile(t))
	a.SaveErr(nil) // a save that succeeds clears the warning line, not the banner
	wantText(t, a, "Profil byl poškozený")
	a.SaveErr(os.ErrPermission) // and a failed one has its own line
	wantText(t, a, "Profil byl poškozený")
	wantText(t, a, "Uložení se nepovedlo")
	a.SaveErr(nil)
	a.Update(down)
	if strings.Contains(screen(a), "Profil byl poškozený") {
		t.Errorf("the banner stayed after a key:\n%s", screen(a))
	}
}

func TestBannerGoesWhenLeavingTheList(t *testing.T) {
	a, _, _ := startWith(t, corruptProfile(t))
	a.Update(enter) // the selected lesson
	wantScreen(t, a, "lesson")
	ui.SetScreen(a, ui.NewLessonsScreen(a))
	if strings.Contains(screen(a), "Profil byl poškozený") {
		t.Errorf("the banner came back:\n%s", screen(a))
	}
}

func TestUnknownLayoutNoticeSurvivesASave(t *testing.T) {
	a, _, _ := startWith(t, func(cfg, _ string) {
		if err := os.WriteFile(filepath.Join(cfg, "config.toml"), []byte("layout = \"gone\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	a.SaveErr(nil)
	wantText(t, a, "Rozložení „gone“")
}

// withoutBraces is the Czech layout's file without { and §.
func withoutBraces(t *testing.T) string {
	t.Helper()
	var keep []string
	drop := false
	for _, l := range strings.Split(czechCopy(t), "\n") {
		if strings.HasPrefix(l, "[[") {
			drop = false
		}
		if strings.HasPrefix(l, `c = "`) && (strings.Contains(l, `"{"`) || strings.Contains(l, `"§"`)) {
			drop = true
			keep = keep[:len(keep)-1] // the [[char]] line above
		}
		if !drop {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

func TestSkippedContentIsToldOnce(t *testing.T) {
	prepare := func(cfg, _ string) {
		if err := os.MkdirAll(filepath.Join(cfg, "layouts"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfg, "layouts", "user.toml"), []byte(withoutBraces(t)), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfg, "config.toml"), []byte("layout = \"user-test\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, st, _ := startWith(t, prepare)
	if a.Layout.ID != "user-test" {
		t.Fatalf("layout = %s", a.Layout.ID)
	}
	wantText(t, a, "Rozložení Testovací rozložení neumí některé znaky")
	wantText(t, a, "Přeskočeno:")
	if !st.Profile().SkipNoted["user-test"] {
		t.Error("the note is not remembered")
	}
	a.Update(down)
	b, _ := reopen(t, st)
	b.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	if strings.Contains(screen(b), "neumí některé znaky") {
		t.Errorf("told twice:\n%s", screen(b))
	}
}

func TestBannerFitsTheSmallestWindow(t *testing.T) {
	a, _, _ := startWith(t, corruptProfile(t))
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	if n := len(strings.Split(ui.RawView(a), "\n")); n > 18 {
		t.Errorf("the list has %d rows with a banner:\n%s", n, screen(a))
	}
	wantText(t, a, "Profil byl poškozený")
	if !strings.Contains(screen(a), "> ") {
		t.Errorf("the selection is off screen:\n%s", screen(a))
	}
}

func TestLaunchOpensTheList(t *testing.T) {
	a, st := newApp(t)
	wantScreen(t, a, "lessons")
	first := a.Course.Lessons()[0]
	if got := ui.LessonsSelected(a); got != first.ID {
		t.Errorf("selected %q, want the first lesson %s", got, first.ID)
	}

	// With a star earned, launching lands on the next lesson, one key from it.
	st.Profile().FirstRunDone = true
	addGraded(t, a, st, first.ID, score.Grade{Speed: 2})
	if err := st.SaveProfile(); err != nil {
		t.Fatal(err)
	}
	b, _ := reopen(t, st)
	wantScreen(t, b, "lessons")
	next := b.Course.Lessons()[1]
	if got := ui.LessonsSelected(b); got != next.ID {
		t.Errorf("selected %q, want the next lesson %s", got, next.ID)
	}
	if got := ui.Grades(b)[first.ID]; got != (score.Grade{Speed: 2}) {
		t.Errorf("grade after the restart = %+v", got)
	}
	b.Update(enter)
	wantScreen(t, b, "lesson")
	if got := ui.LessonScreenID(b); got != next.ID {
		t.Errorf("Enter opened %q, want %s", got, next.ID)
	}
}

// A profile and runs of v0.2 still carry the stars they stored. They load as
// they are, nothing is set aside, and the lesson is graded again from its run.
func TestOldProfileWithStarsLoads(t *testing.T) {
	a, _, data := startWith(t, func(_, data string) {
		for name, body := range map[string]string{
			"profile.json": `{"Schema":2,"FirstRunDone":true,"Stars":{"ch01-01":3}}`,
			"runs.jsonl": `{"Schema":2,"ID":"a3f9c1e","At":"2026-10-30T12:00:00Z","Mode":"test","LessonID":"ch01-01",` +
				`"WPM":7,"Accuracy":0.98,"Score":6.5,"Stars":3,"Length":"","DurationMs":60000,"Paused":false,"HintUsed":false}` + "\n",
		} {
			if err := os.WriteFile(filepath.Join(data, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	})
	wantScreen(t, a, "lessons")
	for _, name := range []string{"profile.json.v2", "runs.jsonl.v2", "profile.json.corrupt-1"} {
		if _, err := os.Stat(filepath.Join(data, name)); err == nil {
			t.Errorf("%s was set aside", name)
		}
	}
	first := a.Course.Lessons()[0]
	if line := lineWith(t, a, first.Title); !hasRows(line, "★☆☆", "★☆☆") || !strings.Contains(line, "35 zn/min") {
		t.Errorf("row %q, want ★☆☆ ★☆☆ from the run at 35 zn/min", line)
	}
	showsAll(t, a, "Intern · ★ 2")
	showsNone(t, a, "ze starší verze")
}
