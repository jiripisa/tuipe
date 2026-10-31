package play_test

import (
	"slices"
	"testing"
	"time"

	"github.com/jiripisa/tuipe/internal/ghost"
	"github.com/jiripisa/tuipe/internal/incident"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/stats"
)

var t0 = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

func at(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }

func eq[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func czech(t *testing.T) *layout.Layout {
	t.Helper()
	all, err := layout.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range all {
		if l.ID == "macos-czech-qwerty" {
			return l
		}
	}
	t.Fatal("czech layout missing")
	return nil
}

// solid returns stats where every rune of text is Solid, so hints depend on behaviour only.
func solid(text string) *stats.Stats {
	s := stats.New()
	for _, r := range text {
		s.SetMastery(r, stats.Solid)
	}
	return s
}

func newSession(t *testing.T, spec play.Spec, focusEvents bool) *play.Session {
	t.Helper()
	if spec.Text == "" {
		spec.Text = "abcd"
	}
	return play.New(spec, czech(t), solid(spec.Text), focusEvents)
}

func press(s *play.Session, r rune, ms int) { s.Key(play.Input{Rune: r}, at(ms)) }

func TestBlurPausesAndFocusResumesOnKey(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'a', 0)
	s.Focus(false, at(1000))
	eq(t, s.Frame(at(2000)).Paused, true)
	s.Focus(true, at(5000))
	eq(t, s.Frame(at(5000)).Paused, true) // focus return does not resume by itself
	press(s, 'b', 6000)                   // the key resumes and counts
	f := s.Frame(at(6000))
	eq(t, f.Paused, false)
	eq(t, f.Pos, 2)
	press(s, 'c', 6500)
	press(s, 'd', 7000)
	r := s.Result(at(7000))
	eq(t, r.Paused, true)
	eq(t, r.Metrics.Duration, 2000*time.Millisecond) // 0..1000 and 6000..7000
	eq(t, len(r.Log), 4)
	eq(t, r.Log[1].T, 1000*time.Millisecond)
}

func TestPausedRoundNeverReplacesGhost(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'a', 0)
	s.Pause(at(100))
	s.Resume(at(900))
	press(s, 'b', 1000)
	press(s, 'c', 1100)
	press(s, 'd', 1200)
	r := s.Result(at(1200))
	eq(t, r.Paused, true)
	eq(t, ghost.ShouldReplace(nil, r.Metrics, r.Paused, false), false)
}

func TestUnpausedRoundHasPausedFalse(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	for i, r := range "abcd" {
		press(s, r, i*100)
	}
	eq(t, s.Result(at(300)).Paused, false)
	eq(t, s.Done(), true)
}

func TestIdleAutoPauseOnlyWithoutFocusEvents(t *testing.T) {
	s := newSession(t, play.Spec{}, false)
	press(s, 'a', 0)
	press(s, 'b', 1000)
	s.Tick(at(4000))
	eq(t, s.Frame(at(4000)).Paused, false)
	s.Tick(at(6500))
	eq(t, s.Frame(at(6500)).Paused, true)
	press(s, 'c', 9000)
	press(s, 'd', 9500)
	r := s.Result(at(9500))
	eq(t, r.Paused, true)
	eq(t, r.Metrics.Duration, 1500*time.Millisecond) // 0..1000 (retroactive to the last key) + 500
	eq(t, r.Log[2].T, 1000*time.Millisecond)

	s = newSession(t, play.Spec{}, true)
	press(s, 'a', 0)
	press(s, 'b', 1000)
	s.Tick(at(4000))
	s.Tick(at(6500))
	eq(t, s.Frame(at(6500)).Paused, false)
}

func TestNoIdlePauseAfterAFocusCall(t *testing.T) {
	s := newSession(t, play.Spec{}, false)
	s.Focus(true, at(0))
	press(s, 'a', 100)
	s.Tick(at(3000))
	s.Tick(at(6500))
	eq(t, s.Frame(at(6500)).Paused, false)
}

func TestNoIdlePauseBeforeFirstKey(t *testing.T) {
	s := newSession(t, play.Spec{}, false)
	s.Tick(at(0))
	s.Tick(at(4000))
	s.Tick(at(9000))
	eq(t, s.Frame(at(9000)).Paused, false)
}

func TestMachineSleepGapIsNotCounted(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'a', 0)
	s.Tick(at(500))
	s.Tick(at(1000))
	hour := int(time.Hour / time.Millisecond)
	s.Tick(at(1000 + hour))
	eq(t, s.Frame(at(1000+hour)).Paused, true)
	press(s, 'b', 2000+hour)
	press(s, 'c', 2500+hour)
	press(s, 'd', 3000+hour)
	r := s.Result(at(3000 + hour))
	eq(t, r.Paused, true)
	eq(t, r.Metrics.Duration, 2000*time.Millisecond) // 0..1000 + 1000
}

func TestSleepGapPausesFromLastTick(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'a', 0)
	s.Tick(at(200))
	press(s, 'b', 1000) // later than the last tick
	s.Tick(at(60000))
	press(s, 'c', 61000)
	press(s, 'd', 61500)
	eq(t, s.Result(at(61500)).Metrics.Duration, 1500*time.Millisecond) // 0..1000 + 500
}

func TestPauseFromIsClampedToLastKeystroke(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'a', 0)
	press(s, 'b', 1000)
	s.Focus(false, at(500)) // earlier than the last keystroke: must not rewind the clock
	s.Resume(at(2000))
	press(s, 'c', 2100)
	r := s.Result(at(2100))
	eq(t, r.Log[1].T, 1000*time.Millisecond)
	eq(t, r.Log[2].T, 1100*time.Millisecond) // 1000 clamped pause point + 100
}

func TestAltKeyInRoundIsNotAnError(t *testing.T) {
	s := newSession(t, play.Spec{Text: "{a"}, true)
	s.Key(play.Input{Rune: 'í', Alt: true}, at(0))
	f := s.Frame(at(0))
	eq(t, f.AltWarning, true)
	eq(t, f.Pos, 0)
	eq(t, len(s.Result(at(0)).Log), 0)
	press(s, '{', 100)
	eq(t, s.Frame(at(100)).AltWarning, false)
	eq(t, s.Frame(at(100)).Pos, 1)
}

func TestAltKeyProducingTheExpectedRuneIsCorrect(t *testing.T) {
	s := newSession(t, play.Spec{Text: "{a"}, true)
	s.Key(play.Input{Rune: '{', Alt: true}, at(0))
	f := s.Frame(at(0))
	eq(t, f.AltWarning, false)
	eq(t, f.Pos, 1)
}

func TestHintUsedRecordedOnce(t *testing.T) {
	s := play.New(play.Spec{Text: "ab"}, czech(t), stats.New(), true) // New mastery: hint at once
	s.Tick(at(0))
	eq(t, s.Result(at(0)).HintUsed, true)
	f := s.Frame(at(0))
	if f.Hint == nil || f.Hint.Rune != 'a' || f.Hint.Text == "" || len(f.Hint.Route) == 0 {
		t.Fatalf("bad hint: %+v", f.Hint)
	}
}

func TestHintUsedRecordedWithoutRender(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	eq(t, s.Result(at(0)).HintUsed, false)
	press(s, 'x', 0)
	press(s, 'x', 100) // second wrong press shows a hint
	eq(t, s.Result(at(100)).HintUsed, true)
}

func TestHintsOffWhenDisabled(t *testing.T) {
	s := play.New(play.Spec{Text: "ab", HintsOff: true}, czech(t), stats.New(), true)
	s.Tick(at(0))
	press(s, 'x', 10)
	press(s, 'x', 20)
	eq(t, s.Frame(at(20)).Hint == nil, true)
	eq(t, s.Result(at(20)).HintUsed, false)
}

func TestFirstWrongPressShowsNoHint(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'x', 0)
	eq(t, s.Frame(at(0)).Hint == nil, true)
	eq(t, s.Result(at(0)).HintUsed, false)
}

func TestSecondWrongPressShowsHint(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'x', 0)
	press(s, 'y', 100)
	f := s.Frame(at(100))
	if f.Hint == nil {
		t.Fatal("expected a hint")
	}
	eq(t, f.Hint.Rune, 'a')
}

func TestHesitationOnlyForTheCurrentPosition(t *testing.T) {
	s := newSession(t, play.Spec{}, true) // Solid: hint after 2 s of hesitation
	s.Tick(at(0))
	press(s, 'a', 1500)
	s.Tick(at(3000)) // 1.5 s since the last correct key
	eq(t, s.Frame(at(3000)).Hint == nil, true)
	eq(t, s.Result(at(3000)).HintUsed, false)
	s.Tick(at(3600)) // 2.1 s
	f := s.Frame(at(3600))
	if f.Hint == nil || f.Hint.Rune != 'b' {
		t.Fatalf("expected hint for b, got %+v", f.Hint)
	}
	press(s, 'b', 3700)
	eq(t, s.Frame(at(3700)).Hint == nil, true)
}

func TestHesitationBeforeFirstKeyCountsFromFirstTick(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	s.Tick(at(1000))
	s.Tick(at(2500))
	eq(t, s.Frame(at(2500)).Hint == nil, true)
	s.Tick(at(3100))
	eq(t, s.Frame(at(3100)).Hint != nil, true)
}

func TestHesitationExcludesPauses(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'a', 0)
	s.Pause(at(500))
	s.Resume(at(10500))
	s.Tick(at(11000)) // 0.5 s + 0.5 s of active hesitation
	eq(t, s.Frame(at(11000)).Hint == nil, true)
	s.Tick(at(12600)) // 2.1 s
	eq(t, s.Frame(at(12600)).Hint != nil, true)
}

func TestEscDuringAutoPauseBecomesManual(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'a', 0)
	s.Focus(false, at(100))
	f := s.Frame(at(150))
	eq(t, f.Paused, true)
	eq(t, f.ManualPause, false)
	s.Pause(at(200))
	press(s, 'b', 300) // ignored: the menu owns the keys
	f = s.Frame(at(300))
	eq(t, f.Paused, true)
	eq(t, f.ManualPause, true)
	eq(t, f.Pos, 1)
	s.Resume(at(400))
	press(s, 'b', 500)
	r := s.Result(at(500))
	eq(t, s.Frame(at(500)).Pos, 2)
	eq(t, len(r.Log), 2)
	eq(t, r.Log[1].T, 200*time.Millisecond) // paused from the blur at 100, 100 active before the key after the resume at 400
}

func TestKeysIgnoredDuringExplicitPause(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'a', 0)
	s.Pause(at(100))
	press(s, 'b', 200)
	f := s.Frame(at(200))
	eq(t, f.Paused, true)
	eq(t, f.Pos, 1)
	s.Resume(at(300))
	press(s, 'b', 400)
	eq(t, s.Frame(at(400)).Pos, 2)
	eq(t, len(s.Result(at(400)).Log), 2)
}

func TestExplicitPauseSurvivesBlurAndFocus(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'a', 0)
	s.Pause(at(100))
	s.Focus(false, at(200))
	s.Focus(true, at(300))
	press(s, 'b', 400)
	eq(t, s.Frame(at(400)).Pos, 1)
	eq(t, s.Frame(at(400)).Paused, true)
}

func TestLeadShownOnlyWithGhost(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'a', 0)
	f := s.Frame(at(100))
	eq(t, f.HasGhost, false)
	eq(t, f.Lead, time.Duration(0))

	g := ghost.Replay([]int64{0, 500, 1000, 1500})
	s = newSession(t, play.Spec{Ghost: g}, true)
	press(s, 'a', 0)
	press(s, 'b', 300)
	f = s.Frame(at(300))
	eq(t, f.HasGhost, true)
	eq(t, f.Lead, 200*time.Millisecond) // ghost finished b at 500, the player at 300
	eq(t, f.GhostPos, 1)
	eq(t, f.Pos, 2)
	press(s, 'c', 600)
	press(s, 'd', 900)
	d := s.Result(at(900)).GhostDelta
	if d == nil {
		t.Fatal("expected a ghost delta")
	}
	eq(t, *d, 600*time.Millisecond)
}

func TestResultGhostDeltaNilWithoutGhost(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	for i, r := range "abcd" {
		press(s, r, i*100)
	}
	eq(t, s.Result(at(300)).GhostDelta == nil, true)
}

func incidentSpec() play.Spec {
	p := incident.NewParams(incident.SEV3, 30, 4)
	return play.Spec{Kind: play.KindIncident, Incident: &p}
}

func TestIncidentBreachEndsRound(t *testing.T) {
	s := newSession(t, incidentSpec(), true)
	press(s, 'a', 0)
	for i := 0; i < 20; i++ {
		press(s, 'z', 100+i*10) // 'b' expected: wrong
	}
	press(s, 'q', 400)
	press(s, 'b', 500) // ignored after the breach
	f := s.Frame(at(500))
	eq(t, f.HasSLA, true)
	eq(t, f.Breached, true)
	eq(t, f.Done, true)
	eq(t, s.Done(), true)
	r := s.Result(at(500))
	if r.Incident == nil {
		t.Fatal("expected an incident outcome")
	}
	eq(t, r.Incident.Won, false)
	eq(t, r.Stars, 0)
	if !slices.Equal(r.Incident.DamageChars, []rune{'b'}) {
		t.Fatalf("damage chars: %q", string(r.Incident.DamageChars))
	}
}

func TestIncidentDamageCharsAreRankedAndCapped(t *testing.T) {
	// Wrong presses are counted against the expected character.
	p := incident.Params{Start: 100, SLO: 99, BurnPerSec: 0, ErrorBurn: 0.1}
	s := newSession(t, play.Spec{Text: "abcdefgh", Incident: &p}, true)
	ms := 0
	step := func(r rune) { ms += 10; press(s, r, ms) }
	for _, c := range []struct {
		expected rune
		wrong    int
	}{{'a', 1}, {'b', 2}, {'c', 3}, {'d', 1}, {'e', 1}, {'f', 1}, {'g', 1}} {
		for i := 0; i < c.wrong; i++ {
			step('#')
		}
		step(c.expected)
	}
	eq(t, s.Done(), true) // the tenth wrong press (at g) breached the SLO
	r := s.Result(at(ms))
	eq(t, r.Incident.Won, false)
	if got := string(r.Incident.DamageChars); got != "cbade" {
		t.Fatalf("damage chars: %q", got)
	}
}

func TestIncidentWinReportsMTTR(t *testing.T) {
	s := newSession(t, incidentSpec(), true)
	for i, r := range "abcd" {
		press(s, r, i*100)
	}
	eq(t, s.Done(), true)
	r := s.Result(at(300))
	if r.Incident == nil {
		t.Fatal("expected an incident outcome")
	}
	eq(t, r.Incident.Won, true)
	eq(t, r.Incident.MTTR, 300*time.Millisecond)
	eq(t, r.Incident.SLA > 99 && r.Incident.SLA <= 100, true)
	eq(t, len(r.Incident.DamageChars), 0)
}

func TestSLABurnsWithActiveTimeOnly(t *testing.T) {
	s := newSession(t, incidentSpec(), true)
	press(s, 'a', 0)
	s.Tick(at(1000))
	before := s.Frame(at(1000)).SLA
	s.Pause(at(1000))
	s.Tick(at(30000))
	eq(t, s.Frame(at(30000)).SLA, before)
	s.Resume(at(30000))
	s.Tick(at(31000))
	eq(t, s.Frame(at(31000)).SLA < before, true)
}

func TestWrongPressBurnsSLA(t *testing.T) {
	s := newSession(t, incidentSpec(), true)
	press(s, 'a', 0)
	before := s.Frame(at(0)).SLA
	press(s, 'x', 0)
	eq(t, s.Frame(at(0)).SLA < before, true)
}

func TestNoSLAOutsideIncident(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	eq(t, s.Frame(at(0)).HasSLA, false)
	eq(t, s.Result(at(0)).Incident == nil, true)
}

func TestFrameMetricsAndWeak(t *testing.T) {
	s := newSession(t, play.Spec{Text: "abcdefgh"}, true)
	press(s, 'a', 0)
	press(s, 'x', 100) // b: 3 wrong
	press(s, 'x', 150)
	press(s, 'x', 200)
	press(s, 'b', 300)
	press(s, 'y', 400) // c: 1 wrong
	press(s, 'c', 500)
	press(s, 'x', 600) // d: 2 wrong
	press(s, 'x', 650)
	press(s, 'd', 700)
	press(s, 'x', 800) // e: 1 wrong
	press(s, 'e', 900)
	press(s, 'x', 1000) // f: 1 wrong
	press(s, 'f', 1100)
	f := s.Frame(at(1200))
	if len(f.Weak) != 4 || f.Weak[0] != 'b' || f.Weak[1] != 'd' {
		t.Fatalf("weak: %q", string(f.Weak))
	}
	eq(t, f.WrongPos[1], true)
	eq(t, f.WrongPos[0], false)
	want := score.Compute(s.Result(at(1200)).Text, s.Result(at(1200)).Log, 1200*time.Millisecond)
	eq(t, f.WPM, want.WPM)
	eq(t, f.Accuracy, want.Accuracy)
	eq(t, len(f.Cells), 8)
}

func TestStarsFromTargets(t *testing.T) {
	spec := play.Spec{Targets: score.Targets{Speed: [3]float64{1, 2, 3}}}
	s := newSession(t, spec, true)
	for i, r := range "abcd" {
		press(s, r, i*100)
	}
	// 4 chars in 0.3 s is far above every target, but solid stats mean no hint
	eq(t, s.Result(at(300)).Stars, 3)
}

func TestPartialLog(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'a', 0)
	press(s, 'x', 100)
	text, log := s.PartialLog()
	eq(t, len(text.Cells), 4)
	eq(t, len(log), 2)
	eq(t, log[1].Correct, false)
}

func TestKeysAfterDoneAreIgnored(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	for i, r := range "abcd" {
		press(s, r, i*100)
	}
	press(s, 'x', 500)
	eq(t, len(s.Result(at(500)).Log), 4)
}

func sleepThen(t *testing.T, spec play.Spec, event func(s *play.Session, wake int)) (*play.Session, int) {
	t.Helper()
	s := newSession(t, spec, true)
	press(s, 'a', 0)
	s.Tick(at(1000))
	wake := 1000 + int(time.Hour/time.Millisecond)
	event(s, wake)
	return s, wake
}

func TestSleepThenFocusTrueFirstIsStillAGap(t *testing.T) {
	s, wake := sleepThen(t, play.Spec{}, func(s *play.Session, wake int) { s.Focus(true, at(wake)) })
	eq(t, s.Frame(at(wake)).Paused, true)
	press(s, 'b', wake+1000)
	press(s, 'c', wake+1500)
	r := s.Result(at(wake + 1500))
	eq(t, r.Paused, true)
	eq(t, r.Metrics.Duration, 1500*time.Millisecond)
}

func TestSleepThenFocusFalseFirstIsStillAGap(t *testing.T) {
	s, wake := sleepThen(t, play.Spec{}, func(s *play.Session, wake int) { s.Focus(false, at(wake)) })
	press(s, 'b', wake+1000)
	press(s, 'c', wake+1500)
	r := s.Result(at(wake + 1500))
	eq(t, r.Paused, true)
	eq(t, r.Metrics.Duration, 1500*time.Millisecond)
}

func TestSleepThenPauseFirstIsStillAGap(t *testing.T) {
	s, wake := sleepThen(t, play.Spec{}, func(s *play.Session, wake int) { s.Pause(at(wake)) })
	f := s.Frame(at(wake))
	eq(t, f.ManualPause, true)
	s.Resume(at(wake + 1000))
	press(s, 'b', wake+1000)
	press(s, 'c', wake+1500)
	r := s.Result(at(wake + 1500))
	eq(t, r.Paused, true)
	eq(t, r.Metrics.Duration, 1500*time.Millisecond)
}

func TestSleepThenFocusDoesNotBreachIncident(t *testing.T) {
	s, wake := sleepThen(t, incidentSpec(), func(s *play.Session, wake int) { s.Focus(true, at(wake)) })
	press(s, 'b', wake+1000)
	f := s.Frame(at(wake + 1000))
	eq(t, f.Breached, false)
	eq(t, f.Done, false)
	eq(t, f.Pos, 2)
	eq(t, f.SLA > 99.5, true)
}

func TestFrameCarriesTheSLOOfItsMeter(t *testing.T) {
	p := incident.Params{Start: 100, SLO: 90, BurnPerSec: 1, ErrorBurn: 0}
	s := newSession(t, play.Spec{Text: "abcd", Incident: &p}, true)
	eq(t, s.Frame(at(0)).SLO, 90.0)
	s = newSession(t, play.Spec{Text: "abcd"}, true)
	eq(t, s.Frame(at(0)).SLO, 0.0)
}

func TestIdleRetroactivePauseRefundsSLA(t *testing.T) {
	p := incident.Params{Start: 100, SLO: 90, BurnPerSec: 1, ErrorBurn: 0}
	s := newSession(t, play.Spec{Text: "abcd", Incident: &p}, false)
	press(s, 'a', 0)
	press(s, 'b', 1000)
	for ms := 2000; ms <= 5000; ms += 1000 {
		s.Tick(at(ms))
	}
	eq(t, s.Frame(at(5000)).SLA, 95.0)
	s.Tick(at(6000)) // idle for 5 s: paused retroactively from the last key
	f := s.Frame(at(6000))
	eq(t, f.Paused, true)
	eq(t, f.SLA, 99.0) // only the 1 s of active time burned
}

func TestOutageCannotLandDuringAPause(t *testing.T) {
	p := incident.Params{Start: 100, SLO: 99, BurnPerSec: 1, ErrorBurn: 0}
	s := newSession(t, play.Spec{Text: "abcd", Incident: &p}, true)
	press(s, 'a', 0)
	s.Pause(at(2000)) // 2 s burned since the last tick: breached before pausing
	eq(t, s.Done(), true)
	eq(t, s.Result(at(2000)).Incident.Won, false)
}

func TestNonPrintableKeyDoesNothing(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'a', 0)
	s.Focus(false, at(100))
	press(s, '\b', 500)
	press(s, 127, 600)
	eq(t, s.Frame(at(600)).Paused, true)
	eq(t, len(s.Result(at(600)).Log), 1)
}

func TestFrameDoesNotShowAHintTickHasNotRecorded(t *testing.T) {
	s := newSession(t, play.Spec{}, true) // Solid: hint after 2 s of hesitation
	s.Tick(at(0))
	eq(t, s.Frame(at(5000)).Hint == nil, true)
	eq(t, s.Result(at(5000)).HintUsed, false)

	n := play.New(play.Spec{Text: "ab"}, czech(t), stats.New(), true)
	eq(t, n.Frame(at(0)).Hint == nil, true) // never ticked
	n.Tick(at(0))
	eq(t, n.Frame(at(0)).Hint != nil, true)
	eq(t, n.Result(at(0)).HintUsed, true)
}

func TestUnfinishedIncidentIsNotWon(t *testing.T) {
	s := newSession(t, incidentSpec(), true)
	press(s, 'a', 0)
	r := s.Result(at(100))
	eq(t, r.Incident.Won, false)
	eq(t, r.Stars, 0)
}

func TestFirstKeyAfterPauseIsMarked(t *testing.T) {
	s := newSession(t, play.Spec{}, true)
	press(s, 'a', 0)
	s.Focus(false, at(100))
	press(s, 'b', 5000)
	press(s, 'c', 5100)
	log := s.Result(at(5100)).Log
	eq(t, log[1].AfterPause, true)
	eq(t, log[2].AfterPause, false)
}
