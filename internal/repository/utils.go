package repository

import (
	"context"
	"database/sql"
	"encoding/json"
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
	oauthProvider, oauthId, err := tenantFromCtx(ctx)
	if err != nil {
		return nil, nil, err
	}
	if r.IsDeleted(models.TenantID(oauthProvider, oauthId)) {
		return nil, nil, ErrTenantDeleted
	}

	key, err := r.tenantKey(ctx, oauthProvider, oauthId)
	if err != nil {
		return nil, nil, err
	}

	db, q, err := r.tm.GetTenant(oauthProvider, oauthId, key)
	if err != nil {
		l := logger.FromCtx(ctx)
		l.Error("getting db from context", zap.Error(err))
		return nil, nil, err
	}
	return db, q, nil
}

// tenantKey is the key of the tenant's database in the context, nil when it's
// not encrypted, ErrTenantLocked when it is and the context has none.
func (r *Repository) tenantKey(ctx context.Context, provider models.OAuthProvider, oauthID string) ([]byte, error) {
	if r.vaults == nil || !r.vaults.IsEncrypted(models.TenantID(provider, oauthID)) {
		return nil, nil
	}
	key, _ := ctx.Value(middleware.VaultKeyCtxKey{}).([]byte)
	if key == nil {
		return nil, ErrTenantLocked
	}
	return key, nil
}

// TenantIDFromCtx is the tenant of the access or refresh claims in the
// context (models.TenantID).
func TenantIDFromCtx(ctx context.Context) (string, error) {
	provider, oauthID, err := tenantFromCtx(ctx)
	if err != nil {
		return "", err
	}
	return models.TenantID(provider, oauthID), nil
}

// monthStartFromCtx is the day the user's months start on, from the access
// claims.
func monthStartFromCtx(ctx context.Context) models.MonthStart {
	if claims, ok := ctx.Value(middleware.AccessClaimsCtxKey{}).(*jwt.AccessClaims); ok {
		return claims.MonthStart
	}
	return 1
}

// tenantFromCtx gets who the tenant is from the access or refresh claims.
func tenantFromCtx(ctx context.Context) (models.OAuthProvider, string, error) {
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
		return 0, "", fmt.Errorf("no access nor refresh claims in the context to get the tenant")
	}
	return oauthProvider, oauthId, nil
}

// marshalStrings encodes a JSON list column (counterparty aliases and labels, account
// sms refs), nil as an empty list.
func marshalStrings(strs []string) ([]byte, error) {
	if strs == nil {
		strs = []string{}
	}
	marshaled, err := json.Marshal(strs)
	if err != nil {
		logger.Get().Error("error on marshaling strings for db", zap.Error(err))
		return nil, err
	}
	return marshaled, nil
}

func unmarshalStrings(dbStrs []byte) ([]string, error) {
	if dbStrs == nil {
		return []string{}, nil
	}

	strs := []string{}
	if err := json.Unmarshal(dbStrs, &strs); err != nil {
		logger.Get().Error("error on unmarshaling strings from db", zap.Error(err))
		return []string{}, err
	}
	return strs, nil
}
