package sqlite

import (
	"database/sql"
	"testing"
)

func TestLocalMinute(t *testing.T) {
	cases := []struct {
		ts, tz string
		want   int64
	}{
		{"2026-06-19 09:06:00+03:00", "Africa/Cairo", 9*60 + 6},
		// Stored in UTC, shown in the user's zone (summer, +03:00).
		{"2026-06-18 20:34:04+00:00", "Africa/Cairo", 23*60 + 34},
		// Winter, +02:00, past midnight.
		{"2026-11-26 22:30:00.413416694+00:00", "Africa/Cairo", 0*60 + 30},
		{"2026-11-27 02:34:33.413416694+02:00", "Africa/Cairo", 2*60 + 34},
		// A day without a time.
		{"2026-11-27 00:00:00+00:00", "Africa/Cairo", -1},
		{"2026-11-27", "Africa/Cairo", -1},
		{"nonsense", "Africa/Cairo", -1},
		// Unknown zone: UTC.
		{"2026-06-18 20:34:04+00:00", "Nowhere/Else", 20*60 + 34},
	}
	for _, c := range cases {
		if got := LocalMinute(c.ts, c.tz); got != c.want {
			t.Errorf("LocalMinute(%q, %q) = %d, want %d", c.ts, c.tz, got, c.want)
		}
	}
}

func TestLocalMinuteSQL(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var m int64
	if err := db.QueryRow("SELECT local_minute('2026-06-19 09:06:00+03:00', 'Africa/Cairo')").Scan(&m); err != nil {
		t.Fatal(err)
	}
	if m != 9*60+6 {
		t.Errorf("got %d", m)
	}
}
