// Package cli dispatches the tuipe command line.
package cli

import (
	"fmt"
	"io"

	"github.com/jiripisa/tuipe/internal/buildinfo"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// Run executes the command line and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer, runApp func() error, runUpgrade func(check bool) error) int {
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

func finish(stderr io.Writer, err error) int {
	if err != nil {
		fmt.Fprintln(stderr, "tuipe:", err)
		return 1
	}
	return 0
}
