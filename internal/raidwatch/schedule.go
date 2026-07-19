package raidwatch

import (
	"fmt"
	"time"

	"github.com/teambition/rrule-go"
)

// scheduleAnchor is the DTSTART used for schedules whose RRULE carries none,
// which is every rule the UI produces. It is fixed (not "now") so a rule like
// FREQ=WEEKLY;INTERVAL=2 keeps the same week phase across restarts, and it is in
// the local timezone so BYHOUR/BYMINUTE mean local wall-clock time.
var scheduleAnchor = time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)

// parseRRule turns a bare RRULE string into an evaluatable rule anchored at
// scheduleAnchor in local time. A DTSTART already in the string is respected.
func parseRRule(rule string) (*rrule.RRule, error) {
	opt, err := rrule.StrToROptionInLocation(rule, time.Local)
	if err != nil {
		return nil, err
	}
	if opt.Dtstart.IsZero() {
		opt.Dtstart = scheduleAnchor
	}
	return rrule.NewRRule(*opt)
}

// ValidateRRule reports whether a schedule's RRULE is parseable. It is used by
// the API to reject a bad rule before it is saved.
func ValidateRRule(rule string) error {
	if rule == "" {
		return fmt.Errorf("empty schedule")
	}
	_, err := parseRRule(rule)
	return err
}

// dueSince reports whether an occurrence of the rule falls in (since, now]. That
// window is what makes a schedule fire once per occurrence: after firing we move
// `since` to now, so the same occurrence is not seen again, while an occurrence
// missed during downtime (since is old) still fires once on the next poll.
func dueSince(rule string, since, now time.Time) (bool, error) {
	r, err := parseRRule(rule)
	if err != nil {
		return false, err
	}
	next := r.After(since, false)
	if next.IsZero() {
		return false, nil
	}
	return !next.After(now), nil
}
