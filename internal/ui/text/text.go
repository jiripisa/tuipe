// Package text is the single home of user-facing Czech strings.
package text

import (
	"fmt"
	"math"
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

// Rounds counts rounds: "1 kolo", "3 kola", "12 kol".
func Rounds(n int) string {
	return fmt.Sprintf("%d %s", n, Plural(n, "kolo", "kola", "kol"))
}

// SkippedContent is the note about what a layout cannot type, told once. It is
// empty when nothing is skipped.
func SkippedContent(layout string, lessons int) string {
	if lessons == 0 {
		return ""
	}
	return fmt.Sprintf("Rozložení %s neumí některé znaky. Přeskočeno: %d %s.", layout, lessons, Plural(lessons, "lekce", "lekce", "lekcí"))
}

// Usage is printed by `tuipe help`.
const Usage = `tuipe - trenažér psaní všemi deseti (česká klávesnice QWERTY)

Použití:
  tuipe                   spustí trenažér
  tuipe upgrade           aktualizuje na nejnovější verzi
  tuipe upgrade --check   jen zjistí, zda je dostupná novější verze
  tuipe reset --score     smaže postup a historii kol
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

// OldDataSetAside is the notice for a file from an older tuipe (%s is the file, then where it went).
const OldDataSetAside = "Soubor %s je ze starší verze tuipe. Je odložený jako %s a začínáte znovu."

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

// HintPause is the footer of the pause menu.
const HintPause = "↑↓ · Enter · Esc pokračovat"

// Pause menu.
const (
	PauseTitle   = "PAUZA"
	PauseResume  = "pokračovat"
	PauseRestart = "začít znovu"
	PauseLeave   = "odejít"
)

// Notes on the results of a round.
const (
	ResultsPaused   = "pauza: kolo se nepočítá jako rekord"
	ResultsFlawless = "bez chyby" // follows three accuracy stars
	ResultsRank     = "Nová hodnost: %s"
)

// Labels of the round screen.
const (
	RoundStreak     = "série"
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
	RoundPanelSpeed    = "RYCHLOST"
	RoundPanelStats    = "STATISTIKA"
	RoundPanelOverview = "PŘEHLED"
	RoundPanelKeyboard = "KLÁVESNICE"
	RoundNextChar      = "DALŠÍ ZNAK"
	RoundWeakest       = "NEJSLABŠÍ"
)

// Statistics.
const (
	StatsEmpty           = "Zatím žádná kola. Odehrajte první a tady se objeví historie."
	StatsHeatEmpty       = "Zatím není co ukázat: klávesy se obarví, až se nasbírá pár kol."
	StatsWeakestFinger   = "nejslabší prst: %s"
	StatsGroupOverall    = "celkem"
	StatsGroupLetters    = "písmena"
	StatsGroupDiacritics = "diakritika"
	StatsGroupDigits     = "čísla"
	StatsGroupShift      = "Shift"
	StatsGroupOption     = "Option"
	StatsGroupCode       = "kód"
	StatsModeTest        = "Test"
	StatsModeTraining    = "Trénink"
	StatsModePlacement   = "Vstupní test"
)

// More statistics strings.
const (
	StatsTitle    = "Statistiky"
	StatsSpeed    = "Rychlost"
	StatsAccuracy = "Přesnost"
	HintStats     = "←→ záložky · ↑↓ · F filtr · Esc zpět · Ctrl+T styl"

	// StatsFilterLabel heads the line that names which runs the history shows.
	StatsFilterLabel = "Zobrazeno"
	StatsNoRuns      = "Tomuhle výběru nic neodpovídá."
	// Rows of the records; they read Tests only, apart from the days in a row.
	StatsRecordSpeed    = "Nejvyšší rychlost v testu"
	StatsRecordAccuracy = "Nejlepší přesnost v testu"
	StatsRecordDays     = "Nejvíc dní v řadě"
	StatsNoRecord       = "-"
)

// StatsFilters name the choices of the history filter, in the order the F key
// cycles through them.
var StatsFilters = [...]string{"Vše", "Trénink", "Test"}

// StatsDaysInARow is the days practised in a row, such as "3 dny v řadě".
func StatsDaysInARow(n int) string {
	return fmt.Sprintf("%d %s v řadě", n, Plural(n, "den", "dny", "dní"))
}

// StatsTabs are the tabs of the statistics screen.
var StatsTabs = [...]string{"Historie", "Klávesy", "Trendy", "Rekordy"}

// Names of the hands and fingers, indexed by layout.Hand and layout.Finger.
var (
	StatsHandNames   = [...]string{"levý", "pravý"}
	StatsFingerNames = [...]string{"malíček", "prsteník", "prostředník", "ukazovák", "palec"}
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
	ForkPlacement     = "Vstupní test"
	ForkPlacementNote = "Asi 4 minuty. Doporučí, kde začít."
	FirstRunForkHint  = "↑↓ výběr · Enter potvrdit · Ctrl+C konec"

	FirstRunHandHint  = "↑↓ posun · Enter pokračovat"
	FirstRunZeroTitle = "Domovská pozice"
	FirstRunZeroText  = "Prsty leží na domovské řadě: levý ukazovák na f, pravý na j. Na obou klávesách nahmatáte malý výstupek, podle něj se trefíte poslepu.\n\nPalce patří na mezerník. Každý prst obsluhuje svou oblast kláves a po stisku se vrací zpět na domovskou pozici.\n\nNa klávesnici se nedívejte, ani když to zpočátku zpomalí. Právě tak se to naučíte."
)

// Unknown layout in the config (%q is its id).
const UnknownLayout = "Rozložení „%s“ jsem nenašel, používám macOS Czech QWERTY."

// Placement test (Vstupní test).
const (
	PlacementTitle      = "Vstupní test"
	PlacementRoundTitle = "Vstupní test %d/%d: %s"
	PlacementStageIntro = "Fáze %d/%d: %s"
	PlacementRules      = "Bez nápovědy. Pište, jak umíte. Když to nepůjde, test skončí a nic se neděje."
	PlacementPrevOK     = "Předchozí fáze splněna: %.0f zn/min, přesnost %.0f %%."
	PlacementStageHint  = "Enter začít · Esc ukončit test · Ctrl+C konec"
	PlacementOptionNote = "Když Option nepíše znaky, fáze skončí po pěti pokusech. Test ukončíte klávesou Esc."

	PlacementStageLetters    = "Písmena"
	PlacementStageDiacritics = "Diakritika"
	PlacementStageDigits     = "Číslice"
	PlacementStageShift      = "Symboly se Shiftem"
	PlacementStageOption     = "Symboly s %s"
	PlacementStageCode       = "Skutečný kód"

	OfferTitle    = "Výsledek vstupního testu"
	OfferFromZero = "Test skončil u první fáze. To je v pořádku, začnete od nuly a každý někde začíná."
	OfferStart    = "Doporučená lekce: %s (%s)."
	OfferPractice = "Všechny lekce jsou otevřené, začít můžete kteroukoli."
	OfferHint     = "Enter pokračovat · Ctrl+C konec"
)

// OfferPassed says how many of the stages were passed.
func OfferPassed(passed, stages int) string {
	return fmt.Sprintf("Zvládli jste %d %s z %d.", passed, Plural(passed, "fázi", "fáze", "fází"), stages)
}

// Settings (Nastavení).
const (
	SettingsTitle       = "Nastavení"
	SettingsStyle       = "Vzhled"
	SettingsLayout      = "Rozložení"
	SettingsUpdateCheck = "Kontrola aktualizací"
	SettingsRetake      = "Znovu vstupní test"
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
	SettingsRetakeNote    = "Změní jen doporučenou lekci, hvězdy zůstanou."
	HintSettings          = "↑↓ výběr · Enter ←→ změnit · Esc zpět · Ctrl+T styl"
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

// UpdateNotice is the quiet line of the lesson list about a newer version.
const UpdateNotice = "Je dostupná nová verze %s. Aktualizujte příkazem tuipe upgrade."

// TutorialFocus opens the explanation of a lesson: what it teaches, such as
// "Naučíte se: f, j, mezera."
func TutorialFocus(chars []rune) string { return "Naučíte se: " + charNames(chars) + "." }

func charNames(chars []rune) string {
	names := make([]string, len(chars))
	for i, r := range chars {
		switch r {
		case ' ':
			names[i] = "mezera"
		case '\n':
			names[i] = "Enter"
		case ',':
			names[i] = "čárka" // the list is joined by commas
		default:
			names[i] = string(r)
		}
	}
	return strings.Join(names, ", ")
}

// The lesson list and the lesson screen.
const (
	LessonStarted      = "rozpracováno"
	LessonRecommended  = "doporučeno"
	LessonsStars       = "★ %d"
	LessonsAccuracy    = "přesnost" // above the accuracy stars of the list
	LessonsSpeed       = "rychlost" // above the speed stars of the list
	HintLessons        = "↑↓ výběr · Enter otevřít · S statistiky · N nastavení"
	HintLesson         = "↑↓ výběr · ←→ délka · Enter spustit · Esc zpět"
	LessonLanguage     = "Jazyk: %s"
	LessonTitle        = "Lekce · %s"
	LessonPartTeach    = "Výuka"
	LessonPartTraining = "Trénink"
	LessonPartTest     = "Test"
	LessonTaught       = "✓ hotovo"
	LessonNoResult     = "zatím bez výsledku"
)

// LessonLengths names the lengths of a Trénink, shortest first.
var LessonLengths = [3]string{"krátký", "střední", "dlouhý"}

// LessonSpeed is a speed in characters per minute, such as "300 zn/min".
func LessonSpeed(cpm float64) string { return fmt.Sprintf("%.0f zn/min", cpm) }

// LessonBest labels the best result of a Test.
func LessonBest(result string) string { return "nejlepší " + result }

// LessonLast labels the last result of a part.
func LessonLast(result string) string { return "poslední " + result }

// LessonReviewFocus lists the characters a review lesson practises.
func LessonReviewFocus(chars []rune) string { return "Procvičíte: " + charNames(chars) }

// Trénink and Test: their rounds and results.
const (
	PartSpeed    = "Rychlost"
	PartAccuracy = "Přesnost"
	PartTime     = "Čas"
	PartMissed   = "Nejvíc chyb"
	// The columns of the results: this round, the last one and the best one.
	PartNow  = "teď"
	PartLast = "minule"
	PartBest = "nejlepší"
	// PartCPMUnit follows the speeds of the table.
	PartCPMUnit = "zn/min"
	// The verdict above the results, on speed.
	PartRecord = "★ Nový rekord rychlosti"
	PartFaster = "▲ Rychlejší než minule"
	PartSlower = "▼ Pomalejší než minule"
	PartSame   = "= Stejně rychle jako minule"
	// PartNotReady is shown when the text of a round cannot be made.
	PartNotReady    = "Text tohoto kola se nepodařilo připravit."
	HintPartResults = "Enter zpět · R znovu · Ctrl+T styl"
)

// PartTitle joins what a round is, such as "Trénink · krátký · f j".
func PartTitle(parts ...string) string { return strings.Join(parts, " · ") }

// PartPercent is an accuracy in 0..1 as a percentage, such as "97,4 %".
func PartPercent(accuracy float64) string {
	return decimal(accuracy*100) + " %"
}

// PartClock is a time in seconds, such as "1:28".
func PartClock(seconds int) string {
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

// PartMissedList names the characters missed most, such as "h · mezera · e".
func PartMissedList(chars []rune) string {
	return strings.ReplaceAll(charNames(chars), ", ", " · ")
}

// PartDelta is how much this round beats an earlier one, with an arrow: up when
// it is better, down when worse. Speed is whole characters per minute; with
// percent it is accuracy in percent with one decimal place.
func PartDelta(v float64, percent bool) string {
	size, unit := fmt.Sprintf("%.0f", math.Abs(v)), ""
	if percent {
		size, unit = decimal(math.Abs(v)), " %"
	}
	switch {
	case v > 0:
		return "▲ +" + size + unit
	case v < 0:
		return "▼ −" + size + unit
	}
	return "= 0" + unit
}

// decimal writes v with one decimal place and a Czech decimal comma.
func decimal(v float64) string {
	return strings.Replace(fmt.Sprintf("%.1f", v), ".", ",", 1)
}

// Výuka, the screen that teaches a lesson.
const (
	// TeachTitle heads the screen: the step first, so a narrow window cuts the lesson title.
	TeachTitle = "Výuka · krok %d/%d · %s"
	// TeachTitleExplain heads the explanation, before the first line starts.
	TeachTitleExplain = "Výuka · %s"
	HintTeachExplain  = "Enter začít · Esc zpět · Ctrl+T styl"
	HintTeach         = "Esc zpět · Ctrl+T styl"
	// TeachUnavailable is shown on the banner of the list when a lesson cannot be taught.
	TeachUnavailable = "Výuku lekce %s se nepodařilo připravit."
)
