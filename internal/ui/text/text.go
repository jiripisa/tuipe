// Package text is the single home of user-facing Czech strings.
package text

// Usage is printed by `tuipe help`.
const Usage = `tuipe - trenažér psaní všemi deseti pro programátory (česká klávesnice QWERTY)

Použití:
  tuipe                  spustí trenažér
  tuipe upgrade          aktualizuje na nejnovější verzi
  tuipe upgrade --check  jen zjistí, zda je dostupná novější verze
  tuipe version          vypíše verzi
  tuipe help             zobrazí tuto nápovědu
`

// UnknownCommand is the format of the error for an unknown subcommand (%q is the command).
const UnknownCommand = "tuipe: neznámý příkaz %q\nNápovědu zobrazíte příkazem: tuipe help\n"
