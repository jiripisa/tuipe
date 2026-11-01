package ui

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/hint"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// The tabs of the statistics screen, in the order of text.StatsTabs.
const (
	tabGitLog = iota
	tabHeatmap
	tabTrends
	tabRecords
)

const (
	trendDays     = 30 // active days in a trend
	weakestShown  = 8
	sparkLevels   = "▁▂▃▄▅▆▇█"
	statsChrome   = 4 // frame, tab bar and footer rows around the content
	trendLabelCol = 11
)

// trendGroups are the character groups of the trends, in display order. The
// first is every run.
var trendGroups = []string{
	text.StatsGroupOverall, text.StatsGroupLetters, text.StatsGroupDiacritics,
	text.StatsGroupDigits, text.StatsGroupShift, text.StatsGroupOption, text.StatsGroupCode,
}

// statsScreen shows the history: git log, heatmap, trends and personal bests.
// The runs are read once when the screen opens.
type statsScreen struct {
	tab     int
	runs    []store.Run // newest first
	sel     int         // index into runs
	up      string
	groupOf map[string]string // lesson id -> trend group
	titles  map[string]string // lesson id -> title
	log     []logRow          // the git log: day headers and runs
	logLine map[int]int       // run index -> index of its row in log
}

// logRow is one line of the git log: a day header (run < 0) or a run.
type logRow struct {
	day string
	run int
}

// NewStatsScreen opens the statistics on the git log.
func NewStatsScreen(a *App) Screen {
	runs, _ := a.Store.Runs() // a damaged history shows as an empty one
	runs = slices.Clone(runs)
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].At.After(runs[j].At) })
	up := a.uptime()
	s := &statsScreen{runs: runs, up: uptimeText(up.Days), groupOf: a.lessonGroups(),
		titles: map[string]string{}, logLine: map[int]int{}}
	for _, ch := range a.Curr.Chapters() {
		for _, l := range ch.Lessons {
			s.titles[l.ID] = l.Title
		}
	}
	day := ""
	for i, r := range runs {
		if d := r.At.Local().Format(dateLayout); d != day {
			day = d
			s.log = append(s.log, logRow{day: d, run: -1})
		}
		s.logLine[i] = len(s.log)
		s.log = append(s.log, logRow{run: i})
	}
	return s
}

// bodyRows is the number of lines the screen leaves to its content in a window h high.
func bodyRows(h int) int { return max(0, h-statsChrome) }

func (s *statsScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	n := len(text.StatsTabs)
	page := max(1, bodyRows(a.H))
	switch k.String() {
	case "esc":
		return NewHomeScreen(a), nil
	case "tab", "right", "l":
		s.tab = (s.tab + 1) % n
	case "shift+tab", "left", "h":
		s.tab = (s.tab + n - 1) % n
	case "up", "k":
		s.move(-1)
	case "down", "j":
		s.move(1)
	case "pgup":
		s.move(-page)
	case "pgdown":
		s.move(page)
	}
	return s, nil
}

func (s *statsScreen) move(d int) {
	if s.tab == tabGitLog {
		s.sel = max(0, min(s.sel+d, len(s.runs)-1))
	}
}

func (s *statsScreen) View(a *App, w, h int) string {
	st := a.Style
	dim := func(t string) string { return st.Base().Foreground(st.Dim).Render(t) }

	var bar []string
	for i, name := range text.StatsTabs {
		switch {
		case i == s.tab:
			bar = append(bar, st.Base().Foreground(st.Accent).Bold(true).Render("["+name+"]"))
		default:
			bar = append(bar, dim(" "+name+" "))
		}
	}
	head := strings.Join(bar, " ") + "   " + dim(text.HomeUptime+" "+s.up)

	rows := bodyRows(h)
	var body []string
	switch {
	case len(s.runs) == 0:
		body = []string{dim(text.StatsEmpty)}
	case s.tab == tabGitLog:
		body = s.gitLog(a, rows)
	case s.tab == tabHeatmap:
		body = s.heatmap(a, w-2, rows)
	case s.tab == tabTrends:
		body = s.trends(a, rows)
	default:
		body = s.records(a)
	}
	var wrapped []string // a message longer than the window wraps instead of being cut
	for _, l := range body {
		if ansi.StringWidth(l) > w-2 {
			l = ansi.Wordwrap(l, max(w-2, 1), "")
		}
		wrapped = append(wrapped, l)
	}
	return style.Frame(st, w, h, text.AppName+" · "+text.MenuStats,
		strings.Join(append([]string{head}, wrapped...), "\n"), text.HintStats)
}

// modeLabel names a non-lesson mode.
func modeLabel(mode string) string {
	switch mode {
	case "lesson":
		return text.StatsModeLesson
	case "code":
		return text.StatsModeCode
	case "incident":
		return text.StatsModeIncident
	case "drill":
		return text.StatsModeDrill
	case "practice":
		return text.StatsModePractice
	case "testout":
		return text.StatsModeTestOut
	case "placement":
		return text.StatsModePlacement
	}
	return mode
}

// gitLog lists the runs newest first, one line each under a dim header per
// day, scrolled so that the selected run stays in view.
func (s *statsScreen) gitLog(a *App, rows int) []string {
	st := a.Style
	paint := func(col color.Color, bold bool, t string) string {
		return st.Base().Foreground(col).Bold(bold).Render(t)
	}
	top, end := 0, len(s.log)
	if rows > 0 && end > rows {
		top = min(max(s.logLine[s.sel]-rows/2, 0), end-rows)
		end = top + rows
	}
	var lines []string
	for _, row := range s.log[top:end] { // only the visible window is rendered
		if row.run < 0 {
			lines = append(lines, paint(st.Dim, false, row.day))
			continue
		}
		r := s.runs[row.run]
		line := s.runLine(r)
		if row.run == s.sel {
			line = paint(st.Accent, true, "> "+line)
		} else {
			line = paint(st.Fg, false, "  "+line)
		}
		if lostIncident(r) {
			line += "  " + paint(st.Bad, false, text.StatsLost)
		}
		lines = append(lines, line)
	}
	return lines
}

// runLine is a run like a git log entry: hash, WPM, accuracy, stars, the lead
// on the ghost and what was typed.
func (s *statsScreen) runLine(r store.Run) string {
	line := fmt.Sprintf("%s  %2.0f wpm  %-7s %s", r.ID, r.WPM, fmt.Sprintf("%.1f %%", r.Accuracy*100), starBar(r.Stars))
	if r.GhostDeltaMs != nil {
		line += fmt.Sprintf("  %+.1f s", float64(*r.GhostDeltaMs)/1000)
	}
	title := s.titles[r.LessonID]
	name := modeLabel(r.Mode)
	if l := sessionLabel(sessionOfRun(r)); l != "" {
		name += " · " + l
	}
	switch {
	case r.Mode == "lesson" && title != "":
		return line + "  " + title
	case title != "":
		return line + "  " + name + ": " + title
	}
	return line + "  " + name
}

// keyOf is the key a character ends on: the last chord of its route.
func keyOf(l *layout.Layout, r rune) (layout.Key, bool) {
	route, ok := l.Route(r)
	if !ok || len(route) == 0 {
		return layout.Key{}, false
	}
	return l.Key(route[len(route)-1].Key)
}

// heatmap draws the keyboard coloured by weakness, the weakest finger and the
// weakest characters with their hints.
func (s *statsScreen) heatmap(a *App, w, rows int) []string {
	st := a.Style
	dim := func(t string) string { return st.Base().Foreground(st.Dim).Render(t) }
	stat := a.stats()

	byKey := map[string]float64{}
	byFinger := map[[2]int]float64{}
	var top float64
	for k := range stat.Chars {
		r, _ := utf8.DecodeRuneInString(k)
		key, ok := keyOf(a.Layout, r)
		if !ok {
			continue
		}
		wk := stat.Weakness(r)
		if wk <= 0 {
			continue
		}
		byKey[key.ID] = max(byKey[key.ID], wk)
		byFinger[[2]int{int(key.Hand), int(key.Finger)}] += wk
		top = max(top, wk)
	}
	if top == 0 {
		return []string{dim(text.StatsHeatEmpty)}
	}
	heat := map[string]float64{}
	for id, wk := range byKey {
		heat[id] = wk / top
	}

	out := strings.Split(style.Keyboard(st, a.Layout, nil, heat, w), "\n")
	out = append(out, st.Base().Foreground(st.Fg).Render(fmt.Sprintf(text.StatsWeakestFinger, weakestFinger(byFinger))))
	hasRoute := func(r rune) bool { return hint.Text(a.Layout, r) != "" }
	for _, r := range stat.WeakestChars(min(weakestShown, max(0, rows-len(out))), hasRoute) {
		out = append(out, st.Base().Foreground(st.Fg).Render(hint.Text(a.Layout, r)))
	}
	return out
}

// weakestFinger names the finger with the largest summed weakness; a tie goes
// to the earlier hand and finger.
func weakestFinger(sums map[[2]int]float64) string {
	best, bestSum := [2]int{}, -1.0
	for hand := range text.StatsHandNames {
		for finger := range text.StatsFingerNames {
			if sum := sums[[2]int{hand, finger}]; sum > bestSum {
				best, bestSum = [2]int{hand, finger}, sum
			}
		}
	}
	name := text.StatsFingerNames[best[1]]
	if layout.Finger(best[1]) == layout.Thumb {
		return name
	}
	return text.StatsHandNames[best[0]] + " " + name
}

// lessonGroups maps every lesson to the trend group of its chapter. The code
// chapters have a group of their own: they use the whole keyboard.
func (a *App) lessonGroups() map[string]string {
	groups := map[string]string{}
	for _, ch := range a.Curr.Chapters() {
		g := a.chapterGroup(ch.Introduces)
		if ch.Number >= codeTrendChapter {
			g = text.StatsGroupCode
		}
		if g == "" {
			continue
		}
		for _, l := range ch.Lessons {
			groups[l.ID] = g
		}
	}
	return groups
}

// chapterGroup is the trend group that most of the characters a chapter
// introduces belong to; a chapter that introduces none belongs to no group.
// A tie goes to the group listed first below.
func (a *App) chapterGroup(introduces string) string {
	count := map[string]int{}
	for _, r := range introduces {
		route, ok := a.Layout.Route(r)
		if !ok {
			continue
		}
		count[charGroup(r, route)]++
	}
	best, bestN := "", 0
	for _, g := range []string{
		text.StatsGroupOption, text.StatsGroupDigits, text.StatsGroupDiacritics,
		text.StatsGroupShift, text.StatsGroupLetters,
	} {
		if count[g] > bestN {
			best, bestN = g, count[g]
		}
	}
	return best
}

// charGroup classifies one character by how it is typed.
func charGroup(r rune, route layout.Route) string {
	shifted := false
	for _, c := range route {
		for _, m := range c.Mods {
			if m == "option" || m == "altgr" {
				return text.StatsGroupOption
			}
			shifted = shifted || m == "shift"
		}
	}
	switch {
	case unicode.IsDigit(r):
		return text.StatsGroupDigits
	case unicode.Is(unicode.Latin, r) && r > unicode.MaxASCII:
		return text.StatsGroupDiacritics
	case shifted:
		return text.StatsGroupShift
	}
	return text.StatsGroupLetters
}

// series is a trend: one point per active day.
type series struct{ wpm, acc []float64 }

// trendSeries gathers the daily medians per group over the last 30 active days.
// Lost incidents are partial, so they are no WPM sample.
func (s *statsScreen) trendSeries() map[string]series {
	type day struct{ wpm, acc []float64 }
	days := map[string]map[string]*day{} // group -> date -> samples
	add := func(group, date string, r store.Run) {
		if days[group] == nil {
			days[group] = map[string]*day{}
		}
		d := days[group][date]
		if d == nil {
			d = &day{}
			days[group][date] = d
		}
		if !lostIncident(r) {
			d.wpm = append(d.wpm, r.WPM)
		}
		d.acc = append(d.acc, r.Accuracy*100)
	}
	for _, r := range s.runs {
		date := r.At.Local().Format(dateLayout)
		if r.Mode != "placement" { // a test is no everyday typing
			add(text.StatsGroupOverall, date, r)
		}
		if g := s.groupOf[r.LessonID]; g != "" {
			add(g, date, r)
		}
	}
	out := map[string]series{}
	for g, byDate := range days {
		dates := make([]string, 0, len(byDate))
		for d := range byDate {
			dates = append(dates, d)
		}
		sort.Strings(dates)
		var sr series
		for _, d := range dates {
			if m, ok := median(byDate[d].wpm); ok {
				sr.wpm = append(sr.wpm, m)
			}
			if m, ok := median(byDate[d].acc); ok {
				sr.acc = append(sr.acc, m)
			}
		}
		sr.wpm = tail(sr.wpm, trendDays)
		sr.acc = tail(sr.acc, trendDays)
		out[g] = sr
	}
	return out
}

func tail(v []float64, n int) []float64 {
	if len(v) > n {
		return v[len(v)-n:]
	}
	return v
}

func median(v []float64) (float64, bool) {
	if len(v) == 0 {
		return 0, false
	}
	c := slices.Clone(v)
	sort.Float64s(c)
	if n := len(c); n%2 == 0 {
		return (c[n/2-1] + c[n/2]) / 2, true
	}
	return c[len(c)/2], true
}

// sparkline draws values as block characters, scaled between their lowest and
// highest. Equal values sit in the middle.
func sparkline(v []float64) string {
	if len(v) == 0 {
		return ""
	}
	lo, hi := slices.Min(v), slices.Max(v)
	levels := []rune(sparkLevels)
	var b strings.Builder
	for _, x := range v {
		i := (len(levels) - 1) / 2
		if hi > lo {
			i = int(math.Round((x - lo) / (hi - lo) * float64(len(levels)-1)))
		}
		b.WriteRune(levels[i])
	}
	return b.String()
}

// trends shows WPM and accuracy over time as sparklines, overall and per group.
// Every value carries its unit, so the section titles can go when the window is
// short.
func (s *statsScreen) trends(a *App, rows int) []string {
	st := a.Style
	dim := func(t string) string { return st.Base().Foreground(st.Dim).Render(t) }
	data := s.trendSeries()
	titled := rows >= 2*(len(trendGroups)+1)
	var out []string
	for _, section := range []struct {
		title string
		pick  func(series) []float64
		last  string
	}{
		{text.StatsWPM, func(x series) []float64 { return x.wpm }, "%.0f " + text.RoundWPM},
		{text.StatsAccuracy, func(x series) []float64 { return x.acc }, "%.1f %%"},
	} {
		if titled {
			out = append(out, dim(section.title))
		}
		for _, g := range trendGroups {
			v := section.pick(data[g])
			line := fmt.Sprintf("%-*s", trendLabelCol, g)
			if len(v) == 0 {
				out = append(out, st.Base().Foreground(st.Fg).Render(line+dim("-")))
				continue
			}
			out = append(out, st.Base().Foreground(st.Fg).Render(
				line+sparkline(v)+"  "+fmt.Sprintf(section.last, v[len(v)-1])))
		}
	}
	return out
}

// sessionOfRun is how a run was started; runs from before sessions were
// recorded count as quick runs.
func sessionOfRun(r store.Run) string {
	if r.Session == "" {
		return sessionQuick
	}
	return r.Session
}

// sessionLabel names a quick run, Standup or Sprint; other sessions have no
// label of their own, since the mode says what was typed.
func sessionLabel(session string) string {
	switch session {
	case sessionQuick:
		return text.StatsSessionQuick
	case sessionStandup:
		return text.StatsSessionStandup
	case sessionSprint:
		return text.StatsSessionSprint
	}
	return ""
}

// recordRow is a personal best category: a run belongs to the one of its
// session (quick run, Standup, Sprint) and to the one of its mode (lesson,
// code, incident).
type recordRow struct {
	label string
	has   func(store.Run) bool
}

var recordRows = []recordRow{
	{text.StatsSessionQuick, func(r store.Run) bool { return sessionOfRun(r) == sessionQuick }},
	{text.StatsModeLesson, func(r store.Run) bool { return r.Mode == "lesson" }},
	{text.StatsModeCode, func(r store.Run) bool { return r.Mode == "code" }},
	{text.StatsModeIncident, func(r store.Run) bool { return r.Mode == "incident" }},
	{text.StatsSessionStandup, func(r store.Run) bool { return sessionOfRun(r) == sessionStandup }},
	{text.StatsSessionSprint, func(r store.Run) bool { return sessionOfRun(r) == sessionSprint }},
}

// records lists the best run of each category by Score. Paused runs and lost
// incidents do not count.
func (s *statsScreen) records(a *App) []string {
	st := a.Style
	var out []string
	for _, row := range recordRows {
		var best *store.Run
		for i := range s.runs {
			r := &s.runs[i]
			if r.Paused || lostIncident(*r) || !row.has(*r) {
				continue
			}
			if best == nil || r.Score > best.Score {
				best = r
			}
		}
		if best == nil {
			continue
		}
		out = append(out, st.Base().Foreground(st.Fg).Render(fmt.Sprintf("%-*s%2.0f wpm  %.1f %%  %s %.1f",
			trendLabelCol, row.label, best.WPM, best.Accuracy*100, text.StatsRecordScore, best.Score)))
	}
	if len(out) == 0 {
		return []string{st.Base().Foreground(st.Dim).Render(text.StatsEmpty)}
	}
	return out
}
