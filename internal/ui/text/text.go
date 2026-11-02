// Package text is the single home of user-facing Czech strings.
package text

import (
	"fmt"
	"strings"
)

// Plural picks the Czech form for n: one for 1, few for 2 to 4, many otherwise.
func Plural(n int, one, few, many string) string {
	switch {
	case n == 1:
		return one
	case n >= 2 && n <= 4:
		return few
	}
	return many
}

// Chapters counts chapters: "1 kapitola", "3 kapitoly", "12 kapitol".
func Chapters(n int) string {
	return fmt.Sprintf("%d %s", n, Plural(n, "kapitola", "kapitoly", "kapitol"))
}

// Rounds counts rounds: "1 kolo", "3 kola", "12 kol".
func Rounds(n int) string {
	return fmt.Sprintf("%d %s", n, Plural(n, "kolo", "kola", "kol"))
}

// SkippedContent is the note about what a layout cannot type, told once. It is
// empty when nothing is skipped.
func SkippedContent(layout string, lessons, incidents int) string {
	var parts []string
	if lessons > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", lessons, Plural(lessons, "lekce", "lekce", "lekcí")))
	}
	if incidents > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", incidents, Plural(incidents, "incident", "incidenty", "incidentů")))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("Rozložení %s neumí některé znaky. Přeskočeno: %s.", layout, strings.Join(parts, " a "))
}

// Usage is printed by `tuipe help`.
const Usage = `tuipe - trenažér psaní všemi deseti pro programátory (česká klávesnice QWERTY)

Použití:
  tuipe                   spustí trenažér
  tuipe upgrade           aktualizuje na nejnovější verzi
  tuipe upgrade --check   jen zjistí, zda je dostupná novější verze
  tuipe reset --score     smaže postup, historii kol a duchy
  tuipe reset --settings  smaže nastavení a vlastní rozložení klávesnice
  tuipe reset --all       smaže obojí, jako by tuipe běžel poprvé
  tuipe version           vypíše verzi
  tuipe help              zobrazí tuto nápovědu

Příkaz reset vypíše, co smaže, a smaže to až po potvrzení slovem „ano“.
`

// Messages of `tuipe reset`.
const (
	ResetHeader       = "Smaže se:"
	ResetQuestion     = "Tohle nejde vzít zpět. Opravdu to chcete smazat? Napište „ano“ a stiskněte Enter: "
	ResetYes          = "ano"
	ResetCancelled    = "Nic se nesmazalo."
	ResetNothing      = "Není co mazat."
	ResetDoneScore    = "Smazáno. Při příštím spuštění začnete od začátku."
	ResetDoneSettings = "Smazáno. Nastavení je zase výchozí."
	ResetFailedPrefix = "Nepovedlo se smazat: "
)

// UnknownCommand is the format of the error for an unknown subcommand (%q is the command).
const UnknownCommand = "tuipe: neznámý příkaz %q\nNápovědu zobrazíte příkazem: tuipe help\n"

// StoreCorruptSetAside is the notice for a damaged data file (%s is the file, then where it went).
const StoreCorruptSetAside = "Soubor %s byl poškozený a je odložený jako %s."

// StoreProfileSetAside is the notice for a damaged profile (%s is where it went).
const StoreProfileSetAside = "Profil byl poškozený, začínáte s prázdným. Původní soubor je odložený jako %s."

// AppName is the name shown in the window.
const AppName = "tuipe"

// TooSmall is shown when the window is under 40 × 8.
const TooSmall = "Potřebuji víc místa.\nZvětšete prosím okno terminálu."

// SaveWarning is the notice for a failed save (%v is the error).
const SaveWarning = "Uložení se nepovedlo: %v"

// StoreNoDir is the error for a folder that cannot be created (%s is the folder).
const StoreNoDir = "Složku %s nelze vytvořit. Zkontrolujte oprávnění a volné místo na disku."

// NewerFile is the warning for one file written by a newer tuipe, which stays as it is.
const NewerFile = "Soubor uložila novější verze tuipe, nechávám ho být. Pomůže příkaz tuipe upgrade."

// SecondInstance is the error when another tuipe holds the data directory.
const SecondInstance = "tuipe už běží v jiném okně. Zavřete ho a zkuste to znovu."

// NewerData is the error when the data was written by a newer tuipe.
const NewerData = "Data uložila novější verze tuipe. Spusťte „tuipe upgrade“ a zkuste to znovu."

// Footer hints of the screens.
const (
	HintHome    = "↑↓ výběr · Enter spustit · Ctrl+T styl · Ctrl+C konec"
	HintResults = "Enter další kolo · Esc domů · Ctrl+T styl"
	// HintResultsRepeat is for a round that can be played again.
	HintResultsRepeat = "Enter další kolo · R znovu · Esc domů · Ctrl+T styl"
	HintPause         = "↑↓ · Enter · Esc pokračovat"
)

// Home screen.
const (
	HomeRank        = "hodnost"
	HomeUptime      = "uptime"
	HomeUptimeDay   = "up 1 day"
	HomeUptimeDays  = "up %d days"
	HomeBudgetSpent = "rozpočet chyb na tento týden je vyčerpaný"
	HomeResume      = "pokračuj: %s"

	MenuCareer   = "Kariéra"
	MenuStats    = "Statistiky"
	MenuSettings = "Nastavení"
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
	RoundAltWarning = "Option je Alt. Změňte to v nastavení terminálu."
	RoundAltUnset   = "Option je Alt, vypněte: %s"
	RoundAltSet     = "Option je Alt, nastavte: %s"
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
	CareerBoss      = "incident: %s %s"
	HintCareer      = "↑↓ výběr · Enter spustit · Esc zpět · Ctrl+T styl"
	TestOutTitle    = "test-out: %s"
	TestOutFailed   = "test-out nesplněn: nic se nezměnilo"
	TestOutNotReady = "test-out této kapitoly nejde sestavit"
)

// CareerTestOutMany is the row of a test-out that closes n chapters, n above 1.
func CareerTestOutMany(n int) string {
	return fmt.Sprintf("test-out: uzavře %s naráz", Chapters(n))
}

// TestOutPassed is the verdict of a test-out that closed n chapters.
func TestOutPassed(n int) string {
	return fmt.Sprintf("test-out splněn: %s %s, bez hvězd", Plural(n, "uzavřena", "uzavřeny", "uzavřeno"), Chapters(n))
}

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
	StatsGroupCode       = "kód"
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

// First run: style choice, layout check, Option check and the fork.
const (
	FirstRunWelcome     = "Vítejte v tuipe. Vyberte si vzhled."
	FirstRunStyleHint   = "↑↓ vzhled · Enter potvrdit · Ctrl+C konec"
	FirstRunSampleTitle = "ukázka"

	FirstRunCheckTitle = "Kontrola klávesnice"
	FirstRunCheckAsk   = "Napište znak:  %s   (%d/%d)"
	FirstRunCheckHow   = "Jak ho napsat: %s"
	FirstRunCheckLater = "Tuhle klávesu zatím znát nemusíte, učí se později. Zkuste ji znovu, nebo kontrolu přeskočte klávesou Esc."
	FirstRunCheckHint  = "Esc přeskočit · Tab jiné rozložení"
	// FirstRunCheckWrong warns about the first character (typed, wanted, typed).
	FirstRunCheckWrong     = "Napsali jste %s místo %s. Napište znak znovu, nebo stiskněte Enter, pokud klávesa opravdu dává %s."
	FirstRunCheckWrongHint = "Enter potvrdit · Esc přeskočit"
	FirstRunCheckSpace     = "mezeru" // the space key in that sentence

	FirstRunPickTitle = "Rozložení klávesnice"
	FirstRunPickAsk   = "Napsaný znak neodpovídá rozložení %s. Které rozložení používáte?"
	FirstRunPickHint  = "↑↓ výběr · Enter potvrdit · Esc přeskočit kontrolu"

	FirstRunOptionTitle = "Option se chová jako Alt"
	FirstRunOptionWhy   = "Terminál poslal Alt a písmeno místo znaku. Option pak nepíše znaky jako { a @, které potřebujete."
	FirstRunOptionHint  = "Enter zkusit znovu · Esc přeskočit kontrolu"

	OptionTerminalApp = "Terminal.app: Nastavení › Profily › Klávesnice, vypněte volbu:"
	OptionITerm       = "iTerm2: Settings › Profiles › Keys › General, nastavte obě klávesy Option (znak @ se píše pravou):"
	OptionGhostty     = "Ghostty: do konfigurace přidejte řádek:"
	OptionKitty       = "kitty: do kitty.conf přidejte řádek:"
	OptionWezTerm     = "WezTerm: do wezterm.lua přidejte řádek:"
	OptionAlacritty   = "Alacritty: do alacritty.toml, sekce [window], přidejte řádek:"
	OptionReload      = "Po změně nastavení terminál znovu načtěte nebo ho restartujte."
	OptionGeneric     = "Váš terminál jsem nepoznal. V nastavení terminálu vypněte, aby Option fungoval jako Meta nebo Alt (anglicky „Option as Meta“ či „Option as Alt“)."

	OptionSettingTerminalApp = `Use Option as Meta key`
	OptionSettingITerm       = "Left Option key = Normal\nRight Option key = Normal"
	OptionSettingGhostty     = `macos-option-as-alt = false`
	OptionSettingKitty       = `macos_option_as_alt no`
	OptionSettingWezTerm     = `config.send_composed_key_when_left_alt_is_pressed = true`
	OptionSettingAlacritty   = `option_as_alt = "None"`

	FirstRunNoLayoutTitle = "Rozložení klávesnice"
	FirstRunNoLayout      = "Pro tento systém (%s) zatím není žádné rozložení klávesnice. Můžete pokračovat s rozložením macOS, jen kontrola kláves se přeskočí."
	FirstRunNoLayoutHow   = "Vlastní rozložení je soubor .toml ve složce:"
	FirstRunNoLayoutDocs  = "Návod najdete v docs/layouts.md v repozitáři tuipe."
	FirstRunContinueHint  = "Enter pokračovat · Ctrl+C konec"

	FirstRunForkTitle = "Kde začínáte?"
	ForkFromZero      = "Začínám od nuly"
	ForkFromZeroNote  = "Začnete od domovské pozice."
	ForkPlacement     = "Vstupní pohovor"
	ForkPlacementNote = "Asi 4 minuty. Přeskočí, co už umíte."
	FirstRunForkHint  = "↑↓ výběr · Enter potvrdit · Ctrl+C konec"

	FirstRunHandHint  = "↑↓ posun · Enter pokračovat"
	FirstRunZeroTitle = "Domovská pozice"
	FirstRunZeroText  = "Prsty leží na domovské řadě: levý ukazovák na f, pravý na j. Na obou klávesách nahmatáte malý výstupek, podle něj se trefíte poslepu.\n\nPalce patří na mezerník. Každý prst obsluhuje svou oblast kláves a po stisku se vrací zpět na domovskou pozici.\n\nNa klávesnici se nedívejte, ani když to zpočátku zpomalí. Právě tak se to naučíte."
)

// Unknown layout in the config (%q is its id).
const UnknownLayout = "Rozložení „%s“ jsem nenašel, používám macOS Czech QWERTY."

// Placement test (Vstupní pohovor).
const (
	PlacementTitle      = "Vstupní pohovor"
	PlacementRoundTitle = "Vstupní pohovor %d/%d: %s"
	PlacementStageIntro = "Fáze %d/%d: %s"
	PlacementRules      = "Bez nápovědy. Pište, jak umíte. Když to nepůjde, pohovor skončí a nic se neděje."
	PlacementPrevOK     = "Předchozí fáze splněna: %.0f WPM, přesnost %.0f %%."
	PlacementStageHint  = "Enter začít · Esc ukončit pohovor · Ctrl+C konec"
	PlacementOptionNote = "Když Option nepíše znaky, fáze skončí po pěti pokusech. Pohovor ukončíte klávesou Esc."

	PlacementStageLetters    = "Písmena"
	PlacementStageDiacritics = "Diakritika"
	PlacementStageDigits     = "Číslice"
	PlacementStageShift      = "Symboly se Shiftem"
	PlacementStageOption     = "Symboly s %s"
	PlacementStageCode       = "Skutečný kód"

	OfferTitle    = "Pracovní nabídka"
	OfferGreeting = "Dobrý den,"
	OfferFromZero = "pohovor skončil u první fáze. To je v pořádku, začínáte od nuly a každý někde začíná."
	OfferHolding  = "Hodnost: %s."
	OfferPractice = "Symboly si můžete procvičit hned v lekcích: Kariéra."
	OfferRank     = "na základě pohovoru Vám nabízíme pozici: %s."
	OfferNoChange = "pohovor nic nepřidal, Vaše hodnost zůstává: %s."
	OfferStart    = "Začínáte v kapitole %d: %s."
	OfferHint     = "Enter pokračovat · Ctrl+C konec"
)

// OfferSkipped is the line of the job offer about the n chapters placement closed.
func OfferSkipped(n int) string {
	return fmt.Sprintf("%s %s před nástupem, bez hvězd. %s lekce zůstávají otevřené.",
		Plural(n, "Odemčena", "Odemčeny", "Odemčeno"), Chapters(n), Plural(n, "Její", "Jejich", "Jejich"))
}

// Settings (Nastavení).
const (
	SettingsTitle       = "Nastavení"
	SettingsStyle       = "Vzhled"
	SettingsLayout      = "Rozložení"
	SettingsUpdateCheck = "Kontrola aktualizací"
	SettingsRetake      = "Znovu vstupní pohovor"
	SettingsOn          = "zapnutá"
	SettingsOff         = "vypnutá"
	SettingsLayoutList  = "Dostupná rozložení:"
	SettingsLayoutWarn  = "Statistiky se vedou zvlášť pro každé rozložení."
	// SettingsBrokenLayout reports a user layout file that could not be read
	// (%s is the file name).
	SettingsBrokenLayout = "Rozložení ze souboru %s jsem nenačetl."
	// SettingsLayoutMissing is shown when the layout in use has no file in the
	// list (%s is its name).
	SettingsLayoutMissing = "Používané rozložení „%s“ není v seznamu."
	SettingsRetakeNote    = "Pohovor nikdy nesníží hodnost."
	HintSettings          = "↑↓ výběr · Enter ←→ změnit · Esc domů · Ctrl+T styl"
)

// Messages of `tuipe upgrade`.
const (
	UpgradeTooManyRedirects   = "příliš mnoho přesměrování"
	UpgradeBadStatus          = "server odpověděl HTTP %d"
	UpgradeOffline            = "Nelze se spojit s GitHubem. Zkontrolujte připojení k internetu a zkuste to znovu."
	UpgradeRateLimited        = "GitHub dočasně odmítl dotaz (příliš mnoho požadavků). Zkuste to za chvíli."
	UpgradeNoRelease          = "Zatím neexistuje žádné vydání ke stažení."
	UpgradeBadResponse        = "GitHub odpověděl nečekaně (HTTP %d)."
	UpgradeDownloadFail       = "Stažení se nepovedlo: %v. Nainstalovaná verze zůstala beze změny."
	UpgradeNoAsset            = "Vydání %s neobsahuje soubor %s pro tento systém."
	UpgradeNoChecksum         = "Vydání %s neobsahuje kontrolní součty (checksums.txt) pro %s."
	UpgradeChecksumFail       = "Kontrolní součet staženého souboru nesouhlasí. Nainstalovaná verze zůstala beze změny."
	UpgradeBadArchive         = "Stažený archiv je neplatný: %v. Nainstalovaná verze zůstala beze změny."
	UpgradeNotWritable        = "Do %s nelze zapisovat. Spusťte příkaz s dostatečným oprávněním (například přes sudo) nebo tuipe nainstalujte znovu do složky, kam smíte zapisovat."
	UpgradeNotWritableWin     = "Do %s nelze zapisovat. Spusťte terminál jako správce nebo tuipe nainstalujte znovu do složky, kam smíte zapisovat."
	UpgradeReadOnly           = "Složka %s je jen pro čtení. Nainstalujte tuipe znovu do složky, kam lze zapisovat."
	UpgradeRestoreFail        = "Výměna se nepovedla a původní program zůstal přejmenovaný na %s. Přejmenujte ho zpět na %s."
	UpgradeNoExecutable       = "Nelze zjistit, kde je tuipe nainstalováno: %v."
	UpgradeInsecureURL        = "Odmítnuto nezabezpečené stahování (jen HTTPS): %s"
	UpgradeBadTag             = "GitHub vrátil nečekanou značku vydání %q, nelze porovnat verze."
	UpgradeTooBig             = "soubor je větší než %d bajtů"
	UpgradeUnsafeEntry        = "nebezpečná položka %q"
	UpgradeUnsupported        = "nepodporovaná položka %q"
	UpgradeDuplicate          = "soubor %s je v archivu víckrát"
	UpgradeMissingBin         = "v archivu chybí %s"
	UpgradeEmptyBin           = "program je prázdný"
	UpgradeWrongPlatform      = "soubor není program pro %s/%s"
	UpgradeAvailableGoInstall = "Je dostupná novější verze: %s (nainstalovaná je %s). Aktualizujte příkazem: go install github.com/jiripisa/tuipe/cmd/tuipe@latest"
	UpgradeReplaceFail        = "Výměna programu se nepovedla: %v. Nainstalovaná verze zůstala beze změny."
	UpgradeNotRelease         = "Tato kopie tuipe (verze %s) nebyla nainstalována z vydání, proto se nedá aktualizovat příkazem upgrade.\nNainstalujte ji znovu příkazem:\n  go install github.com/jiripisa/tuipe/cmd/tuipe@latest"
	UpgradeAvailable          = "Je dostupná novější verze: %s (nainstalovaná je %s). Aktualizujte příkazem: tuipe upgrade"
	UpgradeAlreadyLast        = "Máte nejnovější verzi (%s)."
	UpgradeDone               = "Aktualizováno z verze %s na %s."
)

// Update notice on the home screen.
const UpdateNotice = "Je dostupná nová verze %s. Aktualizujte příkazem tuipe upgrade."
