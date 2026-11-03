package text_test

import (
	"testing"

	"github.com/jiripisa/tuipe/internal/ui/text"
)

func TestCzechPlurals(t *testing.T) {
	for n, want := range map[int]string{0: "0 kol", 1: "1 kolo", 2: "2 kola", 4: "4 kola", 5: "5 kol", 21: "21 kol"} {
		if got := text.Rounds(n); got != want {
			t.Errorf("Rounds(%d) = %q, want %q", n, got, want)
		}
	}
	for n, want := range map[int]string{1: "1 kapitola", 3: "3 kapitoly", 12: "12 kapitol"} {
		if got := text.Chapters(n); got != want {
			t.Errorf("Chapters(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestOfferSkippedAgreesWithItsNumber(t *testing.T) {
	for n, want := range map[int]string{
		1: "Odemčena 1 kapitola před nástupem, bez hvězd. Její lekce zůstávají otevřené.",
		3: "Odemčeny 3 kapitoly před nástupem, bez hvězd. Jejich lekce zůstávají otevřené.",
		6: "Odemčeno 6 kapitol před nástupem, bez hvězd. Jejich lekce zůstávají otevřené.",
	} {
		if got := text.OfferSkipped(n); got != want {
			t.Errorf("OfferSkipped(%d) = %q", n, got)
		}
	}
}

func TestPartCompare(t *testing.T) {
	for _, c := range []struct {
		cpm, points float64
		against     string
		want        string
	}{
		{12, 1.5, text.PartVsLast, "+12 zn/min oproti minule · přesnost +1,5 p. b."},
		{-30, -0.4, text.PartVsBest, "-30 zn/min oproti nejlepšímu · přesnost -0,4 p. b."},
		{0, 0, text.PartVsLast, "0 zn/min oproti minule · přesnost 0 p. b."},
	} {
		if got := text.PartCompare(c.cpm, c.points, c.against); got != c.want {
			t.Errorf("PartCompare(%v, %v) = %q, want %q", c.cpm, c.points, got, c.want)
		}
	}
}

func TestPartFigures(t *testing.T) {
	if got := text.PartPercent(0.974); got != "97,4 %" {
		t.Errorf("PartPercent = %q", got)
	}
	if got := text.PartPercent(1); got != "100,0 %" {
		t.Errorf("PartPercent = %q", got)
	}
	for sec, want := range map[int]string{0: "0:00", 9: "0:09", 88: "1:28", 600: "10:00"} {
		if got := text.PartClock(sec); got != want {
			t.Errorf("PartClock(%d) = %q, want %q", sec, got, want)
		}
	}
}

func TestPartMissedListNamesWhatCannotBeSeen(t *testing.T) {
	want := text.PartMissed + ": mezera, Enter, čárka"
	if got := text.PartMissedList([]rune{' ', '\n', ','}); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSkippedContentNamesWhatIsThere(t *testing.T) {
	if got := text.SkippedContent("X", 0); got != "" {
		t.Errorf("nothing skipped: %q", got)
	}
	want := "Rozložení X neumí některé znaky. Přeskočeno: 5 lekcí."
	if got := text.SkippedContent("X", 5); got != want {
		t.Errorf("got %q", got)
	}
}
