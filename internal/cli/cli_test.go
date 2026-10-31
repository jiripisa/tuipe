package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jiripisa/tuipe/internal/buildinfo"
	"github.com/jiripisa/tuipe/internal/cli"
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
			})
	}
	require(t, run() == 0 && app == 1)
	require(t, run("upgrade") == 0 && up == 1)
	require(t, run("upgrade", "--check") == 0 && up == 11)
	require(t, run("version") == 0 && strings.Contains(out.String(), buildinfo.Version))
	require(t, run("help") == 0)
	require(t, run("nonsense") == 2 && strings.Contains(errb.String(), "help"))
}
