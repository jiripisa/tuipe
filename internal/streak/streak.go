// Package streak computes the "uptime" streak: consecutive days with a
// finished round, where one missed day per ISO week is forgiven by an error
// budget. It never reads the clock; the caller passes today.
package streak

import "time"

const dateLayout = "2006-01-02"

// Status is the streak as shown to the user.
type Status struct {
	Days             int  // active days in the streak
	ForgivenThisWeek bool // the error budget of today's ISO week is spent
	ActiveToday      bool
}

type week struct{ year, number int }

func weekOf(d time.Time) week {
	y, n := d.ISOWeek()
	return week{y, n}
}

// Compute walks back from today over the ISO dates (local time) of days with
// a round. A single missed day between two active days is forgiven once per
// ISO week and is not counted. Today without a round yet neither counts nor
// breaks the streak.
func Compute(days []string, today string) Status {
	now, err := time.Parse(dateLayout, today)
	if err != nil {
		return Status{}
	}
	active := map[time.Time]bool{}
	for _, s := range days {
		if d, err := time.Parse(dateLayout, s); err == nil {
			active[d] = true
		}
	}
	st := Status{ActiveToday: active[now]}
	if st.ActiveToday {
		st.Days = 1
	}
	used := map[week]bool{}
	cur := now
	for {
		prev := cur.AddDate(0, 0, -1)
		switch {
		case active[prev]:
			cur = prev
		case active[prev.AddDate(0, 0, -1)] && !used[weekOf(prev)]:
			used[weekOf(prev)] = true
			cur = prev.AddDate(0, 0, -1)
		default:
			st.ForgivenThisWeek = used[weekOf(now)]
			return st
		}
		st.Days++
	}
}
