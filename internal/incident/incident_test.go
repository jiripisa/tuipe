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
	// Finishing exactly at the required pace with no errors burns half of the budget.
	for sev, mult := range map[incident.Severity]float64{incident.SEV3: 1.00, incident.SEV2: 1.05, incident.SEV1: 1.15} {
		p := incident.NewParams(sev, 40, 200)
		used := p.BurnPerSec * expectedSecs(40, mult, 200)
		near(t, used, 0.5*(p.Start-p.SLO))
	}
}

func TestErrorsBurnBudget(t *testing.T) {
	// Up to 120 characters the budget is 20 errors.
	for _, chars := range []int{50, 120} {
		m := incident.NewMeter(incident.NewParams(incident.SEV3, 40, chars))
		for i := 0; i < 19; i++ {
			m.Error()
		}
		require(t, !m.Breached())
		m.Error()
		require(t, m.Breached())
	}
	// A longer incident forgives proportionally more: 50 errors at 300 characters.
	m := incident.NewMeter(incident.NewParams(incident.SEV3, 40, 300))
	for i := 0; i < 49; i++ {
		m.Error()
	}
	require(t, !m.Breached())
	m.Error()
	require(t, m.Breached())
}

func longestIncident(t *testing.T) int {
	t.Helper()
	lib, err := content.Load()
	if err != nil {
		t.Fatal(err)
	}
	longest := 0
	for _, inc := range lib.Incidents {
		var parts []string
		for _, s := range inc.Steps {
			parts = append(parts, s.Text)
		}
		longest = max(longest, len([]rune(strings.Join(parts, "\n"))))
	}
	return longest
}

// meterAfter is the meter of a typist at the usual pace who gets the given
// share of the characters wrong once.
func meterAfter(chars int, miss float64) *incident.Meter {
	m := incident.NewMeter(incident.NewParams(incident.SEV3, 40, chars))
	m.Advance(time.Duration(expectedSecs(40, 1.0, chars) * float64(time.Second)))
	for range int(float64(chars) * miss) {
		m.Error()
	}
	return m
}

// The accuracy of one star is the line: a 92 % typist at the usual pace survives
// the longest incident, narrowly, and a 90 % one does not.
func TestLongestIncidentIsWinnableAtTheAccuracyOfOneStar(t *testing.T) {
	chars := longestIncident(t)
	if chars < 200 {
		t.Fatalf("the longest incident has %d characters", chars)
	}
	m := meterAfter(chars, 0.08)
	require(t, !m.Breached())
	if left := m.SLA() - m.SLO(); left > 0.1 {
		t.Errorf("%.2f of the budget left at 92 %%, want a narrow win", left)
	}
	require(t, meterAfter(chars, 0.10).Breached())
}

func TestSlowPlayerBreachesAtTwiceTheTime(t *testing.T) {
	p := incident.NewParams(incident.SEV3, 40, 200)
	secs := expectedSecs(40, 1.0, 200)

	m := incident.NewMeter(p)
	m.Advance(time.Duration(1.9 * secs * float64(time.Second)))
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
