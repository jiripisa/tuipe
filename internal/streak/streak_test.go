package streak_test

import (
	"testing"

	"github.com/jiripisa/tuipe/internal/streak"
)

func TestStreakDaysAndBest(t *testing.T) {
	// The 28th is missing: a day without practice ends the run of days.
	s := streak.Compute([]string{"2026-10-26", "2026-10-27", "2026-10-29"}, "2026-10-29")
	if s != (streak.Status{Days: 1, Best: 2, ActiveToday: true}) {
		t.Fatalf("got %+v, want 1 day, best 2, active", s)
	}
	// Best is the longest run in the whole history, not the current one.
	history := []string{"2026-09-01", "2026-09-02", "2026-09-03", "2026-10-28", "2026-10-29"}
	if s := streak.Compute(history, "2026-10-29"); s.Days != 2 || s.Best != 3 {
		t.Fatalf("got %+v, want 2 days, best 3", s)
	}
	// The current run can be the best one.
	if s := streak.Compute([]string{"2026-10-27", "2026-10-28", "2026-10-29"}, "2026-10-29"); s.Days != 3 || s.Best != 3 {
		t.Fatalf("got %+v, want 3 days, best 3", s)
	}
}

func TestStreakEndsWithAMissedDay(t *testing.T) {
	// Two single gaps in a week used to be partly forgiven; now each one ends the run.
	if got := streak.Compute([]string{"2026-10-26", "2026-10-28", "2026-10-30"}, "2026-10-30").Days; got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
	// Yesterday missed and today not played: the run is over.
	s := streak.Compute([]string{"2026-10-26", "2026-10-27"}, "2026-10-29")
	if s.Days != 0 || s.Best != 2 || s.ActiveToday {
		t.Fatalf("got %+v, want 0 days, best 2, not active", s)
	}
}

func TestStreakSurvivesTodayNotPlayedYet(t *testing.T) {
	s := streak.Compute([]string{"2026-10-27", "2026-10-28"}, "2026-10-29")
	if s.Days != 2 || s.ActiveToday {
		t.Fatalf("got %+v, want 2 days, not active today", s)
	}
}

func TestStreakEmptyAndBadInput(t *testing.T) {
	if got := streak.Compute(nil, "2026-10-29"); got != (streak.Status{}) {
		t.Fatalf("got %+v", got)
	}
	if got := streak.Compute([]string{"nonsense", "2026-10-29", "2026-10-29"}, "2026-10-29"); got.Days != 1 || got.Best != 1 {
		t.Fatalf("got %+v, want 1 day, best 1", got)
	}
	if got := streak.Compute([]string{"2026-10-29"}, "today"); got != (streak.Status{}) {
		t.Fatalf("a bad today gives %+v", got)
	}
}

func TestStreakAcrossWeekMonthAndYearEnds(t *testing.T) {
	// Sat, Sun, Mon are plain consecutive days over the ISO week boundary.
	if got := streak.Compute([]string{"2026-10-24", "2026-10-25", "2026-10-26"}, "2026-10-26").Days; got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
	if got := streak.Compute([]string{"2026-12-30", "2026-12-31", "2027-01-01"}, "2027-01-01").Days; got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
	// Sat and Mon with the Sunday missed are two runs of one day.
	if s := streak.Compute([]string{"2027-01-02", "2027-01-04"}, "2027-01-04"); s.Days != 1 || s.Best != 1 {
		t.Fatalf("got %+v, want 1 day, best 1", s)
	}
	// The end of February in a leap year and a daylight saving change do not
	// break a run, since dates are compared as calendar days.
	if got := streak.Compute([]string{"2028-02-28", "2028-02-29", "2028-03-01"}, "2028-03-01").Days; got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
}
