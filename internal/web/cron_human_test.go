package web

import "testing"

func TestHumanizeCron(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"0 8 * * 1-5", "Weekdays 08:00"},
		{"0 18 * * 1-5", "Weekdays 18:00"},
		{"0 7 * * *", "Daily 07:00"},
		{"0 0 * * *", "Daily 00:00"},
		{"30 9 * * 0,6", "Weekends 09:30"},
		{"0 6 * * 1", "Mons 06:00"},
		{"0 6 * * 1,3,5", "Mon, Wed, Fri 06:00"},
		{"0 9 * * 1-3", "Mon–Wed 09:00"},
		{"0 0 1 * *", "Monthly day 1 00:00"},
		{"", ""},
		// Falls back to raw for anything it can't confidently parse.
		{"*/5 * * * *", "*/5 * * * *"},
		{"0 8 * 1 *", "0 8 * 1 *"},
		{"not a cron", "not a cron"},
		{"0 8 * *", "0 8 * *"},
	}
	for _, c := range cases {
		if got := humanizeCron(c.in); got != c.want {
			t.Errorf("humanizeCron(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
