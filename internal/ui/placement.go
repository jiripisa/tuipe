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
// as a round, and at the end the job offer. The rounds are ordinary round
// screens whose end this screen controls.
type placementScreen struct {
	next   func(*App) Screen
	stages []placement.Stage
	at     int               // the stage that is next or running
	passed []placement.Stage // stages passed so far
	last   *score.Metrics    // result of the stage before, for the intro
	offer  bool              // the test is over and the job offer is shown
	placed int               // chapters the test newly completed
	hands  bool              // the hand-position text follows a result that starts from zero
	text   handText
}

// maxIgnoredAlt is how many Alt presses the Option stage puts up with. A user
// whose terminal turns Option into Alt who skipped the check would otherwise
// never get a wrong key logged, and the stage would never end.
const maxIgnoredAlt = 5

// NewPlacementScreen builds the placement test. next opens the screen that
// follows the job offer; the home screen when it is nil.
func NewPlacementScreen(a *App, next func(*App) Screen) Screen {
	p := &placementScreen{next: next, stages: placement.Stages(a.Curr, a.Lib, a.Now().UnixNano())}
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
	case p.offer && k.String() == "enter" && len(a.Store.Profile().Completed) == 0:
		p.hands = true // starting from zero, as after the fork
	case p.offer && k.String() == "enter":
		return p.leave(a), nil
	case !p.offer && k.String() == "enter":
		return p.round(a), nil
	case !p.offer && k.String() == "esc":
		return p.finish(a), nil
	}
	return p, nil
}

func (p *placementScreen) leave(a *App) Screen {
	if p.next == nil {
		return NewHomeScreen(a)
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
		Session:  sessionPlacement,
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

// finish completes the chapters the passed stages cover and shows the offer.
// It runs on top of what is completed, so the rank cannot fall.
func (p *placementScreen) finish(a *App) Screen {
	out := placement.Evaluate(a.Curr, p.passed)
	profile := a.Store.Profile()
	p.placed = 0
	for _, id := range out.Completed {
		if !a.Curr.IsDone(profile, id) {
			p.placed++
		}
	}
	a.Curr.ApplySkip(profile, a.stats(), out.Completed)
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
			body = append(body, prose(fmt.Sprintf(text.PlacementPrevOK, p.last.WPM, p.last.Accuracy*100), dim)...)
		}
		return frame(text.PlacementTitle, body, text.PlacementStageHint)
	}

	profile := a.Store.Profile()
	rank := a.Curr.RankOf(profile).String()
	start := a.Curr.CurrentChapter(profile)
	body := []string{"", fg(text.OfferGreeting)}
	switch {
	case len(profile.Completed) == 0:
		body = append(body, prose(text.OfferFromZero, fg)...)
		body = append(body, "", fg(fmt.Sprintf(text.OfferHolding, rank)))
	case p.placed > 0:
		body = append(body, prose(fmt.Sprintf(text.OfferRank, rank), acc)...)
	default:
		body = append(body, prose(fmt.Sprintf(text.OfferNoChange, rank), fg)...)
	}
	body = append(body, "")
	body = append(body, prose(fmt.Sprintf(text.OfferStart, start.Number, start.Title), fg)...)
	if p.placed > 0 {
		body = append(body, "")
		body = append(body, prose(fmt.Sprintf(text.OfferSkipped, p.placed), dim)...)
	}
	return frame(text.OfferTitle, body, text.OfferHint)
}
