package dates

import (
	"fmt"
	"time"
)

const Layout = "2006-01-02"

func Parse(s string) (time.Time, error) {
	t, err := time.ParseInLocation(Layout, s, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q, expected YYYY-MM-DD", s)
	}
	return t, nil
}

func MustParse(s string) time.Time {
	t, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return t
}

func Format(t time.Time) string {
	return t.Format(Layout)
}

func Today() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func DaysBetween(from, to time.Time) int {
	return int(to.Sub(from).Hours() / 24)
}

func ParsePeriod(s string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01", s, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid period %q, expected YYYY-MM", s)
	}
	return t, nil
}

func EndOfMonth(t time.Time) time.Time {
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return first.AddDate(0, 1, -1)
}

func AddMonths(t time.Time, n int) time.Time {
	y, m, d := t.Date()
	target := time.Date(y, m+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := EndOfMonth(target).Day()
	if d > last {
		d = last
	}
	return time.Date(target.Year(), target.Month(), d, 0, 0, 0, 0, time.UTC)
}
