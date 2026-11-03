package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/streak"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const (
	maxStars     = 3
	testOutStars = 2 // stars that pass a test-out
)

// Finish records a finished round: statistics, the best stars of the lesson,
// the run and the chapter. A Trénink earns no stars, and a Trénink or a Test
// leaves the chapters and Resume alone. A failed save is shown on the status
// line and also returned; the run itself is always returned.
func (a *App) Finish(spec play.Spec, r play.Result) (store.Run, error) {
	now := a.Now()
	p := a.Store.Profile()
	a.stats().Record(r.Text, r.Log, now)

	if spec.Kind == play.KindTraining {
		r.Stars = 0 // stars come from a Test only
	}
	if spec.LessonID != "" && r.Stars > p.Stars[spec.LessonID] {
		p.Stars[spec.LessonID] = r.Stars
	}
	part := isLessonPart(spec.Kind)
	if !part {
		a.completeChapter(spec, r)
	}
	testOut := spec.Kind == play.KindTestOut
	placement := spec.Kind == play.KindPlacement // proves skill like a test-out and is no text to beat
	if testOut && r.Stars >= testOutStars {
		a.Curr.ApplySkip(p, a.stats(), a.skippedBy(spec.ChapterID))
	}
	if !testOut && !placement && !part {
		p.Resume = nil // a test-out is no part of the lesson to continue
	}

	hash := r.Text.Hash

	run := store.Run{
		ID: runID(now, hash), At: now, Mode: modeName(spec),
		Session: a.sessionOf(spec), LessonID: spec.LessonID, TextHash: hash, LayoutID: a.Layout.ID,
		WPM: r.Metrics.WPM, Accuracy: r.Metrics.Accuracy, Score: r.Metrics.Score,
		Stars: runStars(spec, r), Length: spec.Length, DurationMs: r.Metrics.Duration.Milliseconds(),
		Paused: r.Paused, HintUsed: r.HintUsed,
	}
	err := errors.Join(a.Store.AppendRun(run), a.Store.SaveProfile())
	a.SaveErr(err)
	return run, err
}

// completeChapter completes the chapter of the last lesson when it earned a star.
func (a *App) completeChapter(spec play.Spec, r play.Result) {
	if spec.LessonID == "" || r.Stars == 0 {
		return
	}
	p := a.Store.Profile()
	for _, ch := range a.Curr.Chapters() {
		if !slices.ContainsFunc(ch.Lessons, func(l content.LessonDef) bool { return l.ID == spec.LessonID }) {
			continue
		}
		if ch.Lessons[len(ch.Lessons)-1].ID == spec.LessonID {
			a.Curr.CompleteChapter(p, ch.ID)
		}
	}
}

func (a *App) chapterTitle(id string) string {
	for _, ch := range a.Curr.Chapters() {
		if ch.ID == id {
			return ch.Title
		}
	}
	return id
}

// skippedBy lists the chapter of a passed test-out and every earlier chapter
// that is not done. The test-out text uses all characters unlocked up to its
// chapter, so passing it proves the earlier ones too, as placement does.
func (a *App) skippedBy(chapterID string) []string {
	p := a.Store.Profile()
	var ids []string
	for _, ch := range a.Curr.Chapters() {
		if !a.Curr.IsDone(p, ch.ID) || ch.ID == chapterID {
			ids = append(ids, ch.ID)
		}
		if ch.ID == chapterID {
			return ids
		}
	}
	return []string{chapterID}
}

// How a round was started, as stored on its run.
const (
	sessionQuick     = "quick"
	sessionStandup   = "standup"
	sessionSprint    = "sprint"
	sessionCareer    = "career"
	sessionPlacement = "placement"
)

// sessionOf says how the round of spec was started: from the career list, or by
// placement. Old runs without a session count as quick runs.
func (a *App) sessionOf(spec play.Spec) string {
	if spec.Session != "" {
		return spec.Session
	}
	return sessionQuick
}

// runStars are the stars stored on a run. A placement test grants none.
func runStars(spec play.Spec, r play.Result) int {
	if spec.Kind == play.KindPlacement {
		return 0
	}
	return r.Stars
}

func modeName(spec play.Spec) string {
	switch spec.Kind {
	case play.KindLesson:
		return "lesson"
	case play.KindCode:
		return "code"
	case play.KindTestOut:
		return "testout"
	case play.KindPlacement:
		return "placement"
	case play.KindTraining:
		return store.ModeTraining
	case play.KindTest:
		return store.ModeTest
	}
	return "drill"
}

// runID is the short hash that names a run: 7 hex digits of the time and text.
func runID(at time.Time, textHash string) string {
	sum := sha256.Sum256([]byte(at.Format(time.RFC3339Nano) + textHash))
	return hex.EncodeToString(sum[:])[:runIDLen]
}

// uptime is the streak, with days taken in local time.
func (a *App) uptime() streak.Status {
	runs, _ := a.Store.Runs() // a damaged history must not look like a failed save
	days := make([]string, len(runs))
	for i, r := range runs {
		days[i] = r.At.Local().Format(dateLayout)
	}
	return streak.Compute(days, a.Now().Local().Format(dateLayout))
}

// previousBest is the best earlier run on the same text that did not pause.
func (a *App) previousBest(hash string) *store.Run {
	runs, _ := a.Store.Runs()
	var best *store.Run
	for i := range runs {
		r := &runs[i]
		if r.TextHash == hash && !r.Paused && (best == nil || r.Score > best.Score) {
			best = r
		}
	}
	return best
}

// resultsScreen shows the finished round as a diff against the previous best.
type resultsScreen struct {
	run    store.Run
	prev   *store.Run
	res    play.Result
	spec   play.Spec
	next   func(*App) Screen
	closed []string // chapters this round closed
	rank   string   // the promotion line, when the rank changed
}

// NewResultsScreen records the finished round with Finish and shows it. The
// previous best attempt is read first, so the diff is against HEAD~1.
func NewResultsScreen(a *App, r play.Result, spec play.Spec, next func(*App) Screen) Screen {
	prev := a.previousBest(r.Text.Hash)
	p := a.Store.Profile()
	rank := a.Curr.RankOf(p)
	var open []string
	for _, ch := range a.Curr.Chapters() {
		if !a.Curr.IsDone(p, ch.ID) {
			open = append(open, ch.ID)
		}
	}
	run, _ := a.Finish(spec, r) // Finish has shown any save error
	s := &resultsScreen{run: run, prev: prev, res: r, spec: spec, next: next}
	for _, id := range open {
		if a.Curr.IsDone(p, id) {
			s.closed = append(s.closed, id)
		}
	}
	if after := a.Curr.RankOf(p); after != rank {
		s.rank = fmt.Sprintf(text.ResultsRank, after)
	}
	return s
}

func (s *resultsScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "enter":
			if s.next == nil {
				return NewHomeScreen(a), nil
			}
			return s.next(a), nil
		case "esc":
			return NewHomeScreen(a), nil
		case "r", "R", "ctrl+r":
			if s.canRepeat(a) {
				return s.repeat(a), nil
			}
		}
	}
	return s, nil
}

// canRepeat says whether the round can be played again from here. A passed
// test-out has nothing left to prove.
func (s *resultsScreen) canRepeat(a *App) bool {
	passed := s.spec.Kind == play.KindTestOut && s.res.Stars >= testOutStars
	return !passed
}

// repeat plays the same text again.
func (s *resultsScreen) repeat(a *App) Screen {
	return newRound(a, s.spec, s.next)
}

func (s *resultsScreen) View(a *App, w, h int) string {
	st := a.Style
	add := func(l string) string { return st.Base().Foreground(st.OK).Render("+ " + l) }
	del := func(l string) string { return st.Base().Foreground(st.Bad).Render("- " + l) }
	dim := func(l string) string { return st.Base().Foreground(st.Dim).Render(l) }

	m := s.res.Metrics
	commit := dim(fmt.Sprintf(text.ResultsCommit, s.run.ID, s.spec.Title))
	cur := []string{
		fmt.Sprintf("%s %6.1f", text.ResultsWPM, m.WPM),
		fmt.Sprintf("%s %5.1f %%", text.ResultsAccuracy, m.Accuracy*100),
		fmt.Sprintf("%s %6.1f s", text.ResultsTime, m.Duration.Seconds()),
	}
	base := text.ResultsNoBase
	var added, removed []string
	for _, c := range cur {
		added = append(added, add(c))
	}
	if s.prev != nil {
		base = text.ResultsOld
		old := []string{
			fmt.Sprintf("%s %6.1f", text.ResultsWPM, s.prev.WPM),
			fmt.Sprintf("%s %5.1f %%", text.ResultsAccuracy, s.prev.Accuracy*100),
			fmt.Sprintf("%s %6.1f s", text.ResultsTime, float64(s.prev.DurationMs)/1000),
		}
		for _, o := range old {
			removed = append(removed, del(o))
		}
	}
	var verdict []string
	switch {
	case s.spec.Kind == play.KindTestOut && s.res.Stars >= testOutStars:
		verdict = append(verdict, st.Base().Foreground(st.OK).Render(text.TestOutPassed(len(s.closed))))
	case s.spec.Kind == play.KindTestOut:
		verdict = append(verdict, st.Base().Foreground(st.Bad).Render(text.TestOutFailed))
	default:
		for _, id := range s.closed {
			verdict = append(verdict, st.Base().Foreground(st.Accent).Render(fmt.Sprintf(text.ResultsChapterClosed, a.chapterTitle(id))))
		}
	}
	if s.res.Paused {
		verdict = append(verdict, dim(text.ResultsPaused))
	}
	if s.res.HintUsed {
		verdict = append(verdict, dim(text.ResultsHintUsed))
	}
	if s.rank != "" {
		verdict = append(verdict, st.Base().Foreground(st.Accent).Render(s.rank))
	}
	// Only a lesson keeps its stars, so only a lesson shows them.
	var stars []string
	if s.spec.LessonID != "" {
		stars = []string{st.Base().Foreground(st.Accent).Render(s.starsLine())}
	}

	// The window may be short: drop the blank rows, then the header, the old
	// values and the notes, but keep the commit, the new values and the outcome,
	// which is the stars or, for a round without them, the first verdict line.
	outcome := stars
	if len(outcome) == 0 {
		outcome = verdict[:min(1, len(verdict))]
	}
	rows := h - 3
	build := func(blank bool) []string {
		sep := func() []string {
			if blank {
				return []string{""}
			}
			return nil
		}
		full := append([]string{commit, dim("--- " + base), dim("+++ " + text.ResultsNew)}, sep()...)
		for i := range cur {
			if removed != nil {
				full = append(full, removed[i])
			}
			full = append(full, added[i])
		}
		full = append(full, sep()...)
		full = append(full, stars...)
		full = append(full, verdict...)
		return full
	}
	body := build(true)
	if len(body) > rows {
		body = build(false)
	}
	if len(body) > rows {
		style.Clipped("results: dropped the old values and notes")
		body = append([]string{commit}, added...)
		body = append(body, outcome...)
		if len(body) > rows {
			body = append([]string{commit}, outcome...)
		}
	}
	hint := text.HintResults
	if s.canRepeat(a) {
		hint = text.HintResultsRepeat
	}
	return style.Frame(st, w, h, text.AppName, strings.Join(body, "\n"), hint)
}

func (s *resultsScreen) starsLine() string {
	n := max(0, min(s.res.Stars, maxStars))
	out := text.ResultsStars + "  " + strings.Repeat("★", n) + strings.Repeat("☆", maxStars-n)
	if n == 0 {
		out += "  " + text.ResultsNoStars
	}
	return out
}
