//go:build windows

package main

import (
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/jiripisa/tuipe/internal/layout/gen"
)

func init() {
	dumpers["windows"] = dumper{usage: "<KLID>, as 00000405", min: 1, max: 1, dump: dumpWindows}
}

var (
	user32                    = windows.NewLazySystemDLL("user32.dll")
	procMapVirtualKeyExW      = user32.NewProc("MapVirtualKeyExW")
	procGetKeyboardLayoutList = user32.NewProc("GetKeyboardLayoutList")
)

const (
	mapScanToKey = 3    // MAPVK_VSC_TO_VK_EX: a scan code to the virtual key of a layout
	keepState    = 0x4  // a ToUnicodeEx flag: the kernel's keyboard state, dead keys too, stays as it was
	held         = 0x80 // a key that is down, in a key state
)

// dumpWindows reads a Windows keyboard layout, named by its KLID as in the
// registry (00000405 is Czech, 00010405 Czech QWERTY). Shift is VK_SHIFT, and
// the third level is AltGr, which Windows sees as Ctrl and Alt held together.
// The layout is loaded without being activated, and unloaded afterwards unless
// it was loaded already, so the user's list of layouts stays as it was.
func dumpWindows(args []string) (d gen.Dump, err error) {
	klid := strings.ToUpper(args[0])
	lang, err := strconv.ParseUint(klid, 16, 32)
	if err != nil || len(klid) != 8 {
		return gen.Dump{}, fmt.Errorf("a KLID is 8 hex digits, as 00000405, not %q", args[0])
	}

	// ToUnicodeEx keeps a pending dead key in the kernel from one call to the
	// next. Every call comes from this one thread, so that they share it.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	before, err := loadedLayouts()
	if err != nil {
		return gen.Dump{}, err
	}
	name, err := windows.UTF16PtrFromString(klid)
	if err != nil {
		return gen.Dump{}, err
	}
	hkl, err := windows.LoadKeyboardLayout(name, windows.KLF_NOTELLSHELL)
	if err != nil {
		return gen.Dump{}, fmt.Errorf("no Windows keyboard layout %s: %w", klid, err)
	}
	if !slices.Contains(before, hkl) {
		defer func() {
			if uerr := windows.UnloadKeyboardLayout(hkl); uerr != nil && err == nil {
				err = fmt.Errorf("unloading the layout %s: %w", klid, uerr)
			}
		}()
	}
	after, err := loadedLayouts()
	if err != nil {
		return gen.Dump{}, err
	}
	if err := checkLoaded(klid, uint16(lang), hkl, after); err != nil {
		return gen.Dump{}, err
	}

	l := winLayout{hkl: hkl, vk: map[string]uint32{}, scan: map[string]uint16{}}
	for _, p := range positions {
		vk, _, _ := procMapVirtualKeyExW.Call(uintptr(p.Scan), mapScanToKey, uintptr(hkl))
		if vk == 0 {
			return gen.Dump{}, fmt.Errorf("the layout %s has no key with scan code %#02x (%s)", klid, p.Scan, p.ID)
		}
		l.vk[p.ID], l.scan[p.ID] = uint32(vk), p.Scan
	}
	if err := l.clear(); err != nil {
		return gen.Dump{}, err
	}

	d = gen.Dump{
		Source: "Windows keyboard layout " + klid,
		System: "windows",
		Keys:   map[string]gen.Levels{},
		Dead:   map[string]map[string]string{},
	}
	for _, p := range positions {
		d.Keys[p.ID] = gen.Levels{}
		for _, level := range levels {
			text, dead, err := l.press(level, p.ID, keepState)
			if err != nil {
				return gen.Dump{}, err
			}
			if !dead {
				d.Keys[p.ID][level] = text
				continue
			}
			// Should keepState leave the dead key pending after all, the
			// next key must not see it.
			if err := l.clear(); err != nil {
				return gen.Dump{}, err
			}
			d.Keys[p.ID][level] = ""
			followers := map[string]string{}
			for _, f := range positions {
				for _, flevel := range levels[:2] {
					if followers[chordName(flevel, f.ID)], err = l.afterDeadKey(level, p.ID, flevel, f.ID); err != nil {
						return gen.Dump{}, err
					}
				}
			}
			if err := checkDeadBeforeSpace(chordName(level, p.ID), followers); err != nil {
				return gen.Dump{}, err
			}
			d.Dead[chordName(level, p.ID)] = followers
		}
	}
	return d, nil
}

// checkLoaded checks that hkl, which LoadKeyboardLayout gave for klid, is
// that layout and is in the list of loaded layouts. Windows hands back another
// layout, often US English, when it has none by that KLID; the low word of both
// is the language. And ToUnicodeEx given a layout that is not loaded may use
// the thread's own layout without saying so.
func checkLoaded(klid string, lang uint16, hkl windows.Handle, loaded []windows.Handle) error {
	if uint16(hkl) != lang {
		return fmt.Errorf("no Windows keyboard layout %s: Windows gave the layout of language %04X", klid, uint16(hkl))
	}
	if !slices.Contains(loaded, hkl) {
		return fmt.Errorf("the layout %s (%#x) is not among the loaded layouts %#x", klid, hkl, loaded)
	}
	return nil
}

// checkDeadBeforeSpace checks what the dead key typed before space (k49): its
// own accent. A space, or nothing, means the pending dead key did not carry
// from one ToUnicodeEx call to the next, and every follower is then wrong.
func checkDeadBeforeSpace(chord string, followers map[string]string) error {
	if out := followers["k49"]; out == " " || out == "" {
		return fmt.Errorf("the dead key %s typed %q before space, not its accent: it did not wait for the next key", chord, out)
	}
	return nil
}

// winLayout is a loaded Windows layout with the virtual key and the scan code
// of every position.
type winLayout struct {
	hkl  windows.Handle
	vk   map[string]uint32
	scan map[string]uint16
}

// press is what the key id types at a level, and whether it is a dead key,
// which types nothing and waits for the next key.
func (l winLayout) press(level, id string, flags uint32) (text string, dead bool, err error) {
	var state [256]byte
	if strings.Contains(level, "shift") {
		state[windows.VK_SHIFT] = held
	}
	if strings.Contains(level, "alt") {
		state[windows.VK_CONTROL], state[windows.VK_MENU] = held, held
	}
	return l.toUnicode(l.vk[id], uint32(l.scan[id]), &state, flags)
}

// toUnicode is ToUnicodeEx: the text a key types with the keys held in state,
// or that it is a dead key.
func (l winLayout) toUnicode(vk, scan uint32, state *[256]byte, flags uint32) (text string, dead bool, err error) {
	var buf [16]uint16
	n := windows.ToUnicodeEx(vk, scan, &state[0], &buf[0], int32(len(buf)), flags, l.hkl)
	switch {
	case n < 0:
		return "", true, nil
	case int(n) >= len(buf):
		return "", false, fmt.Errorf("a text of %d UTF-16 units does not fit in %d", n, len(buf))
	}
	return string(utf16.Decode(buf[:n])), false, nil
}

// afterDeadKey is what the dead key types before the follower: "" when the
// follower leaves a dead key pending, as a second dead key may. Unlike the
// first look at each key, both presses change the kernel's dead-key state, as
// typing does: the dead key leaves itself pending and the follower takes it.
func (l winLayout) afterDeadKey(level, id, flevel, fid string) (string, error) {
	if err := l.clear(); err != nil {
		return "", err
	}
	if _, dead, err := l.press(level, id, 0); err != nil {
		return "", err
	} else if !dead {
		return "", fmt.Errorf("the dead key %s typed text when pressed again", chordName(level, id))
	}
	text, dead, err := l.press(flevel, fid, 0)
	if err != nil {
		return "", err
	}
	if dead {
		return "", l.clear()
	}
	return text, nil
}

// clear ends a pending dead key, as typing does, by pressing space (k49)
// until it types something.
func (l winLayout) clear() error {
	for range 4 {
		if _, dead, err := l.press("", "k49", 0); err != nil || !dead {
			return err
		}
	}
	return fmt.Errorf("a dead key stays pending after space")
}

// loadedLayouts are the layouts in the user's list. A session always has at
// least one, so none is an error: an empty list would let the dump unload a
// layout the user had.
func loadedLayouts() ([]windows.Handle, error) {
	n, _, err := procGetKeyboardLayoutList.Call(0, 0)
	if n == 0 {
		return nil, fmt.Errorf("GetKeyboardLayoutList gave no layout: %w", err)
	}
	list := make([]windows.Handle, n)
	m, _, err := procGetKeyboardLayoutList.Call(n, uintptr(unsafe.Pointer(&list[0])))
	if m == 0 {
		return nil, fmt.Errorf("GetKeyboardLayoutList: %w", err)
	}
	return list[:m], nil
}
