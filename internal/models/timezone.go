package models

import (
	"errors"
	"strings"
	"time"
	// the time zones have to load in the static release binary too
	_ "time/tzdata"
)

// DefaultTimezone is the time zone of new users' sms, also the default of
// the user.timezone column.
const DefaultTimezone = "Africa/Cairo"

var ErrInvalidTimezone = errors.New("invalid time zone")

// CommonTimezones are suggested when picking a time zone, any IANA name is
// accepted.
var CommonTimezones = []string{
	"Africa/Cairo", "Africa/Casablanca", "Africa/Johannesburg", "Africa/Lagos", "Africa/Nairobi",
	"America/Chicago", "America/Los_Angeles", "America/New_York", "America/Sao_Paulo", "America/Toronto",
	"Asia/Amman", "Asia/Baghdad", "Asia/Beirut", "Asia/Dubai", "Asia/Istanbul", "Asia/Karachi",
	"Asia/Kolkata", "Asia/Kuwait", "Asia/Qatar", "Asia/Riyadh", "Asia/Shanghai", "Asia/Singapore", "Asia/Tokyo",
	"Australia/Sydney", "Europe/Berlin", "Europe/London", "Europe/Madrid", "Europe/Paris", "Europe/Rome",
	"UTC",
}

// LoadTimezone loads an IANA time zone name, e.g. Africa/Cairo.
func LoadTimezone(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	// LoadLocation takes "" and "Local" as the server's zone
	if name == "" || name == "Local" {
		return nil, ErrInvalidTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, ErrInvalidTimezone
	}
	return loc, nil
}
