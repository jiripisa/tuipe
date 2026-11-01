package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/hint"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui/roundview"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const (
	userLayoutDir = "layouts" // under the config folder
	sampleSource  = "if err != nil {\n\treturn err\n}"
	samplePos     = 7 // the cursor stands inside the first line
	sampleWPM     = 62
	sampleAcc     = 0.981
	boxMargin     = 2 // rows kept free below the style box
)

// goos is the operating system the first run adapts to; tests replace it.
var goos = runtime.GOOS

// checkRunes are the telltale characters of the layout and Option checks. The
// first one only needs the layout; the others also need Option to compose.
var checkRunes = []rune{'ě', '{', '@'}

type frStep int

const (
	stepStyle      frStep = iota // choose the look, with a live sample
	stepNoLayout                 // no layout for this system
	stepCheck                    // type the telltale characters
	stepPicker                   // the typed character did not match: pick a layout
	stepOptionHelp               // Option arrives as Alt: explain the terminal setting
	stepFork                     // from zero, or the placement test
	stepZero                     // the hand position, then home
)

// firstRunScreen is the whole first run: style, checks and the fork.
type firstRunScreen struct {
	step    frStep
	checks  []rune // the telltale characters the layout can type
	check   int    // index into checks
	note    string // shown under the current check, such as "not needed yet"
	layouts []*layout.Layout
	pickAt  int
	forkAt  int
	hands   handText
}

// NewFirstRunScreen builds the first-run flow, which starts with the style choice.
func NewFirstRunScreen(a *App) Screen { return &firstRunScreen{} }

// styleIndex is the position of the current style in the cycle.
func styleIndex(a *App) int {
	for i, s := range style.All() {
		if s.ID == a.Style.ID {
			return i
		}
	}
	return 0
}

// telltales are the characters of checkRunes that the layout can type.
func telltales(l *layout.Layout) []rune {
	var out []rune
	for _, r := range checkRunes {
		if _, ok := l.Route(r); ok {
			out = append(out, r)
		}
	}
	return out
}

// needsOption reports whether the layout types r with Option held, which is
// what a terminal that treats Option as Alt breaks.
func needsOption(l *layout.Layout, r rune) bool {
	route, _ := l.Route(r)
	for _, c := range route {
		if slices.Contains(c.Mods, "option") {
			return true
		}
	}
	return false
}

// startCheck begins the check with the characters the layout can type. A layout
// that can type none of them has nothing to check.
func (f *firstRunScreen) startCheck(a *App) {
	f.checks, f.check, f.note = telltales(a.Layout), 0, ""
	f.step = stepCheck
	if len(f.checks) == 0 {
		f.step = stepFork
	}
}

// loadLayouts are the built-in layouts followed by the user's own. A bad user
// file is skipped.
func loadLayouts(st *store.Store) ([]*layout.Layout, error) {
	all, _, err := loadLayoutsReport(st)
	return all, err
}

// loadLayoutsReport is loadLayouts that also returns what went wrong with the
// user's files, one error for each skipped file.
func loadLayoutsReport(st *store.Store) ([]*layout.Layout, []error, error) {
	all, err := layout.Builtin()
	if err != nil {
		return nil, nil, err
	}
	user, errs := layout.LoadDir(filepath.Join(st.ConfigDir(), userLayoutDir))
	return append(all, user...), errs, nil
}

// availableLayouts is loadLayouts for the first run, where built-ins cannot fail.
func availableLayouts(a *App) []*layout.Layout {
	all, _ := loadLayouts(a.Store)
	return all
}

func hasLayoutFor(ls []*layout.Layout, system string) bool {
	for _, l := range ls {
		if l.OS == system {
			return true
		}
	}
	return false
}

// finishFirstRun saves that the first run is over and opens the home screen.
func finishFirstRun(a *App) Screen {
	a.Store.Profile().FirstRunDone = true
	a.SaveErr(a.Store.SaveProfile())
	return NewHomeScreen(a)
}

func (f *firstRunScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return f, nil
	}
	switch f.step {
	case stepStyle:
		return f.updateStyle(k, a)
	case stepNoLayout:
		if k.String() == "enter" {
			f.step = stepFork
		}
	case stepCheck:
		f.updateCheck(k, a)
	case stepPicker:
		f.updatePicker(k, a)
	case stepOptionHelp:
		switch k.String() {
		case "enter":
			f.step = stepCheck
		case "esc":
			f.step = stepFork
		}
	case stepFork:
		return f.updateFork(k, a)
	case stepZero:
		if k.String() == "enter" {
			return finishFirstRun(a), nil
		}
		f.hands.key(k)
	}
	return f, nil
}

func (f *firstRunScreen) updateStyle(k tea.KeyPressMsg, a *App) (Screen, tea.Cmd) {
	styles := style.All()
	switch k.String() {
	case "up", "k":
		a.Style = styles[(styleIndex(a)+len(styles)-1)%len(styles)] // the sample switches live
	case "down", "j":
		a.Style = styles[(styleIndex(a)+1)%len(styles)]
	case "enter":
		cfg := a.Store.Config()
		cfg.Style = string(a.Style.ID)
		a.SaveErr(a.Store.SaveConfig(cfg))
		f.layouts = availableLayouts(a)
		if hasLayoutFor(f.layouts, goos) {
			f.startCheck(a)
		} else {
			f.step = stepNoLayout
		}
	}
	return f, nil
}

// typedRune is the character a key press delivers, and whether Alt was held.
// The terminal decoder drops the text of an Alt chord, so its code stands in
// (as in a round).
func typedRune(k tea.KeyPressMsg) (r rune, alt, ok bool) {
	if k.Mod.Contains(tea.ModCtrl) || k.Mod.Contains(tea.ModMeta) {
		return 0, false, false
	}
	alt = k.Mod.Contains(tea.ModAlt)
	if k.Text != "" {
		return []rune(k.Text)[0], alt, true
	}
	if alt && unicode.IsPrint(k.Code) {
		return k.Code, true, true
	}
	return 0, alt, false
}

func (f *firstRunScreen) updateCheck(k tea.KeyPressMsg, a *App) {
	switch k.String() {
	case "esc":
		f.step = stepFork
		return
	case "tab": // the way to the picker for a layout that really differs
		f.openPicker(a)
		return
	}
	r, alt, ok := typedRune(k)
	want := f.checks[f.check]
	f.note = ""
	switch {
	case !ok:
	case r == want:
		if f.check++; f.check == len(f.checks) {
			f.step = stepFork
		}
	case alt && needsOption(a.Layout, want):
		f.step = stepOptionHelp // Option arrived as Alt: nothing is saved, the user re-tests
	case alt:
	case f.check == 0:
		f.openPicker(a) // the very first key is the layout's own: it differs
	default:
		f.note = text.FirstRunCheckLater // a key not yet learned says nothing about the layout
	}
}

func (f *firstRunScreen) openPicker(a *App) {
	f.layouts = availableLayouts(a)
	f.pickAt = 0
	for i, l := range f.layouts {
		if l.ID == a.Layout.ID {
			f.pickAt = i
		}
	}
	f.step = stepPicker
}

func (f *firstRunScreen) updatePicker(k tea.KeyPressMsg, a *App) {
	n := len(f.layouts)
	switch k.String() {
	case "up", "k":
		f.pickAt = (f.pickAt + n - 1) % n
	case "down", "j":
		f.pickAt = (f.pickAt + 1) % n
	case "esc":
		f.step = stepFork
	case "enter":
		l := f.layouts[f.pickAt]
		a.setLayout(l)
		f.startCheck(a)
	}
}

func (f *firstRunScreen) updateFork(k tea.KeyPressMsg, a *App) (Screen, tea.Cmd) {
	switch k.String() {
	case "up", "k", "down", "j":
		f.forkAt = 1 - f.forkAt
	case "enter":
		if f.forkAt == 0 {
			f.step = stepZero
			return f, nil
		}
		return NewPlacementScreen(a, finishFirstRun), nil
	}
	return f, nil
}

func (f *firstRunScreen) View(a *App, w, h int) string {
	st := a.Style
	acc := func(s string) string { return st.Base().Foreground(st.Accent).Bold(true).Render(s) }
	dim := func(s string) string { return st.Base().Foreground(st.Dim).Render(s) }
	fg := func(s string) string { return st.Base().Foreground(st.Fg).Render(s) }
	bw := max(w-4, 10) // inside the frame, with a margin
	prose := func(s string) []string { return strings.Split(ansi.Wordwrap(s, bw, ""), "\n") }
	frame := func(title string, body []string, hintLine string) string {
		return style.Frame(st, w, h, title, " "+strings.Join(body, "\n "), hintLine)
	}
	choice := func(i, sel int, label, note string) []string {
		if i == sel {
			return []string{acc("> " + label), dim("  " + note)}
		}
		return []string{fg("  " + label), dim("  " + note)}
	}

	switch f.step {
	case stepStyle:
		return f.styleView(a, w, h)
	case stepNoLayout:
		var body []string
		for _, l := range prose(fmt.Sprintf(text.FirstRunNoLayout, goos)) {
			body = append(body, fg(l))
		}
		body = append(body, "", fg(text.FirstRunNoLayoutHow),
			acc(shortPath(filepath.Join(a.Store.ConfigDir(), userLayoutDir), bw)), "", fg(text.FirstRunNoLayoutDocs))
		return frame(text.FirstRunNoLayoutTitle, body, text.FirstRunContinueHint)
	case stepCheck:
		r := f.checks[f.check]
		body := []string{"", acc(fmt.Sprintf(text.FirstRunCheckAsk, string(r), f.check+1, len(f.checks)))}
		if how := hint.Text(a.Layout, r); how != "" {
			_, how, _ = strings.Cut(how, " → ") // the character itself is already asked for
			body = append(body, "")
			for _, l := range prose(fmt.Sprintf(text.FirstRunCheckHow, how)) {
				body = append(body, dim(l))
			}
		}
		if f.note != "" {
			body = append(body, "")
			for _, l := range prose(f.note) {
				body = append(body, fg(l))
			}
		}
		return frame(text.FirstRunCheckTitle, body, text.FirstRunCheckHint)
	case stepPicker:
		var head, list, tail []string
		for _, l := range prose(fmt.Sprintf(text.FirstRunPickAsk, a.Layout.Name)) {
			head = append(head, fg(l))
		}
		head = append(head, "")
		for i, l := range f.layouts {
			if i == f.pickAt {
				list = append(list, acc("> "+l.Name))
			} else {
				list = append(list, fg("  "+l.Name))
			}
		}
		tail = []string{"", dim(text.FirstRunNoLayoutHow),
			acc(shortPath(filepath.Join(a.Store.ConfigDir(), userLayoutDir), bw)), dim(text.FirstRunNoLayoutDocs)}
		room := h - 3 - len(head) - len(tail)
		body := append(head, scrollTo(list, f.pickAt, room)...)
		return frame(text.FirstRunPickTitle, append(body, tail...), text.FirstRunPickHint)
	case stepOptionHelp:
		var body []string
		for _, l := range prose(text.FirstRunOptionWhy) {
			body = append(body, fg(l))
		}
		body = append(body, "")
		intro, setting := optionHelp()
		for _, l := range prose(intro) {
			body = append(body, acc(l))
		}
		for _, l := range strings.Split(setting, "\n") {
			if l != "" {
				body = append(body, fg(" "+l))
			}
		}
		body = append(body, "")
		for _, l := range prose(text.OptionReload) {
			body = append(body, dim(l))
		}
		return frame(text.FirstRunOptionTitle, body, text.FirstRunOptionHint)
	case stepFork:
		body := []string{""}
		body = append(body, choice(0, f.forkAt, text.ForkFromZero, text.ForkFromZeroNote)...)
		body = append(body, "")
		body = append(body, choice(1, f.forkAt, text.ForkPlacement, text.ForkPlacementNote)...)
		return frame(text.FirstRunForkTitle, body, text.FirstRunForkHint)
	}
	return f.hands.view(a, w, h)
}

// handText is the hand-position text of chapter 0. It scrolls when the window
// is too small to show all of it.
type handText struct{ off, max int }

func (t *handText) key(k tea.KeyPressMsg) {
	switch k.String() {
	case "up", "k":
		t.off--
	case "down", "j":
		t.off++
	case "pgup":
		t.off -= 5
	case "pgdown":
		t.off += 5
	}
	t.off = max(0, min(t.off, t.max))
}

func (t *handText) view(a *App, w, h int) string {
	st := a.Style
	var lines []string
	for _, l := range strings.Split(ansi.Wordwrap(text.FirstRunZeroText, max(w-4, 10), ""), "\n") {
		lines = append(lines, " "+st.Base().Foreground(st.Fg).Render(l))
	}
	rows := h - 3
	if len(lines) > rows && rows > 0 { // drop the blank separators before scrolling
		lines = slices.DeleteFunc(lines, func(l string) bool { return strings.TrimSpace(ansi.Strip(l)) == "" })
	}
	t.max = max(0, len(lines)-rows)
	t.off = min(t.off, t.max)
	if rows > 0 && len(lines) > rows {
		lines = lines[t.off : t.off+rows]
	}
	return style.Frame(st, w, h, text.FirstRunZeroTitle, strings.Join(lines, "\n"), text.FirstRunHandHint)
}

// scrollTo keeps line sel within the first rows lines of body.
func scrollTo(body []string, sel, rows int) []string {
	if rows <= 0 || len(body) <= rows {
		return body
	}
	top := min(max(sel-rows+1, 0), len(body)-rows)
	return body[top : top+rows]
}

// styleView draws a real round in the chosen style, with the choice laid over it.
func (f *firstRunScreen) styleView(a *App, w, h int) string {
	st := a.Style
	t := engine.Prepare(sampleSource)
	v := roundview.View{
		Frame: play.Frame{
			Cells: t.Cells, Pos: samplePos, WPM: sampleWPM, Accuracy: sampleAcc, WrongPos: map[int]bool{},
		},
		Title: text.FirstRunSampleTitle, Lang: "go", Layout: a.Layout,
	}
	base := roundview.Render(st, v, w, h)

	box := []string{st.Base().Foreground(st.Accent).Bold(true).Render(text.FirstRunWelcome), ""}
	for _, s := range style.All() {
		if s.ID == st.ID {
			box = append(box, st.Base().Foreground(st.Accent).Bold(true).Render("> "+s.Name))
		} else {
			box = append(box, st.Base().Foreground(st.Fg).Render("  "+s.Name))
		}
	}
	box = append(box, "", st.Base().Foreground(st.Dim).Render(text.FirstRunStyleHint))
	return overlayAtBottom(st, base, box, w)
}

// overlayAtBottom paints box over the last rows of a rendered view, centred.
func overlayAtBottom(s style.Style, view string, box []string, w int) string {
	lines := strings.Split(view, "\n")
	widest := 0
	for _, b := range box {
		widest = max(widest, ansi.StringWidth(b))
	}
	pad := strings.Repeat(" ", max(0, (w-widest)/2))
	top := max(0, len(lines)-len(box)-boxMargin)
	for i, b := range box {
		if top+i < len(lines) {
			lines[top+i] = s.Paint(pad+b, w)
		}
	}
	return strings.Join(lines, "\n")
}

// shortPath writes a path in at most w cells: the home folder as ~, and when it
// is still too long only its end.
func shortPath(p string, w int) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home) {
		p = "~" + strings.TrimPrefix(p, home)
	}
	if r := []rune(p); len(r) > w {
		p = "…" + string(r[len(r)-w+1:])
	}
	return p
}

// terminalKind names the terminal the Option help is written for.
type terminalKind int

const (
	termUnknown terminalKind = iota
	termTerminalApp
	termITerm
	termGhostty
	termKitty
	termWezTerm
	termAlacritty
)

// detectTerminal reads the environment the terminal sets for its children.
func detectTerminal() terminalKind {
	switch os.Getenv("TERM_PROGRAM") {
	case "Apple_Terminal":
		return termTerminalApp
	case "iTerm.app":
		return termITerm
	case "ghostty":
		return termGhostty
	case "WezTerm":
		return termWezTerm
	}
	switch {
	case os.Getenv("KITTY_WINDOW_ID") != "":
		return termKitty
	case os.Getenv("ALACRITTY_WINDOW_ID") != "", os.Getenv("TERM") == "alacritty":
		return termAlacritty
	}
	return termUnknown
}

// altHelp is the one line a round shows when Option arrives as Alt: the setting
// of the detected terminal, or nothing when the terminal is unknown.
func altHelp() string {
	_, setting := optionHelp()
	if setting == "" {
		return ""
	}
	first, _, _ := strings.Cut(setting, "\n")
	if detectTerminal() == termTerminalApp { // the one setting that is a tick to clear
		return fmt.Sprintf(text.RoundAltUnset, first)
	}
	return fmt.Sprintf(text.RoundAltSet, first)
}

// optionHelp is the explanation for the detected terminal and, where the fix
// is a line of configuration, that line.
func optionHelp() (explanation, setting string) {
	switch detectTerminal() {
	case termTerminalApp:
		return text.OptionTerminalApp, text.OptionSettingTerminalApp
	case termITerm:
		return text.OptionITerm, text.OptionSettingITerm
	case termGhostty:
		return text.OptionGhostty, text.OptionSettingGhostty
	case termKitty:
		return text.OptionKitty, text.OptionSettingKitty
	case termWezTerm:
		return text.OptionWezTerm, text.OptionSettingWezTerm
	case termAlacritty:
		return text.OptionAlacritty, text.OptionSettingAlacritty
	}
	return text.OptionGeneric, ""
}
