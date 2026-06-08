package web

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"
)

// templateFuncs are helpers exposed to page templates and partials.
var templateFuncs = template.FuncMap{
	"humanCron": humanizeCron,
}

// humanizeCron turns a standard 5-field cron expression into a short,
// human-readable phrase like "Weekdays 08:00" or "Daily 07:00".
//
// It deliberately only humanizes the common patterns this tool produces
// (a fixed minute+hour with a simple day rule). Anything it doesn't
// confidently understand falls back to the raw expression, so the UI never
// shows a misleading description.
func humanizeCron(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return expr
	}
	minute, hour, dom, month, dow := fields[0], fields[1], fields[2], fields[3], fields[4]

	clock, ok := humanClock(minute, hour)
	if !ok {
		return expr
	}
	day, ok := humanDay(dom, month, dow)
	if !ok {
		return expr
	}
	if day == "" {
		return clock
	}
	return day + " " + clock
}

// humanClock formats a plain integer minute and hour as "HH:MM".
// Returns ok=false for ranges, lists, steps, or wildcards it can't render.
func humanClock(minute, hour string) (string, bool) {
	m, err := strconv.Atoi(minute)
	if err != nil || m < 0 || m > 59 {
		return "", false
	}
	h, err := strconv.Atoi(hour)
	if err != nil || h < 0 || h > 23 {
		return "", false
	}
	return fmt.Sprintf("%02d:%02d", h, m), true
}

var dayNames = [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

// humanDay describes the day-of-month / month / day-of-week portion.
// An empty string means "every day" with no qualifier needed beyond the clock.
func humanDay(dom, month, dow string) (string, bool) {
	// Only handle the everyday "month = *" case; specific months are rare here.
	if month != "*" {
		return "", false
	}

	domAll := dom == "*"
	dowAll := dow == "*"

	switch {
	case domAll && dowAll:
		return "Daily", true
	case domAll && !dowAll:
		return humanWeekday(dow)
	case !domAll && dowAll:
		if d, err := strconv.Atoi(dom); err == nil && d >= 1 && d <= 31 {
			return "Monthly day " + strconv.Itoa(d), true
		}
		return "", false
	default:
		return "", false
	}
}

// humanWeekday renders a day-of-week field: ranges, common weekday/weekend
// shorthands, single days, and short lists.
func humanWeekday(dow string) (string, bool) {
	switch dow {
	case "1-5":
		return "Weekdays", true
	case "0,6", "6,0", "0,7", "6,7":
		return "Weekends", true
	}

	// Range a-b → "Mon–Fri"
	if a, b, ok := parseDowRange(dow); ok {
		return dayNames[a] + "–" + dayNames[b], true
	}

	// Single day → "Mondays"
	if d, ok := parseDow(dow); ok {
		return dayNames[d] + "s", true
	}

	// Short list "1,3,5" → "Mon, Wed, Fri"
	parts := strings.Split(dow, ",")
	if len(parts) >= 2 && len(parts) <= 4 {
		var names []string
		for _, p := range parts {
			d, ok := parseDow(p)
			if !ok {
				return "", false
			}
			names = append(names, dayNames[d])
		}
		return strings.Join(names, ", "), true
	}

	return "", false
}

// parseDow parses a single day-of-week token (0-7, where 7 == Sunday).
func parseDow(s string) (int, bool) {
	d, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || d < 0 || d > 7 {
		return 0, false
	}
	if d == 7 {
		d = 0
	}
	return d, true
}

func parseDowRange(s string) (int, int, bool) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	a, ok1 := parseDow(parts[0])
	b, ok2 := parseDow(parts[1])
	if !ok1 || !ok2 || a >= b {
		return 0, 0, false
	}
	return a, b, true
}
