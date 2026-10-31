// Package layout models physical keyboard layouts and the keystroke routes
// that produce each character.
package layout

import "strings"

// Hand is the hand that presses a key.
type Hand int

// Hands.
const (
	Left Hand = iota
	Right
)

// Finger is the finger that presses a key.
type Finger int

// Fingers.
const (
	Pinky Finger = iota
	Ring
	Middle
	Index
	Thumb
)

// Key is a physical key.
type Key struct {
	ID, Label string
	Hand      Hand
	Finger    Finger
}

// Chord is one key press with modifiers held. Mods are "shift", "option" or "altgr".
type Chord struct {
	Mods []string
	Key  string
}

// String renders the chord as "mod+mod+key".
func (c Chord) String() string {
	return strings.Join(append(append([]string{}, c.Mods...), c.Key), "+")
}

// Route is a sequence of chords that produces one character.
type Route []Chord

// String renders the route as space-separated chords.
func (r Route) String() string {
	parts := make([]string, len(r))
	for i, c := range r {
		parts[i] = c.String()
	}
	return strings.Join(parts, " ")
}

// Layout is a keyboard layout: physical geometry plus character routes.
type Layout struct {
	ID, Name, OS, ModifierName string
	Rows                       [][]Key // physical geometry, top row first

	keys  map[string]Key
	chars map[rune]Route
}

// Route returns the preferred route for r.
func (l *Layout) Route(r rune) (Route, bool) {
	route, ok := l.chars[r]
	if !ok {
		return nil, false
	}
	return append(Route(nil), route...), true
}

// Key returns the physical key with the given id.
func (l *Layout) Key(id string) (Key, bool) {
	k, ok := l.keys[id]
	return k, ok
}

// Unresolved returns the distinct runes of text that have no route, in order of first appearance.
func (l *Layout) Unresolved(text string) []rune {
	var out []rune
	seen := map[rune]bool{}
	for _, r := range text {
		if _, ok := l.chars[r]; !ok && !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}
