package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) transact(ctx context.Context, txFunc func(*queries.Queries) error) (err error) {
	db, queries, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	txn, err := db.Begin()
	if err != nil {
		return err
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
	var oauthId string
	var oauthProvider models.OAuthProvider

	accessCtxValue := ctx.Value(middleware.AccessClaimsCtxKey{})
	if accessCtxValue != nil {
		claims := accessCtxValue.(*jwt.AccessClaims)
		oauthId = claims.OAuthID
		oauthProvider = claims.OAuthProvider
	}

	refreshCtxValue := ctx.Value(middleware.RefreshClaimsCtxKey{})
	if refreshCtxValue != nil {
		claims := refreshCtxValue.(*jwt.RefreshClaims)
		oauthId = claims.OAuthID
		oauthProvider = claims.OAuthProvider
	}

	if oauthId == "" {
		return nil, nil, fmt.Errorf("no access nor refresh claims in the context to get the tenant")
	}

	db, q, err := r.tm.GetTenant(oauthProvider, oauthId)
	if err != nil {
		l := logger.FromCtx(ctx)
		l.Error("getting db from context", zap.Error(err))
		return nil, nil, err
	}
	return db, q, nil
}
