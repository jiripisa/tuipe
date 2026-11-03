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
	figureLabel = 10 // width of the labels of the figures on the results
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
		LessonID: ls.ID, Lang: ls.Lang, Length: n.String(), Session: sessionCareer, Text: txt,
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
		LessonID: ls.ID, Lang: ls.Lang, Session: sessionCareer, Text: txt, Targets: targets, HintsOff: true,
	}, nil
}

// isLessonPart reports a Trénink or a Test, the rounds of the course. They
// leave the progress of the old curriculum alone.
func isLessonPart(k play.Kind) bool { return k == play.KindTraining || k == play.KindTest }

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
	r.begin(a)
	return r, nil
}

// partResults shows a finished Trénink or Test against the last and best
// attempts at the same part.
type partResults struct {
	part       lessonPart
	title      string
	run        store.Run
	last, best *store.Run // nil when there is none to compare with
	stars      bool       // a Test shows its stars
	missed     []rune
	rank       string // the promotion line, when the rank rose
	note       string
}

// newPartResults records the finished round with Finish and shows it. The
// earlier attempts and the rank are read first, so this run is not among them.
func newPartResults(a *App, part lessonPart, spec play.Spec, res play.Result) Screen {
	recs := a.recordsOf()[spec.LessonID]
	key := partKey(modeName(spec), spec.Length)
	stars := a.Store.Profile().Stars
	before := a.Course.Rank(stars)
	run, _ := a.Finish(spec, res) // Finish has shown any save error
	s := &partResults{
		part: part, title: spec.Title, run: run, last: recs.Last[key], best: recs.Best[key],
		stars: spec.Kind == play.KindTest, missed: res.Missed(missedShown),
	}
	if after := a.Course.Rank(stars); after > before {
		s.rank = fmt.Sprintf(text.ResultsRank, before, after)
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

// What goes when the window is short, first to last. The speed, the accuracy
// and the stars never go.
const (
	goesGap = iota
	goesBest
	goesLast
	goesNote
	goesMissed
	goesTime
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

// compareLine says how the run differs from an earlier one, from the figures as
// they are shown, so that the difference agrees with them.
func compareLine(run store.Run, was *store.Run, against string) string {
	accuracy := func(r store.Run) float64 { return math.Round(r.Accuracy*1000) / 10 }
	cpm := math.Round(score.CPM(run.WPM)) - math.Round(score.CPM(was.WPM))
	return text.PartCompare(cpm, accuracy(run)-accuracy(*was), against)
}

func (s *partResults) View(a *App, w, h int) string {
	st := a.Style
	paint := func(col color.Color, t string) string {
		return st.Base().Foreground(col).Render(ansi.Truncate(t, max(w-3, 1), "…"))
	}
	figure := func(label, value string, col color.Color) string {
		return paint(st.Dim, fmt.Sprintf("%-*s", figureLabel, label)) + paint(col, value)
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

	figures := []partLine{
		{figure(text.PartSpeed, speedText(&s.run), st.Fg), goesNever},
		{figure(text.PartAccuracy, text.PartPercent(s.run.Accuracy), st.Fg), goesNever},
		{figure(text.PartTime, text.PartClock(int(math.Round(float64(s.run.DurationMs)/1000))), st.Fg), goesTime},
	}
	if s.stars {
		line := starBar(s.run.Stars)
		if s.run.Stars == 0 {
			line += "  " + text.ResultsNoStars
		}
		figures = append(figures, partLine{figure(text.PartStars, line, st.Accent), goesNever})
	}
	block(figures...)

	var compare []partLine
	if s.last != nil {
		compare = append(compare, partLine{paint(st.Fg, compareLine(s.run, s.last, text.PartVsLast)), goesLast})
	}
	if s.best != nil {
		compare = append(compare, partLine{paint(st.Fg, compareLine(s.run, s.best, text.PartVsBest)), goesBest})
	}
	block(compare...)

	if len(s.missed) > 0 {
		block(partLine{paint(st.Fg, text.PartMissedList(s.missed)), goesMissed})
	}

	var notes []partLine
	if s.rank != "" {
		notes = append(notes, partLine{paint(st.Accent, s.rank), goesRank})
	}
	if s.run.Paused {
		notes = append(notes, partLine{paint(st.Dim, text.ResultsPaused), goesNote})
	}
	if s.note != "" {
		notes = append(notes, partLine{paint(st.Dim, s.note), goesNote})
	}
	block(notes...)

	body := fitLines(lines, h-3)
	return style.Frame(st, w, h, s.title, strings.Join(body, "\n"), text.HintPartResults)
}
