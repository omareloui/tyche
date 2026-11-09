package csvimport

import "time"

// ImportStatus is where an import is at.
type ImportStatus string

const (
	// ImportPending: uploaded, its mapping and rows being picked.
	ImportPending ImportStatus = "pending"
	// ImportDone: its transactions were saved, its file dropped.
	ImportDone ImportStatus = "imported"
)

// Import is a CSV file imported into an account.
type Import struct {
	ID          string
	AccountID   string
	AccountName string
	FileName    string
	// Content is the file, until imported.
	Content       []byte
	Mapping       Mapping
	Status        ImportStatus
	RowCount      int
	ImportedCount int
	CreatedAt     time.Time
	ImportedAt    time.Time
}
