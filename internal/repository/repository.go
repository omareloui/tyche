package repository

import (
	"database/sql"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/logger"
	"go.uber.org/zap"
)

type Repository struct {
	db *sql.DB
	q  *db.Queries
	l *zap.Logger
}

func New(db *sql.DB, q *db.Queries) *Repository {
	return &Repository{db: db, q: q, l: logger.Get()}
}
