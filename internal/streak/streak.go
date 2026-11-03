// Package streak counts the days practised in a row: consecutive days with a
// finished round. A day without practice ends the run. It never reads the
// clock; the caller passes today.
package streak

import (
	"sort"
	"time"
)

const dateLayout = "2006-01-02"

// Status is the days in a row as shown to the user.
type Status struct {
	Days        int // days in the current run
	Best        int // the longest run in the whole history
	ActiveToday bool
}

// Compute counts the run of days that ends today, or yesterday when today has
// no round yet, over the ISO dates (local time) of days with a round, and the
// longest run anywhere in them.
func Compute(days []string, today string) Status {
	now, err := time.Parse(dateLayout, today)
	if err != nil {
		return Status{}
	}
	active := map[time.Time]bool{}
	var dates []time.Time
	for _, s := range days {
		if d, err := time.Parse(dateLayout, s); err == nil && !active[d] {
			active[d] = true
			dates = append(dates, d)
		}
	}
	st := Status{ActiveToday: active[now]}

	cur := now
	if !st.ActiveToday {
		cur = now.AddDate(0, 0, -1) // today without a round yet does not break the run
	}
	for active[cur] {
		st.Days++
		cur = cur.AddDate(0, 0, -1)
	}

	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	run := 0
	for i, d := range dates {
		if i > 0 && d.Equal(dates[i-1].AddDate(0, 0, 1)) {
			run++
		} else {
			run = 1
		}
		st.Best = max(st.Best, run)
	}
	return st
}
