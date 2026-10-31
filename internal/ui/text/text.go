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

// TooSmall is shown when the window is under 40 × 8.
const TooSmall = "Potřebuji víc místa.\nZvětšete prosím okno terminálu."

// SaveWarning is the notice for a failed save (%v is the error).
const SaveWarning = "Uložení se nepovedlo: %v"

// SecondInstance is the error when another tuipe holds the data directory.
const SecondInstance = "tuipe už běží v jiném okně. Zavřete ho a zkuste to znovu."

// NewerData is the error when the data was written by a newer tuipe.
const NewerData = "Data uložila novější verze tuipe. Spusťte „tuipe upgrade“ a zkuste to znovu."

// Footer hints of the screens.
const (
	HintHome    = "↑↓ výběr · Enter spustit · Ctrl+T styl · Ctrl+C konec"
	HintResults = "Enter další kolo · Esc domů · Ctrl+T styl"
	HintPause   = "↑↓ · Enter · Esc pokračovat"
)

// Home screen.
const (
	HomeRank         = "hodnost"
	HomeUptime       = "uptime"
	HomeUptimeFormat = "up %d d"
	HomeBudgetSpent  = "rozpočet chyb na tento týden je vyčerpaný"
	HomeQuickRun     = "Rychlovka"
	HomeResume       = "pokračuj: %s"
	HomeNotReady     = "zatím není hotové"

	MenuStandup  = "Denní standup"
	MenuSprint   = "Sprint"
	MenuCareer   = "Kariéra"
	MenuPractice = "Trénink"
	MenuIncident = "Incident v produkci"
	MenuStats    = "Statistiky"
	MenuSettings = "Nastavení"

	PickDrill    = "drill na slabá místa"
	PickIncident = "incident v produkci"
	PickCode     = "kód: %s"
)

// Pause menu.
const (
	PauseTitle   = "PAUZA"
	PauseResume  = "pokračovat"
	PauseRestart = "začít znovu"
	PauseLeave   = "odejít"
)

// Results screen. The diff labels are padded to one width.
const (
	ResultsOld      = "HEAD~1"
	ResultsNew      = "ty"
	ResultsWPM      = "wpm  "
	ResultsAccuracy = "přes."
	ResultsTime     = "čas  "
	ResultsStars    = "hvězdy"
	ResultsPaused   = "pauza: kolo se nezapisuje do ducha"
	ResultsHintUsed = "nápověda použita: třetí hvězda není možná"
	ResultsLost     = "SLA porušeno, incident prohrán"
	ResultsWon      = "incident vyřešen, SLA %.2f %%"
	ResultsNoStars  = "bez hvězdy"
)

// Labels of the round screen. HEAD~1, SLA and wpm are the app's own terms.
const (
	RoundGhost      = "HEAD~1"
	RoundYou        = "ty"
	RoundAhead      = "před duchem"
	RoundBehind     = "za duchem"
	RoundStreak     = "série"
	RoundSLA        = "SLA"
	RoundWPM        = "wpm"
	RoundAccuracy   = "přes."
	RoundTyping     = "PSANÍ"
	RoundPaused     = "PAUZA"
	RoundResume     = "pokračujte stiskem klávesy"
	RoundAltWarning = "Terminál bere Option jako Alt. V nastavení to změňte."
	RoundProgress   = "postup"
	RoundChars      = "znaky"
	RoundLine       = "řádek"
	RoundErrors     = "chyby"
	RoundWeak       = "nejslabší"
	RoundOverview   = "přehled"
	RoundProseFile  = "text.txt"
)

// Panel titles of the Velín layout.
const (
	RoundPanelProgress = "POSTUP"
	RoundThroughput    = "PROPUSTNOST"
	RoundPanelStats    = "STATISTIKA"
	RoundPanelOverview = "PŘEHLED"
	RoundPanelKeyboard = "KLÁVESNICE"
	RoundNextChar      = "DALŠÍ ZNAK"
	RoundWeakest       = "NEJSLABŠÍ"
)
