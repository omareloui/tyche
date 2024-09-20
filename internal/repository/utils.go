package repository

import (
	"github.com/omareloui/financial-planner/internal/db"
)

func (r *Repository) transact(txFunc func(*db.Queries) error) error {
	txn, err := r.db.Begin()
	if err != nil {
		return nil
	}
	defer func() {
		if p := recover(); p != nil {
			_ = txn.Rollback()
			panic(p)
		} else if err != nil {
			_ = txn.Rollback()
		} else {
			err = txn.Commit()
		}
	}()

	qtx := r.q.WithTx(txn)
	err = txFunc(qtx)
	return err
}
