package models

import "time"

type Device struct {
	ID   string
	Name string
	// InstallID is the app install's stable id, signing in again from it
	// reuses the device.
	InstallID string
	// TokenID is the current device token's id, older tokens are rejected.
	TokenID    string
	LastSeenAt time.Time
	RevokedAt  time.Time
	CreatedAt  time.Time
}

func (d *Device) IsRevoked() bool {
	return !d.RevokedAt.IsZero()
}
