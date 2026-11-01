# Keyboard layouts

A terminal reports the character a key produced, not the key. So to tell you how
to type `{`, tuipe has to know your exact layout, and that knowledge lives in a
data file. Adding a layout means writing a file, not changing the app.

tuipe ships one layout: `macos-czech-qwerty`, in
[`internal/layout/data/`](../internal/layout/data/macos-czech-qwerty.toml). Read
it next to this guide: it is the complete, real example.

## The format

A layout is a TOML file. This one is small, but it loads:

```toml
id = "example-small"
name = "Example: a few keys"
os = "linux"
modifier_name = "AltGr"

[[row]]
keys = [
  { id = "k19", label = "ě", hand = "left", finger = "ring" },
  { id = "k24", label = "´", hand = "right", finger = "pinky" },
]

[[row]]
keys = [
  { id = "k2", label = "d", hand = "left", finger = "middle" },
  { id = "k3", label = "f", hand = "left", finger = "index" },
  { id = "k36", label = "↵", hand = "right", finger = "pinky" },
]

[[row]]
keys = [
  { id = "k49", label = "mezera", hand = "right", finger = "thumb" },
]

[[char]]
c = "f"
route = ["k3"]

[[char]]
c = "F"
route = ["shift+k3"]

[[char]]
c = "d"
route = ["k2"]

[[char]]
c = "ě"
route = ["k19"]

[[char]]
c = "@"
route = ["altgr+k19"]

[[char]]
c = "ď"
route = ["shift+k24", "k2"]

[[char]]
c = " "
route = ["k49"]

[[char]]
c = "\n"
route = ["k36"]
```

`internal/layout/docs_test.go` reads the block above from this file and loads it,
so it cannot go stale.

### Identity

| Field | Required | Meaning |
|---|---|---|
| `id` | yes | Stable identifier. It is stored in `config.toml`, and statistics are kept per id. |
| `name` | yes | Shown in the layout picker and in settings. |
| `os` | no | `darwin`, `linux` or `windows`. On first launch tuipe runs the keyboard check only if a layout exists for the current system. |
| `modifier_name` | needed for a layout with a third level | What the third-level key is called, used in the name of the Option stage of the placement test. Set it on every layout that has third-level keys. A name other than `Option` also draws `Alt` instead of `⌥` on the keyboard map. |

### Keys

Each `[[row]]` is one row of the physical keyboard, top row first. Each key has:

| Field | Values |
|---|---|
| `id` | Any non-empty string that is unique in the file and has no `+` in it. |
| `label` | What the key is labelled with on the keyboard map, in the hints and on the heatmap. |
| `hand` | `left` or `right`. |
| `finger` | `pinky`, `ring`, `middle`, `index` or `thumb`. |

Use the id `k49` for the space bar and `k36` for Enter. The loader accepts any
id, but the app looks for these two: the keyboard map puts `k49` in the row of
modifiers, and hints read `mezera` and `Enter` for them. The other ids are free.
The built-in layout uses Macintosh keycodes (`k<code>`).

### Characters

Each `[[char]]` says how to produce one character:

| Field | Meaning |
|---|---|
| `c` | Exactly one character. Each character may appear once. Write a newline as `"\n"`, a quote as `"\""` and a backslash as `"\\"`. |
| `route` | The keystrokes, in order. At least one. |
| `alternates` | Optional list of strings. It is documentation only: the app never reads it. |

A route is a list of chords, and a chord is any number of modifiers and one key
id, joined with `+`. The modifiers are `shift`, `option` and `altgr`. The chord
`shift+k24` is Shift held with the key `k24`.

A route has more than one chord when a dead key is involved. `ď` above is
`["shift+k24", "k2"]`: press Shift and the dead key, let go, press `d`. In the
built-in layout `°` is `["option+k27", "k49"]`: a dead key followed by space.

A character can often be typed in several ways. Pick one for `route`, the one you
want taught. Since the terminal only shows the result, the other ways still work
while typing.

### What the loader rejects

A file is skipped, and named in settings, when:

- it is not valid TOML, or `id` or `name` is missing;
- a key has an unknown `hand` or `finger`, or an id that is empty or repeated;
- a `c` is not exactly one character, or a character is defined twice;
- a `route` is empty, names a key that is not in a row, or has an unknown modifier.

The loader does not check that a layout is complete. A lesson or incident with a
character the layout has no route for is left out, without a message. If a
chapter's incident cannot be typed, the chapter completes through its lessons, so
progress never gets stuck. Define at least the space (`" "`), the newline
(`"\n"`) and the alphabet, or little will remain to play. The curriculum needs
all of these to run in full:

```text
a-z A-Z  0-9
ě š č ř ž ý á í é ú ů ó ď ť ň
Ě Š Č Ř Ž Ý Á Í É Ú Ů Ó Ď Ť Ň
- _ : ? ! + = % / ( ) " ` , .
@ # $ { } [ ] ; ' ~ ^ & * < > \ |
° ˇ ¨ §
```

## Dumping a macOS layout

`scripts/dump-macos-layout.swift` prints what the active macOS layout produces.
It only reads. Switch to the layout you want in the menu bar, then run:

```sh
swift scripts/dump-macos-layout.swift
```

It prints the layout id and the physical keyboard type, then one line per keycode
from 0 to 50, with the output for no modifier (`-`), Shift (`S`), Option (`O`)
and Shift+Option (`SO`). A trailing `*` marks a dead key. This is the real output
for keycode 0 and keycode 24 on Czech QWERTY:

```text
layout: com.apple.keylayout.Czech-QWERTY
kbdType: 110 physical: ISO
0: -=[a] S=[A] O=[ą] SO=[Ą]
24: -=[]* S=[]* O=[]* SO=[]*
```

After the keycodes comes a `dead keys:` section with what each dead key does to
the keys that follow it:

```text
S+key24: space=ˇ a=ˇa s=š d=ď ...
```

So `ď` is `S+key24` then keycode 2, which gives the route
`["shift+k24", "k2"]`. Which finger presses a key is your decision: the script
cannot know it.

The script is for macOS only. For Linux and Windows, write the file by hand.

## Using your own file

Put the file into the `layouts` folder next to your config:

| System | Folder |
|---|---|
| macOS, Linux | `~/.config/tuipe/layouts/` |
| Windows | `%USERPROFILE%\.config\tuipe\layouts\` |

If `XDG_CONFIG_HOME` is set, the folder is `$XDG_CONFIG_HOME/tuipe/layouts/` on
every system.

The folder is read at start and on each visit to settings, so you do not need to
restart. In tuipe open **Nastavení** and change **Rozložení** with `←` and `→`.
The file must end in `.toml`. If it does not load, settings say
`Rozložení ze souboru … jsem nenačetl.` and name the file. To see the reason, copy
the file into `internal/layout/data/` in a checkout and run
`go test ./internal/layout`: the failure quotes the file and the problem.

## Contributing a layout

1. Add the file as `internal/layout/data/<id>.toml`. Everything in that folder is
   embedded in the binary.
2. Add a test with the routes you verified on a real keyboard, like
   `TestCzechRoutes` in `internal/layout/layout_test.go`.
3. Run `go test ./...`.
4. Open a pull request. Say which system and physical keyboard you used and how
   you checked the routes.
