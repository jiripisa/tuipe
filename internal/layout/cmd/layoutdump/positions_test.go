package main

import (
	"slices"
	"testing"

	"github.com/jiripisa/tuipe/internal/layout/gen"
)

// TestPositionsMatchTheGenerator checks that the dumper knows exactly the keys
// the generator lays out, and that no XKB key name or scan code stands for two
// of them.
func TestPositionsMatchTheGenerator(t *testing.T) {
	var want []string
	for _, row := range gen.Positions() {
		for _, p := range row {
			want = append(want, p.ID)
		}
	}
	var got []string
	xkb := map[string]string{}
	scan := map[uint16]string{}
	for _, p := range positions {
		got = append(got, p.ID)
		if other, ok := xkb[p.XKB]; ok {
			t.Errorf("XKB name %s is both %s and %s", p.XKB, other, p.ID)
		}
		xkb[p.XKB] = p.ID
		if other, ok := scan[p.Scan]; ok {
			t.Errorf("scan code %#x is both %s and %s", p.Scan, other, p.ID)
		}
		scan[p.Scan] = p.ID
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("ids %v, want those of gen/positions.toml %v", got, want)
	}
}

// TestEdgeKeys pins the keys whose place differs between keyboards.
func TestEdgeKeys(t *testing.T) {
	want := map[string]struct {
		xkb  string
		scan uint16
	}{
		"k10": {"TLDE", 0x29}, "k50": {"LSGT", 0x56}, "k42": {"BKSL", 0x2B},
		"k36": {"RTRN", 0x1C}, "k49": {"SPCE", 0x39},
	}
	for _, p := range positions {
		if w, ok := want[p.ID]; ok && (p.XKB != w.xkb || p.Scan != w.scan) {
			t.Errorf("%s: %s %#x, want %s %#x", p.ID, p.XKB, p.Scan, w.xkb, w.scan)
		}
	}
}
