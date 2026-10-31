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
	HomeRank        = "hodnost"
	HomeUptime      = "uptime"
	HomeUptimeDay   = "up 1 day"
	HomeUptimeDays  = "up %d days"
	HomeBudgetSpent = "rozpočet chyb na tento týden je vyčerpaný"
	HomeQuickRun    = "Rychlovka"
	HomeResume      = "pokračuj: %s"
	HomeNotReady    = "zatím není hotové"

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
	ResultsNoBase   = "/dev/null"
	ResultsCommit   = "commit %s  %s"
	DrillTitle      = "drill"
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

// Career map and test-outs.
const (
	CareerTitle     = "Kariéra"
	CareerTestOut   = "test-out: přeskočit kapitolu"
	CareerDone      = "hotovo"
	CareerLocked    = "zamčeno"
	HintCareer      = "↑↓ výběr · Enter spustit · Esc zpět · Ctrl+T styl"
	TestOutTitle    = "test-out: %s"
	TestOutPassed   = "test-out splněn: kapitola je hotová, bez hvězd"
	TestOutFailed   = "test-out nesplněn: nic se nezměnilo"
	TestOutNotReady = "test-out této kapitoly nejde sestavit"
)

// Practice.
const (
	PracticeTitle      = "Trénink"
	PracticeChapter    = "Kapitola"
	PracticeLesson     = "Lekce"
	PracticeLanguage   = "Jazyk"
	PracticeChars      = "Vybrané znaky"
	PracticeCharsAsk   = "znaky k procvičení (nejvýše %d):"
	PracticeCharsEmpty = "napište alespoň jeden znak, který klávesnice umí"
	PracticeNoSnippet  = "pro tento jazyk není na této klávesnici žádná ukázka"
	PracticeDrill      = "trénink: %s"
	HintPractice       = "↑↓ výběr · Enter spustit · Esc zpět · Ctrl+T styl"
	HintPracticeChars  = "Enter spustit · Backspace smazat · Esc zpět · Ctrl+T styl"
)

// Incidents: the severity picker, the list, the round and the two endings.
const (
	IncidentTitle     = "Incident v produkci"
	IncidentSeverity  = "Závažnost incidentu"
	IncidentBaseWPM   = "tvé základní tempo: %.0f wpm"
	IncidentSev3      = "SEV3  běžné tempo, nikdo nevolá"
	IncidentSev2      = "SEV2  o 5 % rychleji, někdo už píše do chatu"
	IncidentSev1      = "SEV1  o 15 % rychleji, volá ředitel"
	IncidentListEmpty = "Žádný odemčený incident. V produkci je zatím klid."
	IncidentListBest  = "[%s]"
	IncidentListNone  = "[ ]"
	HintIncident      = "↑↓ výběr · Enter spustit · Esc zpět · Ctrl+T styl"
	RoundIncidentInfo = "%s  %d/%d  %d:%02d"

	IncidentWon      = "INCIDENT VYŘEŠEN"
	IncidentMTTR     = "MTTR      %.1f s"
	IncidentSLALeft  = "SLA       %.2f %%"
	IncidentSevShown = "závažnost %s"
	IncidentFirst    = "první řešení tohoto incidentu, není s čím srovnávat"
	IncidentPrev     = "minule: %s, MTTR %.1f s, SLA %.2f %%"
	IncidentAhead    = "duch: o %.1f s před minulým řešením"
	IncidentBehind   = "duch: o %.1f s za minulým řešením"
	IncidentPromoted = "Kapitola „%s“ uzavřena. Nikdo nepanikaří."
	IncidentRank     = "Hodnost: %s → %s. HR byla informována."

	PostmortemTitle    = "POSTMORTEM (bez hledání viníka)"
	PostmortemSummary  = "Souhrn: SLA kleslo na SLO, služba je nedostupná."
	PostmortemImpact   = "Dopad: tyto znaky selhaly, píšou se takto:"
	PostmortemCause    = "Příčina: prsty, ne lidé. Nikdo nebude potrestán."
	PostmortemSlow     = "Příčina: pomalost, ne chyby. Nikdo nebude potrestán."
	PostmortemAction   = "Opatření: drill na tyto znaky. Termín: hned."
	PostmortemDrill    = "trénink po incidentu: %s"
	HintPostmortem     = "d drill na znaky · Enter další · Esc domů · Ctrl+T styl"
	HintPostmortemBare = "Enter další · Esc domů · Ctrl+T styl"
	HintIncidentWon    = "Enter další · Esc domů · Ctrl+T styl"
)

// Standup, Sprint and Retro.
const (
	ChainStandup = "Standup %d/%d"
	ChainSprint  = "Sprint %d:%02d / %d:00"

	HomeDone     = "hotovo"
	HomeContinue = "pokračovat"

	SprintTitle  = "Sprint"
	SprintAsk    = "Jak dlouhý sprint?"
	SprintOption = "%d minut"
	HintSprint   = "↑↓ výběr · Enter spustit · Esc zpět · Ctrl+T styl"

	RetroTitle      = "Retro"
	RetroHead       = "git log --stat --since=7.days"
	RetroRuns       = "%d kol v tomto sprintu"
	RetroNoRuns     = "Prázdný commit: žádné dokončené kolo, není co hodnotit."
	RetroWPM        = "wpm   %6.1f  (%+.1f)"
	RetroAccuracy   = "přes. %5.1f %%  (%+.1f p. b.)"
	RetroWPMNew     = "wpm   %6.1f"
	RetroAccNew     = "přes. %5.1f %%"
	RetroNoPrev     = "Předchozích 7 dní bez kol: není s čím srovnávat."
	RetroImproved   = "zlepšeno: %s"
	RetroWorsened   = "zhoršeno: %s"
	RetroCharShift  = "%s %+.0f p. b."
	RetroNoShift    = "Znaky: žádná změna, která by stála za záznam."
	RetroNoSnapshot = "Znaky: chybí snímek ze startu sprintu, nemám co porovnat."
	HintRetro       = "Enter domů · Esc domů · Ctrl+T styl"
)

// Statistics.
const (
	StatsEmpty           = "Zatím žádná kola. Odehrajte první a tady se objeví historie."
	StatsHeatEmpty       = "Zatím není co ukázat: klávesy se obarví, až se nasbírá pár kol."
	StatsLost            = "prohráno"
	StatsWeakestFinger   = "nejslabší prst: %s"
	StatsGroupOverall    = "celkem"
	StatsGroupLetters    = "písmena"
	StatsGroupDiacritics = "diakritika"
	StatsGroupDigits     = "čísla"
	StatsGroupShift      = "Shift"
	StatsGroupOption     = "Option"
	StatsModeLesson      = "Lekce"
	StatsModeCode        = "Kód"
	StatsModeIncident    = "Incident"
	StatsModeDrill       = "Drill"
)

// More statistics strings.
const (
	StatsModePractice  = "Trénink"
	StatsModeTestOut   = "Test-out"
	StatsModePlacement = "Vstupní pohovor"
	StatsRecordScore   = "skóre"
	StatsWPM           = "WPM"
	StatsAccuracy      = "Přesnost"
	HintStats          = "←→ záložky · ↑↓ PgUp PgDn · Esc domů · Ctrl+T styl"
)

// StatsTabs are the tabs of the statistics screen; git log is the app's own term.
var StatsTabs = [...]string{"git log", "heatmap", "trendy", "rekordy"}

// Names of the hands and fingers, indexed by layout.Hand and layout.Finger.
var (
	StatsHandNames   = [...]string{"levý", "pravý"}
	StatsFingerNames = [...]string{"malíček", "prsteník", "prostředník", "ukazovák", "palec"}
)

// Sessions: how a round was started.
const (
	StatsSessionQuick   = "Rychlovka"
	StatsSessionStandup = "Standup"
	StatsSessionSprint  = "Sprint"
)
