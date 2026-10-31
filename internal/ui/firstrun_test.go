package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
)

const userLayout = `id = "user-test"
name = "Testovací rozložení"
os = "linux"

[[row]]
keys = [{ id = "k1", label = "a", hand = "left", finger = "pinky" }]
`

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
	a, err := ui.New(st, lib)
	if err != nil {
		t.Fatal(err)
	}
	clk := &clock{t: time.Date(2026, 10, 31, 12, 0, 0, 0, time.Local)}
	a.Now = clk.now
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
	showsAll(t, a, "Začínám od nuly", "Vstupní pohovor")
	fits(t, a)
	if st.Config().Layout != "" {
		t.Errorf("a passing check saved a layout: %q", st.Config().Layout)
	}
}

func TestLayoutMismatchOpensPicker(t *testing.T) {
	a, st, _ := firstRunApp(t)
	dir := filepath.Join(st.ConfigDir(), "layouts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user.toml"), []byte(userLayout), 0o644); err != nil {
		t.Fatal(err)
	}
	a.Update(enter)
	a.Update(key('2')) // a US layout types 2 where ě should come
	showsAll(t, a, "macOS Czech QWERTY", "Testovací rozložení")
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
		{"iTerm2", map[string]string{"TERM_PROGRAM": "iTerm.app"}, []string{"Left Option key", "Normal"}},
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
			fits(t, a)
			if st.Config().Layout != "" || st.Profile().Completed["x"] || st.Profile().FirstRunDone {
				t.Errorf("the help saved something: %+v", st.Config())
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
	showsAll(t, a, "docs/layouts.md", "cfg/layouts")
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

func TestStartingFromZeroSkipsPlacement(t *testing.T) {
	a, st, _ := firstRunApp(t)
	passChecks(t, a)
	a.Update(enter) // Začínám od nuly is the first choice
	showsAll(t, a, "Domovská pozice", "f", "j")
	fits(t, a)
	if st.Profile().FirstRunDone {
		t.Error("first run marked done before its end")
	}
	a.Update(enter)
	if ui.ScreenName(a) != "home" {
		t.Fatalf("screen = %s, want home", ui.ScreenName(a))
	}
	if !st.Profile().FirstRunDone {
		t.Error("FirstRunDone not set at the end")
	}
	if len(st.Profile().Completed) != 0 {
		t.Errorf("starting from zero completed %v", st.Profile().Completed)
	}
	if got := a.Curr.CurrentChapter(st.Profile()).Number; got != 0 {
		t.Errorf("current chapter = %d, want 0", got)
	}
}

func TestClosingDuringFirstRunRestartsIt(t *testing.T) {
	a, st, _ := firstRunApp(t)
	passChecks(t, a)
	a.Update(enter) // the hand-position text, closed here
	b, err := ui.New(st, a.Lib)
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

func TestPlacementResultIsAJobOfferAndAppliesSkip(t *testing.T) {
	a, st, clk := firstRunApp(t)
	p := st.Profile()
	p.Resume = &store.Resume{Kind: "lesson", LessonID: "keep-me"}
	toPlacement(t, a)
	for i := range 6 {
		showsAll(t, a, "Vstupní pohovor", string(rune('1'+i))+"/6")
		fits(t, a)
		playStage(t, a, clk)
	}
	chs := a.Curr.Chapters()
	last := chs[len(chs)-1]
	showsAll(t, a, "Staff", last.Title, "nabíd")
	fits(t, a)
	for _, ch := range chs {
		if want := ch.Number <= 11; p.Completed[ch.ID] != want {
			t.Errorf("chapter %d completed = %v, want %v", ch.Number, p.Completed[ch.ID], want)
		}
	}
	if len(p.Stars) != 0 {
		t.Errorf("placement wrote stars: %v", p.Stars)
	}
	if p.Resume == nil || p.Resume.LessonID != "keep-me" {
		t.Errorf("Resume = %+v, placement must not touch it", p.Resume)
	}
	runs, _ := st.Runs()
	if len(runs) != 6 {
		t.Fatalf("runs = %d, want 6", len(runs))
	}
	for _, r := range runs {
		if r.Mode != "placement" {
			t.Errorf("run mode = %q", r.Mode)
		}
		if g, _ := st.Ghost(r.TextHash); g != nil {
			t.Errorf("placement saved a ghost for %s", r.TextHash)
		}
	}
	s := p.Stats[a.Layout.ID]
	if m := s.MasteryOf('@'); m != stats.Solid {
		t.Errorf("mastery of @ = %v, want solid after the test", m)
	}
	if p.FirstRunDone {
		t.Error("first run done before Enter on the offer")
	}
	a.Update(enter)
	if ui.ScreenName(a) != "home" || !p.FirstRunDone {
		t.Errorf("after the offer: %s, done %v", ui.ScreenName(a), p.FirstRunDone)
	}
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
	showsAll(t, a, "od nuly", "v pořádku", "Intern")
	showsNone(t, a, "selhal", "neuspěl")
	fits(t, a)
	if len(st.Profile().Completed) != 0 {
		t.Errorf("completed %v after a failed first stage", st.Profile().Completed)
	}
	if e := st.Profile().Stats[a.Layout.ID].Chars; len(e) == 0 {
		t.Error("the aborted stage left no evidence in the statistics")
	}
	a.Update(enter)
	if ui.ScreenName(a) != "home" {
		t.Errorf("screen = %s, want home", ui.ScreenName(a))
	}
}

func TestSlowFirstStageFailsWithoutHumiliation(t *testing.T) {
	a, st, clk := firstRunApp(t)
	toPlacement(t, a)
	a.Update(enter)
	for _, r := range ui.RoundKeys(a) { // clean but 3 s per key: far under the minimum speed
		clk.advance(3 * time.Second)
		a.Update(key(r))
	}
	showsAll(t, a, "od nuly", "v pořádku")
	if len(st.Profile().Completed) != 0 {
		t.Errorf("completed %v", st.Profile().Completed)
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
	p := st.Profile()
	for _, ch := range a.Curr.Chapters() {
		if want := ch.Number <= 4; p.Completed[ch.ID] != want {
			t.Errorf("chapter %d completed = %v, want %v", ch.Number, p.Completed[ch.ID], want)
		}
	}
	showsAll(t, a, "Junior", a.Curr.CurrentChapter(p).Title)
}

func TestPlacementCanBeRetaken(t *testing.T) {
	a, st, clk := firstRunApp(t)
	p := st.Profile()
	p.FirstRunDone = true
	chs := a.Curr.Chapters()
	for _, ch := range chs {
		if ch.Number <= 8 {
			a.Curr.CompleteChapter(p, ch.ID)
		}
	}
	back := false
	ui.SetScreen(a, ui.NewPlacementScreen(a, func(a *ui.App) ui.Screen {
		back = true
		return ui.NewHomeScreen(a)
	}))
	failStage(t, a, clk)
	showsAll(t, a, "Senior") // the rank of chapter 8 stays
	for _, ch := range chs {
		if ch.Number <= 8 && !p.Completed[ch.ID] {
			t.Errorf("chapter %d lost its completion", ch.Number)
		}
	}
	a.Update(enter)
	if !back || ui.ScreenName(a) != "home" {
		t.Errorf("next called %v, screen %s", back, ui.ScreenName(a))
	}
}
