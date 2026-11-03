package ui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/course"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// The rows of the lesson screen.
const (
	rowTeach = iota
	rowTraining
	rowTest
	lessonRows
)

// lessonScreen is the inside of a lesson: what it teaches, and its three
// parts, Výuka, Trénink and Test.
type lessonScreen struct {
	id      string
	lesson  course.Lesson
	chapter string
	fingers []string // how to type each character; empty for a review or code lesson
	recs    lessonRecord
	row     int
	length  course.Length // the Trénink length; kept while the screen lives
	note    string
}

// chapterTitle is the title of a chapter of the course.
func (a *App) chapterTitle(id string) string {
	for _, ch := range a.Course.Chapters() {
		if ch.ID == id {
			return ch.Title
		}
	}
	return ""
}

// NewLessonScreen builds the screen of a lesson. An unknown lesson gives the
// lesson list.
func NewLessonScreen(a *App, lessonID string) Screen {
	ls, ok := a.Course.Lesson(lessonID)
	if !ok {
		return NewLessonsScreen(a)
	}
	s := &lessonScreen{
		id: lessonID, lesson: ls, chapter: a.chapterTitle(ls.ChapterID),
		recs: a.recordsOf()[lessonID], length: course.Medium,
	}
	if ls.Lang == "" && !ls.Review {
		if tu, err := a.Course.Tutorial(lessonID, 0); err == nil && len(tu.Explain) > 1 {
			s.fingers = tu.Explain[1:]
		}
	}
	return s
}

func (s *lessonScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	s.note = ""
	switch k.String() {
	case "up", "k":
		s.row = (s.row + lessonRows - 1) % lessonRows
	case "down", "j":
		s.row = (s.row + 1) % lessonRows
	case "left":
		if s.row == rowTraining {
			s.length = max(s.length-1, course.Short)
		}
	case "right":
		if s.row == rowTraining {
			s.length = min(s.length+1, course.Long)
		}
	case "esc":
		return newLessonsScreen(a, s.id), nil
	case "enter":
		if s.row == rowTeach {
			back := func(*App) Screen { return s }
			taught := func(*App) Screen { // Trénink is the next step
				s.row = rowTraining
				return s
			}
			return newTeachScreen(a, s.id, back, taught), nil
		}
		return s.startPart(a)
	}
	return s, nil
}

// startPart runs the Trénink at the chosen length, or the Test. Either returns
// here with its result on the screen.
func (s *lessonScreen) startPart(a *App) (Screen, tea.Cmd) {
	spec := func(a *App) (play.Spec, error) { return a.testSpec(s.id) }
	if s.row == rowTraining {
		length := s.length
		spec = func(a *App) (play.Spec, error) { return a.trainingSpec(s.id, length) }
	}
	part := lessonPart{spec: spec, back: func(a *App) Screen {
		s.recs = a.recordsOf()[s.id]
		return s
	}}
	round, err := part.round(a)
	if err != nil {
		s.note = text.PartNotReady
		return s, nil
	}
	return round, nil
}

// focusLines says what the lesson is about, wrapped to width: the language of
// a code lesson, the characters of the others.
func (s *lessonScreen) focusLines(width int) []string {
	var t string
	switch {
	case s.lesson.Lang != "":
		t = fmt.Sprintf(text.LessonLanguage, s.lesson.Title)
	case s.lesson.Review:
		t = text.LessonReviewFocus(s.lesson.Focus)
	default:
		t = text.TutorialFocus(s.lesson.Focus)
	}
	return strings.Split(ansi.Wrap(t, width, ""), "\n")
}

// partLines are the lines of the three rows, each as a list of lines whose
// first carries the marker.
func (s *lessonScreen) partLines(a *App, width int) [lessonRows][]string {
	st := a.Style
	paint := func(col color.Color, t string) string { return st.Base().Foreground(col).Render(t) }
	p := a.Store.Profile()
	const label, indent = 9, 9
	pad := func(t string) string { return t + strings.Repeat(" ", max(label-ansi.StringWidth(t), 0)) }
	result := func(parts ...string) string {
		t := strings.Join(parts, " · ")
		if t == "" {
			t = text.LessonNoResult
		}
		return strings.Repeat(" ", indent) + paint(st.Dim, ansi.Truncate(t, max(width-indent, 1), "…"))
	}

	var out [lessonRows][]string
	teach := pad(text.LessonPartTeach)
	if p.Taught[s.id] {
		teach += paint(st.OK, text.LessonTaught)
	}
	out[rowTeach] = []string{teach}

	names := make([]string, len(text.LessonLengths))
	for i, n := range text.LessonLengths {
		names[i] = n
		if course.Length(i) == s.length {
			names[i] = "[" + n + "]"
		}
	}
	var training []string
	if r := s.recs.Last["training/"+s.length.String()]; r != nil {
		training = append(training, text.LessonLast(speedText(r)))
	}
	out[rowTraining] = []string{pad(text.LessonPartTraining) + strings.Join(names, " · "), result(training...)}

	var test []string
	if r := s.recs.Best["test"]; r != nil {
		test = append(test, text.LessonBest(speedText(r)))
	}
	if r := s.recs.Last["test"]; r != nil {
		test = append(test, text.LessonLast(speedText(r)))
	}
	out[rowTest] = []string{pad(text.LessonPartTest) + starBar(p.Stars[s.id]), result(test...)}
	return out
}

func (s *lessonScreen) View(a *App, w, h int) string {
	st := a.Style
	paint := func(col color.Color, bold bool, t string) string {
		return st.Base().Foreground(col).Bold(bold).Render(t)
	}
	paintAll := func(lines []string, col color.Color) []string {
		out := make([]string, len(lines))
		for i, l := range lines {
			out[i] = paint(col, false, l)
		}
		return out
	}
	width := max(w-4, 1) // inside the border and past the marker
	rows := max(h-3, 0)

	parts := s.partLines(a, width)
	var menu []string
	selLine := 0
	for i, p := range parts {
		for j, l := range p {
			marker := "  "
			if j == 0 && i == s.row {
				selLine = len(menu)
				marker = "> "
			}
			if i == s.row {
				l = paint(st.Accent, true, marker) + l
			} else {
				l = marker + l
			}
			menu = append(menu, l)
		}
	}
	var note []string
	if s.note != "" {
		note = []string{"", paint(st.Dim, false, ansi.Truncate(s.note, max(w-3, 1), "…"))}
	}

	// What the window cannot hold goes, first to last: the gaps between the
	// parts, the chapter, the fingers, the characters. The parts and the
	// selection are what the screen is for.
	focus := s.focusLines(width)
	room := rows - len(menu) - len(note) - 1 // a blank row under the head
	var head []string
	if len(focus) <= room {
		head = paintAll(focus, st.Fg)
		room -= len(focus)
		if len(s.fingers) <= room {
			head = append(head, paintAll(s.fingers, st.Fg)...)
			room -= len(s.fingers)
		}
		if s.chapter != "" && room >= 1 {
			head = append(paintAll([]string{ansi.Truncate(s.chapter, width, "…")}, st.Dim), head...)
		}
	}

	if len(menu) > rows { // a tiny window: scroll the parts to the selection
		top := min(max(selLine-rows/2, 0), len(menu)-rows)
		menu = menu[top : top+max(rows, 0)]
		note = nil
	}
	var body []string
	if len(head) > 0 {
		body = append(head, "")
	}
	body = append(body, menu...)
	body = append(body, note...)
	title := fmt.Sprintf(text.LessonTitle, s.lesson.Title)
	return style.Frame(st, w, h, title, strings.Join(body, "\n"), text.HintLesson)
}
