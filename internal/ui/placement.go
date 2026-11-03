package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/placement"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// placementScreen runs the placement test: an intro for each stage, the stage
// as a round, and at the end the lesson it recommends. The rounds are ordinary
// round screens whose end this screen controls.
type placementScreen struct {
	next   func(*App) Screen
	stages []placement.Stage
	at     int               // the stage that is next or running
	passed []placement.Stage // stages passed so far
	last   *score.Metrics    // result of the stage before, for the intro
	offer  bool              // the test is over and its result is shown
	ended  bool              // a stage came to an end, so the test measured something
	kept   bool              // the test was given up and an earlier recommendation stands
	hands  bool              // the hand-position text follows a result that starts from zero
	text   handText
}

// maxIgnoredAlt is how many Alt presses the Option stage puts up with. A user
// whose terminal turns Option into Alt who skipped the check would otherwise
// never get a wrong key logged, and the stage would never end.
const maxIgnoredAlt = 5

// NewPlacementScreen builds the placement test. next opens the screen that
// follows the result; the lesson list when it is nil.
func NewPlacementScreen(a *App, next func(*App) Screen) Screen {
	p := &placementScreen{next: next, stages: placement.Stages(a.Course, a.Lib, a.Now().UnixNano())}
	if len(p.stages) == 0 || p.stages[0].Text == "" {
		return p.finish(a)
	}
	return p
}

func (p *placementScreen) stageTitle(a *App, st placement.Stage) string {
	switch st.ID {
	case "letters":
		return text.PlacementStageLetters
	case "diacritics":
		return text.PlacementStageDiacritics
	case "digits":
		return text.PlacementStageDigits
	case "shift":
		return text.PlacementStageShift
	case "option":
		return fmt.Sprintf(text.PlacementStageOption, a.Layout.ModifierName)
	case "code":
		return text.PlacementStageCode
	}
	return st.Title
}

func (p *placementScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	switch {
	case p.hands && k.String() == "enter":
		return p.leave(a), nil
	case p.hands:
		p.text.key(k)
	case p.offer && k.String() == "enter" && p.fromZero(a):
		p.hands = true // as after the fork
	case p.offer && k.String() == "enter":
		return p.leave(a), nil
	case !p.offer && k.String() == "enter":
		return p.round(a), nil
	case !p.offer && k.String() == "esc":
		return p.finish(a), nil
	}
	return p, nil
}

// fromZero reports someone who passed no stage and has neither a star nor a
// recommendation from earlier.
func (p *placementScreen) fromZero(a *App) bool {
	return len(p.passed) == 0 && len(a.Store.Profile().Stars) == 0 && !p.kept
}

func (p *placementScreen) leave(a *App) Screen {
	if p.next == nil {
		return NewLessonsScreen(a)
	}
	return p.next(a)
}

// round starts the current stage, without hints.
func (p *placementScreen) round(a *App) Screen {
	st := p.stages[p.at]
	spec := play.Spec{
		Kind:     play.KindPlacement,
		Title:    fmt.Sprintf(text.PlacementRoundTitle, p.at+1, len(p.stages), p.stageTitle(a, st)),
		Text:     st.Text,
		HintsOff: true,
	}
	if st.ID == "code" {
		spec.Lang = "go"
	}
	r := &roundScreen{spec: spec}
	r.ctl = &roundControl{
		stop: func(r *roundScreen) bool {
			_, log := r.sess.PartialLog()
			wrong := 0
			for _, k := range log {
				if !k.Correct {
					wrong++
				}
			}
			return placement.AbortEarly(len(log), wrong) || r.sess.AltIgnored() >= maxIgnoredAlt
		},
		done: func(a *App, res play.Result, now time.Time) Screen {
			return p.stageEnded(a, st, spec, res, r.sess.Done(), now)
		},
		leave: p.finish,
	}
	r.begin(a)
	return r
}

// stageEnded takes the result of a stage. Typed characters become evidence in
// the statistics either way; a failed stage ends the test.
func (p *placementScreen) stageEnded(a *App, st placement.Stage, spec play.Spec, res play.Result, finished bool, now time.Time) Screen {
	p.ended = true
	if finished {
		_, _ = a.Finish(spec, res) // Finish has shown any save error
	} else {
		a.stats().Record(res.Text, res.Log, now)
		a.SaveErr(a.Store.SaveProfile())
	}
	if !finished || !placement.StageSucceeded(st, res.Metrics) {
		return p.finish(a)
	}
	p.passed = append(p.passed, st)
	p.last = &res.Metrics
	if p.at++; p.at < len(p.stages) && p.stages[p.at].Text != "" {
		return p
	}
	return p.finish(a)
}

// finish stores the recommendation, which is all the test leaves behind, and
// shows it. A test given up before any stage ended measured nothing, so an
// earlier recommendation stands.
func (p *placementScreen) finish(a *App) Screen {
	profile := a.Store.Profile()
	p.kept = !p.ended && profile.Recommended != ""
	if !p.kept {
		profile.Recommended = placement.Evaluate(a.Course, p.passed)
	}
	a.SaveErr(a.Store.SaveProfile())
	p.offer = true
	return p
}

func (p *placementScreen) View(a *App, w, h int) string {
	st := a.Style
	fg := func(s string) string { return st.Base().Foreground(st.Fg).Render(s) }
	dim := func(s string) string { return st.Base().Foreground(st.Dim).Render(s) }
	acc := func(s string) string { return st.Base().Foreground(st.Accent).Bold(true).Render(s) }
	prose := func(s string, f func(string) string) []string {
		var out []string
		for _, l := range strings.Split(ansi.Wordwrap(s, max(w-4, 10), ""), "\n") {
			out = append(out, f(l))
		}
		return out
	}
	frame := func(title string, body []string, hint string) string {
		return style.Frame(st, w, h, title, " "+strings.Join(body, "\n "), hint)
	}

	if p.hands {
		return p.text.view(a, w, h)
	}
	if !p.offer {
		cur := p.stages[p.at]
		body := []string{"", acc(fmt.Sprintf(text.PlacementStageIntro, p.at+1, len(p.stages), p.stageTitle(a, cur))), ""}
		body = append(body, prose(text.PlacementRules, fg)...)
		if cur.ID == "option" {
			body = append(body, "")
			body = append(body, prose(text.PlacementOptionNote, dim)...)
		}
		if p.last != nil {
			body = append(body, "")
			body = append(body, prose(fmt.Sprintf(text.PlacementPrevOK, score.CPM(p.last.WPM), p.last.Accuracy*100), dim)...)
		}
		return frame(text.PlacementTitle, body, text.PlacementStageHint)
	}

	body := []string{""}
	if p.fromZero(a) {
		body = append(body, prose(text.OfferFromZero, fg)...)
	} else {
		body = append(body, prose(text.OfferPassed(len(p.passed), len(p.stages)), fg)...)
	}
	if ls, ok := a.Course.Lesson(a.Store.Profile().Recommended); ok {
		body = append(body, "")
		body = append(body, prose(fmt.Sprintf(text.OfferStart, ls.Title, a.chapterTitle(ls.ChapterID)), acc)...)
	}
	body = append(body, "")
	body = append(body, prose(text.OfferPractice, dim)...)
	return frame(text.OfferTitle, body, text.OfferHint)
}
