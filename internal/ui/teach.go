package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/course"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/ui/roundview"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const explainStep = -1 // the step of the explanation, before the first line

// teachScreen is the Výuka of a lesson: it says what is learned and how, then
// leads the user through the lines one by one. It keeps no statistics and no
// run; the only trace is Profile.Taught at the end.
type teachScreen struct {
	id     string
	lesson course.Lesson
	tut    course.Tutorial
	next   func(*App) Screen // where leaving goes
	done   func(*App) Screen // where finishing goes
	step   int               // explainStep, or the index of the line being typed
	sess   *play.Session     // the line being typed, on a session of its own
	now    time.Time
}

// NewTeachScreen builds the Výuka of a lesson. Leaving it, or finishing it,
// goes to next.
func NewTeachScreen(a *App, lessonID string, next func(*App) Screen) Screen {
	return newTeachScreen(a, lessonID, next, next)
}

// newTeachScreen is NewTeachScreen with a screen of its own for a finished
// Výuka: leaving goes to next, finishing to done.
func newTeachScreen(a *App, lessonID string, next, done func(*App) Screen) Screen {
	if next == nil {
		next = NewLessonsScreen
	}
	if done == nil {
		done = next
	}
	ls, ok := a.Course.Lesson(lessonID)
	tut, err := course.Tutorial{}, error(nil)
	if ok {
		tut, err = a.Course.Tutorial(lessonID, a.Now().UnixNano())
	}
	if !ok || err != nil {
		a.Banner = append(a.Banner, fmt.Sprintf(text.TeachUnavailable, lessonID))
		return next(a)
	}
	return &teachScreen{id: lessonID, lesson: ls, tut: tut, next: next, done: done, step: explainStep, now: a.Now()}
}

// startLine begins the session of line i. The session gets no statistics, so
// nothing typed here can reach a.stats(), and it is told that focus events
// exist, so that it never pauses on idle: nothing here is timed.
func (t *teachScreen) startLine(a *App, i int) {
	t.step = i
	spec := play.Spec{Title: t.lesson.Title, LessonID: t.id, Text: t.tut.Lines[i], HintsOff: true}
	t.sess = play.New(spec, a.Layout, nil, true)
}

func (t *teachScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	t.now = a.Now()
	m, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return t, nil
	}
	switch {
	case m.Code == tea.KeyEscape:
		return t.next(a), nil
	case t.step == explainStep:
		if m.Code == tea.KeyEnter {
			return t.advance(a), nil
		}
		return t, nil
	case m.Mod.Contains(tea.ModCtrl) || m.Mod.Contains(tea.ModMeta):
		return t, nil // a shortcut, never typed input
	}
	// Input is read as in a round: Alt on the wrong character is dropped by the
	// session, with a warning, instead of counting as a mistake for ever.
	alt := m.Mod.Contains(tea.ModAlt)
	switch {
	case m.Code == tea.KeyEnter:
		t.sess.Key(play.Input{Rune: '\n', Alt: alt}, t.now)
	case m.Text == "" && alt && unicode.IsPrint(m.Code):
		t.sess.Key(play.Input{Rune: m.Code, Alt: true}, t.now) // the decoder drops Text for Alt chords
	default:
		for _, c := range m.Text {
			t.sess.Key(play.Input{Rune: c, Alt: alt}, t.now)
		}
	}
	if t.sess.Done() {
		return t.advance(a), nil
	}
	return t, nil
}

// advance moves to the next line. After the last one the lesson is marked as
// taught and the screen gives way to done.
func (t *teachScreen) advance(a *App) Screen {
	if i := t.step + 1; i < len(t.tut.Lines) {
		t.startLine(a, i)
		return t
	}
	a.Store.Profile().Taught[t.id] = true
	a.SaveErr(a.Store.SaveProfile())
	return t.done(a)
}

// highlights are the keys to mark: those of the lesson, and the keys of the
// next character with the Shift or Option it needs.
func (t *teachScreen) highlights(a *App) []string {
	ids := slices.Clone(t.tut.Keys)
	var runes []rune
	if t.step == explainStep {
		runes = t.lesson.Focus
	} else if f := t.sess.Frame(t.now); f.Pos < len(f.Cells) {
		runes = []rune{f.Cells[f.Pos].R}
	}
	for _, r := range runes {
		route, ok := a.Layout.Route(r)
		if !ok {
			continue
		}
		for _, id := range roundview.Highlights(a.Layout, route) {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func (t *teachScreen) View(a *App, w, h int) string {
	st := a.Style
	cw := max(w-4, 1) // inside the border, with a space on each side
	rows := max(h-3, 0)

	var expl []string
	for i, e := range t.tut.Explain {
		fg := st.Fg
		if i == 0 {
			fg = st.Accent
		}
		for _, l := range strings.Split(ansi.Wrap(e, cw, ""), "\n") {
			expl = append(expl, st.Base().Foreground(fg).Render(l))
		}
	}
	draw := strings.Split(style.Keyboard(st, a.Layout, t.highlights(a), nil, cw), "\n")

	// What is dropped when the window is short, first to last: the finished
	// lines, the drawing, the explanation. The line being typed always stays.
	var cur, done []string
	if t.step != explainStep {
		cur = t.currentRows(st, cw, rows)
		for _, l := range t.tut.Lines[:t.step] {
			done = append(done, st.Base().Foreground(st.OK).Render(ansi.Truncate(l, cw, "…")))
		}
	}
	total, blocks := len(cur), min(len(cur), 1)
	room := func(n int) bool { return total+n+min(blocks, 1) <= rows }
	var showExpl, showDraw bool
	if room(len(expl)) {
		showExpl, total, blocks = true, total+len(expl)+min(blocks, 1), blocks+1
	} else if t.step == explainStep && rows > 0 { // the explanation is the screen: cut it, do not drop it
		rest := strings.Join(expl[rows-1:], " ")
		expl = append(expl[:rows-1], ansi.Truncate(ansi.Strip(rest), cw, "…"))
		showExpl, total, blocks = true, rows, blocks+1
		style.Clipped("explanation")
	} else {
		style.Clipped("explanation")
	}
	if showExpl && room(len(draw)) {
		showDraw, total = true, total+len(draw)+min(blocks, 1)
	}
	if len(cur) > 0 && showDraw { // a finished line never outlasts the drawing
		done = done[max(0, len(done)-max(0, rows-total)):]
	} else {
		done = nil
	}

	var body []string
	add := func(lines []string) {
		if len(body) > 0 {
			body = append(body, "")
		}
		body = append(body, lines...)
	}
	if showExpl {
		add(expl)
	}
	if showDraw {
		add(draw)
	}
	add(append(done, cur...))
	for i, l := range body {
		body[i] = " " + l
	}

	title := fmt.Sprintf(text.TeachTitleExplain, t.lesson.Title)
	if t.step != explainStep {
		title = fmt.Sprintf(text.TeachTitle, t.step+1, len(t.tut.Lines), t.lesson.Title)
	}
	return style.Frame(st, w, h, title, strings.Join(body, "\n"), t.footer(a, w-2))
}

// footer is the hint of the screen, or the line about Option arriving as Alt.
func (t *teachScreen) footer(a *App, w int) string {
	switch {
	case t.step == explainStep:
		return text.HintTeachExplain
	case t.sess.Frame(t.now).AltWarning:
		help := altHelp()
		if help == "" {
			help = text.RoundAltWarning
		}
		return a.Style.Base().Foreground(a.Style.Bad).Render(ansi.Truncate(help, w, "…"))
	}
	return text.HintTeach
}

// currentRows draws the line being typed, wrapped to width cells. A line taller
// than limit rows scrolls to keep the cursor in view.
func (t *teachScreen) currentRows(st style.Style, width, limit int) []string {
	f := t.sess.Frame(t.now)
	spans := roundview.WrapSpans(f.Cells, width)
	cursor := len(spans) - 1
	for i, sp := range spans {
		if f.Pos < sp[1] {
			cursor = i
			break
		}
	}
	if len(spans) > limit {
		start := max(min(cursor-1, len(spans)-limit), 0)
		spans = spans[start:][:limit]
	}
	out := make([]string, len(spans))
	for i, sp := range spans {
		out[i] = roundview.Span(st, f, sp[0], sp[1])
	}
	return out
}
