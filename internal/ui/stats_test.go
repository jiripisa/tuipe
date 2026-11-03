package ui_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/hint"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

var (
	tab    = tea.KeyPressMsg{Code: tea.KeyTab}
	right  = tea.KeyPressMsg{Code: tea.KeyRight}
	left   = tea.KeyPressMsg{Code: tea.KeyLeft}
	pgDown = tea.KeyPressMsg{Code: tea.KeyPgDown}
	pgUp   = tea.KeyPressMsg{Code: tea.KeyPgUp}
)

// statsApp is an app on the fixed clock with a window of the given size.
func statsApp(t *testing.T, w, h int) (*ui.App, *store.Store, *clock) {
	t.Helper()
	a, st, clk := playApp(t)
	a.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return a, st, clk
}

func openStats(t *testing.T, a *ui.App) {
	t.Helper()
	ui.SetScreen(a, ui.NewLessonsScreen(a))
	a.Update(key('s'))
	wantScreen(t, a, "stats")
}

func addRun(t *testing.T, st *store.Store, r store.Run) {
	t.Helper()
	if err := st.AppendRun(r); err != nil {
		t.Fatal(err)
	}
}

// daysAgo is noon, n days before the fixed clock.
func daysAgo(clk *clock, n int) time.Time { return clk.t.AddDate(0, 0, -n) }

// lessonOf returns the first lesson of the chapter with the given number.
func lessonOf(t *testing.T, a *ui.App, number int) (id, title string) {
	t.Helper()
	ls := firstOfChapter(t, a, fmt.Sprintf("ch%02d", number))
	return ls.ID, ls.Title
}

// lineWith returns the n-th (0-based) line of the screen that starts with prefix after trimming.
func nthLine(s, prefix string, n int) string {
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(strings.Trim(strings.TrimSpace(l), "│"))
		if strings.HasPrefix(l, prefix) {
			if n == 0 {
				return l
			}
			n--
		}
	}
	return ""
}

// historyLines are the lines of the screen that hold a run: they show a speed.
func historyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, "zn/min") {
			out = append(out, strings.TrimSpace(strings.Trim(strings.TrimSpace(l), "│")))
		}
	}
	return out
}

func TestHistoryIsPlain(t *testing.T) {
	a, st, clk := statsApp(t, 100, 30)
	id, title := lessonOf(t, a, 1)
	addRun(t, st, store.Run{ID: "a3f9c1e", At: daysAgo(clk, 0), Mode: store.ModeTest, LessonID: id,
		WPM: 62, Accuracy: 0.981, Stars: 2})
	addRun(t, st, store.Run{ID: "b7d20aa", At: daysAgo(clk, 1), Mode: store.ModePlacement, WPM: 40, Accuracy: 0.9})
	addRun(t, st, store.Run{ID: "c0ffee1", At: daysAgo(clk, 1).Add(-time.Hour), Mode: store.ModeTraining, LessonID: id, Length: "short", WPM: 30, Accuracy: 0.8})
	openStats(t, a)
	s := screen(a)

	lines := historyLines(s)
	if len(lines) != 3 {
		t.Fatalf("history has %d lines, want 3:\n%s", len(lines), s)
	}
	for _, want := range []string{"31. 10. 2026", text.StatsModeTest, "310 zn/min", "98,1 %", "★★☆", title} {
		if !strings.HasPrefix(lines[0], "> ") || !strings.Contains(lines[0], want) {
			t.Errorf("first line lacks %q: %q", want, lines[0])
		}
	}
	// A run of no lesson shows its part; the others follow, newest first.
	if !strings.Contains(lines[1], text.StatsModePlacement) || !strings.Contains(lines[1], "200 zn/min") {
		t.Errorf("second line = %q, want the placement test", lines[1])
	}
	if !strings.Contains(lines[2], text.StatsModeTraining) || !strings.Contains(lines[2], title) {
		t.Errorf("third line = %q, want the Trénink with its lesson", lines[2])
	}
	for _, no := range []string{"a3f9c1e", "b7d20aa", "c0ffee1", "wpm", "git log", "uptime"} {
		if strings.Contains(s, no) {
			t.Errorf("the history shows %q:\n%s", no, s)
		}
	}
	// One line per run: no header per day.
	if strings.Count(s, "30. 10. 2026") != 2 {
		t.Errorf("the date belongs on every line:\n%s", s)
	}
	if !strings.Contains(s, "2 dny v řadě") {
		t.Errorf("header lacks the days in a row:\n%s", s)
	}
}

func TestHistoryNamesTheLengthOfATrainingInPlainCzech(t *testing.T) {
	a, st, clk := statsApp(t, 100, 30)
	id, _ := lessonOf(t, a, 1)
	for i, length := range []string{"short", "medium", "long"} {
		addRun(t, st, store.Run{ID: length, At: daysAgo(clk, 0).Add(-time.Duration(i) * time.Minute), Mode: store.ModeTraining,
			Length: length, LessonID: id, WPM: 30, Accuracy: 0.9})
	}
	openStats(t, a)
	lines := historyLines(screen(a))
	for i, want := range []string{"krátký", "střední", "dlouhý"} {
		if len(lines) != 3 || !strings.Contains(lines[i], want) {
			t.Errorf("line %d lacks %q: %q", i, want, lines)
		}
	}
	for _, no := range []string{"short", "medium", "long"} {
		if strings.Contains(screen(a), no) {
			t.Errorf("the history shows the stored length %q", no)
		}
	}
}

func TestHistoryFilter(t *testing.T) {
	a, st, clk := statsApp(t, 100, 30)
	id, _ := lessonOf(t, a, 1)
	addRun(t, st, store.Run{ID: "t", At: daysAgo(clk, 0), Mode: store.ModeTest, LessonID: id, WPM: 62, Accuracy: 0.98, Stars: 2})
	addRun(t, st, store.Run{ID: "k", At: daysAgo(clk, 1), Mode: store.ModeTraining, Length: "short", LessonID: id, WPM: 30, Accuracy: 0.9})
	addRun(t, st, store.Run{ID: "p", At: daysAgo(clk, 2), Mode: store.ModePlacement, WPM: 40, Accuracy: 0.9})
	openStats(t, a)

	// F cycles Vše, Trénink, Test and round again.
	for _, c := range []struct {
		filter string
		shows  []string
		hides  []string
	}{
		{text.StatsFilters[0], []string{"310 zn/min", "150 zn/min", "200 zn/min"}, nil},
		{text.StatsFilters[1], []string{"150 zn/min"}, []string{"310 zn/min", "200 zn/min"}},
		{text.StatsFilters[2], []string{"310 zn/min"}, []string{"150 zn/min", "200 zn/min"}},
		{text.StatsFilters[0], []string{"310 zn/min", "150 zn/min", "200 zn/min"}, nil},
	} {
		s := screen(a)
		if !strings.Contains(s, text.StatsFilterLabel+": "+c.filter) {
			t.Fatalf("the screen does not name the filter %q:\n%s", c.filter, s)
		}
		showsAll(t, a, c.shows...)
		showsNone(t, a, c.hides...)
		a.Update(key('f'))
	}
	// The filter is a matter of the history alone, and the F key does nothing elsewhere.
	a.Update(tab)
	a.Update(key('f'))
	a.Update(left)
	showsAll(t, a, text.StatsFilterLabel+": "+text.StatsFilters[1])
}

// The hint says F, and Shift or Caps Lock gives a capital: both letters work.
func TestHistoryFilterTakesACapitalF(t *testing.T) {
	a, st, clk := statsApp(t, 100, 30)
	addRun(t, st, store.Run{ID: "k", At: daysAgo(clk, 0), Mode: store.ModeTraining, Length: "short", WPM: 30, Accuracy: 0.9})
	openStats(t, a)
	a.Update(key('F'))
	showsAll(t, a, text.StatsFilterLabel+": "+text.StatsFilters[1])
}

func TestHistoryFilterWithNothingToShow(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	addRun(t, st, store.Run{ID: "k", At: daysAgo(clk, 0), Mode: store.ModeTraining, Length: "short", WPM: 30, Accuracy: 0.9})
	openStats(t, a)
	a.Update(key('f'))
	a.Update(key('f')) // Test
	showsAll(t, a, text.StatsNoRuns)
	a.Update(down) // must not panic or select anything
	a.Update(key('f'))
	showsAll(t, a, "150 zn/min")
}

func TestPlacementInHistory(t *testing.T) {
	a, st, clk := statsApp(t, 100, 30)
	addRun(t, st, store.Run{ID: "p", At: daysAgo(clk, 0), Mode: store.ModePlacement, WPM: 40, Accuracy: 0.9})
	addRun(t, st, store.Run{ID: "q", At: daysAgo(clk, 1), Mode: store.ModeTest, WPM: 20, Accuracy: 0.95})
	openStats(t, a)
	showsAll(t, a, text.StatsModePlacement)
	a.Update(key('f'))
	showsNone(t, a, text.StatsModePlacement)
	a.Update(key('f'))
	showsNone(t, a, text.StatsModePlacement)
	// Nowhere else: not in the trends, not in the records, not in the days in a row.
	a.Update(key('f'))
	a.Update(tab)
	a.Update(tab)
	a.Update(tab)
	showsNone(t, a, "200 zn/min")
	showsAll(t, a, "1 den v řadě")
}

func TestStreakDaysAndBest(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	// Five days in a row ended, then one missed, then three days up to today.
	for _, d := range []int{9, 8, 7, 6, 5, 2, 1, 0} {
		addRun(t, st, store.Run{ID: fmt.Sprintf("r%d", d), At: daysAgo(clk, d), Mode: store.ModeTraining, WPM: 30, Accuracy: 0.95})
	}
	// A placement test fills the missed day, but it is no practice.
	addRun(t, st, store.Run{ID: "p", At: daysAgo(clk, 3), Mode: store.ModePlacement, WPM: 30, Accuracy: 0.95})
	openStats(t, a)
	showsAll(t, a, "3 dny v řadě")
	showsNone(t, a, "uptime", "up 3")
	for range 3 {
		a.Update(tab)
	}
	s := screen(a)
	if l := nthLine(s, text.StatsRecordDays, 0); !strings.HasSuffix(l, "5") {
		t.Errorf("most days in a row = %q, want 5:\n%s", l, s)
	}
}

func TestStatsReadRunsOnceOnEntering(t *testing.T) {
	a, st, clk := statsApp(t, 100, 30)
	addRun(t, st, store.Run{ID: "a", At: daysAgo(clk, 0), Mode: store.ModeTraining, WPM: 30, Accuracy: 0.95})
	openStats(t, a)
	addRun(t, st, store.Run{ID: "b", At: daysAgo(clk, 0), Mode: store.ModeTraining, WPM: 40, Accuracy: 0.95})
	showsNone(t, a, "200 zn/min")
	a.Update(esc)
	openStats(t, a)
	showsAll(t, a, "200 zn/min")
}

func TestHistoryScrollsBeyondWindowHeight(t *testing.T) {
	a, st, clk := statsApp(t, 60, 18)
	const n = 50
	for i := range n { // i = 0 is the newest; its speed names it
		addRun(t, st, store.Run{ID: fmt.Sprintf("run%04d", i), At: daysAgo(clk, 0).Add(-time.Duration(i) * time.Minute),
			Mode: store.ModeTraining, WPM: float64(10 + i), Accuracy: 0.95})
	}
	openStats(t, a)
	selected := func() string { return nthLine(screen(a), ">", 0) }
	speed := func(i int) string { return fmt.Sprintf("%d zn/min", 5*(10+i)) }

	if s := selected(); !strings.Contains(s, speed(0)) {
		t.Fatalf("the newest run must be selected first: %q", s)
	}
	for range 30 {
		a.Update(down)
	}
	if s := selected(); !strings.Contains(s, speed(30)) {
		t.Fatalf("after 30 downs the selection is %q, want %s:\n%s", s, speed(30), screen(a))
	}
	a.Update(up)
	if s := selected(); !strings.Contains(s, speed(29)) {
		t.Errorf("after Up the selection is %q, want %s", s, speed(29))
	}
	a.Update(pgDown)
	first := selected()
	if first == "" || strings.Contains(first, speed(29)) {
		t.Errorf("PgDn did not move the selection: %q", first)
	}
	for range 10 {
		a.Update(pgDown)
	}
	if s := selected(); !strings.Contains(s, speed(49)) {
		t.Errorf("PgDn to the end must select the oldest run: %q\n%s", s, screen(a))
	}
	for range 10 {
		a.Update(pgUp)
	}
	if s := selected(); !strings.Contains(s, speed(0)) {
		t.Errorf("PgUp to the top must select the newest run: %q\n%s", s, screen(a))
	}
	if got := len(strings.Split(screen(a), "\n")); got != 18 {
		t.Errorf("screen has %d lines, want 18", got)
	}
}

func openHeatmap(t *testing.T, a *ui.App) {
	t.Helper()
	openStats(t, a)
	a.Update(tab)
}

func setStats(a *ui.App, chars map[rune]stats.Entry) {
	s := stats.New()
	for r, e := range chars {
		e := e
		s.Chars[string(r)] = &e
	}
	a.Store.Profile().Stats[a.Layout.ID] = s
}

// keyStyle returns the escape sequences that precede the five-column label of a key in the raw view.
func keyStyle(t *testing.T, raw, label string) string {
	t.Helper()
	re := regexp.MustCompile(`((?:\x1b\[[0-9;:]*m)+)\s*` + regexp.QuoteMeta(label) + `\s*\x1b\[`)
	m := re.FindStringSubmatch(raw)
	if m == nil {
		t.Fatalf("key %q not found in the raw view", label)
	}
	return m[1]
}

func TestHeatmapColoursByWeakness(t *testing.T) {
	a, st, clk := statsApp(t, 100, 40)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: store.ModeTraining, WPM: 30, Accuracy: 0.95})
	setStats(a, map[rune]stats.Entry{
		'a': {LatencyMs: 800, Samples: 10}, // weakness 0.8: the highest
		'd': {LatencyMs: 400, Samples: 10}, // half of it
		'f': {LatencyMs: 800, Samples: 2},  // too few samples: not weak
	})
	openHeatmap(t, a)
	raw := a.View().Content
	sa, sd, sf := keyStyle(t, raw, "a"), keyStyle(t, raw, "d"), keyStyle(t, raw, "f")
	if sa == sd || sa == sf || sd == sf {
		t.Errorf("weak, half-weak and untouched keys must differ:\nA %q\nD %q\nF %q", sa, sd, sf)
	}
	// An untouched key looks the same as on an empty map, which is the dim style.
	setStats(a, map[rune]stats.Entry{'a': {LatencyMs: 800, Samples: 10}})
	a.Update(esc)
	openHeatmap(t, a)
	if got := keyStyle(t, a.View().Content, "s"); got != sf {
		t.Errorf("an untouched key changed colour: %q vs %q", got, sf)
	}
}

func TestHeatmapUsesLastChordKeyOfTheRoute(t *testing.T) {
	a, st, clk := statsApp(t, 100, 40)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: store.ModeTraining, WPM: 30, Accuracy: 0.95})
	// "A" is typed with Shift on the key of "a": the key heats, not a Shift key.
	setStats(a, map[rune]stats.Entry{'A': {LatencyMs: 800, Samples: 10}})
	openHeatmap(t, a)
	raw := a.View().Content
	if keyStyle(t, raw, "a") == keyStyle(t, raw, "f") {
		t.Error("the key of a Shift+a character must be heated")
	}
	// "ď" is a dead key followed by "d": the route ends on the key of "d".
	setStats(a, map[rune]stats.Entry{'ď': {LatencyMs: 800, Samples: 10}})
	a.Update(esc)
	openHeatmap(t, a)
	raw = a.View().Content
	if keyStyle(t, raw, "d") == keyStyle(t, raw, "f") {
		t.Error("the last key of a two-chord route must be heated")
	}
}

func TestFingerSummaryNamesWeakestFinger(t *testing.T) {
	a, st, clk := statsApp(t, 100, 40)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: store.ModeTraining, WPM: 30, Accuracy: 0.95})
	// The left ring finger has the weakest single key (s), but the left pinky
	// has the larger sum (a + q).
	setStats(a, map[rune]stats.Entry{
		's': {LatencyMs: 600, Samples: 10},
		'a': {LatencyMs: 400, Samples: 10},
		'q': {LatencyMs: 400, Samples: 10},
	})
	openHeatmap(t, a)
	s := screen(a)
	want := fmt.Sprintf(text.StatsWeakestFinger, "levý malíček")
	if !strings.Contains(s, want) {
		t.Errorf("finger summary lacks %q:\n%s", want, s)
	}
	for _, r := range []rune{'s', 'a', 'q'} {
		h := hint.Text(a.Layout, r)
		if !strings.Contains(s, h) {
			t.Errorf("weakest characters lack the hint %q:\n%s", h, s)
		}
	}
	if strings.Index(s, hint.Text(a.Layout, 's')) > strings.Index(s, hint.Text(a.Layout, 'a')) {
		t.Errorf("the weakest character must come first:\n%s", s)
	}
}

func TestHeatmapListsAtMostEightCharsAndFitsMinimumWindow(t *testing.T) {
	a, st, clk := statsApp(t, 60, 18)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: store.ModeTraining, WPM: 30, Accuracy: 0.95})
	chars := map[rune]stats.Entry{}
	for i, r := range "asdfjklqwerty" {
		chars[r] = stats.Entry{LatencyMs: float64(1000 - 50*i), Samples: 10}
	}
	setStats(a, chars)
	openHeatmap(t, a)
	s := screen(a)
	shown := 0
	for _, r := range "asdfjklqwerty" {
		if strings.Contains(s, hint.Text(a.Layout, r)) {
			shown++
		}
	}
	if shown != 8 {
		t.Errorf("shows %d weakest characters, want 8:\n%s", shown, s)
	}
	if got := len(strings.Split(s, "\n")); got != 18 {
		t.Errorf("screen has %d lines, want 18", got)
	}
}

func TestHeatmapOfEmptyStatsShowsMessage(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: store.ModeTraining, WPM: 30, Accuracy: 0.95})
	openHeatmap(t, a)
	if s := screen(a); !strings.Contains(s, text.StatsHeatEmpty) {
		t.Errorf("empty heatmap lacks %q:\n%s", text.StatsHeatEmpty, s)
	}
}

func TestShowingAHintHereIsNotHintUse(t *testing.T) {
	a, st, clk := statsApp(t, 100, 40)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: store.ModeTraining, WPM: 30, Accuracy: 0.95})
	setStats(a, map[rune]stats.Entry{'a': {LatencyMs: 800, Samples: 10, Mastery: stats.Learning}})
	before := *a.Store.Profile().Stats[a.Layout.ID].Chars["a"]
	openHeatmap(t, a)
	_ = screen(a)
	if after := *a.Store.Profile().Stats[a.Layout.ID].Chars["a"]; after != before {
		t.Errorf("viewing the heatmap changed the statistics: %+v -> %+v", before, after)
	}
	if runs, _ := st.Runs(); len(runs) != 1 {
		t.Errorf("viewing the heatmap wrote runs: %d", len(runs))
	}
}

func openTrends(t *testing.T, a *ui.App) {
	t.Helper()
	openStats(t, a)
	a.Update(tab)
	a.Update(tab)
}

func TestTrendsOverallAndPerGroup(t *testing.T) {
	a, st, clk := statsApp(t, 60, 18)
	letters, _ := lessonOf(t, a, 1)
	diacritics, _ := lessonOf(t, a, 5)
	digits, _ := lessonOf(t, a, 6)
	shift, _ := lessonOf(t, a, 4)
	option, _ := lessonOf(t, a, 8)
	n := 0
	run := func(day int, lesson string, wpm, acc float64) {
		n++
		addRun(t, st, store.Run{ID: fmt.Sprintf("t%d", n), At: daysAgo(clk, day), Mode: store.ModeTest, LessonID: lesson, WPM: wpm, Accuracy: acc})
	}
	// Diacritics rise 10, 20, 30; letters fall 30, 20, 10; the others have one day.
	for i, w := range []float64{10, 20, 30} {
		run(2-i, diacritics, w, 0.90+0.02*float64(i))
		run(2-i, letters, 40-w, 0.99-0.02*float64(i))
	}
	run(0, digits, 25, 0.95)
	run(0, shift, 26, 0.96)
	run(0, option, 27, 0.97)
	// A Trénink and a placement test are no part of any trend.
	addRun(t, st, store.Run{ID: "drill", At: daysAgo(clk, 3), Mode: store.ModeTraining, LessonID: letters, WPM: 20, Accuracy: 0.9})
	addRun(t, st, store.Run{ID: "place", At: daysAgo(clk, 4), Mode: store.ModePlacement, WPM: 20, Accuracy: 0.9})
	openTrends(t, a)
	s := screen(a)

	glyphs := func(l string) string {
		var g []rune
		for _, r := range l {
			if r >= '▁' && r <= '█' {
				g = append(g, r)
			}
		}
		return string(g)
	}
	for label, want := range map[string]string{
		text.StatsGroupDiacritics: "▁▅█",
		text.StatsGroupLetters:    "█▅▁",
		text.StatsGroupDigits:     "▄",
		text.StatsGroupShift:      "▄",
		text.StatsGroupOption:     "▄",
	} {
		if got := glyphs(nthLine(s, label, 0)); got != want {
			t.Errorf("speed trend of %s = %q, want %q:\n%s", label, got, want, s)
		}
		if nthLine(s, label, 1) == "" {
			t.Errorf("accuracy trend of %s is missing:\n%s", label, s)
		}
	}
	// Overall: the three days of Tests.
	if got := glyphs(nthLine(s, text.StatsGroupOverall, 0)); len([]rune(got)) != 3 {
		t.Errorf("overall speed trend = %q, want 3 days:\n%s", got, s)
	}
	if got := glyphs(nthLine(s, text.StatsGroupOverall, 1)); len([]rune(got)) != 3 {
		t.Errorf("overall accuracy trend = %q, want 3 days:\n%s", got, s)
	}
	if l := nthLine(s, text.StatsGroupDiacritics, 0); !strings.Contains(l, "150 zn/min") {
		t.Errorf("the latest speed should be shown in characters per minute: %q", l)
	}
	if l := nthLine(s, text.StatsGroupDiacritics, 1); !strings.Contains(l, "94,0 %") {
		t.Errorf("the latest accuracy should be shown with a decimal comma: %q", l)
	}
}

func TestTrendsUseDailyMediansOfTheLast30ActiveDays(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	// 35 active days; the sparkline keeps the newest 30. A day of three runs
	// with 10, 20 and 90 WPM has the median 20.
	for d := range 35 {
		addRun(t, st, store.Run{ID: fmt.Sprintf("d%02d", d), At: daysAgo(clk, d*2), Mode: store.ModeTest, WPM: 50, Accuracy: 0.95})
	}
	for i, w := range []float64{10, 90, 20} {
		addRun(t, st, store.Run{ID: fmt.Sprintf("m%d", i), At: daysAgo(clk, 1).Add(time.Duration(i) * time.Minute), Mode: store.ModeTest, WPM: w, Accuracy: 0.95})
	}
	openTrends(t, a)
	l := nthLine(screen(a), text.StatsGroupOverall, 0)
	count := 0
	for _, r := range l {
		if r >= '▁' && r <= '█' {
			count++
		}
	}
	if count != 30 {
		t.Errorf("sparkline has %d points, want 30: %q", count, l)
	}
}

func TestTrendsReadTestsOnly(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	id, _ := lessonOf(t, a, 1)
	addRun(t, st, store.Run{ID: "t1", At: daysAgo(clk, 2), Mode: store.ModeTest, LessonID: id, WPM: 10, Accuracy: 0.9})
	addRun(t, st, store.Run{ID: "t2", At: daysAgo(clk, 0), Mode: store.ModeTest, LessonID: id, WPM: 20, Accuracy: 0.95})
	// Runs of the other parts, on a day of their own and on the day of a Test.
	addRun(t, st, store.Run{ID: "k1", At: daysAgo(clk, 1), Mode: store.ModeTraining, Length: "long", LessonID: id, WPM: 90, Accuracy: 1})
	addRun(t, st, store.Run{ID: "k2", At: daysAgo(clk, 0).Add(time.Minute), Mode: store.ModeTraining, Length: "long", LessonID: id, WPM: 90, Accuracy: 1})
	addRun(t, st, store.Run{ID: "p1", At: daysAgo(clk, 2).Add(time.Minute), Mode: store.ModePlacement, WPM: 90, Accuracy: 1})
	openTrends(t, a)
	s := screen(a)
	for _, group := range []string{text.StatsGroupOverall, text.StatsGroupLetters} {
		l := nthLine(s, group, 0)
		if n := strings.Count(l, "▁") + strings.Count(l, "█"); n != 2 {
			t.Errorf("%s trend = %q, want the two days of Tests:\n%s", group, l, s)
		}
		if !strings.Contains(l, "100 zn/min") || strings.Contains(l, "450") {
			t.Errorf("%s trend = %q, want 100 zn/min and no Trénink speed", group, l)
		}
	}
}

func TestEmptyHistoryShowsFriendlyMessage(t *testing.T) {
	a, _, _ := statsApp(t, 80, 24)
	openStats(t, a)
	for i, name := range []string{"Historie", "Klávesy", "Trendy", "Rekordy"} {
		if i > 0 {
			a.Update(tab)
		}
		s := screen(a)
		if !strings.Contains(s, text.StatsEmpty) {
			t.Errorf("tab %s lacks %q:\n%s", name, text.StatsEmpty, s)
		}
		if !strings.Contains(s, name) {
			t.Errorf("tab bar lacks %q:\n%s", name, s)
		}
	}
}

func TestRecordsAreTheThreePlainRows(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	id, _ := lessonOf(t, a, 1)
	add := func(r store.Run) {
		r.At = daysAgo(clk, 0)
		addRun(t, st, r)
	}
	add(store.Run{ID: "l1", Mode: store.ModeTest, LessonID: id, WPM: 50, Accuracy: 0.99, Score: 44})
	add(store.Run{ID: "l2", Mode: store.ModeTest, LessonID: id, WPM: 55, Accuracy: 0.96, Score: 46.7}) // fastest
	add(store.Run{ID: "l3", Mode: store.ModeTest, LessonID: id, WPM: 90, Accuracy: 1, Score: 86, Paused: true})
	add(store.Run{ID: "t1", Mode: store.ModeTraining, LessonID: id, Length: "short", WPM: 80, Accuracy: 1, Score: 79})
	add(store.Run{ID: "p1", Mode: store.ModePlacement, WPM: 70, Accuracy: 1, Score: 69})
	openStats(t, a)
	for range 3 {
		a.Update(tab)
	}
	s := screen(a)
	for label, want := range map[string]string{
		text.StatsRecordSpeed:    "275 zn/min",
		text.StatsRecordAccuracy: "99,0 %",
		text.StatsRecordDays:     "1",
	} {
		if l := nthLine(s, label, 0); !strings.HasSuffix(l, want) {
			t.Errorf("record %q = %q, want it to end with %q:\n%s", label, l, want, s)
		}
	}
	// A paused run, a Trénink and the placement test are no records.
	for _, not := range []string{"450 zn/min", "400 zn/min", "350 zn/min", "100,0 %", "skóre", "wpm"} {
		if strings.Contains(s, not) {
			t.Errorf("%s counted as a record:\n%s", not, s)
		}
	}
	if got := len(historyLines(s)); got != 1 {
		t.Errorf("the records are three rows with one speed, found %d lines of speed:\n%s", got, s)
	}
}

func TestRecordsWithoutATestShowDashes(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	addRun(t, st, store.Run{ID: "k", At: daysAgo(clk, 0), Mode: store.ModeTraining, Length: "short", WPM: 30, Accuracy: 0.9})
	openStats(t, a)
	for range 3 {
		a.Update(tab)
	}
	s := screen(a)
	for _, label := range []string{text.StatsRecordSpeed, text.StatsRecordAccuracy} {
		if l := nthLine(s, label, 0); !strings.HasSuffix(l, text.StatsNoRecord) {
			t.Errorf("record %q = %q, want a dash:\n%s", label, l, s)
		}
	}
	if l := nthLine(s, text.StatsRecordDays, 0); !strings.HasSuffix(l, "1") {
		t.Errorf("most days in a row = %q:\n%s", l, s)
	}
}

func TestTabsSwitchWithTabAndArrowsAndEscGoesToTheList(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: store.ModeTraining, WPM: 30, Accuracy: 0.95})
	openStats(t, a)
	bracket := regexp.MustCompile(`\[[^\]]+\]`)
	marker := func() string { // the active tab is bracketed
		for _, l := range strings.Split(screen(a), "\n") {
			if strings.Contains(l, "Rekordy") {
				return bracket.FindString(l)
			}
		}
		return ""
	}
	if m := marker(); !strings.Contains(m, "[Historie]") {
		t.Fatalf("active tab = %q, want [Historie]", m)
	}
	a.Update(right)
	if m := marker(); !strings.Contains(m, "[Klávesy]") {
		t.Errorf("after Right: %q", m)
	}
	a.Update(tab)
	if m := marker(); !strings.Contains(m, "[Trendy]") {
		t.Errorf("after Tab: %q", m)
	}
	a.Update(left)
	a.Update(left)
	if m := marker(); !strings.Contains(m, "[Historie]") {
		t.Errorf("after two Left: %q", m)
	}
	a.Update(left)
	if m := marker(); !strings.Contains(m, "[Rekordy]") {
		t.Errorf("Left from the first tab must wrap: %q", m)
	}
	a.Update(esc)
	wantScreen(t, a, "lessons")
}

func TestHistoryColumnsAlign(t *testing.T) {
	a, st, clk := statsApp(t, 100, 30)
	id, title := lessonOf(t, a, 1)
	addRun(t, st, store.Run{ID: "a", At: daysAgo(clk, 0), Mode: store.ModeTest, LessonID: id, WPM: 70, Accuracy: 1, Stars: 3})
	addRun(t, st, store.Run{ID: "b", At: daysAgo(clk, 0).Add(-time.Minute), Mode: store.ModeTraining, LessonID: id, WPM: 5, Accuracy: 0.5})
	addRun(t, st, store.Run{ID: "c", At: daysAgo(clk, 3).Add(-2 * time.Minute), Mode: store.ModeTest, LessonID: id, WPM: 30, Accuracy: 0.9})
	openStats(t, a)
	lines := historyLines(screen(a))
	if len(lines) != 3 {
		t.Fatalf("lines = %q", lines)
	}
	col := func(l string) int {
		l = strings.TrimPrefix(l, "> ")
		return len([]rune(l[:strings.Index(l, title)]))
	}
	if col(lines[0]) != col(lines[1]) || col(lines[1]) != col(lines[2]) {
		t.Errorf("the lesson column is not aligned (100,0 %% and a row without stars push it):\n%s", strings.Join(lines, "\n"))
	}
}

// A Trénink and the placement test judge nothing, so their rows show no stars
// at all rather than three empty ones, which would read as a failed Test.
func TestHistoryShowsStarsOnATestOnly(t *testing.T) {
	a, st, clk := statsApp(t, 100, 30)
	id, _ := lessonOf(t, a, 1)
	addRun(t, st, store.Run{ID: "t", At: daysAgo(clk, 0), Mode: store.ModeTest, LessonID: id, WPM: 30, Accuracy: 0.9})
	addRun(t, st, store.Run{ID: "k", At: daysAgo(clk, 1), Mode: store.ModeTraining, Length: "short", LessonID: id, WPM: 30, Accuracy: 0.9})
	addRun(t, st, store.Run{ID: "p", At: daysAgo(clk, 2), Mode: store.ModePlacement, WPM: 40, Accuracy: 0.9})
	openStats(t, a)
	lines := historyLines(screen(a))
	if len(lines) != 3 {
		t.Fatalf("lines = %q", lines)
	}
	if !strings.Contains(lines[0], "☆☆☆") {
		t.Errorf("a Test without a star shows its empty stars: %q", lines[0])
	}
	for _, l := range lines[1:] {
		if strings.ContainsAny(l, "★☆") {
			t.Errorf("a row that judges nothing shows stars: %q", l)
		}
	}
}

// Code chapters train everything at once, so neither letters nor any other
// group of keys is the right place for them.
func TestCodeChaptersHaveATrendOfTheirOwn(t *testing.T) {
	a, st, clk := playApp(t)
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	addRun(t, st, store.Run{ID: "c11", At: daysAgo(clk, 2), Mode: store.ModeTest, LessonID: "ch11-01", WPM: 30, Accuracy: 0.95})
	addRun(t, st, store.Run{ID: "c12", At: daysAgo(clk, 1), Mode: store.ModeTest, LessonID: "ch12-01", WPM: 40, Accuracy: 0.96})
	openTrends(t, a)
	s := screen(a)
	line := nthLine(s, text.StatsGroupCode, 0)
	if strings.Count(line, "▁")+strings.Count(line, "█") != 2 {
		t.Errorf("trend of %s = %q, want the two days of chapters 11 and 12:\n%s", text.StatsGroupCode, line, s)
	}
	if l := nthLine(s, text.StatsGroupLetters, 0); strings.ContainsAny(l, "▁▂▃▄▅▆▇█") {
		t.Errorf("code landed among the letters: %q", l)
	}
}
