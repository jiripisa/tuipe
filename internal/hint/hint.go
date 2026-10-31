// Package hint decides when a typing hint is shown and how it reads.
package hint

import (
	"strings"
	"time"

	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/stats"
)

const (
	learningDelay = 800 * time.Millisecond
	solidDelay    = 2 * time.Second
)

// Input is what Show needs to decide.
type Input struct {
	Mastery    stats.Mastery
	Hesitation time.Duration
	WrongAtPos int
	Disabled   bool
}

// Show reports whether a hint is visible now.
func Show(in Input) bool {
	if in.Disabled {
		return false
	}
	switch in.Mastery {
	case stats.NewChar:
		return true
	case stats.Learning, stats.Solid, stats.Automatic:
		if in.WrongAtPos >= 2 {
			return true
		}
	}
	switch in.Mastery {
	case stats.Learning:
		return in.Hesitation >= learningDelay
	case stats.Solid:
		return in.Hesitation >= solidDelay
	}
	return false
}

var fingers = map[layout.Finger]string{
	layout.Pinky: "malíček", layout.Ring: "prsteník", layout.Middle: "prostředník",
	layout.Index: "ukazovák", layout.Thumb: "palec",
}

var mods = map[string]string{"shift": "Shift", "option": "Option", "altgr": "AltGr"}

func handName(h layout.Hand) string {
	if h == layout.Left {
		return "levý"
	}
	return "pravý"
}

func keyLabel(k layout.Key) string {
	switch k.ID {
	case "k49":
		return "mezera"
	case "k36":
		return "Enter"
	}
	return k.Label
}

// Text describes how to type r, such as "{ → levý Option + í (pravý prsteník)".
// It returns "" when the layout has no route for r.
func Text(l *layout.Layout, r rune) string {
	route, ok := l.Route(r)
	if !ok || len(route) == 0 {
		return ""
	}
	var steps []string
	var last layout.Key
	for _, c := range route {
		k, ok := l.Key(c.Key)
		if !ok {
			return ""
		}
		last = k
		step := keyLabel(k)
		if len(c.Mods) > 0 {
			names := make([]string, len(c.Mods))
			for i, m := range c.Mods {
				names[i] = mods[m]
				if names[i] == "" {
					names[i] = m
				}
			}
			modHand := layout.Left
			if k.Hand == layout.Left {
				modHand = layout.Right
			}
			step = handName(modHand) + " " + strings.Join(names, " + ") + " + " + step
		}
		steps = append(steps, step)
	}
	shown := string(r)
	switch r {
	case ' ':
		shown = "mezera"
	case '\n':
		shown = "Enter"
	}
	finger := fingers[last.Finger]
	if last.Finger != layout.Thumb {
		finger = handName(last.Hand) + " " + finger
	}
	if r == ' ' {
		return shown + " → " + finger
	}
	return shown + " → " + strings.Join(steps, ", pak ") + " (" + finger + ")"
}
