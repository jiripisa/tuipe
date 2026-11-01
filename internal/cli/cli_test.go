package cli_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/jiripisa/tuipe/internal/buildinfo"
	"github.com/jiripisa/tuipe/internal/cli"
	"github.com/jiripisa/tuipe/internal/store"
)

func require(t *testing.T, cond bool) {
	t.Helper()
	if !cond {
		t.Fatal("requirement failed")
	}
}

func TestDispatch(t *testing.T) {
	var out, errb bytes.Buffer
	app, up := 0, 0
	run := func(a ...string) int {
		return cli.Run(a, &out, &errb,
			func() error { app++; return nil },
			func(check bool) error {
				if check {
					up += 10
				} else {
					up++
				}
				return nil
			},
			func(store.ResetScope) error { return nil })
	}
	require(t, run() == 0 && app == 1)
	require(t, run("upgrade") == 0 && up == 1)
	require(t, run("upgrade", "--check") == 0 && up == 11)
	require(t, run("version") == 0 && strings.Contains(out.String(), buildinfo.Version))
	require(t, run("help") == 0)
	require(t, run("nonsense") == 2 && strings.Contains(errb.String(), "help"))
}

func TestUpgradeRejectsUnknownArguments(t *testing.T) {
	for _, args := range [][]string{
		{"upgrade", "--chek"},
		{"upgrade", "--check", "extra"},
		{"upgrade", "x"},
	} {
		var out, errb bytes.Buffer
		ran := false
		code := cli.Run(args, &out, &errb,
			func() error { return nil },
			func(bool) error { ran = true; return nil },
			func(store.ResetScope) error { return nil })
		require(t, code == 2 && !ran)
		require(t, strings.Contains(errb.String(), "tuipe upgrade --check"))
	}
}

func TestResetReadsItsScope(t *testing.T) {
	both := store.ResetScope{Settings: true, Score: true}
	for _, c := range []struct {
		args []string
		want store.ResetScope
	}{
		{[]string{"reset", "--all"}, both},
		{[]string{"reset", "--settings"}, store.ResetScope{Settings: true}},
		{[]string{"reset", "--score"}, store.ResetScope{Score: true}},
		{[]string{"reset", "--settings", "--score"}, both},
		{[]string{"reset", "--score", "--all"}, both},
	} {
		var out, errb bytes.Buffer
		var got store.ResetScope
		code := cli.Run(c.args, &out, &errb,
			func() error { return nil },
			func(bool) error { return nil },
			func(s store.ResetScope) error { got = s; return nil })
		if code != 0 || got != c.want {
			t.Errorf("%v: code %d, scope %+v", c.args, code, got)
		}
	}
}

func TestResetWithoutAClearScopeDoesNothing(t *testing.T) {
	for _, args := range [][]string{
		{"reset"},
		{"reset", "--scor"},
		{"reset", "all"},
		{"reset", "--all", "--yes"},
	} {
		var out, errb bytes.Buffer
		ran := false
		code := cli.Run(args, &out, &errb,
			func() error { return nil },
			func(bool) error { return nil },
			func(store.ResetScope) error { ran = true; return nil })
		require(t, code == 2 && !ran)
		require(t, strings.Contains(errb.String(), "tuipe reset --all"))
	}
}

func TestResetFailureEndsWithAnError(t *testing.T) {
	var out, errb bytes.Buffer
	code := cli.Run([]string{"reset", "--all"}, &out, &errb,
		func() error { return nil },
		func(bool) error { return nil },
		func(store.ResetScope) error { return errors.New("nelze") })
	require(t, code == 1 && strings.Contains(errb.String(), "nelze"))
}
