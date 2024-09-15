package dbconn

import (
	"database/sql"

	"github.com/omareloui/financial-planner/internal/db"
)

func New(datasource string) (*sql.DB, *db.Queries, error) {
	dbconn, err := sql.Open("sqlite3", datasource)
	if err != nil {
		return nil, nil, err
	}
	return dbconn, db.New(dbconn), nil
}
