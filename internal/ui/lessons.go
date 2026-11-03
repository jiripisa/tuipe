package ui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/course"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// lessonRecord is what the run history says about one lesson. Both maps are
// keyed by "test" or "training/<length>"; Last is the latest run of a part,
// Best the one with the highest score that did not pause.
type lessonRecord struct{ Last, Best map[string]*store.Run }

// partKey is the key of a part in lessonRecord: "test", or "training/<length>".
func partKey(mode, length string) string {
	if mode == store.ModeTraining {
		return mode + "/" + length
	}
	return mode
}

// lessonRecords sorts the run history by lesson. Runs of any other mode, such
// as the placement test or the old rounds, are ignored.
func lessonRecords(runs []store.Run) map[string]lessonRecord {
	out := map[string]lessonRecord{}
	for i := range runs {
		r := &runs[i]
		if r.LessonID == "" || (r.Mode != store.ModeTest && r.Mode != store.ModeTraining) {
			continue
		}
		rec, ok := out[r.LessonID]
		if !ok {
			rec = lessonRecord{Last: map[string]*store.Run{}, Best: map[string]*store.Run{}}
			out[r.LessonID] = rec
		}
		key := partKey(r.Mode, r.Length)
		rec.Last[key] = r
		if b := rec.Best[key]; !r.Paused && (b == nil || r.Score > b.Score) {
			rec.Best[key] = r
		}
	}
	return out
}

// recordsOf reads the run history once. A history that cannot be read must
// not break a screen, so what was read is used.
func (a *App) recordsOf() map[string]lessonRecord {
	runs, _ := a.Store.Runs()
	return lessonRecords(runs)
}

// speedText is the speed of a run in characters per minute.
func speedText(r *store.Run) string { return text.LessonSpeed(score.CPM(r.WPM)) }

// starBar draws earned and missing stars: ★★☆.
func starBar(n int) string {
	n = max(0, min(n, maxStars))
	return strings.Repeat("★", n) + strings.Repeat("☆", maxStars-n)
}

// bannerLines wraps the banner to the window and paints it. It takes at most
// half of the rows; a path has no spaces, so it is broken where it must be.
func (a *App) bannerLines(w, rows int) []string {
	var out []string
	for _, b := range a.Banner {
		for _, l := range strings.Split(ansi.Wrap(b, max(w-4, 10), ""), "\n") {
			out = append(out, a.Style.Base().Foreground(a.Style.Bad).Render(l))
		}
	}
	return out[:min(len(out), max(rows/2, 1))]
}

// lessonsRow is one line of the list: a chapter heading, or a lesson.
type lessonsRow struct {
	chapter string // the title, for a heading
	lesson  course.Lesson
	pick    bool
}

// lessonsScreen lists every lesson of the course, grouped by chapter, with its
// stars and speed. Nothing in it is locked.
type lessonsScreen struct {
	rows []lessonsRow
	recs map[string]lessonRecord
	sel  int // among the lessons
}

// NewLessonsScreen builds the lesson list. It opens on the next lesson, or on
// the first one when every lesson has a star.
func NewLessonsScreen(a *App) Screen { return newLessonsScreen(a, "") }

// newLessonsScreen builds the list with the selection on lesson at, or on the
// default when at is empty or unknown.
func newLessonsScreen(a *App, at string) *lessonsScreen {
	a.noteSkipped()
	l := &lessonsScreen{recs: a.recordsOf()}
	for _, ch := range a.Course.Chapters() {
		l.rows = append(l.rows, lessonsRow{chapter: ch.Title})
		for _, ls := range ch.Lessons {
			l.rows = append(l.rows, lessonsRow{lesson: ls, pick: true})
		}
	}
	if _, ok := a.Course.Lesson(at); !ok {
		p := a.Store.Profile()
		next, _ := a.Course.Next(p.Stars, p.Recommended) // none leaves the first lesson
		at = next.ID
	}
	i := 0
	for _, r := range l.rows {
		if !r.pick {
			continue
		}
		if r.lesson.ID == at {
			l.sel = i
		}
		i++
	}
	return l
}

func (l *lessonsScreen) count() int {
	n := 0
	for _, r := range l.rows {
		if r.pick {
			n++
		}
	}
	return n
}

// selectedID is the lesson under the selection, empty when the list is empty.
func (l *lessonsScreen) selectedID() string {
	i := 0
	for _, r := range l.rows {
		if r.pick {
			if i == l.sel {
				return r.lesson.ID
			}
			i++
		}
	}
	return ""
}

func (l *lessonsScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return l, nil
	}
	a.Banner = nil
	n := l.count()
	switch k.String() {
	case "up", "k":
		if n > 0 {
			l.sel = (l.sel + n - 1) % n
		}
	case "down", "j":
		if n > 0 {
			l.sel = (l.sel + 1) % n
		}
	case "enter":
		if id := l.selectedID(); id != "" {
			return NewLessonScreen(a, id), nil
		}
	case "s", "S":
		return NewStatsScreen(a), nil
	case "n", "N":
		return NewSettingsScreen(a), nil
	}
	return l, nil
}

// status is what the right-hand side of a lesson row says: the best speed of
// a Test that earned a star, else whether the lesson is recommended or begun.
func (l *lessonsScreen) status(p *store.Profile, ls course.Lesson) string {
	if p.Stars[ls.ID] > 0 {
		if best := l.recs[ls.ID].Best["test"]; best != nil {
			return speedText(best)
		}
		return ""
	}
	var parts []string
	if p.Taught[ls.ID] || len(l.recs[ls.ID].Last) > 0 {
		parts = append(parts, text.LessonStarted)
	}
	if p.Recommended == ls.ID {
		parts = append(parts, text.LessonRecommended)
	}
	return strings.Join(parts, " · ")
}

func (l *lessonsScreen) View(a *App, w, h int) string {
	st := a.Style
	p := a.Store.Profile()
	paint := func(col color.Color, bold bool, s string) string {
		return st.Base().Foreground(col).Bold(bold).Render(s)
	}
	got, _ := a.Course.Stars(p.Stars)
	head := []string{
		paint(st.Accent, false, a.Course.Rank(p.Stars).String()) +
			paint(st.Dim, false, " · ") + paint(st.Fg, false, fmt.Sprintf(text.LessonsStars, got)),
		"",
	}

	width := 0
	for _, r := range l.rows {
		width = max(width, ansi.StringWidth(r.lesson.Title))
	}
	var lines []string
	selLine, i := 0, 0
	for _, r := range l.rows {
		if !r.pick {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, paint(st.Dim, true, ansi.Truncate(r.chapter, max(w-4, 1), "…")))
			continue
		}
		title := r.lesson.Title + strings.Repeat(" ", width-ansi.StringWidth(r.lesson.Title))
		plain := starBar(p.Stars[r.lesson.ID]) + "  " + title
		status := l.status(p, r.lesson)
		if status != "" {
			plain += "  " + status
		}
		plain = ansi.Truncate(plain, max(w-4, 1), "…")
		if i == l.sel {
			selLine = len(lines)
			lines = append(lines, paint(st.Accent, true, "> "+plain))
		} else {
			lines = append(lines, paint(st.Fg, false, "  "+plain))
		}
		i++
	}

	// The banner and the update notice take rows of their own.
	rows := h - 3
	banner := a.bannerLines(w, rows)
	if len(banner) > 0 {
		rows -= len(banner) + 1
	}
	notice := ""
	if a.UpdateNotice != "" {
		notice = paint(st.Dim, false, ansi.Truncate(a.UpdateNotice, max(w-3, 1), "…"))
		rows--
	}
	// Scroll the list so that the selection stays visible, centred when it can be.
	avail := rows - len(head)
	if avail > 0 && len(lines) > avail {
		top := min(max(selLine-avail/2, 0), len(lines)-avail)
		lines = lines[top : top+avail]
	}
	body := append(head, lines...)
	body = body[:min(len(body), max(rows, 0))]
	if notice != "" {
		body = append(body, notice)
	}
	if len(banner) > 0 {
		body = append(append(banner, ""), body...)
	}
	return style.Frame(st, w, h, text.AppName, strings.Join(body, "\n"), text.HintLessons)
}
