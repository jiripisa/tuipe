//go:build !windows

package upgrade

import "os"

// swap atomically renames tmp over exe.
func swap(tmp, exe string) error {
	return os.Rename(tmp, exe)
}
