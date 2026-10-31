package engine_test

import (
	"testing"
	"time"

	"github.com/jiripisa/tuipe/internal/engine"
)

func require(t *testing.T, cond bool) {
	t.Helper()
	if !cond {
		t.Fatal("requirement failed")
	}
}

func TestStopOnError(t *testing.T) {
	r := engine.New(engine.Prepare("ab"))
	t0 := time.Unix(0, 0)
	require(t, r.Press('x', t0) == false && r.Pos() == 0 && r.WrongAtPos() == 1)
	require(t, r.Press('a', t0.Add(time.Second)) && r.Pos() == 1 && r.WrongAtPos() == 0)
}

func TestTimerStartsOnFirstKeystroke(t *testing.T) {
	r := engine.New(engine.Prepare("ab"))
	t0 := time.Unix(100, 0)
	require(t, !r.Started() && r.Elapsed(t0) == 0)
	r.Press('a', t0)
	require(t, r.Elapsed(t0.Add(3*time.Second)) == 3*time.Second)
}

func TestPauseIsNotCounted(t *testing.T) {
	r := engine.New(engine.Prepare("abc"))
	t0 := time.Unix(0, 0)
	r.Press('a', t0)
	r.Pause(t0.Add(time.Second))
	require(t, r.Paused())
	r.Resume(t0.Add(time.Hour))
	require(t, !r.Paused())
	require(t, r.Elapsed(t0.Add(time.Hour+time.Second)) == 2*time.Second && r.WasPaused())
}

func TestRetroactivePauseStart(t *testing.T) {
	r := engine.New(engine.Prepare("abc"))
	t0 := time.Unix(0, 0)
	r.Press('a', t0)
	r.PauseFrom(t0.Add(time.Second))
	r.Resume(t0.Add(time.Minute))
	require(t, r.Elapsed(t0.Add(time.Minute)) == time.Second)
}

func TestNewlineSkipsIndentation(t *testing.T) {
	r := engine.New(engine.Prepare("a\n    b\n"))
	t0 := time.Unix(0, 0)
	r.Press('a', t0)
	r.Press('\n', t0)
	require(t, r.Press('b', t0)) // the four spaces are Auto
	require(t, !r.Done())
	r.Press('\n', t0)
	require(t, r.Done())
}

func TestBackspaceAndNoPressesAfterDoneDoNothing(t *testing.T) {
	r := engine.New(engine.Prepare("a"))
	t0 := time.Unix(0, 0)
	require(t, !r.Press('\b', t0) && !r.Started() && len(r.Log()) == 0)
	require(t, r.Press('a', t0) && r.Done())
	n := len(r.Log())
	require(t, !r.Press('a', t0) && !r.Press('\b', t0) && len(r.Log()) == n)
}

func TestLogAndPrepare(t *testing.T) {
	txt := engine.Prepare("\tx\n\ty")
	require(t, txt.Cells[0].Auto == false && len(txt.Hash) == 64)
	require(t, txt.Cells[3].Auto && !txt.Cells[4].Auto)
	r := engine.New(engine.Prepare("ab"))
	t0 := time.Unix(0, 0)
	r.Press('x', t0)
	r.Press('a', t0.Add(time.Second))
	log := r.Log()
	require(t, len(log) == 2 && !log[0].Correct && log[0].Pos == 0 && log[0].Typed == 'x')
	require(t, log[1].Correct && log[1].T == time.Second)
}

func TestFirstKeystrokeAfterResumeIsMarked(t *testing.T) {
	r := engine.New(engine.Prepare("abc"))
	base := time.Unix(1000, 0)
	r.Press('a', base)
	r.PauseFrom(base.Add(time.Second))
	r.Resume(base.Add(time.Hour))
	r.Press('b', base.Add(time.Hour+time.Second))
	r.Press('c', base.Add(time.Hour+2*time.Second))
	log := r.Log()
	if log[0].AfterPause || !log[1].AfterPause || log[2].AfterPause {
		t.Fatalf("AfterPause flags: %+v", log)
	}
}
