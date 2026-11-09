//go:build windows

package main

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestAKLIDIsEightHexDigits(t *testing.T) {
	for _, klid := range []string{"405", "0000040G", "000000405"} {
		var out, errOut bytes.Buffer
		if code := run([]string{"windows", klid}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "8 hex digits") {
			t.Errorf("%s: exit %d, stderr %q", klid, code, errOut.String())
		}
	}
}

func TestALayoutWindowsLacksFails(t *testing.T) {
	if _, err := dumpWindows([]string{"0000FFFF"}); err == nil {
		t.Error("a layout 0000FFFF was dumped")
	}
}

// TestDumpingTwiceGivesTheSameAndKeepsTheLayoutList checks that no dead key
// is left pending from one dump to the next, and that the layout loaded for
// the dump is gone afterwards.
func TestDumpingTwiceGivesTheSameAndKeepsTheLayoutList(t *testing.T) {
	before, err := loadedLayouts()
	if err != nil {
		t.Fatal(err)
	}
	first, err := dumpWindows([]string{"00000405"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := dumpWindows([]string{"00000405"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Error("a second dump differs from the first")
	}
	if len(first.Dead) == 0 {
		t.Error("the Czech layout has no dead key")
	}
	after, err := loadedLayouts()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before, after) {
		t.Errorf("layouts %#x before the dumps, %#x after", before, after)
	}
}
