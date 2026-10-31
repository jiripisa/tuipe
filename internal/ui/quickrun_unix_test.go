//go:build !windows

package ui_test

import (
	"os"
	"syscall"
	"testing"
)

func TestSIGHUPMidRoundKeepsResumeAndKeystrokes(t *testing.T) {
	testLeave(t, "", func() { _ = syscall.Kill(os.Getpid(), syscall.SIGHUP) })
}
