package dbconn

import (
	"database/sql"

	"github.com/omareloui/financial-planner/internal/db"
)

func Connect(datasource string) (*db.Queries, error) {
	dbconn, err := sql.Open("sqlite3", datasource)
	if err != nil {
		return nil, err
	}

	return db.New(dbconn), nil
}
