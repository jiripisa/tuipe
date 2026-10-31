// Package incident implements the SLA meter of an incident scenario.
package incident

import (
	"math"
	"time"
)

// Severity sets how much faster than the player's usual pace an incident asks.
type Severity int

const (
	SEV3 Severity = iota // about the usual pace
	SEV2                 // about 5 % faster
	SEV1                 // about 15 % faster
)

func (s Severity) String() string {
	switch s {
	case SEV2:
		return "SEV2"
	case SEV1:
		return "SEV1"
	default:
		return "SEV3"
	}
}

func (s Severity) paceMultiplier() float64 {
	switch s {
	case SEV2:
		return 1.05
	case SEV1:
		return 1.15
	default:
		return 1.00
	}
}

const (
	startSLA       = 100.0
	sloSLA         = 99.0
	paceBudgetUse  = 0.6  // share of the budget burnt by finishing exactly at pace
	errorBudgetUse = 0.05 // share of the budget burnt by one error
	minBaseWPM     = 15.0
	charsPerWord   = 5.0
	epsilon        = 1e-9
)

// Params are the numbers that drive a Meter.
type Params struct {
	Start, SLO, BurnPerSec, ErrorBurn float64
}

// NewParams calibrates the burn against the player's recent WPM. baseWPM is
// floored at 15 and chars below 1 is treated as 1.
func NewParams(sev Severity, baseWPM float64, chars int) Params {
	baseWPM = math.Max(baseWPM, minBaseWPM)
	chars = max(chars, 1)
	budget := startSLA - sloSLA
	expectedSecs := float64(chars) / (baseWPM * sev.paceMultiplier() * charsPerWord) * 60
	return Params{
		Start:      startSLA,
		SLO:        sloSLA,
		BurnPerSec: paceBudgetUse * budget / expectedSecs,
		ErrorBurn:  errorBudgetUse * budget,
	}
}

// Meter tracks the SLA over time and errors.
type Meter struct {
	p       Params
	elapsed time.Duration
	errors  int
}

func NewMeter(p Params) *Meter { return &Meter{p: p} }

// Advance burns budget for d of elapsed time.
func (m *Meter) Advance(d time.Duration) { m.elapsed += d }

// Error burns one chunk of budget.
func (m *Meter) Error() { m.errors++ }

func (m *Meter) raw() float64 {
	return m.p.Start - m.p.BurnPerSec*m.elapsed.Seconds() - m.p.ErrorBurn*float64(m.errors)
}

// SLA is the remaining SLA, never below the SLO.
func (m *Meter) SLA() float64 { return math.Max(m.raw(), m.p.SLO) }

// SLO is the level at which the SLA means an outage.
func (m *Meter) SLO() float64 { return m.p.SLO }

// Breached reports an outage: the SLA reached the SLO.
func (m *Meter) Breached() bool { return m.raw() <= m.p.SLO+epsilon }
