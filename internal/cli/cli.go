// Package cli dispatches the tuipe command line.
package cli

import (
	"fmt"
	"io"

	"github.com/jiripisa/tuipe/internal/buildinfo"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// Run executes the command line and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer, runApp func() error, runUpgrade func(check bool) error, runReset func(store.ResetScope) error) int {
	if len(args) == 0 {
		return finish(stderr, runApp())
	}
	switch args[0] {
	case "upgrade":
		switch {
		case len(args) == 1:
			return finish(stderr, runUpgrade(false))
		case len(args) == 2 && args[1] == "--check":
			return finish(stderr, runUpgrade(true))
		default:
			fmt.Fprint(stderr, text.Usage)
			return 2
		}
	case "reset":
		scope, ok := resetScope(args[1:])
		if !ok {
			fmt.Fprint(stderr, text.Usage)
			return 2
		}
		return finish(stderr, runReset(scope))
	case "version":
		fmt.Fprintln(stdout, buildinfo.String())
		return 0
	case "help":
		fmt.Fprint(stdout, text.Usage)
		return 0
	default:
		fmt.Fprintf(stderr, text.UnknownCommand, args[0])
		return 2
	}
}

// resetScope reads the flags of `tuipe reset`. It needs at least one, so that
// nothing is deleted on a bare `tuipe reset`.
func resetScope(flags []string) (store.ResetScope, bool) {
	var s store.ResetScope
	for _, f := range flags {
		switch f {
		case "--all":
			s.Settings, s.Score = true, true
		case "--settings":
			s.Settings = true
		case "--score":
			s.Score = true
		default:
			return store.ResetScope{}, false
		}
	}
	return s, s.Settings || s.Score
}

func finish(stderr io.Writer, err error) int {
	if err != nil {
		fmt.Fprintln(stderr, "tuipe:", err)
		return 1
	}
	return 0
}
