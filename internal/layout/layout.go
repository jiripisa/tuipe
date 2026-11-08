// Package layout models physical keyboard layouts and the keystroke routes
// that produce each character.
package layout

import (
	"slices"
	"strings"
)

// Shape is the physical shape of the keyboard: the layout file describes the ISO
// one, and an optional overlay says how the US-shaped (ANSI) one differs.
type Shape string

// Shapes.
const (
	ISO  Shape = "iso"
	ANSI Shape = "ansi"
)

// ParseShape reads a shape from the config. An empty or unknown value is ISO.
func ParseShape(s string) Shape {
	if Shape(s) == ANSI {
		return ANSI
	}
	return ISO
}

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

	ansi  *overlay // the [ansi] section of the file, nil when there is none
	shape Shape
	base  *Layout // the ISO layout this copy was made from
}

// overlay is what the US-shaped keyboard changes in a layout.
type overlay struct {
	missing []string
	moved   map[string]string // from -> to
	chars   map[rune]Route
}

// Shape returns the shape the layout is in.
func (l *Layout) Shape() Shape {
	if l.shape == ANSI {
		return ANSI
	}
	return ISO
}

// HasANSI reports whether the file describes a US-shaped keyboard.
func (l *Layout) HasANSI() bool { return l.ansi != nil || l.base != nil }

// WithShape returns the layout in the given shape. It returns l itself when
// nothing changes: for ISO, and when the file has no [ansi] section. Otherwise
// it returns a copy and leaves l alone.
func (l *Layout) WithShape(s Shape) *Layout {
	if s != ANSI {
		if l.base != nil {
			return l.base
		}
		return l
	}
	if l.base != nil { // already shaped: apply the overlay to the ISO original, once
		l = l.base
	}
	if l.ansi == nil {
		return l
	}
	o := l.ansi
	c := &Layout{
		ID: l.ID, Name: l.Name, OS: l.OS, ModifierName: l.ModifierName,
		keys: map[string]Key{}, chars: map[rune]Route{},
		ansi: o, shape: ANSI, base: l,
	}
	for r, route := range l.chars {
		c.chars[r] = route
	}
	for _, row := range l.Rows {
		c.Rows = append(c.Rows, slices.Clone(row))
	}
	// Rule 1: a moved key's place takes over the routes of the key that moved.
	froms := make([]string, 0, len(o.moved))
	for from := range o.moved {
		froms = append(froms, from)
	}
	slices.Sort(froms)
	for _, from := range froms {
		to := o.moved[from]
		for r, route := range c.chars {
			if routeUses(route, to) {
				delete(c.chars, r)
			}
		}
		for r, route := range c.chars {
			if routeUses(route, from) {
				c.chars[r] = renameKey(route, from, to)
			}
		}
		fromKey, _ := l.Key(from)
		for i, row := range c.Rows {
			for j := range row {
				if row[j].ID == to {
					c.Rows[i][j].Label = fromKey.Label
				}
			}
		}
		c.dropKey(from)
	}
	// Rule 2: a key the US keyboard lacks.
	for _, id := range o.missing {
		for r, route := range c.chars {
			if routeUses(route, id) {
				delete(c.chars, r)
			}
		}
		c.dropKey(id)
	}
	// Rule 3: routes the file gives for this shape.
	for r, route := range o.chars {
		c.chars[r] = route
	}
	for _, row := range c.Rows {
		for _, k := range row {
			c.keys[k.ID] = k
		}
	}
	return c
}

// dropKey removes a key from its row.
func (l *Layout) dropKey(id string) {
	for i, row := range l.Rows {
		l.Rows[i] = slices.DeleteFunc(row, func(k Key) bool { return k.ID == id })
	}
}

func routeUses(route Route, key string) bool {
	for _, c := range route {
		if c.Key == key {
			return true
		}
	}
	return false
}

func renameKey(route Route, from, to string) Route {
	out := make(Route, len(route))
	for i, c := range route {
		out[i] = Chord{Mods: slices.Clone(c.Mods), Key: c.Key}
		if c.Key == from {
			out[i].Key = to
		}
	}
	return out
}

// Output returns the character that is exactly the key with no modifier.
func (l *Layout) Output(keyID string) (rune, bool) {
	var best rune
	found := false
	for r, route := range l.chars {
		if len(route) == 1 && len(route[0].Mods) == 0 && route[0].Key == keyID && (!found || r < best) {
			best, found = r, true
		}
	}
	return best, found
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
