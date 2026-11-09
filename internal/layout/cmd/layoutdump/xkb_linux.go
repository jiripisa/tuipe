//go:build xkb && linux

package main

/*
#cgo LDFLAGS: -lxkbcommon
#include <stdlib.h>
#include <xkbcommon/xkbcommon.h>
#include <xkbcommon/xkbcommon-compose.h>
*/
import "C"

import (
	"fmt"
	"os"
	"strings"
	"unsafe"

	"github.com/jiripisa/tuipe/internal/layout/gen"
)

func init() {
	dumpers["linux"] = dumper{usage: "<layout> [<variant>]", min: 1, max: 2, dump: dumpLinux}
}

// composeLocales are the locales whose Compose table resolves the dead keys, in
// the order they are tried: the Czech one, then the ones every system has.
var composeLocales = []string{"cs_CZ.UTF-8", "C.UTF-8", "en_US.UTF-8"}

// dumpLinux reads an XKB layout of xkeyboard-config, as the evdev rules build
// it for a pc105 keyboard. Shift is the left Shift key and the third level is
// the right Alt key, which must be ISO_Level3_Shift (AltGr). A dead key is
// resolved with the Compose table of the first locale of composeLocales that
// has one.
func dumpLinux(args []string) (gen.Dump, error) {
	layout, variant := args[0], ""
	if len(args) == 2 {
		variant = args[1]
	}
	cleanup, err := ignoreTheUsersFiles()
	if err != nil {
		return gen.Dump{}, err
	}
	defer cleanup()

	ctx := C.xkb_context_new(C.XKB_CONTEXT_NO_ENVIRONMENT_NAMES)
	if ctx == nil {
		return gen.Dump{}, fmt.Errorf("xkb_context_new failed")
	}
	defer C.xkb_context_unref(ctx)

	rules, model, cl, cv := C.CString("evdev"), C.CString("pc105"), C.CString(layout), C.CString(variant)
	defer func() {
		for _, c := range []*C.char{rules, model, cl, cv} {
			C.free(unsafe.Pointer(c))
		}
	}()
	names := C.struct_xkb_rule_names{rules: rules, model: model, layout: cl, variant: cv}
	keymap := C.xkb_keymap_new_from_names(ctx, &names, C.XKB_KEYMAP_COMPILE_NO_FLAGS)
	if keymap == nil {
		return gen.Dump{}, fmt.Errorf("no XKB layout %s", xkbName(layout, variant))
	}
	defer C.xkb_keymap_unref(keymap)

	keycode := func(name string) (C.xkb_keycode_t, error) {
		c := C.CString(name)
		defer C.free(unsafe.Pointer(c))
		kc := C.xkb_keymap_key_by_name(keymap, c)
		if kc == C.XKB_KEYCODE_INVALID {
			return 0, fmt.Errorf("the keymap has no key <%s>", name)
		}
		return kc, nil
	}
	shift, err := keycode("LFSH")
	if err != nil {
		return gen.Dump{}, err
	}
	altgr, err := keycode("RALT")
	if err != nil {
		return gen.Dump{}, err
	}
	codes := map[string]C.xkb_keycode_t{}
	for _, p := range positions {
		if codes[p.ID], err = keycode(p.XKB); err != nil {
			return gen.Dump{}, err
		}
	}

	// syms[level][id] is the keysym a key gives at a level.
	syms := map[string]map[string]C.xkb_keysym_t{}
	texts := map[string]map[string]string{}
	for _, level := range levels {
		state := C.xkb_state_new(keymap)
		if state == nil {
			return gen.Dump{}, fmt.Errorf("xkb_state_new failed")
		}
		if level == "" {
			if s := C.xkb_state_key_get_one_sym(state, altgr); s != C.XKB_KEY_ISO_Level3_Shift {
				C.xkb_state_unref(state)
				return gen.Dump{}, fmt.Errorf("the right Alt key is %s, not ISO_Level3_Shift", symName(s))
			}
		}
		if strings.Contains(level, "shift") {
			C.xkb_state_update_key(state, shift, C.XKB_KEY_DOWN)
		}
		if strings.Contains(level, "alt") {
			C.xkb_state_update_key(state, altgr, C.XKB_KEY_DOWN)
		}
		syms[level], texts[level] = map[string]C.xkb_keysym_t{}, map[string]string{}
		for _, p := range positions {
			syms[level][p.ID] = C.xkb_state_key_get_one_sym(state, codes[p.ID])
			texts[level][p.ID] = keyText(state, codes[p.ID])
		}
		C.xkb_state_unref(state)
	}

	table, locale := composeTable(ctx)
	if table == nil {
		return gen.Dump{}, fmt.Errorf("no Compose table for any of %s", strings.Join(composeLocales, ", "))
	}
	defer C.xkb_compose_table_unref(table)

	d := gen.Dump{
		Source: fmt.Sprintf("Linux xkeyboard-config: %s, with the Compose table of %s", xkbName(layout, variant), locale),
		System: "linux",
		Keys:   map[string]gen.Levels{},
		Dead:   map[string]map[string]string{},
	}
	for _, p := range positions {
		d.Keys[p.ID] = gen.Levels{}
		for _, level := range levels {
			sym := syms[level][p.ID]
			if !isDead(sym) || !startsSequence(table, sym) {
				d.Keys[p.ID][level] = texts[level][p.ID]
				continue
			}
			d.Keys[p.ID][level] = ""
			followers := map[string]string{}
			for _, f := range positions {
				for _, flevel := range levels[:2] {
					followers[chordName(flevel, f.ID)] = compose(table, sym, syms[flevel][f.ID])
				}
			}
			d.Dead[chordName(level, p.ID)] = followers
		}
	}
	return d, nil
}

// ignoreTheUsersFiles makes the dump depend on the system's files only: XKB
// and Compose also read files in the home folder and paths from the
// environment, which would differ from one machine to the next. The home
// folder is an empty one until cleanup.
func ignoreTheUsersFiles() (cleanup func(), err error) {
	home, err := os.MkdirTemp("", "layoutdump")
	if err != nil {
		return nil, err
	}
	cleanup = func() { _ = os.RemoveAll(home) }
	for _, k := range []string{"XCOMPOSEFILE", "XCOMPOSECACHE", "XLOCALEDIR", "XKB_CONFIG_ROOT", "XKB_CONFIG_EXTRA_PATH"} {
		if err := os.Unsetenv(k); err != nil {
			cleanup()
			return nil, err
		}
	}
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME"} {
		if err := os.Setenv(k, home); err != nil {
			cleanup()
			return nil, err
		}
	}
	return cleanup, nil
}

func xkbName(layout, variant string) string {
	if variant == "" {
		return layout
	}
	return layout + "(" + variant + ")"
}

// composeTable returns the Compose table of the first locale that has one.
func composeTable(ctx *C.struct_xkb_context) (*C.struct_xkb_compose_table, string) {
	for _, l := range composeLocales {
		c := C.CString(l)
		t := C.xkb_compose_table_new_from_locale(ctx, c, C.XKB_COMPOSE_COMPILE_NO_FLAGS)
		C.free(unsafe.Pointer(c))
		if t != nil {
			return t, l
		}
	}
	return nil, ""
}

// keyText is what a key types in a state.
func keyText(state *C.struct_xkb_state, kc C.xkb_keycode_t) string {
	var buf [64]C.char
	n := C.xkb_state_key_get_utf8(state, kc, &buf[0], C.size_t(len(buf)))
	if n <= 0 {
		return ""
	}
	return C.GoStringN(&buf[0], n)
}

func symName(sym C.xkb_keysym_t) string {
	var buf [64]C.char
	if C.xkb_keysym_get_name(sym, &buf[0], C.size_t(len(buf))) < 0 {
		return fmt.Sprintf("keysym %#x", uint32(sym))
	}
	return C.GoString(&buf[0])
}

func symText(sym C.xkb_keysym_t) string {
	var buf [64]C.char
	if n := C.xkb_keysym_to_utf8(sym, &buf[0], C.size_t(len(buf))); n <= 1 {
		return ""
	}
	return C.GoString(&buf[0])
}

// isDead reports whether a keysym is a dead key, as dead_acute.
func isDead(sym C.xkb_keysym_t) bool {
	return strings.HasPrefix(symName(sym), "dead_")
}

// startsSequence reports whether the Compose table waits for a key after sym.
// A dead keysym that starts no sequence types nothing and waits for nothing.
func startsSequence(table *C.struct_xkb_compose_table, sym C.xkb_keysym_t) bool {
	st := C.xkb_compose_state_new(table, C.XKB_COMPOSE_STATE_NO_FLAGS)
	defer C.xkb_compose_state_unref(st)
	C.xkb_compose_state_feed(st, sym)
	return C.xkb_compose_state_get_status(st) == C.XKB_COMPOSE_COMPOSING
}

// compose is what the dead keysym followed by another keysym types: "" when
// the Compose table has no such sequence, or when it still waits, as after a
// second dead key.
func compose(table *C.struct_xkb_compose_table, dead, follower C.xkb_keysym_t) string {
	if follower == C.XKB_KEY_NoSymbol {
		return ""
	}
	st := C.xkb_compose_state_new(table, C.XKB_COMPOSE_STATE_NO_FLAGS)
	defer C.xkb_compose_state_unref(st)
	C.xkb_compose_state_feed(st, dead)
	C.xkb_compose_state_feed(st, follower)
	if C.xkb_compose_state_get_status(st) != C.XKB_COMPOSE_COMPOSED {
		return ""
	}
	var buf [64]C.char
	if n := C.xkb_compose_state_get_utf8(st, &buf[0], C.size_t(len(buf))); n > 0 {
		return C.GoStringN(&buf[0], n)
	}
	return symText(C.xkb_compose_state_get_one_sym(st))
}
