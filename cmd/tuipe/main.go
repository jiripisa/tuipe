package main

import (
	"errors"
	"os"

	"github.com/jiripisa/tuipe/internal/cli"
	"github.com/jiripisa/tuipe/internal/ui"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr,
		ui.Run,
		func(bool) error { return errors.New("not implemented") }))
}
