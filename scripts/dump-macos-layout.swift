// Dumps the active macOS keyboard layout, read-only, to help write tuipe layout files.
//
// Run:  swift scripts/dump-macos-layout.swift
//
// Switch to the layout you want to dump first (menu bar input source).
// Prints the input source id, the physical keyboard type, and for keycodes 0 to 50
// the output with no modifier (-), Shift (S), Option (O) and Shift+Option (SO).
// A trailing "*" marks a dead key; the bracket then shows the dead key's own character.
// Below that, for each dead key, the result of pressing it before a set of follower keys.
import Carbon
import Foundation

let src = TISCopyCurrentKeyboardLayoutInputSource().takeRetainedValue()
let idPtr = TISGetInputSourceProperty(src, kTISPropertyInputSourceID)!
let id = Unmanaged<CFString>.fromOpaque(idPtr).takeUnretainedValue() as String
guard let dataPtr = TISGetInputSourceProperty(src, kTISPropertyUnicodeKeyLayoutData) else {
    print("no unicode layout data")
    exit(1)
}
let data = Unmanaged<CFData>.fromOpaque(dataPtr).takeUnretainedValue() as Data

func fourCC(_ v: UInt32) -> String {
    let bytes = [24, 16, 8, 0].map { UInt8((v >> UInt32($0)) & 0xff) }
    return String(bytes: bytes, encoding: .macOSRoman) ?? String(v)
}
let kbdType = UInt32(LMGetKbdType())
print("layout:", id)
print("kbdType:", kbdType, "physical:", fourCC(UInt32(KBGetLayoutType(Int16(kbdType)))))

let S = UInt32(shiftKey >> 8), O = UInt32(optionKey >> 8)
let mods: [(String, UInt32)] = [("-", 0), ("S", S), ("O", O), ("SO", S | O)]

data.withUnsafeBytes { (raw: UnsafeRawBufferPointer) in
    let layout = raw.baseAddress!.assumingMemoryBound(to: UCKeyboardLayout.self)

    func translate(_ code: UInt16, _ mod: UInt32, _ deadState: inout UInt32, noDead: Bool = false) -> String {
        var chars = [UniChar](repeating: 0, count: 4)
        var len = 0
        UCKeyTranslate(layout, code, UInt16(kUCKeyActionDown), mod, kbdType,
                       noDead ? OptionBits(kUCKeyTranslateNoDeadKeysBit) : 0,
                       &deadState, 4, &len, &chars)
        return String(utf16CodeUnits: chars, count: len)
    }

    var deadKeys: [(String, UInt16, UInt32)] = []
    for code in 0..<51 {
        var row = "\(code):"
        for (label, mod) in mods {
            var dead: UInt32 = 0
            let out = translate(UInt16(code), mod, &dead)
            if dead != 0 {
                var unused: UInt32 = 0
                let shown = translate(UInt16(code), mod, &unused, noDead: true)
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
            _ = translate(code, mod, &dead)
            let out = translate(fcode, fmod, &dead)
            row += " \(fname)=\(out)"
        }
        print(row)
    }
}
