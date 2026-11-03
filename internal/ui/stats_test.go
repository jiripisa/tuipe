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

func TestGitLogShowsHashWPMAccuracyAndStars(t *testing.T) {
	a, st, clk := statsApp(t, 100, 30)
	id, title := lessonOf(t, a, 1)
	addRun(t, st, store.Run{ID: "a3f9c1e", At: daysAgo(clk, 0), Mode: store.ModeTest, LessonID: id,
		WPM: 62, Accuracy: 0.981, Stars: 2})
	addRun(t, st, store.Run{ID: "b7d20aa", At: daysAgo(clk, 1), Mode: store.ModePlacement, WPM: 40, Accuracy: 0.9, Stars: 0})
	addRun(t, st, store.Run{ID: "c0ffee1", At: daysAgo(clk, 1).Add(-time.Hour), Mode: store.ModeTraining, LessonID: id, Length: "short", WPM: 30, Accuracy: 0.8})
	openStats(t, a)
	s := screen(a)

	want := "a3f9c1e  62 wpm  98.1 %  ★★☆  " + text.StatsModeTest + ": " + title
	if !strings.Contains(s, want) {
		t.Fatalf("git log lacks the line %q:\n%s", want, s)
	}
	if !strings.Contains(s, "b7d20aa") || !strings.Contains(s, text.StatsModePlacement) {
		t.Errorf("a run of no lesson must show its mode:\n%s", s)
	}
	if !strings.Contains(s, text.StatsModeTraining+": "+title) {
		t.Errorf("a Trénink must show its mode and lesson:\n%s", s)
	}
	if !(strings.Index(s, "a3f9c1e") < strings.Index(s, "b7d20aa") && strings.Index(s, "b7d20aa") < strings.Index(s, "c0ffee1")) {
		t.Errorf("runs are not newest first:\n%s", s)
	}
	for _, d := range []string{"2026-10-31", "2026-10-30"} {
		if strings.Count(s, d) != 1 {
			t.Errorf("date header %s must appear once:\n%s", d, s)
		}
	}
	if !strings.Contains(s, "up 2 days") {
		t.Errorf("header lacks the uptime:\n%s", s)
	}
}

func TestUptimeOnHeaderCountsConsecutiveDays(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	for i := range 3 {
		addRun(t, st, store.Run{ID: fmt.Sprintf("r%d", i), At: daysAgo(clk, i), Mode: store.ModeTraining, WPM: 30, Accuracy: 0.95})
	}
	openStats(t, a)
	if s := screen(a); !strings.Contains(s, "up 3 days") {
		t.Errorf("header lacks %q:\n%s", "up 3 days", s)
	}
}

func TestGitLogReadsRunsOnceOnEntering(t *testing.T) {
	a, st, clk := statsApp(t, 100, 30)
	addRun(t, st, store.Run{ID: "aaaaaaa", At: daysAgo(clk, 0), Mode: store.ModeTraining, WPM: 30, Accuracy: 0.95})
	openStats(t, a)
	addRun(t, st, store.Run{ID: "bbbbbbb", At: daysAgo(clk, 0), Mode: store.ModeTraining, WPM: 30, Accuracy: 0.95})
	if s := screen(a); strings.Contains(s, "bbbbbbb") {
		t.Errorf("runs were read again after entering the screen:\n%s", s)
	}
	a.Update(esc)
	openStats(t, a)
	if s := screen(a); !strings.Contains(s, "bbbbbbb") {
		t.Errorf("a new visit must read the runs again:\n%s", s)
	}
}

func TestGitLogScrollsBeyondWindowHeight(t *testing.T) {
	a, st, clk := statsApp(t, 60, 18)
	const n = 50
	for i := range n { // i = 0 is the newest
		addRun(t, st, store.Run{ID: fmt.Sprintf("run%04d", i), At: daysAgo(clk, 0).Add(-time.Duration(i) * time.Minute),
			Mode: store.ModeTraining, WPM: 30, Accuracy: 0.95})
	}
	openStats(t, a)
	selected := func() string { return nthLine(screen(a), ">", 0) }

	if s := selected(); !strings.Contains(s, "run0000") {
		t.Fatalf("the newest run must be selected first: %q", s)
	}
	for range 30 {
		a.Update(down)
	}
	if s := selected(); !strings.Contains(s, "run0030") {
		t.Fatalf("after 30 downs the selection is %q, want run0030:\n%s", s, screen(a))
	}
	a.Update(up)
	if s := selected(); !strings.Contains(s, "run0029") {
		t.Errorf("after Up the selection is %q, want run0029", s)
	}
	a.Update(pgDown)
	first := selected()
	if first == "" || strings.Contains(first, "run0029") {
		t.Errorf("PgDn did not move the selection: %q", first)
	}
	for range 10 {
		a.Update(pgDown)
	}
	if s := selected(); !strings.Contains(s, "run0049") {
		t.Errorf("PgDn to the end must select the oldest run: %q\n%s", s, screen(a))
	}
	for range 10 {
		a.Update(pgUp)
	}
	if s := selected(); !strings.Contains(s, "run0000") {
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
	// A run of no lesson has no chapter: overall only.
	addRun(t, st, store.Run{ID: "drill", At: daysAgo(clk, 3), Mode: store.ModeTraining, WPM: 20, Accuracy: 0.9})
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
			t.Errorf("WPM trend of %s = %q, want %q:\n%s", label, got, want, s)
		}
		if nthLine(s, label, 1) == "" {
			t.Errorf("accuracy trend of %s is missing:\n%s", label, s)
		}
	}
	// Overall: the day without a lesson and the three lesson days.
	if got := glyphs(nthLine(s, text.StatsGroupOverall, 0)); len([]rune(got)) != 4 {
		t.Errorf("overall WPM trend = %q, want 4 days:\n%s", got, s)
	}
	if got := glyphs(nthLine(s, text.StatsGroupOverall, 1)); len([]rune(got)) != 4 {
		t.Errorf("overall accuracy trend = %q, want 4 days:\n%s", got, s)
	}
	if !strings.Contains(nthLine(s, text.StatsGroupDiacritics, 0), "30") {
		t.Errorf("the latest WPM should be shown: %q", nthLine(s, text.StatsGroupDiacritics, 0))
	}
}

func TestTrendsUseDailyMediansOfTheLast30ActiveDays(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	// 35 active days; the sparkline keeps the newest 30. A day of three runs
	// with 10, 20 and 90 WPM has the median 20.
	for d := range 35 {
		addRun(t, st, store.Run{ID: fmt.Sprintf("d%02d", d), At: daysAgo(clk, d*2), Mode: store.ModeTraining, WPM: 50, Accuracy: 0.95})
	}
	for i, w := range []float64{10, 90, 20} {
		addRun(t, st, store.Run{ID: fmt.Sprintf("m%d", i), At: daysAgo(clk, 1).Add(time.Duration(i) * time.Minute), Mode: store.ModeTraining, WPM: w, Accuracy: 0.95})
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

func TestEmptyHistoryShowsFriendlyMessage(t *testing.T) {
	a, _, _ := statsApp(t, 80, 24)
	openStats(t, a)
	for i, name := range []string{"git log", "heatmap", "trendy", "rekordy"} {
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

func TestPersonalBestIsOfTheTests(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	id, _ := lessonOf(t, a, 1)
	add := func(r store.Run) {
		r.At = daysAgo(clk, 0)
		addRun(t, st, r)
	}
	add(store.Run{ID: "l1", Mode: store.ModeTest, LessonID: id, WPM: 50, Accuracy: 0.97, Score: 44})
	add(store.Run{ID: "l2", Mode: store.ModeTest, LessonID: id, WPM: 55, Accuracy: 0.96, Score: 46.7}) // best
	add(store.Run{ID: "l3", Mode: store.ModeTest, LessonID: id, WPM: 90, Accuracy: 0.99, Score: 86, Paused: true})
	add(store.Run{ID: "t1", Mode: store.ModeTraining, LessonID: id, Length: "short", WPM: 80, Accuracy: 0.99, Score: 79})
	add(store.Run{ID: "p1", Mode: store.ModePlacement, WPM: 70, Accuracy: 0.99, Score: 69})
	openStats(t, a)
	for range 3 {
		a.Update(tab)
	}
	s := screen(a)
	if l := nthLine(s, text.StatsModeTest, 0); !strings.Contains(l, "55 wpm  96.0 %") {
		t.Errorf("record of the tests = %q:\n%s", l, s)
	}
	for _, not := range []string{"90 wpm", "80 wpm", "70 wpm"} {
		if strings.Contains(s, not) {
			t.Errorf("%s is a paused run or no Test, and counted as a record:\n%s", not, s)
		}
	}
}

func TestTabsSwitchWithTabAndArrowsAndEscGoesToTheList(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: store.ModeTraining, WPM: 30, Accuracy: 0.95})
	openStats(t, a)
	bracket := regexp.MustCompile(`\[[^\]]+\]`)
	marker := func() string { // the active tab is bracketed
		for _, l := range strings.Split(screen(a), "\n") {
			if strings.Contains(l, "rekordy") {
				return bracket.FindString(l)
			}
		}
		return ""
	}
	if m := marker(); !strings.Contains(m, "[git log]") {
		t.Fatalf("active tab = %q, want [git log]", m)
	}
	a.Update(right)
	if m := marker(); !strings.Contains(m, "[heatmap]") {
		t.Errorf("after Right: %q", m)
	}
	a.Update(tab)
	if m := marker(); !strings.Contains(m, "[trendy]") {
		t.Errorf("after Tab: %q", m)
	}
	a.Update(left)
	a.Update(left)
	if m := marker(); !strings.Contains(m, "[git log]") {
		t.Errorf("after two Left: %q", m)
	}
	a.Update(left)
	if m := marker(); !strings.Contains(m, "[rekordy]") {
		t.Errorf("Left from the first tab must wrap: %q", m)
	}
	a.Update(esc)
	wantScreen(t, a, "lessons")
}

func TestGitLogNamesTheModeAndAlignsAccuracy(t *testing.T) {
	a, st, clk := statsApp(t, 100, 30)
	addRun(t, st, store.Run{ID: "aaaaaaa", At: daysAgo(clk, 0), Mode: store.ModeTest, WPM: 70, Accuracy: 1, Stars: 3})
	addRun(t, st, store.Run{ID: "bbbbbbb", At: daysAgo(clk, 0).Add(-time.Minute), Mode: store.ModeTraining, WPM: 5, Accuracy: 0.5})
	addRun(t, st, store.Run{ID: "ccccccc", At: daysAgo(clk, 0).Add(-2 * time.Minute), Mode: store.ModePlacement, WPM: 30, Accuracy: 0.9})
	openStats(t, a)
	s := screen(a)
	stars := func(id string) int { // the offset of the stars from the hash
		for _, l := range strings.Split(s, "\n") {
			if i := strings.Index(l, id); i >= 0 {
				return strings.IndexAny(l, "★☆") - i
			}
		}
		return -1
	}
	if stars("aaaaaaa") != stars("bbbbbbb") || stars("bbbbbbb") != stars("ccccccc") {
		t.Errorf("the stars column is not aligned (100.0 %% pushes it):\n%s", s)
	}
	for id, want := range map[string]string{
		"aaaaaaa": text.StatsModeTest, "bbbbbbb": text.StatsModeTraining, "ccccccc": text.StatsModePlacement,
	} {
		if l := nthLine(s, id, 0); !strings.Contains(l, want) {
			if l = nthLine(s, "> "+id, 0); !strings.Contains(l, want) {
				t.Errorf("line of %s lacks the mode %q: %q", id, want, l)
			}
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
