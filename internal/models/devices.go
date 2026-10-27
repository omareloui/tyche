package models

import "time"

type Device struct {
	ID         string
	Name       string
	LastSeenAt time.Time
	RevokedAt  time.Time
	CreatedAt  time.Time
}

func (d *Device) IsRevoked() bool {
	return !d.RevokedAt.IsZero()
}
