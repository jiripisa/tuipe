package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestAltGrTextCountsAsTyped(t *testing.T) {
	k := tea.KeyPressMsg{Code: '{', Text: "{", Mod: tea.ModCtrl | tea.ModAlt}
	if !altGr(k) {
		t.Fatal("Ctrl+Alt with text is not AltGr")
	}
	r, alt, ok := typedRune(k)
	if !ok || r != '{' || alt {
		t.Fatalf("typedRune = %q %v %v, want '{' false true", r, alt, ok)
	}
}

func TestCtrlWithoutAltIsStillNotTyped(t *testing.T) {
	if _, _, ok := typedRune(tea.KeyPressMsg{Code: 'r', Text: "r", Mod: tea.ModCtrl}); ok {
		t.Fatal("Ctrl+R was typed")
	}
}

func TestAltGrNeedsTextAndNoMeta(t *testing.T) {
	for name, k := range map[string]tea.KeyPressMsg{
		"no text": {Code: 'r', Mod: tea.ModCtrl | tea.ModAlt},
		"meta":    {Code: '{', Text: "{", Mod: tea.ModCtrl | tea.ModAlt | tea.ModMeta},
		"alt":     {Code: '{', Text: "{", Mod: tea.ModAlt},
	} {
		if altGr(k) {
			t.Errorf("%s counts as AltGr", name)
		}
	}
}
