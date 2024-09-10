package repository

import (
	"github.com/omareloui/financial-plans/internal/db"
)

type Repository struct {
	db *db.Queries
}

func New(db *db.Queries) *Repository {
	return &Repository{db: db}
}
