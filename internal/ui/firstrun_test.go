package ui_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/course"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const userLayout = `id = "user-test"
name = "Testovací rozložení"
os = "linux"

[[row]]
keys = [{ id = "k1", label = "a", hand = "left", finger = "pinky" }]
`

// czechCopy is the shipped layout under another id and name, as a user would
// write a layout file for a keyboard that differs only a little.
func czechCopy(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../layout/data/macos-czech-qwerty.toml")
	if err != nil {
		t.Fatal(err)
	}
	out := strings.Replace(string(b), `id = "macos-czech-qwerty"`, `id = "user-test"`, 1)
	return strings.Replace(out, `name = "macOS Czech QWERTY"`, `name = "Testovací rozložení"`, 1)
}

func writeUserLayout(t *testing.T, st *store.Store, content string) {
	t.Helper()
	dir := filepath.Join(st.ConfigDir(), "layouts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// firstRunApp is an app on a fresh profile in a 60 × 18 window, on macOS, in an
// unknown terminal. The first-run screen is on top.
func firstRunApp(t *testing.T) (*ui.App, *store.Store, *clock) {
	t.Helper()
	for _, k := range []string{"TERM_PROGRAM", "KITTY_WINDOW_ID", "ALACRITTY_WINDOW_ID", "TERM"} {
		t.Setenv(k, "")
	}
	t.Cleanup(ui.SetGOOS("darwin"))
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
	clk := newClock()
	a, err := ui.New(st, lib, ui.WithClock(clk.now))
	if err != nil {
		t.Fatal(err)
	}
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	return a, st, clk
}

// fits fails unless the screen is exactly 60 × 18.
func fits(t *testing.T, a *ui.App) {
	t.Helper()
	lines := strings.Split(ansi.Strip(a.View().Content), "\n")
	if len(lines) != 18 {
		t.Fatalf("%d lines, want 18", len(lines))
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w != 60 {
			t.Fatalf("line width %d, want 60: %q", w, l)
		}
	}
}

func showsAll(t *testing.T, a *ui.App, parts ...string) {
	t.Helper()
	v := view(a)
	for _, p := range parts {
		if !strings.Contains(v, p) {
			t.Errorf("screen lacks %q:\n%s", p, v)
		}
	}
}

func showsNone(t *testing.T, a *ui.App, parts ...string) {
	t.Helper()
	v := view(a)
	for _, p := range parts {
		if strings.Contains(v, p) {
			t.Errorf("screen has %q:\n%s", p, v)
		}
	}
}

func typeRunes(a *ui.App, s string) {
	for _, r := range s {
		a.Update(key(r))
	}
}

// passChecks goes through the style choice and the layout check.
func passChecks(t *testing.T, a *ui.App) {
	t.Helper()
	a.Update(enter)
	typeRunes(a, "ě{@")
	showsAll(t, a, "Začínám od nuly")
}

func TestStyleChoiceComesFirstAndPersists(t *testing.T) {
	a, st, _ := firstRunApp(t)
	if ui.ScreenName(a) != "first-run" {
		t.Fatalf("start screen = %s, want first-run", ui.ScreenName(a))
	}
	showsAll(t, a, "Vítejte", "Fosfor", "Editor", "Velín", "err != nil")
	fits(t, a)
	before := screen(a)
	a.Update(down)
	if a.Style.ID != "velin" {
		t.Fatalf("style after Down = %s, want velin (live switch)", a.Style.ID)
	}
	if screen(a) == before {
		t.Error("the sample did not change with the style")
	}
	showsAll(t, a, "err != nil")
	fits(t, a)
	if st.Config().Style != "" {
		t.Errorf("style saved before Enter: %q", st.Config().Style)
	}
	a.Update(enter)
	if st.Config().Style != "velin" {
		t.Errorf("config style = %q, want velin", st.Config().Style)
	}
	showsAll(t, a, "ě") // the layout check follows
	showsNone(t, a, "Vítejte")
	fits(t, a)
}

func TestLayoutCheckPassesOnExpectedChars(t *testing.T) {
	a, st, _ := firstRunApp(t)
	a.Update(enter)
	showsAll(t, a, "ě")
	a.Update(key('ě'))
	showsAll(t, a, "{")
	showsNone(t, a, "Začínám od nuly")
	fits(t, a)
	a.Update(key('{'))
	showsAll(t, a, "@")
	a.Update(key('@'))
	showsAll(t, a, "Začínám od nuly", text.PlacementTitle)
	fits(t, a)
	if st.Config().Layout != "" {
		t.Errorf("a passing check saved a layout: %q", st.Config().Layout)
	}
}

func TestLayoutMismatchOpensPicker(t *testing.T) {
	a, st, _ := firstRunApp(t)
	writeUserLayout(t, st, czechCopy(t))
	a.Update(enter)
	a.Update(key('2')) // a US layout types 2 where ě should come
	a.Update(enter)    // it really is the key's own character
	showsAll(t, a, "macOS Czech QWERTY", "Testovací rozložení", "docs/layouts.md", filepath.Join("cfg", "layouts"))
	showsNone(t, a, "(darwin)", "(linux)")
	showsNone(t, a, "Vítejte")
	fits(t, a)
	a.Update(down)
	a.Update(enter)
	if st.Config().Layout != "user-test" || a.Layout.ID != "user-test" {
		t.Errorf("layout = %q / config %q, want user-test", a.Layout.ID, st.Config().Layout)
	}
	showsAll(t, a, "ě") // the check runs again from the start
	showsNone(t, a, "Testovací rozložení")
	typeRunes(a, "ě{@")
	showsAll(t, a, "Začínám od nuly")
}

func TestLayoutCheckCanBeSkippedWithOneKey(t *testing.T) {
	a, _, _ := firstRunApp(t)
	a.Update(enter)
	a.Update(esc)
	showsAll(t, a, "Začínám od nuly")
}

func TestSkippingFromThePickerKeepsTheLayout(t *testing.T) {
	a, st, _ := firstRunApp(t)
	a.Update(enter)
	a.Update(key('2'))
	a.Update(enter) // it really is the key's own character
	showsAll(t, a, "macOS Czech QWERTY")
	a.Update(esc)
	showsAll(t, a, "Začínám od nuly")
	if st.Config().Layout != "" || a.Layout.ID != "macos-czech-qwerty" {
		t.Errorf("layout changed: %q / %q", a.Layout.ID, st.Config().Layout)
	}
}

func TestOptionAsMetaShowsTerminalSpecificHelp(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"Terminal.app", map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, []string{"Use Option as Meta key"}},
		{"iTerm2", map[string]string{"TERM_PROGRAM": "iTerm.app"}, []string{"Left Option key", "Right Option key", "Normal"}},
		{"Ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, []string{"macos-option-as-alt = false"}},
		{"kitty", map[string]string{"KITTY_WINDOW_ID": "1"}, []string{"macos_option_as_alt no"}},
		{"WezTerm", map[string]string{"TERM_PROGRAM": "WezTerm"}, []string{"send_composed_key_when_left_alt_is_pressed = true"}},
		{"Alacritty id", map[string]string{"ALACRITTY_WINDOW_ID": "1"}, []string{`option_as_alt = "None"`}},
		{"Alacritty TERM", map[string]string{"TERM": "alacritty"}, []string{`option_as_alt = "None"`}},
		{"unknown", nil, []string{"nastavení terminálu"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, st, _ := firstRunApp(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			a.Update(enter)
			a.Update(key('ě'))
			a.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModAlt}) // Option+b arrives as Alt+b
			showsAll(t, a, tc.want...)
			showsAll(t, a, "znovu načtěte")
			fits(t, a)
			if st.Config().Layout != "" || st.Profile().Recommended != "" || st.Profile().FirstRunDone {
				t.Errorf("the help saved something: config %+v, recommended %q, done %v",
					st.Config(), st.Profile().Recommended, st.Profile().FirstRunDone)
			}
			// Retry goes back to the same step and the fixed key passes.
			a.Update(enter)
			showsAll(t, a, "{")
			showsNone(t, a, tc.want...)
			a.Update(key('{'))
			a.Update(tea.KeyPressMsg{Code: 'q', Mod: tea.ModAlt}) // the @ step has the same check
			showsAll(t, a, tc.want...)
			a.Update(esc) // skip
			showsAll(t, a, "Začínám od nuly")
		})
	}
}

func TestComposedCharWithAltFlagStillPasses(t *testing.T) {
	a, _, _ := firstRunApp(t)
	a.Update(enter)
	a.Update(key('ě'))
	a.Update(tea.KeyPressMsg{Code: 'í', Mod: tea.ModAlt, Text: "{"})
	showsAll(t, a, "@")
}

func TestNonDarwinWithoutLayoutExplainsAndCanContinue(t *testing.T) {
	a, _, _ := firstRunApp(t)
	restore := ui.SetGOOS("linux")
	defer restore()
	a.Update(enter)
	showsAll(t, a, "docs/layouts.md", filepath.Join("cfg", "layouts"))
	showsNone(t, a, "Napište")
	fits(t, a)
	a.Update(enter)
	showsAll(t, a, "Začínám od nuly")
	if a.Layout.ID != "macos-czech-qwerty" {
		t.Errorf("layout = %s, want the macOS layout", a.Layout.ID)
	}
}

func TestNonDarwinWithUserLayoutRunsTheCheck(t *testing.T) {
	a, st, _ := firstRunApp(t)
	restore := ui.SetGOOS("linux")
	defer restore()
	dir := filepath.Join(st.ConfigDir(), "layouts")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "user.toml"), []byte(userLayout), 0o644)
	a.Update(enter)
	showsNone(t, a, "docs/layouts.md")
	showsAll(t, a, "ě")
}

func TestClosingDuringFirstRunRestartsIt(t *testing.T) {
	a, st, _ := firstRunApp(t)
	passChecks(t, a)
	a.Update(enter) // the hand-position text, closed here
	b, err := ui.New(st, a.Lib, ui.WithClock(newClock().now))
	if err != nil {
		t.Fatal(err)
	}
	if ui.ScreenName(b) != "first-run" {
		t.Errorf("screen = %s, want first-run again", ui.ScreenName(b))
	}
}

// ---- Placement ----

// toPlacement goes through the first run to the first placement stage's intro.
func toPlacement(t *testing.T, a *ui.App) {
	t.Helper()
	passChecks(t, a)
	a.Update(down)
	a.Update(enter)
	if ui.ScreenName(a) != "placement" {
		t.Fatalf("screen = %s, want placement", ui.ScreenName(a))
	}
}

// playStage starts the stage on its intro screen and types it at 60 WPM pace.
func playStage(t *testing.T, a *ui.App, clk *clock) {
	t.Helper()
	a.Update(enter)
	if ui.ScreenName(a) != "round" {
		t.Fatalf("after the intro: %s, want round", ui.ScreenName(a))
	}
	typeKeys(a, clk, -1)
}

// failStage starts the stage and types a wrong key before every right one.
// It returns the number of presses until the round ended.
func failStage(t *testing.T, a *ui.App, clk *clock) int {
	t.Helper()
	a.Update(enter)
	presses := 0
	for _, r := range ui.RoundKeys(a) {
		for _, k := range []rune{'§', r} {
			if ui.ScreenName(a) != "round" {
				return presses
			}
			clk.advance(200 * time.Millisecond)
			a.Update(key(k))
			presses++
		}
	}
	return presses
}

func TestPlacementHasNoHints(t *testing.T) {
	a, _, clk := firstRunApp(t)
	toPlacement(t, a)
	a.Update(enter)
	spec := ui.RoundSpec(a)
	if spec.Kind != play.KindPlacement || !spec.HintsOff {
		t.Errorf("spec kind %v hintsOff %v, want placement without hints", spec.Kind, spec.HintsOff)
	}
	// Wrong presses at one position and a long hesitation show a hint in a lesson.
	for range 4 {
		clk.advance(3 * time.Second)
		a.Update(key('§'))
		a.Update(ui.TickMsg(a))
		if h := ui.RoundHint(a); h != nil {
			t.Fatalf("hint shown during placement: %+v", h)
		}
	}
}

func TestBadStageEndsPlacementEarly(t *testing.T) {
	a, st, clk := firstRunApp(t)
	toPlacement(t, a)
	presses := failStage(t, a, clk)
	if presses < 20 || presses > 22 {
		t.Errorf("round ended after %d presses, want right after 20 (more than a quarter wrong)", presses)
	}
	if ui.ScreenName(a) != "placement" {
		t.Fatalf("screen = %s, want the offer", ui.ScreenName(a))
	}
	showsAll(t, a, "v pořádku", "každý někde", a.Course.Lessons()[0].Title)
	showsNone(t, a, "selhal", "neuspěl")
	fits(t, a)
	if got, want := st.Profile().Recommended, a.Course.Lessons()[0].ID; got != want {
		t.Errorf("Recommended = %q, want %s", got, want)
	}
	if e := st.Profile().Stats[a.Layout.ID].Chars; len(e) == 0 {
		t.Error("the aborted stage left no evidence in the statistics")
	}
	a.Update(enter)
	showsAll(t, a, "Domovská pozice", "ukazovák") // as when starting from zero
	a.Update(enter)
	wantScreen(t, a, "lessons")
}

func TestSlowFirstStageFailsWithoutHumiliation(t *testing.T) {
	a, st, clk := firstRunApp(t)
	toPlacement(t, a)
	a.Update(enter)
	for _, r := range ui.RoundKeys(a) { // clean but 3 s per key: far under the minimum speed
		clk.advance(3 * time.Second)
		a.Update(key(r))
	}
	showsAll(t, a, "v pořádku", "každý někde")
	if got, want := st.Profile().Recommended, a.Course.Lessons()[0].ID; got != want {
		t.Errorf("Recommended = %q, want %s", got, want)
	}
}

func TestFailedStageEndsTheTestAndKeepsEarlierStages(t *testing.T) {
	a, st, clk := firstRunApp(t)
	toPlacement(t, a)
	playStage(t, a, clk)
	showsAll(t, a, "2/6")
	failStage(t, a, clk)
	if ui.ScreenName(a) != "placement" {
		t.Fatalf("screen = %s", ui.ScreenName(a))
	}
	showsNone(t, a, "3/6")
	want := firstOfChapter(t, a, "ch05")
	if got := st.Profile().Recommended; got != want.ID {
		t.Errorf("Recommended = %q, want %s", got, want.ID)
	}
	showsAll(t, a, text.OfferPassed(1, 6), want.Title, "Česká diakritika")
}

// ---- Review fixes ----

func reopen(t *testing.T, st *store.Store) (*ui.App, *store.Store) {
	t.Helper()
	cfg := st.ConfigDir()
	_ = st.Close()
	st2, err := store.Open(cfg, filepath.Join(filepath.Dir(cfg), "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	lib, _ := content.Load()
	a, err := ui.New(st2, lib, ui.WithClock(newClock().now))
	if err != nil {
		t.Fatalf("ui.New after the restart: %v", err)
	}
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	return a, st2
}

// upgraderApp is the app of someone who used tuipe v0.1.0: the store opens on
// a schema 1 profile and run history, as ui.Run does, and nothing is forced.
func upgraderApp(t *testing.T) (*ui.App, *store.Store) {
	t.Helper()
	for _, k := range []string{"TERM_PROGRAM", "KITTY_WINDOW_ID", "ALACRITTY_WINDOW_ID", "TERM"} {
		t.Setenv(k, "")
	}
	t.Cleanup(ui.SetGOOS("darwin"))
	dir := t.TempDir()
	cfg, data := filepath.Join(dir, "cfg"), filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"profile.json": `{"Schema":1,"FirstRunDone":true,"Stars":{"ch01-01":3}}`,
		"runs.jsonl":   `{"Schema":1,"ID":"a3f9c1e","Mode":"lesson","LessonID":"ch01-01","WPM":40}` + "\n",
	} {
		if err := os.WriteFile(filepath.Join(data, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Open(cfg, data)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	lib, err := content.Load()
	if err != nil {
		t.Fatal(err)
	}
	a, err := ui.New(st, lib, ui.WithClock(newClock().now))
	if err != nil {
		t.Fatal(err)
	}
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	return a, st
}

// The notices of an upgrade, at the start of a line in any window.
var upgradeNotices = []string{"Soubor profile.json je ze starší verze", "Soubor runs.jsonl je ze starší verze"}

// Someone coming from v0.1.0 starts with a first run, because their profile
// was set aside. They are told so on its first screen, and the notice waits
// for the lesson list: quitting before it keeps the notice for the next launch.
func TestUpgraderIsToldDuringTheFirstRunUntilTheListShowsIt(t *testing.T) {
	a, st := upgraderApp(t)
	wantScreen(t, a, "first-run")
	showsAll(t, a, upgradeNotices...)
	showsAll(t, a, "Vítejte", "Editor") // the style choice is still all there
	fits(t, a)
	a.Update(tea.WindowSizeMsg{Width: 40, Height: 8}) // must not panic
	_ = a.View()
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})

	// Ctrl+C during the first run: Run saves the profile on the way out.
	a.Update(enter)
	a.Shutdown()
	a, st = reopen(t, st)
	wantScreen(t, a, "first-run")
	showsAll(t, a, upgradeNotices...)

	passChecks(t, a)
	a.Update(enter) // from zero
	a.Update(enter) // the hand position
	wantScreen(t, a, "lessons")
	showsAll(t, a, upgradeNotices...)
	a.Update(down)
	showsNone(t, a, upgradeNotices...)

	a.Shutdown()
	a, _ = reopen(t, st)
	wantScreen(t, a, "lessons")
	showsNone(t, a, upgradeNotices...)
}

func TestPickedUserLayoutSurvivesARestart(t *testing.T) {
	a, st, _ := firstRunApp(t)
	writeUserLayout(t, st, czechCopy(t))
	a.Update(enter)
	a.Update(key('2'))
	a.Update(enter) // it really is the key's own character
	a.Update(down)
	a.Update(enter)
	if a.Layout.ID != "user-test" {
		t.Fatalf("layout = %s", a.Layout.ID)
	}
	b, _ := reopen(t, st)
	if b.Layout.ID != "user-test" {
		t.Errorf("layout after the restart = %s, want user-test", b.Layout.ID)
	}
}

func TestUnknownLayoutInTheConfigFallsBackWithANotice(t *testing.T) {
	a, st, _ := firstRunApp(t)
	cfg := st.Config()
	cfg.Layout = "gone"
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	st.Profile().FirstRunDone = true
	if err := st.SaveProfile(); err != nil {
		t.Fatal(err)
	}
	b, _ := reopen(t, st)
	_ = a
	if b.Layout.ID != "macos-czech-qwerty" {
		t.Errorf("layout = %s, want the default", b.Layout.ID)
	}
	showsAll(t, b, "gone")
}

func TestWrongKeyAfterTheFirstCharKeepsProgressAndNeverOpensThePicker(t *testing.T) {
	a, _, _ := firstRunApp(t)
	a.Update(enter)
	a.Update(key('ě'))
	a.Update(key('x')) // a beginner who does not know the { key yet
	showsAll(t, a, "{", "2/3", "později", "Esc")
	showsNone(t, a, "Které rozložení")
	fits(t, a)
	a.Update(key('{'))
	a.Update(key('y'))
	showsAll(t, a, "@", "3/3", "později")
	showsNone(t, a, "Které rozložení", "1/3")
	a.Update(key('@'))
	showsAll(t, a, "Začínám od nuly")
}

func TestPickerIsReachableByTabAtAnyCheckStep(t *testing.T) {
	a, _, _ := firstRunApp(t)
	a.Update(enter)
	a.Update(key('ě'))
	a.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	showsAll(t, a, "Které rozložení", "docs/layouts.md")
	a.Update(enter) // the same layout again restarts the check
	showsAll(t, a, "1/3")
}

func TestPickedLayoutWithoutTelltaleCharsSkipsTheCheck(t *testing.T) {
	a, st, _ := firstRunApp(t)
	writeUserLayout(t, st, userLayout) // no characters at all
	a.Update(enter)
	a.Update(key('2'))
	a.Update(enter) // it really is the key's own character
	a.Update(down)
	a.Update(enter)
	showsAll(t, a, "Začínám od nuly")
}

func TestPickedLayoutChecksOnlyWhatItCanType(t *testing.T) {
	a, st, _ := firstRunApp(t)
	writeUserLayout(t, st, userLayout+"\n[[char]]\nc = \"{\"\nroute = [\"option+k1\"]\n")
	a.Update(enter)
	a.Update(key('2'))
	a.Update(enter) // it really is the key's own character
	a.Update(down)
	a.Update(enter)
	showsAll(t, a, "{", "1/1")
	a.Update(tea.KeyPressMsg{Code: 'q', Mod: tea.ModAlt})
	showsAll(t, a, "Option se chová jako Alt")
	a.Update(enter)
	a.Update(key('{'))
	showsAll(t, a, "Začínám od nuly")
}

func TestStyleMarkerFollowsCtrlT(t *testing.T) {
	a, _, _ := firstRunApp(t)
	a.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}) // editor -> velin
	if a.Style.ID != "velin" {
		t.Fatalf("style = %s", a.Style.ID)
	}
	if !strings.Contains(view(a), "> Velín") {
		t.Errorf("the marker did not follow:\n%s", view(a))
	}
	a.Update(down) // fosfor, the next one after velin
	if a.Style.ID != "fosfor" {
		t.Errorf("Down after Ctrl+T: style = %s, want fosfor", a.Style.ID)
	}
}

func TestHandPositionTextScrollsInASmallWindow(t *testing.T) {
	a, _, _ := firstRunApp(t)
	passChecks(t, a)
	a.Update(enter)
	a.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	if strings.Contains(view(a), "naučíte") {
		t.Fatal("the whole text cannot fit 40 x 8; the test needs a scroll")
	}
	for range 30 {
		a.Update(down)
	}
	showsAll(t, a, "naučíte")
}

func TestOptionStageEndsAfterFiveIgnoredAltPresses(t *testing.T) {
	a, st, clk := firstRunApp(t)
	toPlacement(t, a)
	for range 4 {
		playStage(t, a, clk)
	}
	showsAll(t, a, "5/6", "Esc") // the option stage, with its note
	a.Update(enter)
	meta := tea.KeyPressMsg{Code: 'ƒ', Mod: tea.ModAlt}
	for i := range 4 {
		clk.advance(200 * time.Millisecond)
		a.Update(meta)
		if ui.ScreenName(a) != "round" {
			t.Fatalf("the stage ended after %d presses, want 5", i+1)
		}
	}
	clk.advance(200 * time.Millisecond)
	a.Update(meta)
	if ui.ScreenName(a) != "placement" {
		t.Fatalf("screen after 5 ignored Alt presses = %s, want the offer", ui.ScreenName(a))
	}
	want := firstOfChapter(t, a, "ch08")
	if got := st.Profile().Recommended; got != want.ID {
		t.Errorf("Recommended = %q, want %s", got, want.ID)
	}
	showsAll(t, a, want.Title)
}

func TestPlacementRunsHaveNoStarsAndTheirOwnMode(t *testing.T) {
	a, st, clk := firstRunApp(t)
	toPlacement(t, a)
	for range 6 {
		playStage(t, a, clk)
	}
	runs, _ := st.Runs()
	if len(runs) != 6 {
		t.Fatalf("runs = %d", len(runs))
	}
	for _, r := range runs {
		if r.Stars != 0 || r.Mode != store.ModePlacement {
			t.Errorf("run %s: stars %d mode %q, want 0 and placement", r.ID, r.Stars, r.Mode)
		}
	}
	ui.SetScreen(a, ui.NewStatsScreen(a))
	showsAll(t, a, "Vstupní test")
	showsNone(t, a, "★")
	a.Update(tab)
	a.Update(tab)
	a.Update(tab) // records
	showsNone(t, a, "Vstupní test")
}

func TestPlacementRunsAreNoOverallTrend(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	for i := range 6 {
		addRun(t, st, store.Run{ID: fmt.Sprintf("p%d", i), At: clk.t.Add(-time.Duration(i+1) * time.Minute), Mode: store.ModePlacement, WPM: 60, Accuracy: 1})
	}
	addRun(t, st, store.Run{ID: "q", At: clk.t, Mode: store.ModeTest, WPM: 40, Accuracy: 1})
	openStats(t, a)
	a.Update(tab)
	a.Update(tab)
	var overall []string
	for _, l := range strings.Split(view(a), "\n") {
		if f := strings.Fields(strings.TrimPrefix(l, "│")); len(f) > 0 && f[0] == "celkem" {
			overall = append(overall, strings.Join(f[1:], " "))
		}
	}
	if len(overall) != 2 {
		t.Fatalf("overall rows = %v in\n%s", overall, view(a))
	}
	// The placement runs share the day with a Test of 200 zn/min: the median must be the Test's.
	if !strings.Contains(overall[0], "200 zn/min") || strings.Contains(overall[0], "300") {
		t.Errorf("overall speed trend = %q, want only the Test of 200 zn/min", overall[0])
	}
}

func TestFirstRunGivenUpRecommendsTheFirstLesson(t *testing.T) {
	a, st, clk := firstRunApp(t)
	toPlacement(t, a)
	a.Update(enter)
	typeKeys(a, clk, 3)
	a.Update(esc)
	a.Update(down)
	a.Update(down)
	a.Update(enter) // the pause menu: leave
	if got, want := st.Profile().Recommended, a.Course.Lessons()[0].ID; got != want {
		t.Errorf("Recommended = %q, want the first lesson %s", got, want)
	}
	showsAll(t, a, "v pořádku")
}

func TestPlacementResultPointsToTheLessons(t *testing.T) {
	a, _, _ := playApp(t)
	ui.SetScreen(a, ui.NewPlacementScreen(a, nil))
	a.Update(esc) // give up: the result of a beginner
	showsAll(t, a, text.OfferPractice)
	showsNone(t, a, "Kariéra", "kapitol", "nabíd", "pozici", "Hodnost", "Odemčen", "Intern")
}

// Someone with stars who gives up the retake is not starting from zero.
func TestRetakeWithStarsSkipsTheHandPosition(t *testing.T) {
	a, st, _ := playApp(t)
	st.Profile().Stars[a.Course.Lessons()[0].ID] = 1
	ui.SetScreen(a, ui.NewPlacementScreen(a, nil))
	a.Update(esc)
	showsAll(t, a, text.OfferPassed(0, 6))
	showsNone(t, a, "v pořádku")
	a.Update(enter)
	wantScreen(t, a, "lessons")
}

// firstRunStops are the screens of a first run, in the longest state each has.
var firstRunStops = []struct {
	name  string
	reach func(*testing.T, *ui.App, *clock)
}{
	{"style", func(*testing.T, *ui.App, *clock) {}},
	{"check", func(t *testing.T, a *ui.App, _ *clock) { a.Update(enter) }},
	{"check, later key", func(t *testing.T, a *ui.App, _ *clock) {
		a.Update(enter)
		a.Update(key('ě'))
		a.Update(key('x'))
	}},
	{"check, wrong character", func(t *testing.T, a *ui.App, _ *clock) {
		a.Update(enter)
		a.Update(key('x'))
	}},
	{"picker", func(t *testing.T, a *ui.App, _ *clock) {
		a.Update(enter)
		a.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	}},
	{"option help", func(t *testing.T, a *ui.App, _ *clock) {
		a.Update(enter)
		a.Update(key('ě'))
		a.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModAlt})
	}},
	{"no layout", func(t *testing.T, a *ui.App, _ *clock) {
		t.Cleanup(ui.SetGOOS("linux"))
		a.Update(enter)
	}},
	{"fork", func(t *testing.T, a *ui.App, _ *clock) { passChecks(t, a) }},
	{"hand position", func(t *testing.T, a *ui.App, _ *clock) {
		passChecks(t, a)
		a.Update(enter)
	}},
	{"stage intro", func(t *testing.T, a *ui.App, _ *clock) { toPlacement(t, a) }},
	{"stage round", func(t *testing.T, a *ui.App, _ *clock) {
		toPlacement(t, a)
		a.Update(enter)
	}},
	{"option stage intro", func(t *testing.T, a *ui.App, clk *clock) {
		toPlacement(t, a)
		for range 4 {
			playStage(t, a, clk)
		}
	}},
	{"result from zero", func(t *testing.T, a *ui.App, clk *clock) {
		toPlacement(t, a)
		failStage(t, a, clk)
	}},
	{"result passed", func(t *testing.T, a *ui.App, clk *clock) {
		toPlacement(t, a)
		for range 6 {
			playStage(t, a, clk)
		}
	}},
}

// "přeskočit" stays allowed on its own: the keyboard check can still be skipped.
func TestFirstRunTextsMakeNoOldPromises(t *testing.T) {
	old := []string{
		"Kariéra", "kariér", "kapitol", "Kapitol", "Přeskočí, co", "odemkn", "Odemkn", "Odemč", "odemč",
		"hodnost", "Hodnost", "pohovor", "Pohovor", "nabíd", "Nabíd", "test-out",
		"Intern", "Junior", "Medior", "Senior", "Staff", "Principal",
	}
	for _, stop := range firstRunStops {
		t.Run(stop.name, func(t *testing.T) {
			a, _, clk := firstRunApp(t)
			stop.reach(t, a, clk)
			a.Update(tea.WindowSizeMsg{Width: 120, Height: 45}) // nothing scrolled away
			showsNone(t, a, old...)
		})
	}
}

// reopenFirstRunDone restarts on the saved data with the user layout chosen.
func reopenFirstRunDone(t *testing.T, st *store.Store) (*ui.App, *store.Store) {
	t.Helper()
	st.Profile().FirstRunDone = true
	if err := st.SaveProfile(); err != nil {
		t.Fatal(err)
	}
	cfg := st.Config()
	cfg.Layout = "user-test"
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	return reopen(t, st)
}

func TestWrongFirstCharWarnsAndLetsYouTypeItAgain(t *testing.T) {
	a, st, _ := firstRunApp(t)
	a.Update(enter)
	a.Update(key('é'))
	showsAll(t, a, "1/3", "Napsali jste é místo ě", text.FirstRunCheckWrongHint)
	showsNone(t, a, "Které rozložení", text.FirstRunCheckHint)
	fits(t, a)

	a.Update(key('ě'))
	showsAll(t, a, "{", "2/3", text.FirstRunCheckHint)
	showsNone(t, a, "Napsali jste", text.FirstRunCheckWrongHint)
	if st.Config().Layout != "" {
		t.Errorf("a typo changed the layout: %q", st.Config().Layout)
	}
}

func TestAnotherWrongCharWarnsAboutTheNewOne(t *testing.T) {
	a, _, _ := firstRunApp(t)
	a.Update(enter)
	a.Update(key('é'))
	a.Update(key('3'))
	showsAll(t, a, "1/3", "Napsali jste 3 místo ě")
	showsNone(t, a, "Které rozložení")
}

func TestConfirmingTheWrongCharOpensThePicker(t *testing.T) {
	a, _, _ := firstRunApp(t)
	a.Update(enter)
	a.Update(key('2'))
	a.Update(enter)
	showsAll(t, a, "Které rozložení", "docs/layouts.md")
}

func TestEnterConfirmsNothingWithoutAWrongChar(t *testing.T) {
	a, _, _ := firstRunApp(t)
	a.Update(enter)
	a.Update(enter)
	showsAll(t, a, "1/3")
	showsNone(t, a, "Které rozložení")
	a.Update(key('ě'))
	a.Update(key('x')) // a later key is only not learned yet
	a.Update(enter)
	showsAll(t, a, "2/3")
	showsNone(t, a, "Které rozložení")
}

func TestWrongCharCanStillBeSkipped(t *testing.T) {
	a, _, _ := firstRunApp(t)
	a.Update(enter)
	a.Update(key('2'))
	a.Update(esc)
	showsAll(t, a, "Začínám od nuly")
}

func TestWrongCharWarningIsSeenInTheSmallestWindow(t *testing.T) {
	a, _, _ := firstRunApp(t)
	a.Update(enter)
	a.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	a.Update(key('é'))
	showsAll(t, a, "Napsali jste é místo ě")
}

func TestWrongSpaceIsNamed(t *testing.T) {
	a, _, _ := firstRunApp(t)
	a.Update(enter)
	a.Update(key(' '))
	showsAll(t, a, "Napsali jste mezeru místo ě")
}

// firstOfChapter is the first lesson of the chapter with the given id.
func firstOfChapter(t *testing.T, a *ui.App, id string) course.Lesson {
	t.Helper()
	for _, ch := range a.Course.Chapters() {
		if ch.ID == id {
			return ch.Lessons[0]
		}
	}
	t.Fatalf("no chapter %s", id)
	return course.Lesson{}
}

func TestPlacementRecommendsOnly(t *testing.T) {
	a, st, clk := firstRunApp(t)
	p := st.Profile()
	toPlacement(t, a)
	typed := map[rune]bool{}
	for i := range 6 {
		showsAll(t, a, text.PlacementTitle, "Esc ukončit", string(rune('1'+i))+"/6")
		fits(t, a)
		a.Update(enter)
		for _, r := range ui.RoundKeys(a) {
			typed[r] = true
		}
		typeKeys(a, clk, -1)
	}
	lessons := a.Course.Lessons()
	last := lessons[len(lessons)-1]
	if p.Recommended != last.ID {
		t.Errorf("Recommended = %q, want the last lesson %s", p.Recommended, last.ID)
	}
	showsAll(t, a, last.Title)
	fits(t, a)
	if len(p.Stars) != 0 {
		t.Errorf("placement wrote stars: %v", p.Stars)
	}
	if got := a.Course.Rank(ui.GradesOf(p.Stars)); got != course.Intern {
		t.Errorf("rank = %s, want Intern", got)
	}
	if got := len(p.Taught); got != 0 {
		t.Errorf("placement marked %d lessons as taught", got)
	}
	// A character the test never asked for is not granted anything.
	s := p.Stats[a.Layout.ID]
	for _, ls := range lessons {
		for _, r := range ls.Focus {
			if !typed[r] && s.MasteryOf(r) != stats.NewChar {
				t.Errorf("%q has mastery %v without being typed", r, s.MasteryOf(r))
			}
		}
	}
	runs, _ := st.Runs()
	if len(runs) != 6 {
		t.Fatalf("runs = %d, want 6", len(runs))
	}
	for _, r := range runs {
		if r.Mode != store.ModePlacement || r.Stars != 0 || r.LessonID != "" {
			t.Errorf("run = %+v, want a placement run without stars or a lesson", r)
		}
	}
	if p.FirstRunDone {
		t.Error("first run done before Enter on the result")
	}
	a.Update(enter)
	wantScreen(t, a, "lessons")
	if !p.FirstRunDone {
		t.Error("FirstRunDone not set at the end")
	}
}

func TestFirstRunEndsOnTheListAtTheRecommendation(t *testing.T) {
	a, st, clk := firstRunApp(t)
	toPlacement(t, a)
	playStage(t, a, clk) // letters
	playStage(t, a, clk) // diacritics
	failStage(t, a, clk) // digits
	want := firstOfChapter(t, a, "ch06")
	if got := st.Profile().Recommended; got != want.ID {
		t.Errorf("Recommended = %q, want %s", got, want.ID)
	}
	showsAll(t, a, want.Title)
	a.Update(enter)
	wantScreen(t, a, "lessons")
	if got := ui.LessonsSelected(a); got != want.ID {
		t.Errorf("the list opens on %q, want %s", got, want.ID)
	}
	showsAll(t, a, text.LessonRecommended)
}

func TestStartFromZeroRecommendsTheFirstLesson(t *testing.T) {
	a, st, _ := firstRunApp(t)
	passChecks(t, a)
	a.Update(enter) // Začínám od nuly is the first choice
	showsAll(t, a, "Domovská pozice", "f", "j")
	fits(t, a)
	if st.Profile().FirstRunDone {
		t.Error("first run marked done before its end")
	}
	a.Update(enter)
	wantScreen(t, a, "lessons")
	if !st.Profile().FirstRunDone {
		t.Error("FirstRunDone not set at the end")
	}
	first := a.Course.Lessons()[0]
	if got := st.Profile().Recommended; got != first.ID {
		t.Errorf("Recommended = %q, want the first lesson %s", got, first.ID)
	}
	if got := ui.LessonsSelected(a); got != first.ID {
		t.Errorf("the list opens on %q, want %s", got, first.ID)
	}
	if len(st.Profile().Stars) != 0 {
		t.Errorf("starting from zero wrote stars: %v", st.Profile().Stars)
	}
}

func TestRetakeKeepsStars(t *testing.T) {
	a, st, clk := playApp(t)
	p := st.Profile()
	ls := a.Course.Lessons()
	p.Stars[ls[0].ID], p.Stars[ls[1].ID] = 3, 2
	p.Taught[ls[0].ID] = true
	p.Recommended = ls[10].ID
	rank := a.Course.Rank(ui.GradesOf(p.Stars))
	openSettings(t, a)
	onRow(a, rowRetake)
	a.Update(enter)
	wantScreen(t, a, "placement")
	playStage(t, a, clk) // letters
	failStage(t, a, clk) // diacritics
	want := firstOfChapter(t, a, "ch05")
	if p.Recommended != want.ID {
		t.Errorf("Recommended = %q, want %s", p.Recommended, want.ID)
	}
	if p.Stars[ls[0].ID] != 3 || p.Stars[ls[1].ID] != 2 || len(p.Stars) != 2 {
		t.Errorf("the retake changed the stars: %v", p.Stars)
	}
	if !p.Taught[ls[0].ID] {
		t.Error("the retake dropped a Výuka mark")
	}
	if got := a.Course.Rank(ui.GradesOf(p.Stars)); got != rank {
		t.Errorf("rank = %s, want %s", got, rank)
	}
	a.Update(enter)
	wantScreen(t, a, "lessons")
	if got := ui.LessonsSelected(a); got != want.ID {
		t.Errorf("the list opens on %q, want %s", got, want.ID)
	}
}
