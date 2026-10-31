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

// StoreCorruptSetAside is the notice for a damaged data file (%s is the file, then the new name).
const StoreCorruptSetAside = "Soubor %s byl poškozený a byl odložen jako %s."

// AppName is the name shown in the window.
const AppName = "tuipe"

// Placeholder is the body of the temporary start screen.
const Placeholder = "Trenažér psaní všemi deseti pro programátory"

// TooSmall is shown when the window is under 40 × 8.
const TooSmall = "Potřebuji víc místa.\nZvětšete prosím okno terminálu."

// SaveWarning is the notice for a failed save (%v is the error).
const SaveWarning = "Uložení se nepovedlo: %v"

// SecondInstance is the error when another tuipe holds the data directory.
const SecondInstance = "tuipe už běží v jiném okně. Zavřete ho a zkuste to znovu."

// NewerData is the error when the data was written by a newer tuipe.
const NewerData = "Data uložila novější verze tuipe. Spusťte „tuipe upgrade“ a zkuste to znovu."

// HintQuit is the footer hint of the start screen.
const HintQuit = "Ctrl+T styl · Ctrl+C konec"
