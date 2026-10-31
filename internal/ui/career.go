package ui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/recommend"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// listMode says what a pick in the chapter list does.
type listMode int

const (
	listCareer          listMode = iota // play a lesson, or test a locked chapter out
	listPracticeLesson                  // practise a lesson of an open chapter
	listPracticeChapter                 // drill the characters of an open chapter
)

type rowKind int

const (
	rowChapter rowKind = iota
	rowLesson
	rowTestOut
)

// careerRow is one line of the list. Lessons and test-outs can be picked; so
// can a chapter when its characters are practised.
type careerRow struct {
	kind    rowKind
	chapter content.ChapterDef
	lesson  content.LessonDef
	pick    bool
}

func (r careerRow) sameAs(o careerRow) bool {
	return r.kind == o.kind && r.chapter.ID == o.chapter.ID && r.lesson.ID == o.lesson.ID
}

// careerScreen lists the chapters with their lessons and stars. It is also the
// chapter and lesson picker of Practice.
type careerScreen struct {
	mode listMode
	sel  int               // among the rows that can be picked
	back func(*App) Screen // where Esc goes; the home screen when nil
	note string
}

// NewCareerScreen builds the career map.
func NewCareerScreen(a *App) Screen { return &careerScreen{mode: listCareer} }

func newChapterList(mode listMode, back func(*App) Screen) *careerScreen {
	return &careerScreen{mode: mode, back: back}
}

// rows lays out the list for the current progress. An open chapter lists its
// lessons, completed ones too, so they can be improved. A locked chapter offers
// only its test-out. Practice lists open chapters only.
func (c *careerScreen) rows(a *App) []careerRow {
	p := a.Store.Profile()
	var rows []careerRow
	for _, ch := range a.Curr.Chapters() {
		open := a.Curr.IsUnlocked(p, ch.ID)
		if !open && c.mode != listCareer {
			continue
		}
		rows = append(rows, careerRow{kind: rowChapter, chapter: ch, pick: c.mode == listPracticeChapter})
		switch {
		case c.mode == listPracticeChapter:
		case open:
			for _, l := range ch.Lessons {
				rows = append(rows, careerRow{kind: rowLesson, chapter: ch, lesson: l, pick: true})
			}
		default:
			rows = append(rows, careerRow{kind: rowTestOut, chapter: ch, pick: true})
		}
	}
	return rows
}

// picked returns the row under the selection and keeps the selection in range.
func (c *careerScreen) picked(rows []careerRow) (careerRow, bool) {
	n := 0
	for _, r := range rows {
		if r.pick {
			n++
		}
	}
	if n == 0 {
		return careerRow{}, false
	}
	c.sel = (c.sel%n + n) % n
	i := 0
	for _, r := range rows {
		if r.pick {
			if i == c.sel {
				return r, true
			}
			i++
		}
	}
	return careerRow{}, false
}

func (c *careerScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	c.note = ""
	rows := c.rows(a)
	switch k.String() {
	case "up", "k":
		c.sel--
	case "down", "j":
		c.sel++
	case "esc":
		if c.back != nil {
			return c.back(a), nil
		}
		return NewHomeScreen(a), nil
	case "enter":
		if r, ok := c.picked(rows); ok {
			return c.start(a, r)
		}
	}
	c.picked(rows)
	return c, nil
}

// start plays the picked row. Every round returns to this list.
func (c *careerScreen) start(a *App, r careerRow) (Screen, tea.Cmd) {
	again := func(*App) Screen { return c }
	var spec play.Spec
	var err error
	switch {
	case r.kind == rowTestOut:
		spec, err = a.testOutSpec(r.chapter)
	case r.kind == rowChapter:
		spec, err = a.chapterDrillSpec(r.chapter)
		spec.Practice = true
	default:
		spec, err = a.SpecFor(recommend.Pick{Kind: recommend.Lesson, LessonID: r.lesson.ID})
		spec.Practice = c.mode == listPracticeLesson
	}
	if err != nil {
		c.note = err.Error()
		return c, nil
	}
	return newRound(a, spec, again), nil
}

// testOutSpec is the round that skips a chapter: its test-out text, without
// hints, measured against the chapter's targets.
func (a *App) testOutSpec(ch content.ChapterDef) (play.Spec, error) {
	txt, targets := a.Curr.TestOutText(ch.ID)
	if txt == "" {
		return play.Spec{}, fmt.Errorf(text.TestOutNotReady)
	}
	return play.Spec{
		Kind: play.KindTestOut, Title: fmt.Sprintf(text.TestOutTitle, ch.Title),
		ChapterID: ch.ID, Text: txt, Targets: targets, HintsOff: true,
	}, nil
}

// starBar draws earned and missing stars: ★★☆.
func starBar(n int) string {
	n = max(0, min(n, maxStars))
	return strings.Repeat("★", n) + strings.Repeat("☆", maxStars-n)
}

func (c *careerScreen) View(a *App, w, h int) string {
	st := a.Style
	p := a.Store.Profile()
	paint := func(col color.Color, bold bool, s string) string {
		return st.Base().Foreground(col).Bold(bold).Render(s)
	}

	var head []string
	title := text.AppName + " · " + text.PracticeTitle
	if c.mode == listCareer {
		title = text.AppName + " · " + text.CareerTitle
		head = []string{
			paint(st.Dim, false, text.HomeRank+"  ") + paint(st.Accent, false, a.Curr.RankOf(p).String()), "",
		}
	}

	rows := c.rows(a)
	cur, hasCur := c.picked(rows)
	var lines []string
	selLine := 0
	for _, r := range rows {
		var plain string
		col := st.Fg
		switch r.kind {
		case rowChapter:
			plain = c.chapterLine(a, r.chapter)
			if !a.Curr.IsUnlocked(p, r.chapter.ID) {
				col = st.Dim
			}
		case rowLesson:
			plain, col = starBar(p.Stars[r.lesson.ID])+"  "+r.lesson.Title, st.Fg
		case rowTestOut:
			plain, col = text.CareerTestOut, st.Dim
		}
		switch {
		case hasCur && r.pick && r.sameAs(cur):
			selLine = len(lines)
			lines = append(lines, paint(st.Accent, true, "> "+plain))
		case r.pick:
			lines = append(lines, paint(col, false, "  "+plain))
		default:
			lines = append(lines, paint(col, r.kind == rowChapter, plain))
		}
	}

	// Scroll the list so that the selection stays visible, centred when it can be.
	avail := h - 3 - len(head)
	if c.note != "" {
		avail -= 2
	}
	if avail > 0 && len(lines) > avail {
		top := min(max(selLine-avail/2, 0), len(lines)-avail)
		lines = lines[top : top+avail]
	}
	body := append(head, lines...)
	if c.note != "" {
		body = append(body, "", paint(st.Dim, false, c.note))
	}
	return style.Frame(st, w, h, title, strings.Join(body, "\n"), text.HintCareer)
}

// chapterLine is a chapter's heading: number, title, the rank it earns, and
// whether it is done or locked.
func (c *careerScreen) chapterLine(a *App, ch content.ChapterDef) string {
	p := a.Store.Profile()
	line := fmt.Sprintf("%d  %s  %s", ch.Number, ch.Title, ch.Rank)
	switch {
	case a.Curr.IsDone(p, ch.ID):
		line += "  " + text.CareerDone
	case !a.Curr.IsUnlocked(p, ch.ID):
		line += "  " + text.CareerLocked
	}
	return line
}
