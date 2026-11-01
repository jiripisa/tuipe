package store

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"

	"github.com/jiripisa/tuipe/internal/ui/text"
)

// ResetScope says what `tuipe reset` removes.
type ResetScope struct {
	Settings bool // settings and the user's layouts
	Score    bool // progress, history and ghosts
}

// Only what tuipe wrote is removed, so a folder shared with anything else
// keeps the rest. A name also covers what is derived from it, such as a file
// set aside as corrupted or an unfinished write.
var (
	settingsNames = []string{"config.toml", "layouts"}
	scoreNames    = []string{profileFile, runsFile, ghostsDir}
)

// Reset names what the scope would remove, asks on out, reads the answer from
// in, and removes it only after the answer is yes. It holds the lock from before
// the question, so a running tuipe refuses it and none can start under it.
func Reset(configDir, dataDir string, scope ResetScope, in io.Reader, out io.Writer) error {
	var targets []string
	if scope.Settings {
		found, err := named(configDir, settingsNames)
		if err != nil {
			return err
		}
		targets = append(targets, found...)
	}
	if scope.Score {
		found, err := named(dataDir, scoreNames)
		if err != nil {
			return err
		}
		targets = append(targets, found...)
	}
	if len(targets) == 0 {
		fmt.Fprintln(out, text.ResetNothing)
		return nil
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf(text.StoreNoDir, dataDir)
	}
	lock, err := takeLock(dataDir)
	if errors.Is(err, ErrLocked) {
		return errors.New(text.SecondInstance)
	}
	if err != nil {
		return errors.New(text.ResetFailedPrefix + dataDir)
	}
	defer unlock(lock)

	fmt.Fprintln(out, text.ResetHeader)
	for _, t := range targets {
		fmt.Fprintln(out, "  "+t)
	}
	fmt.Fprint(out, "\n"+text.ResetQuestion)
	if !confirmed(in) {
		fmt.Fprintln(out, text.ResetCancelled)
		return nil
	}

	var failed []string
	for _, t := range targets {
		if err := os.RemoveAll(t); err != nil {
			failed = append(failed, t)
		}
	}
	if len(failed) > 0 {
		return errors.New(text.ResetFailedPrefix + strings.Join(failed, ", "))
	}
	if scope.Score {
		fmt.Fprintln(out, text.ResetDoneScore)
	} else {
		fmt.Fprintln(out, text.ResetDoneSettings)
	}
	return nil
}

// named lists what dir holds under the names. A missing folder holds nothing.
func named(dir string, names []string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New(text.ResetFailedPrefix + dir)
	}
	var out []string
	for _, e := range entries {
		for _, n := range names {
			if e.Name() == n || strings.HasPrefix(e.Name(), n+".") {
				out = append(out, filepath.Join(dir, e.Name()))
				break
			}
		}
	}
	return out, nil
}

// confirmed reads one line and takes only the full word as a yes.
func confirmed(in io.Reader) bool {
	line, _ := bufio.NewReader(in).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(line), text.ResetYes)
}

func unlock(l *flock.Flock) { _ = l.Unlock() }
