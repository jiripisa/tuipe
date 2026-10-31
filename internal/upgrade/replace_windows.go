//go:build windows

package upgrade

import "os"

// swap moves the running exe aside (a running exe cannot be overwritten but
// can be renamed), then renames tmp into place. If that fails, the old exe is
// restored. The leftover .old file is removed by CleanupOld at the next start.
func swap(tmp, exe string) error {
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Rename(old, exe)
		return err
	}
	return nil
}
