package main

import (
	"errors"
	"os"

	"github.com/jiripisa/tuipe/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr,
		func() error { return errors.New("not implemented") },
		func(bool) error { return errors.New("not implemented") }))
}
