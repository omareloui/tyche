package csvimport

import "time"

// ImportStatus is where an import is at.
type ImportStatus string

const (
	// ImportPending: uploaded, its mapping and rows being picked.
	ImportPending ImportStatus = "pending"
	// ImportDone: some of its rows were imported; while its file is kept
	// (Kept) it's opened again to import more.
	ImportDone ImportStatus = "imported"
)

// Source is what an import was uploaded as.
type Source string

const (
	SourceCSV Source = "csv"
	// SourcePDF: a PDF statement the AI read into CSV rows (Content).
	SourcePDF Source = "pdf"
)

// Import is a CSV file (or a PDF statement read into one) imported into an
// account.
type Import struct {
	ID          string
	AccountID   string
	AccountName string
	FileName    string
	Source      Source
	// SourceHash is a PDF's sha256 (hex), reading it again reusing Content.
	SourceHash string
	// Warning is what reading a PDF found off (rows possibly missed).
	Warning string
	// Content is the file (a PDF's rows), nil for the ones imported before
	// files were kept; Kept tells it's there when listed without it.
	Content       []byte
	Kept          bool
	Mapping       Mapping
	Status        ImportStatus
	RowCount      int
	ImportedCount int
	CreatedAt     time.Time
	ImportedAt    time.Time
}
