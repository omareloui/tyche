package dbconn

import (
	"context"
	"database/sql"
	_ "embed"

	"github.com/omareloui/financial-plans/internal/db"
)

//go:embed sql/schema.sql
var ddl string

func Connect(datasource string) (*db.Queries, error) {
	dbconn, err := sql.Open("sqlite3", datasource)
	if err != nil {
		return nil, err
	}
	// defer dbconn.Close()

	// create schema
	if _, err := dbconn.ExecContext(context.Background(), ddl); err != nil {
		return nil, err
	}
	return db.New(dbconn), nil
}
