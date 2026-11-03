package style_test

import (
	"image/color"
	"testing"

	"github.com/jiripisa/tuipe/internal/ui/style"
)

// distance is how far apart two colours are, summed over red, green and blue.
func distance(a, b color.Color) int {
	r1, g1, b1, _ := a.RGBA()
	r2, g2, b2, _ := b.RGBA()
	abs := func(x, y uint32) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return abs(r1, r2) + abs(g1, g2) + abs(b1, b2)
}

func TestCursorIsSofterThanTheAccent(t *testing.T) {
	for _, s := range style.All() {
		cur := s.Cursor()
		if distance(cur, s.Bg) == 0 {
			t.Errorf("%s: the cursor is invisible on the background", s.ID)
		}
		if distance(cur, s.Bg) >= distance(s.Accent, s.Bg)/2 {
			t.Errorf("%s: the cursor %v is not much softer than the accent %v", s.ID, cur, s.Accent)
		}
	}
}

func TestTintMixesIntoTheBackground(t *testing.T) {
	s := style.ByID(style.Editor)
	if got := s.Tint(s.Accent, 0); distance(got, s.Bg) != 0 {
		t.Errorf("share 0 gave %v, want the background", got)
	}
	if got := s.Tint(s.Accent, 1); distance(got, s.Accent) > 3*257 {
		t.Errorf("share 1 gave %v, want the accent", got)
	}
}
