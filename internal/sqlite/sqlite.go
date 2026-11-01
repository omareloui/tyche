// Package sqlite registers the sqlite3 driver, built with SQLCipher (go.mod
// replaces mattn/go-sqlite3 with a fork having it) so tenant databases can be
// encrypted; plain ones open as usual. Import it instead of the driver: it
// checks the binary was built with the sqlcipher tag (set by the Makefile
// through GOFLAGS), else the driver is plain SQLite and encrypted databases
// can't be opened.
package sqlite

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func init() {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	var version string
	if err := db.QueryRow("PRAGMA cipher_version").Scan(&version); err != nil || version == "" {
		panic("sqlite: built without SQLCipher, build with `-tags sqlcipher` (or through make)")
	}
}
