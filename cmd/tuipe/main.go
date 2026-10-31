package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/jiripisa/tuipe/internal/cli"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/upgrade"
)

func main() {
	exe, exeErr := os.Executable()
	if exeErr == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		upgrade.CleanupOld(exe)
	}
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr,
		ui.Run,
		func(check bool) error {
			if exeErr != nil {
				return exeErr
			}
			return upgrade.Run(context.Background(), upgrade.NewClient(), exe, check, os.Stdout)
		}))
}
