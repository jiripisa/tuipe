// Dumps a macOS keyboard layout, read-only: as text to look at, or as the JSON
// dump that tuipe's layout files are generated from.
//
// Run:
//
//   swift scripts/dump-macos-layout.swift                  # the active layout, as text
//   swift scripts/dump-macos-layout.swift <id>             # an installed layout, as text
//   swift scripts/dump-macos-layout.swift <id> --json      # an installed layout, as a dump
//
// For example:
//
//   swift scripts/dump-macos-layout.swift com.apple.keylayout.Czech --json > internal/layout/source/macos-czech-qwertz.json
//
// A layout given by its id is looked up among all installed input sources, so it
// need not be enabled. The script never selects, enables or switches an input source.
//
// The text dump prints the input source id, the physical keyboard type, and for
// keycodes 0 to 50 the output with no modifier (-), Shift (S), Option (O) and
// Shift+Option (SO). A trailing "*" marks a dead key; the bracket then shows the
// dead key's own character. Below that, for each dead key, the result of pressing
// it before a set of follower keys.
//
// The JSON dump is the Dump of internal/layout/gen: for each key k0 to k50 the
// output at the levels "", "shift", "alt" (Option) and "shift+alt", where a dead
// key has "", and for each dead key the result of pressing it before space and
// before every position key, with no modifier and with Shift. A result is "" when
// the follower leaves a dead key pending, as a second dead key does.
import Carbon
import Foundation

let S = UInt32(shiftKey >> 8), O = UInt32(optionKey >> 8)

func fourCC(_ v: UInt32) -> String {
    let bytes = [24, 16, 8, 0].map { UInt8((v >> UInt32($0)) & 0xff) }
    return String(bytes: bytes, encoding: .macOSRoman) ?? String(v)
}

func fail(_ message: String, _ code: Int32 = 1) -> Never {
    FileHandle.standardError.write((message + "\n").data(using: .utf8)!)
    exit(code)
}

func sourceID(_ src: TISInputSource) -> String {
    let ptr = TISGetInputSourceProperty(src, kTISPropertyInputSourceID)!
    return Unmanaged<CFString>.fromOpaque(ptr).takeUnretainedValue() as String
}

// installedSource finds an installed input source by its id, without touching
// which sources are enabled or selected.
func installedSource(_ id: String) -> TISInputSource {
    let filter = [kTISPropertyInputSourceID as String: id] as CFDictionary
    guard let list = TISCreateInputSourceList(filter, true)?.takeRetainedValue() as? [TISInputSource],
          let src = list.first(where: { sourceID($0) == id })
    else {
        fail("no installed input source \(id)")
    }
    return src
}

func layoutData(_ src: TISInputSource) -> Data {
    guard let ptr = TISGetInputSourceProperty(src, kTISPropertyUnicodeKeyLayoutData) else {
        fail("no unicode layout data")
    }
    return Unmanaged<CFData>.fromOpaque(ptr).takeUnretainedValue() as Data
}

// Translator types keys on one layout and keyboard type.
struct Translator {
    let layout: UnsafePointer<UCKeyboardLayout>
    let kbdType: UInt32

    func translate(_ code: UInt16, _ mod: UInt32, _ deadState: inout UInt32, noDead: Bool = false) -> String {
        var chars = [UniChar](repeating: 0, count: 4)
        var len = 0
        UCKeyTranslate(layout, code, UInt16(kUCKeyActionDown), mod, kbdType,
                       noDead ? OptionBits(kUCKeyTranslateNoDeadKeysBit) : 0,
                       &deadState, 4, &len, &chars)
        return String(utf16CodeUnits: chars, count: len)
    }
}

func printText(_ id: String, _ t: Translator) {
    print("layout:", id)
    print("kbdType:", t.kbdType, "physical:", fourCC(UInt32(KBGetLayoutType(Int16(t.kbdType)))))

    let mods: [(String, UInt32)] = [("-", 0), ("S", S), ("O", O), ("SO", S | O)]
    var deadKeys: [(String, UInt16, UInt32)] = []
    for code in 0..<51 {
        var row = "\(code):"
        for (label, mod) in mods {
            var dead: UInt32 = 0
            let out = t.translate(UInt16(code), mod, &dead)
            if dead != 0 {
                var unused: UInt32 = 0
                let shown = t.translate(UInt16(code), mod, &unused, noDead: true)
                row += " \(label)=[\(shown)]*"
                deadKeys.append(("\(label)+key\(code)", UInt16(code), mod))
            } else {
                row += " \(label)=[\(out)]"
            }
        }
        print(row)
    }

    let followers: [(String, UInt16, UInt32)] = [("space", 49, 0)]
    let letters: [(String, UInt16)] = [
        ("a", 0), ("s", 1), ("d", 2), ("f", 3), ("h", 4), ("g", 5), ("z", 6), ("x", 7), ("c", 8), ("v", 9),
        ("b", 11), ("q", 12), ("w", 13), ("e", 14), ("r", 15), ("y", 16), ("t", 17), ("o", 31), ("u", 32),
        ("i", 34), ("p", 35), ("l", 37), ("j", 38), ("k", 40), ("n", 45), ("m", 46),
    ]
    let all = followers + letters.map { ($0.0, $0.1, UInt32(0)) } + letters.map { ($0.0.uppercased(), $0.1, S) }

    print("dead keys:")
    for (name, code, mod) in deadKeys {
        var row = name + ":"
        for (fname, fcode, fmod) in all {
            var dead: UInt32 = 0
            _ = t.translate(code, mod, &dead)
            let out = t.translate(fcode, fmod, &dead)
            row += " \(fname)=\(out)"
        }
        print(row)
    }
}

// quote writes s as a JSON string.
func quote(_ s: String) -> String {
    var out = "\""
    for u in s.unicodeScalars {
        switch u {
        case "\"": out += "\\\""
        case "\\": out += "\\\\"
        case "\n": out += "\\n"
        case "\r": out += "\\r"
        case "\t": out += "\\t"
        case _ where u.value < 0x20: out += String(format: "\\u%04X", u.value)
        default: out.unicodeScalars.append(u)
        }
    }
    return out + "\""
}

// pending reports whether a dead-key state still waits for a key. UCKeyTranslate
// keeps the waiting dead key in the low 16 bits of the state; after a dead key
// is resolved, the high bits can still be set, with nothing waiting.
func pending(_ state: UInt32) -> Bool {
    state & 0xFFFF != 0
}

// isoKbdType is the keyboard type the JSON dump is made for: an ISO keyboard,
// the one measured on the MacBook the layouts were first dumped on. The dump
// must not depend on the keyboard that happens to be attached, as on a CI
// runner; the Czech layouts type the same on every keyboard type anyway.
let isoKbdType: UInt32 = 92

func printJSON(_ id: String, _ t: Translator) {
    let levels: [(String, UInt32)] = [("", 0), ("shift", S), ("alt", O), ("shift+alt", S | O)]
    // Every key of the ISO keyboard; Tab (48) is the one keycode up to 50 that is not.
    let positions = (0...50).filter { $0 != 48 }.map(UInt16.init)
    func chord(_ level: String, _ code: UInt16) -> String {
        level.isEmpty ? "k\(code)" : "\(level)+k\(code)"
    }

    var keyLines: [String] = []
    var deadKeys: [(String, UInt16, UInt32)] = []
    for code in UInt16(0)...50 {
        var fields: [String] = []
        for (level, mod) in levels {
            var dead: UInt32 = 0
            var out = t.translate(code, mod, &dead)
            if pending(dead) {
                out = ""
                deadKeys.append((chord(level, code), code, mod))
            }
            fields.append("\(quote(level)): \(quote(out))")
        }
        keyLines.append("    \(quote("k\(code)")): { \(fields.joined(separator: ", ")) }")
    }

    var deadBlocks: [String] = []
    for (name, code, mod) in deadKeys {
        var lines: [String] = []
        for fcode in positions {
            for (flevel, fmod) in levels.prefix(2) {
                var dead: UInt32 = 0
                _ = t.translate(code, mod, &dead)
                var out = t.translate(fcode, fmod, &dead)
                if pending(dead) {
                    out = ""
                }
                lines.append("      \(quote(chord(flevel, fcode))): \(quote(out))")
            }
        }
        deadBlocks.append("    \(quote(name)): {\n\(lines.joined(separator: ",\n"))\n    }")
    }

    print("{")
    print("  \"source\": \(quote("macOS AppleKeyboardLayouts: \(id)")),")
    print("  \"system\": \"darwin\",")
    print("  \"keys\": {\n\(keyLines.joined(separator: ",\n"))\n  },")
    print("  \"dead\": {\n\(deadBlocks.joined(separator: ",\n"))\n  }")
    print("}")
}

let args = Array(CommandLine.arguments.dropFirst())
let src: TISInputSource
let asJSON: Bool
switch args.count {
case 0:
    src = TISCopyCurrentKeyboardLayoutInputSource().takeRetainedValue()
    asJSON = false
case 1 where !args[0].hasPrefix("-"):
    src = installedSource(args[0])
    asJSON = false
case 2 where !args[0].hasPrefix("-") && args[1] == "--json":
    src = installedSource(args[0])
    asJSON = true
default:
    fail("usage: swift scripts/dump-macos-layout.swift [<input source id> [--json]]", 2)
}

let id = sourceID(src)
let data = layoutData(src)
data.withUnsafeBytes { (raw: UnsafeRawBufferPointer) in
    let layout = raw.baseAddress!.assumingMemoryBound(to: UCKeyboardLayout.self)
    if asJSON {
        printJSON(id, Translator(layout: layout, kbdType: isoKbdType))
    } else {
        printText(id, Translator(layout: layout, kbdType: UInt32(LMGetKbdType())))
    }
}
