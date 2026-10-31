package streak_test

import (
	"testing"

	"github.com/jiripisa/tuipe/internal/streak"
)

func TestStreakBudget(t *testing.T) {
	// The missing 28th is forgiven; the count keeps active days only.
	s := streak.Compute([]string{"2026-10-26", "2026-10-27", "2026-10-29"}, "2026-10-29")
	if s.Days != 3 || !s.ForgivenThisWeek || !s.ActiveToday {
		t.Fatalf("got %+v, want 3 days, forgiven, active", s)
	}
	// Two missed days in a row break it.
	if got := streak.Compute([]string{"2026-10-20", "2026-10-21", "2026-10-24"}, "2026-10-24").Days; got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

func TestStreakTwoGapsInAWeekBreak(t *testing.T) {
	// Mon, Wed, Fri with Tue and Thu missed: only one gap is forgiven.
	s := streak.Compute([]string{"2026-10-26", "2026-10-28", "2026-10-30"}, "2026-10-30")
	if s.Days != 2 {
		t.Fatalf("got %d, want 2", s.Days)
	}
}

func TestStreakBudgetIsPerWeek(t *testing.T) {
	// Tue 20 and Tue 27 are gaps in two different ISO weeks: both forgiven.
	days := []string{"2026-10-19", "2026-10-21", "2026-10-22", "2026-10-23", "2026-10-24", "2026-10-25", "2026-10-26", "2026-10-28"}
	if got := streak.Compute(days, "2026-10-28").Days; got != 8 {
		t.Fatalf("got %d, want 8", got)
	}
}

func TestStreakSurvivesTodayNotPlayedYet(t *testing.T) {
	s := streak.Compute([]string{"2026-10-27", "2026-10-28"}, "2026-10-29")
	if s.Days != 2 || s.ActiveToday {
		t.Fatalf("got %+v, want 2 days, not active today", s)
	}
}

func TestStreakYesterdayMissedIsForgivenOnce(t *testing.T) {
	s := streak.Compute([]string{"2026-10-26", "2026-10-27"}, "2026-10-29")
	if s.Days != 2 || !s.ForgivenThisWeek {
		t.Fatalf("got %+v, want 2 days, forgiven", s)
	}
	if got := streak.Compute([]string{"2026-10-26"}, "2026-10-30").Days; got != 0 {
		t.Fatalf("a stale streak is %d, want 0", got)
	}
}

func TestStreakEmptyAndBadInput(t *testing.T) {
	if got := streak.Compute(nil, "2026-10-29"); got != (streak.Status{}) {
		t.Fatalf("got %+v", got)
	}
	if got := streak.Compute([]string{"nonsense", "2026-10-29", "2026-10-29"}, "2026-10-29").Days; got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

func TestStreakAcrossSundayToMonday(t *testing.T) {
	// Sat, Sun, Mon are plain consecutive days over the ISO week boundary.
	if got := streak.Compute([]string{"2026-10-24", "2026-10-25", "2026-10-26"}, "2026-10-26").Days; got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
	// Sunday gap between Sat and Mon: forgiven in last week's budget.
	s := streak.Compute([]string{"2026-10-24", "2026-10-26"}, "2026-10-26")
	if s.Days != 2 || s.ForgivenThisWeek {
		t.Fatalf("got %+v, want 2 days, not forgiven this week", s)
	}
	// Fri, Sun, Mon: Saturday is forgiven, and last week's budget is then spent.
	// Thu 22, Sat 24, Mon 26: Friday and Sunday are both in the same week.
	if got := streak.Compute([]string{"2026-10-22", "2026-10-24", "2026-10-26"}, "2026-10-26").Days; got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}

func TestStreakAcrossYearEnd(t *testing.T) {
	if got := streak.Compute([]string{"2026-12-30", "2026-12-31", "2027-01-01"}, "2027-01-01").Days; got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
	// 2027-01-01 is a Friday in ISO week 53 of 2026, like 2026-12-31.
	s := streak.Compute([]string{"2026-12-30", "2027-01-01"}, "2027-01-01")
	if s.Days != 2 || !s.ForgivenThisWeek {
		t.Fatalf("got %+v, want 2 days, forgiven", s)
	}
	// Monday 2027-01-04 starts week 1; Sunday 3rd is last week's gap.
	s = streak.Compute([]string{"2027-01-02", "2027-01-04"}, "2027-01-04")
	if s.Days != 2 || s.ForgivenThisWeek {
		t.Fatalf("got %+v, want 2 days, not forgiven this week", s)
	}
}
