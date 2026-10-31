package incident_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/incident"
)

func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func require(t *testing.T, ok bool) {
	t.Helper()
	if !ok {
		t.Fatal("requirement failed")
	}
}

func expectedSecs(wpm, mult float64, chars int) float64 {
	return float64(chars) / (wpm * mult * 5) * 60
}

func TestPaceMultipliers(t *testing.T) {
	// Finishing exactly at the required pace with no errors burns 60 % of the budget.
	for sev, mult := range map[incident.Severity]float64{incident.SEV3: 1.00, incident.SEV2: 1.05, incident.SEV1: 1.15} {
		p := incident.NewParams(sev, 40, 200)
		used := p.BurnPerSec * expectedSecs(40, mult, 200)
		near(t, used, 0.6*(p.Start-p.SLO))
	}
}

func TestErrorsBurnBudget(t *testing.T) {
	// Up to 150 characters the budget is 20 errors.
	for _, chars := range []int{50, 150} {
		m := incident.NewMeter(incident.NewParams(incident.SEV3, 40, chars))
		for i := 0; i < 19; i++ {
			m.Error()
		}
		require(t, !m.Breached())
		m.Error()
		require(t, m.Breached())
	}
	// A longer incident forgives proportionally more: 40 errors at 300 characters.
	m := incident.NewMeter(incident.NewParams(incident.SEV3, 40, 300))
	for i := 0; i < 39; i++ {
		m.Error()
	}
	require(t, !m.Breached())
	m.Error()
	require(t, m.Breached())
}

// The longest incident, at the player's pace and 95 % accuracy, can be won.
func TestLongestIncidentIsWinnableAtNinetyFivePercent(t *testing.T) {
	lib, err := content.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, inc := range lib.Incidents {
		if inc.ID != "INC-1012" {
			continue
		}
		var parts []string
		for _, s := range inc.Steps {
			parts = append(parts, s.Text)
		}
		chars := len([]rune(strings.Join(parts, "\n")))
		p := incident.NewParams(incident.SEV3, 40, chars)
		m := incident.NewMeter(p)
		m.Advance(time.Duration(expectedSecs(40, 1.0, chars) * float64(time.Second)))
		wrong := int(float64(chars) * 0.05 / 0.95) // 5 % of all key presses
		for i := 0; i < wrong; i++ {
			m.Error()
		}
		require(t, !m.Breached())
		return
	}
	t.Fatal("INC-1012 not found")
}

func TestSlowPlayerBreachesAtTwiceTheTime(t *testing.T) {
	p := incident.NewParams(incident.SEV3, 40, 200)
	secs := expectedSecs(40, 1.0, 200)

	m := incident.NewMeter(p)
	m.Advance(time.Duration(secs * float64(time.Second)))
	require(t, !m.Breached())

	m = incident.NewMeter(p)
	m.Advance(time.Duration(2 * secs * float64(time.Second)))
	require(t, m.Breached())
}

func TestBaseWPMHasFloorOf15(t *testing.T) {
	require(t, incident.NewParams(incident.SEV3, 3, 200) == incident.NewParams(incident.SEV3, 15, 200))
}

func TestSLAStartsAtStartAndNeverDropsBelowSLO(t *testing.T) {
	p := incident.NewParams(incident.SEV2, 40, 200)
	m := incident.NewMeter(p)
	near(t, m.SLA(), p.Start)
	near(t, m.SLO(), p.SLO)
	m.Advance(time.Hour)
	near(t, m.SLA(), p.SLO)
	require(t, m.Breached())
}

func TestNonPositiveCharsIsTreatedAsOne(t *testing.T) {
	require(t, incident.NewParams(incident.SEV3, 40, 0) == incident.NewParams(incident.SEV3, 40, 1))
	p := incident.NewParams(incident.SEV3, 40, -5)
	require(t, !math.IsInf(p.BurnPerSec, 0) && !math.IsNaN(p.BurnPerSec))
}

func TestSeverityString(t *testing.T) {
	require(t, incident.SEV3.String() == "SEV3")
	require(t, incident.SEV2.String() == "SEV2")
	require(t, incident.SEV1.String() == "SEV1")
}
