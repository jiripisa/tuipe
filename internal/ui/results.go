package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/ghost"
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

// outcomeLost reports an incident that was not won: it never earns a ghost or a star.
func outcomeLost(r play.Result) bool { return r.Incident != nil && !r.Incident.Won }

// Finish records a finished round: statistics, the best stars of the lesson,
// the ghost, the run and the chapter. A failed save is shown on the status
// line and also returned; the run itself is always returned.
func (a *App) Finish(spec play.Spec, r play.Result) (store.Run, error) {
	now := a.Now()
	p := a.Store.Profile()
	a.stats().Record(r.Text, r.Log, now)

	if spec.LessonID != "" && !outcomeLost(r) && r.Stars > p.Stars[spec.LessonID] {
		p.Stars[spec.LessonID] = r.Stars
	}
	a.completeChapter(spec, r)
	testOut := spec.Kind == play.KindTestOut
	placement := spec.Kind == play.KindPlacement // proves skill like a test-out and is no text to beat
	if testOut && r.Stars >= testOutStars {
		a.Curr.ApplySkip(p, a.stats(), a.skippedBy(spec.ChapterID))
	}
	if !testOut && !placement && !sprintResume(p.Resume) {
		p.Resume = nil // a test-out is no part of the lesson to continue; a Sprint outranks everything
	}

	var errs []error
	hash := r.Text.Hash
	if !outcomeLost(r) && !testOut && !placement { // neither is a text to beat
		old, err := a.Store.Ghost(hash)
		errs = append(errs, err)
		if ghost.ShouldReplace(old, r.Metrics, r.Paused, spec.GhostIsPace) {
			errs = append(errs, a.Store.SaveGhost(store.Ghost{
				TextHash: hash, Score: r.Metrics.Score, TimesMs: ghost.FromLog(r.Text, r.Log),
			}))
		}
	}

	run := store.Run{
		ID: runID(now, hash), At: now, Mode: modeName(spec),
		Session: a.sessionOf(spec), LessonID: spec.LessonID, TextHash: hash, LayoutID: a.Layout.ID,
		WPM: r.Metrics.WPM, Accuracy: r.Metrics.Accuracy, Score: r.Metrics.Score,
		Stars: r.Stars, DurationMs: r.Metrics.Duration.Milliseconds(),
		Paused: r.Paused, HintUsed: r.HintUsed,
	}
	if r.GhostDelta != nil {
		d := r.GhostDelta.Milliseconds()
		run.GhostDeltaMs = &d
	}
	if r.Incident != nil {
		run.Incident = &store.IncidentResult{
			Severity: spec.Severity, Won: r.Incident.Won, SLA: r.Incident.SLA, MTTRMs: r.Incident.MTTR.Milliseconds(),
		}
	}
	a.chain.finished(a, r, run.ID) // keeps the Sprint to resume or marks the Standup
	errs = append(errs, a.Store.AppendRun(run), a.Store.SaveProfile())

	err := errors.Join(errs...)
	a.SaveErr(err)
	return run, err
}

// completeChapter completes the chapter of a lesson without an incident once
// its last lesson has a star. A chapter with an incident waits for the incident.
func (a *App) completeChapter(spec play.Spec, r play.Result) {
	if spec.LessonID == "" || r.Stars == 0 {
		return
	}
	for _, ch := range a.Curr.Chapters() {
		n := len(ch.Lessons)
		if n == 0 || ch.Lessons[n-1].ID != spec.LessonID {
			continue
		}
		if _, hasIncident := a.Curr.IncidentDef(ch.ID); !hasIncident {
			a.Curr.CompleteChapter(a.Store.Profile(), ch.ID)
		}
	}
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
	sessionQuick    = "quick"
	sessionStandup  = kindStandup
	sessionSprint   = kindSprint
	sessionCareer   = "career"
	sessionPractice = "practice"
)

// sessionOf says how the round of spec was started: inside a Standup or Sprint,
// by hand in Practice, from the career or incident list, or as a quick run.
func (a *App) sessionOf(spec play.Spec) string {
	switch {
	case a.chain != nil:
		return a.chain.kind
	case spec.Practice:
		return sessionPractice
	case spec.Session != "":
		return spec.Session
	}
	return sessionQuick
}

func modeName(spec play.Spec) string {
	if spec.Practice {
		return "practice"
	}
	switch spec.Kind {
	case play.KindLesson:
		return "lesson"
	case play.KindCode:
		return "code"
	case play.KindIncident:
		return "incident"
	case play.KindTestOut:
		return "testout"
	case play.KindPlacement:
		return "placement"
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

// lostIncident reports a stored run of an incident that was not won.
func lostIncident(r store.Run) bool { return r.Incident != nil && !r.Incident.Won }

// previousBest is the best earlier run on the same text that did not pause and
// was not a lost incident: the one the ghost was recorded from.
func (a *App) previousBest(hash string) *store.Run {
	runs, _ := a.Store.Runs()
	var best *store.Run
	for i := range runs {
		r := &runs[i]
		if r.TextHash == hash && !r.Paused && !lostIncident(*r) && (best == nil || r.Score > best.Score) {
			best = r
		}
	}
	return best
}

// resultsScreen shows the finished round as a diff against the previous best.
type resultsScreen struct {
	run  store.Run
	prev *store.Run
	res  play.Result
	spec play.Spec
	next func(*App) Screen
}

// NewResultsScreen records the finished round with Finish and shows it. The
// previous best attempt is read first, so the diff is against HEAD~1.
func NewResultsScreen(a *App, r play.Result, spec play.Spec, next func(*App) Screen) Screen {
	prev := a.previousBest(r.Text.Hash)
	run, _ := a.Finish(spec, r) // Finish has shown any save error
	return &resultsScreen{run: run, prev: prev, res: r, spec: spec, next: next}
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
		}
	}
	return s, nil
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
	case s.res.Incident != nil && s.res.Incident.Won:
		verdict = append(verdict, fmt.Sprintf(text.ResultsWon, s.res.Incident.SLA))
	case outcomeLost(s.res):
		verdict = append(verdict, st.Base().Foreground(st.Bad).Render(text.ResultsLost))
	case s.spec.Kind == play.KindTestOut && s.res.Stars >= testOutStars:
		verdict = append(verdict, st.Base().Foreground(st.OK).Render(text.TestOutPassed))
	case s.spec.Kind == play.KindTestOut:
		verdict = append(verdict, st.Base().Foreground(st.Bad).Render(text.TestOutFailed))
	}
	if s.res.Paused {
		verdict = append(verdict, dim(text.ResultsPaused))
	}
	if s.res.HintUsed {
		verdict = append(verdict, dim(text.ResultsHintUsed))
	}
	stars := st.Base().Foreground(st.Accent).Render(s.starsLine())

	// The window may be short: drop the header, then the old values, then the
	// notes, but keep the commit, the new values and the stars.
	rows := h - 3
	full := []string{commit, dim("--- " + base), dim("+++ " + text.ResultsNew), ""}
	for i := range cur {
		if removed != nil {
			full = append(full, removed[i])
		}
		full = append(full, added[i])
	}
	full = append(full, "", stars)
	full = append(full, verdict...)
	if label := a.chain.label(0); label != "" {
		full = append(full, "", dim(label))
	}
	body := full
	if len(body) > rows {
		body = append([]string{commit}, added...)
		body = append(body, stars)
		if len(body) > rows {
			body = []string{commit, stars}
		}
	}
	return style.Frame(st, w, h, text.AppName, strings.Join(body, "\n"), text.HintResults)
}

func (s *resultsScreen) starsLine() string {
	n := max(0, min(s.res.Stars, maxStars))
	out := text.ResultsStars + "  " + strings.Repeat("★", n) + strings.Repeat("☆", maxStars-n)
	if n == 0 {
		out += "  " + text.ResultsNoStars
	}
	return out
}
