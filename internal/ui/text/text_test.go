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

func TestSkippedContentNamesWhatIsThere(t *testing.T) {
	if got := text.SkippedContent("X", 0); got != "" {
		t.Errorf("nothing skipped: %q", got)
	}
	want := "Rozložení X neumí některé znaky. Přeskočeno: 5 lekcí."
	if got := text.SkippedContent("X", 5); got != want {
		t.Errorf("got %q", got)
	}
}
