package ui

import tea "charm.land/bubbletea/v2"

// altGr reports whether a key press is an AltGr character. Windows terminals
// send AltGr as Ctrl+Alt together with the typed text, so the press must be
// read as plain typing instead of a shortcut.
func altGr(k tea.KeyPressMsg) bool {
	return k.Text != "" &&
		k.Mod.Contains(tea.ModCtrl) && k.Mod.Contains(tea.ModAlt) &&
		!k.Mod.Contains(tea.ModMeta)
}
