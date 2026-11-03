package ui

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/course"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const (
	missedShown = 3  // characters the results name as missed most
	figureLabel = 13 // width of the labels of the figures on the results
	figureCell  = 11 // width of a column of the results table
)

// trainingSpec is a Trénink of a lesson on new text. It has no targets: stars
// come from a Test only.
func (a *App) trainingSpec(lessonID string, n course.Length) (play.Spec, error) {
	txt, err := a.Course.Training(lessonID, n, a.Now().UnixNano())
	if err != nil {
		return play.Spec{}, err
	}
	ls, _ := a.Course.Lesson(lessonID) // Training has found it
	return play.Spec{
		Kind: play.KindTraining, Title: text.PartTitle(text.LessonPartTraining, text.LessonLengths[n], ls.Title),
		LessonID: ls.ID, Lang: ls.Lang, Length: n.String(), Text: txt,
	}, nil
}

// testSpec is a Test of a lesson on new text, without hints.
func (a *App) testSpec(lessonID string) (play.Spec, error) {
	txt, targets, err := a.Course.Test(lessonID, a.Now().UnixNano())
	if err != nil {
		return play.Spec{}, err
	}
	ls, _ := a.Course.Lesson(lessonID) // Test has found it
	return play.Spec{
		Kind: play.KindTest, Title: text.PartTitle(text.LessonPartTest, ls.Title),
		LessonID: ls.ID, Lang: ls.Lang, Text: txt, Targets: targets, HintsOff: true,
	}, nil
}

// lessonPart is the Trénink or the Test of a lesson, ready to be played.
type lessonPart struct {
	spec func(*App) (play.Spec, error) // new text on every call
	back func(*App) Screen             // where the part ends: the lesson screen
}

// round starts the part on new text.
func (p lessonPart) round(a *App) (Screen, error) {
	spec, err := p.spec(a)
	if err != nil {
		return nil, err
	}
	r := &roundScreen{spec: spec, ctl: &roundControl{
		done:  func(a *App, res play.Result, _ time.Time) Screen { return newPartResults(a, p, spec, res) },
		leave: p.back,
	}}
	if spec.Kind == play.KindTest { // a Trénink does not judge
		best := a.recordsOf()[spec.LessonID].Grade
		r.stars = &best
	}
	r.begin(a)
	return r, nil
}

// partResults shows a finished Trénink or Test against the last and best
// attempts at the same part.
type partResults struct {
	part       lessonPart
	title      string
	run        store.Run
	last, best *store.Run   // nil when there is none to compare with
	grade      *score.Grade // what a Test earned; nil for a Trénink and a paused Test, which never counts
	missed     []rune
	rank       string // the promotion line, when the rank rose
	note       string
}

// newPartResults records the finished round with Finish and shows it. The
// earlier attempts are read first, so this run is not among them; the rank
// after it is that of the same history with this run added.
func newPartResults(a *App, part lessonPart, spec play.Spec, res play.Result) Screen {
	runs, _ := a.Store.Runs() // a history that cannot be read must not break the results
	before := lessonRecords(runs, a.Course.Targets)
	recs := before[spec.LessonID]
	key := partKey(modeName(spec), spec.Length)
	run, _ := a.Finish(spec, res) // Finish has shown any save error
	s := &partResults{
		part: part, title: spec.Title, run: run, last: recs.Last[key], best: recs.Best[key],
		missed: res.Missed(missedShown),
	}
	if spec.Kind == play.KindTest && !run.Paused {
		g := gradeOf(&run, a.Course.Targets(spec.LessonID))
		s.grade = &g
	}
	after := lessonRecords(append(runs, run), a.Course.Targets)
	if rank := a.Course.Rank(gradesOf(after)); rank > a.Course.Rank(gradesOf(before)) {
		s.rank = fmt.Sprintf(text.ResultsRank, rank)
	}
	return s
}

func (s *partResults) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	s.note = ""
	switch k.String() {
	case "enter", "esc":
		return s.part.back(a), nil
	case "r", "R", "ctrl+r":
		round, err := s.part.round(a)
		if err != nil {
			s.note = text.PartNotReady
			return s, nil
		}
		return round, nil
	}
	return s, nil
}

// What goes when the window is short, first to last. The head of the table and
// the rows of speed and accuracy, which end with the stars of a Test, never go.
const (
	goesGap = iota
	goesArrows
	goesNote
	goesMissed
	goesTime
	goesVerdict
	goesRank
	goesNever
)

// partLine is one row of the results and when it goes.
type partLine struct {
	text string
	goes int
}

// fitLines drops what does not fit into rows.
func fitLines(lines []partLine, rows int) []string {
	for goes := goesGap; goes < goesNever && len(lines) > rows; goes++ {
		n := len(lines)
		lines = slices.DeleteFunc(lines, func(l partLine) bool { return l.goes == goes })
		if goes != goesGap && len(lines) != n {
			style.Clipped("part results: dropped rows")
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.text
	}
	return out
}

// The figures as they are shown, so that every difference agrees with them.
func shownCPM(r *store.Run) float64      { return math.Round(score.CPM(r.WPM)) }
func shownAccuracy(r *store.Run) float64 { return math.Round(r.Accuracy*1000) / 10 }

// column is one earlier attempt the results compare with.
type column struct {
	head string
	run  *store.Run
}

// columns are the earlier attempts there are: minule, then nejlepší.
func (s *partResults) columns() []column {
	var cols []column
	if s.last != nil {
		cols = append(cols, column{text.PartLast, s.last})
	}
	if s.best != nil {
		cols = append(cols, column{text.PartBest, s.best})
	}
	return cols
}

// verdict is the line above the table: a new record of speed, or how the speed
// compares with the last attempt. A paused round never sets a record.
func (s *partResults) verdict(st style.Style) (string, color.Color) {
	now := shownCPM(&s.run)
	switch {
	case s.best != nil && !s.run.Paused && now > shownCPM(s.best):
		return text.PartRecord, st.Accent
	case s.last == nil:
		return "", nil
	case now > shownCPM(s.last):
		return text.PartFaster, st.OK
	case now < shownCPM(s.last):
		return text.PartSlower, st.Bad
	}
	return text.PartSame, st.Dim
}

func (s *partResults) View(a *App, w, h int) string {
	st := a.Style
	room := max(w-3, 1)
	fit := func(t string) string { return ansi.Truncate(t, room, "…") }
	paint := func(col color.Color, t string) string { return st.Base().Foreground(col).Render(t) }
	pad := func(t string, n int) string { return t + strings.Repeat(" ", max(n-ansi.StringWidth(t), 1)) }
	cols := s.columns()

	// cells lays out one line of the table: the label, this round, then the
	// earlier attempts, each in its own column.
	cells := func(label string, now string, earlier []string, colour []color.Color, unit string) string {
		line := paint(st.Dim, pad(label, figureLabel))
		cells := append([]string{now}, earlier...)
		for i, c := range cells {
			if i < len(cells)-1 || unit != "" {
				c = pad(c, figureCell)
			}
			ink := st.Fg
			if i == 0 {
				line += st.Base().Foreground(ink).Bold(true).Render(c)
				continue
			}
			if colour != nil {
				ink = colour[i-1]
			}
			line += paint(ink, c)
		}
		if unit != "" {
			line += paint(st.Dim, unit)
		}
		return line
	}
	row := func(label string, now string, earlier []string, colour []color.Color, unit string) string {
		return fit(cells(label, now, earlier, colour, unit))
	}
	// arrows is the row under a figure: how this round beats each earlier one.
	arrows := func(delta func(*store.Run) float64, percent bool) partLine {
		var cells []string
		var ink []color.Color
		for _, c := range cols {
			d := delta(c.run)
			cells = append(cells, text.PartDelta(d, percent))
			switch {
			case d > 0:
				ink = append(ink, st.OK)
			case d < 0:
				ink = append(ink, st.Bad)
			default:
				ink = append(ink, st.Dim)
			}
		}
		return partLine{row("", "", cells, ink, ""), goesArrows}
	}
	earlier := func(f func(*store.Run) string) []string {
		var out []string
		for _, c := range cols {
			out = append(out, f(c.run))
		}
		return out
	}

	var lines []partLine
	block := func(rows ...partLine) {
		if len(rows) == 0 {
			return
		}
		if len(lines) > 0 {
			lines = append(lines, partLine{goes: goesGap})
		}
		lines = append(lines, rows...)
	}

	if v, ink := s.verdict(st); v != "" {
		block(partLine{fit(paint(ink, v)), goesVerdict})
	}

	cpm := func(r *store.Run) string { return fmt.Sprintf("%.0f", shownCPM(r)) }
	accuracy := func(r *store.Run) string { return text.PartPercent(r.Accuracy) }
	// figures lays out the head and the rows of speed and accuracy. The unit of
	// speed follows the speeds, or with unitInHead stands in the head instead.
	var head, speed, accurate string
	figures := func(unitInHead bool) {
		unit, headLabel := text.PartCPMUnit, ""
		if unitInHead {
			unit, headLabel = "", text.PartCPMUnit
		}
		if len(cols) > 0 {
			heads := earlier(func(*store.Run) string { return "" })
			for i, c := range cols {
				heads[i] = c.head
			}
			head = cells(headLabel, text.PartNow, heads, nil, "")
			speed = cells(text.PartSpeed, cpm(&s.run), earlier(cpm), nil, unit)
		} else {
			speed = cells(text.PartSpeed, speedText(&s.run), nil, nil, "")
		}
		accurate = cells(text.PartAccuracy, accuracy(&s.run), earlier(accuracy), nil, "")
	}
	figures(false)
	// A Test ends the rows of speed and accuracy with the stars it earned there,
	// in one column. When they would not fit, the unit gives them its room.
	if g := s.grade; g != nil {
		speedStars, accuracyStars := starBar(g.Speed), starBar(g.Accuracy)
		if g.Accuracy == maxStars {
			accuracyStars += " " + text.ResultsFlawless
		}
		at := func() int { return max(ansi.StringWidth(speed), ansi.StringWidth(accurate)) + 2 }
		if len(cols) > 0 && at()+ansi.StringWidth(accuracyStars) > room {
			figures(true)
		}
		col := at()
		speed = pad(speed, col) + paint(st.Accent, speedStars)
		accurate = pad(accurate, col) + paint(st.Accent, accuracyStars)
	}

	var table []partLine
	if len(cols) > 0 {
		// The head is dim, as labels are.
		table = append(table, partLine{fit(paint(st.Dim, ansi.Strip(head))), goesNever})
	}
	table = append(table, partLine{fit(speed), goesNever})
	if len(cols) > 0 {
		table = append(table, arrows(func(r *store.Run) float64 { return shownCPM(&s.run) - shownCPM(r) }, false))
	}
	// A gap after each group of the table, so that the rows do not crowd.
	table = append(table, partLine{goes: goesGap})
	table = append(table, partLine{fit(accurate), goesNever})
	if len(cols) > 0 {
		table = append(table, arrows(func(r *store.Run) float64 { return shownAccuracy(&s.run) - shownAccuracy(r) }, true))
	}
	table = append(table, partLine{goes: goesGap})
	table = append(table, partLine{row(text.PartTime, text.PartClock(int(math.Round(float64(s.run.DurationMs)/1000))), nil, nil, ""), goesTime})
	block(table...)

	if len(s.missed) > 0 {
		block(partLine{fit(paint(st.Dim, pad(text.PartMissed, figureLabel)) + paint(st.Fg, text.PartMissedList(s.missed))), goesMissed})
	}

	var notes []partLine
	if s.rank != "" {
		notes = append(notes, partLine{fit(paint(st.Accent, s.rank)), goesRank})
	}
	if s.run.Paused {
		notes = append(notes, partLine{fit(paint(st.Dim, text.ResultsPaused)), goesNote})
	}
	if s.note != "" {
		notes = append(notes, partLine{fit(paint(st.Dim, s.note)), goesNote})
	}
	block(notes...)

	body := fitLines(lines, h-3)
	return style.Frame(st, w, h, s.title, strings.Join(body, "\n"), text.HintPartResults)
}
