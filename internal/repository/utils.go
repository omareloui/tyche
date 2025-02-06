package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/queries"
)

func (r *Repository) transact(ctx context.Context, txFunc func(*queries.Queries) error) error {
	db, queries, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	txn, err := db.Begin()
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

	qtx := queries.WithTx(txn)
	err = txFunc(qtx)
	return err
}

func (r *Repository) dbFromCtx(ctx context.Context) (*sql.DB, *queries.Queries, error) {
	ctxValue := ctx.Value(middleware.AccessClaimsCtxKey{})

	if ctxValue == nil {
		return nil, nil, fmt.Errorf("")
	}

	claims := ctxValue.(jwt.AccessClaims)
	return r.tm.GetTenant(claims.OAuthProvider, claims.OAuthID)
}
