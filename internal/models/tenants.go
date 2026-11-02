package models

import "time"

// TenantInfo is a tenant database, as listed for the admins.
type TenantInfo struct {
	ID      string
	Size    int64
	ModTime time.Time
	// HasBackup is the database kept by an import (.bak).
	HasBackup bool
	// Open is when it has an open connection.
	Open        bool
	Encrypted   bool
	EncryptedAt time.Time
	// Sessions and Devices are the browsers and phones unlocking an
	// encrypted database.
	Sessions int64
	Devices  int64
	// FileMissing is a vault without its database.
	FileMissing bool
}
