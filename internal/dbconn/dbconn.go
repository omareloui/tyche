package dbconn

import (
	"database/sql"
	"fmt"

	"github.com/omareloui/tyche/internal/db"
)

func New(datasource string) (*sql.DB, *db.Queries, error) {
	dbconn, err := sql.Open("sqlite3", fmt.Sprintf("%s?_foreign_keys=on&_journal_mode=WAL", datasource))
	if err != nil {
		return nil, nil, err
	}
	return dbconn, db.New(dbconn), nil
}
