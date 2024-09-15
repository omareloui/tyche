package repository

import (
	"database/sql"

	"github.com/omareloui/financial-planner/internal/db"
)

type Repository struct {
	db *sql.DB
	q  *db.Queries
}

func New(db *sql.DB, q *db.Queries) *Repository {
	return &Repository{db: db, q: q}
}
