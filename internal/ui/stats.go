package ui

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/course"
	"github.com/jiripisa/tuipe/internal/hint"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/streak"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// The tabs of the statistics screen, in the order of text.StatsTabs.
const (
	tabHistory = iota
	tabHeatmap
	tabTrends
	tabRecords
)

// historyFilter picks which runs the history lists. The values index
// text.StatsFilters.
type historyFilter int

const (
	filterAll historyFilter = iota
	filterTraining
	filterTest
	filterCount
)

const (
	trendDays      = 30 // active days in a trend
	weakestShown   = 8
	sparkLevels    = "▁▂▃▄▅▆▇█"
	statsChrome    = 4 // frame, tab bar and footer rows around the content
	trendLabelCol  = 11
	recordLabelCol = 26 // the widest record label and a gap
)

// trendGroups are the character groups of the trends, in display order. The
// first is every run.
var trendGroups = []string{
	text.StatsGroupOverall, text.StatsGroupLetters, text.StatsGroupDiacritics,
	text.StatsGroupDigits, text.StatsGroupShift, text.StatsGroupOption, text.StatsGroupCode,
}

// statsScreen shows the history, the keyboard map, the trends and the records.
// The runs are read once when the screen opens.
type statsScreen struct {
	from    string // the lesson selected on the list, to return to
	tab     int
	filter  historyFilter
	runs    []store.Run // newest first
	shown   []store.Run // the runs the filter lets through, newest first
	tests   []store.Run // the Tests, newest first: trends and records read these only
	sel     int         // index into shown
	days    streak.Status
	groupOf map[string]string // lesson id -> trend group
	titles  map[string]string // lesson id -> title
}

// NewStatsScreen opens the statistics on the history.
func NewStatsScreen(a *App) Screen { return newStatsScreen(a, "") }

// newStatsScreen is NewStatsScreen that returns to lesson from on the list.
func newStatsScreen(a *App, from string) Screen {
	runs, _ := a.Store.Runs() // a damaged history shows as an empty one
	runs = slices.Clone(runs)
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].At.After(runs[j].At) })
	s := &statsScreen{from: from, runs: runs, days: daysInARow(runs, a.Now()), groupOf: a.lessonGroups(),
		titles: map[string]string{}, tests: runsFor(runs, filterTest)}
	for _, l := range a.Course.Lessons() {
		s.titles[l.ID] = l.Title
	}
	s.shown = runsFor(runs, s.filter)
	return s
}

// runsFor keeps the runs the filter lets through: everything, the Tréninky or
// the Tests. Only the first lists the placement test.
func runsFor(runs []store.Run, filter historyFilter) []store.Run {
	var mode string
	switch filter {
	case filterTraining:
		mode = store.ModeTraining
	case filterTest:
		mode = store.ModeTest
	default:
		return slices.Clone(runs)
	}
	var out []store.Run
	for _, r := range runs {
		if r.Mode == mode {
			out = append(out, r)
		}
	}
	return out
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
		return newLessonsScreen(a, s.from), nil
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
	case "f":
		if s.tab == tabHistory {
			s.filter = (s.filter + 1) % filterCount
			s.shown = runsFor(s.runs, s.filter)
			s.sel = 0
		}
	}
	return s, nil
}

func (s *statsScreen) move(d int) {
	if s.tab == tabHistory {
		s.sel = max(0, min(s.sel+d, len(s.shown)-1))
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
	head := strings.Join(bar, " ") + "   " + dim(text.StatsDaysInARow(s.days.Days))

	rows := bodyRows(h)
	var body []string
	switch {
	case len(s.runs) == 0:
		body = []string{dim(text.StatsEmpty)}
	case s.tab == tabHistory:
		body = s.history(a, w-2, rows)
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
	return style.Frame(st, w, h, text.AppName+" · "+text.StatsTitle,
		strings.Join(append([]string{head}, wrapped...), "\n"), text.HintStats)
}

// modeLabel names a mode of a run.
func modeLabel(mode string) string {
	switch mode {
	case store.ModeTest:
		return text.StatsModeTest
	case store.ModeTraining:
		return text.StatsModeTraining
	case store.ModePlacement:
		return text.StatsModePlacement
	}
	return mode
}

// history lists the filtered runs newest first, one plain line each, scrolled
// so that the selected run stays in view. The first row names the filter.
func (s *statsScreen) history(a *App, w, rows int) []string {
	st := a.Style
	paint := func(col color.Color, bold bool, t string) string {
		return st.Base().Foreground(col).Bold(bold).Render(t)
	}
	out := []string{paint(st.Dim, false, text.StatsFilterLabel+": "+text.StatsFilters[s.filter])}
	if len(s.shown) == 0 {
		return append(out, paint(st.Dim, false, text.StatsNoRuns))
	}
	top, end := 0, len(s.shown)
	if room := rows - 1; room > 0 && end > room {
		top = min(max(s.sel-room/2, 0), end-room)
		end = top + room
	}
	for i := top; i < end; i++ { // only the visible window is rendered
		line := s.runLine(s.shown[i])
		if i == s.sel {
			line = paint(st.Accent, true, ansi.Truncate("> "+line, w, "…"))
		} else {
			line = paint(st.Fg, false, ansi.Truncate("  "+line, w, "…"))
		}
		out = append(out, line)
	}
	return out
}

// Widths of the columns of a history line: the date, the part and the speed.
const (
	dateCol  = 12
	partCol  = 12
	speedCol = 11
)

// runLine is a run as a plain line: date, part, speed, accuracy, stars, and
// last what was typed: the length of a Trénink and the lesson.
func (s *statsScreen) runLine(r store.Run) string {
	line := fmt.Sprintf("%-*s  %-*s  %*s  %7s  %s", dateCol, r.At.Local().Format("2. 1. 2006"), partCol, modeLabel(r.Mode),
		speedCol, text.LessonSpeed(score.CPM(r.WPM)), text.PartPercent(r.Accuracy), starBar(r.Stars))
	var what []string
	if r.Mode == store.ModeTraining {
		for i, name := range text.LessonLengths {
			if course.Length(i).String() == r.Length {
				what = append(what, name)
			}
		}
	}
	if title := s.titles[r.LessonID]; title != "" {
		what = append(what, title)
	}
	if len(what) == 0 {
		return line
	}
	return line + "  " + text.PartTitle(what...)
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

// lessonGroups maps every lesson to the trend group of its chapter. Code has a
// group of its own, and so has all that follows it: it uses the whole keyboard.
func (a *App) lessonGroups() map[string]string {
	groups := map[string]string{}
	code := false
	for _, ch := range a.Course.Chapters() {
		var taught []rune
		for _, l := range ch.Lessons {
			code = code || l.Lang != ""
			if !l.Review {
				taught = append(taught, l.Focus...)
			}
		}
		g := a.chapterGroup(taught)
		if code {
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

// chapterGroup is the trend group that most of the characters a chapter teaches
// belong to; a chapter that teaches none belongs to no group. A tie goes to the
// group listed first below.
func (a *App) chapterGroup(taught []rune) string {
	count := map[string]int{}
	for _, r := range taught {
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
type series struct{ speed, acc []float64 }

// trendSeries gathers the daily medians per group over the last 30 days with a
// Test. Speed is in characters per minute and accuracy a share of 1.
func (s *statsScreen) trendSeries() map[string]series {
	type day struct{ speed, acc []float64 }
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
		d.speed = append(d.speed, score.CPM(r.WPM))
		d.acc = append(d.acc, r.Accuracy)
	}
	for _, r := range s.tests { // trends read the Tests only
		date := r.At.Local().Format(dateLayout)
		add(text.StatsGroupOverall, date, r)
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
			if m, ok := median(byDate[d].speed); ok {
				sr.speed = append(sr.speed, m)
			}
			if m, ok := median(byDate[d].acc); ok {
				sr.acc = append(sr.acc, m)
			}
		}
		sr.speed = tail(sr.speed, trendDays)
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

// trends shows speed and accuracy over time as sparklines, overall and per group.
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
		last  func(float64) string
	}{
		{text.StatsSpeed, func(x series) []float64 { return x.speed }, text.LessonSpeed},
		{text.StatsAccuracy, func(x series) []float64 { return x.acc }, text.PartPercent},
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
				line+sparkline(v)+"  "+section.last(v[len(v)-1])))
		}
	}
	return out
}

// records lists the three plain records: the best speed and the best accuracy
// of the Tests, and the most days in a row. Paused runs do not count.
func (s *statsScreen) records(a *App) []string {
	st := a.Style
	var speed, accuracy float64
	for _, r := range s.tests {
		if !r.Paused {
			speed = max(speed, score.CPM(r.WPM))
			accuracy = max(accuracy, r.Accuracy)
		}
	}
	show := func(ok bool, v string) string {
		if !ok {
			return text.StatsNoRecord
		}
		return v
	}
	var out []string
	for _, row := range [][2]string{
		{text.StatsRecordSpeed, show(speed > 0, text.LessonSpeed(speed))},
		{text.StatsRecordAccuracy, show(accuracy > 0, text.PartPercent(accuracy))},
		{text.StatsRecordDays, show(s.days.Best > 0, strconv.Itoa(s.days.Best))},
	} {
		out = append(out, st.Base().Foreground(st.Fg).Render(fmt.Sprintf("%-*s  %s", recordLabelCol, row[0], row[1])))
	}
	return out
}
