// Package sqlite registers the sqlite3 driver, built with SQLCipher (go.mod
// replaces mattn/go-sqlite3 with a fork having it) so tenant databases can be
// encrypted; plain ones open as usual. Import it instead of the driver: it
// checks the binary was built with the sqlcipher tag (set by the Makefile
// through GOFLAGS), else the driver is plain SQLite and encrypted databases
// can't be opened. It also adds the app's SQL functions to every connection
// (see LocalMinute).
package sqlite

import (
	"database/sql"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-sqlite3"
)

func init() {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// The registered driver itself: every later sql.Open("sqlite3", …) gets
	// the functions, before any connection is made.
	drv, ok := db.Driver().(*sqlite3.SQLiteDriver)
	if !ok {
		panic("sqlite: unexpected sqlite3 driver")
	}
	drv.ConnectHook = func(c *sqlite3.SQLiteConn) error {
		return c.RegisterFunc("local_minute", LocalMinute, true)
	}

	var version string
	if err := db.QueryRow("PRAGMA cipher_version").Scan(&version); err != nil || version == "" {
		panic("sqlite: built without SQLCipher, build with `-tags sqlcipher` (or through make)")
	}
}

var locs sync.Map

func loadLocation(name string) *time.Location {
	if l, ok := locs.Load(name); ok {
		return l.(*time.Location)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		loc = time.UTC
	}
	locs.Store(name, loc)
	return loc
}

// LocalMinute (SQL local_minute(ts, tz)) is the minute of the day a stored
// time is at in the time zone tz (an IANA name, UTC when unknown), as the
// lists show it, or -1 when it has no time of day: a day without one is
// stored at midnight of its zone (UTC for the ones picked in forms).
func LocalMinute(ts, tz string) int64 {
	ts = strings.TrimSpace(ts)
	var t time.Time
	parsed := false
	for _, layout := range sqlite3.SQLiteTimestampFormats {
		if v, err := time.Parse(layout, ts); err == nil {
			t, parsed = v, true
			break
		}
	}
	if !parsed {
		return -1
	}
	h, m, s := t.Clock()
	if h == 0 && m == 0 && s == 0 && t.Nanosecond() == 0 {
		return -1
	}
	t = t.In(loadLocation(tz))
	return int64(t.Hour()*60 + t.Minute())
}
