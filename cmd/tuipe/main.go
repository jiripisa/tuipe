package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jiripisa/tuipe/internal/cli"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/text"
	"github.com/jiripisa/tuipe/internal/upgrade"
)

// executablePath returns the real path of the running binary.
func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf(text.UpgradeNoExecutable, err)
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return exe, nil
}

func main() {
	if exe, err := executablePath(); err == nil {
		upgrade.CleanupOld(exe)
	}
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr,
		ui.Run,
		func(check bool) error {
			exe := ""
			if !check { // --check does not touch the binary
				var err error
				if exe, err = executablePath(); err != nil {
					return err
				}
			}
			return upgrade.Run(context.Background(), upgrade.NewClient(), exe, check, os.Stdout)
		}))
}
