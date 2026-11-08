package repository

import (
	"context"
	"fmt"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

// EnableEncryption encrypts the tenant's database with the password, returning
// its key and recovery code. Its vault is created first, so a database left
// in plain text by a crash is encrypted on its next open (TenantManager.open).
func (r *Repository) EnableEncryption(ctx context.Context, password string) (key []byte, recoveryCode string, err error) {
	l := logger.FromCtx(ctx)

	provider, oauthID, err := tenantFromCtx(ctx)
	if err != nil {
		return nil, "", err
	}
	tenantID := models.TenantID(provider, oauthID)
	if r.vaults.IsEncrypted(tenantID) {
		return nil, "", fmt.Errorf("the database is already encrypted")
	}

	key, recoveryCode, err = r.vaults.Create(ctx, tenantID, password)
	if err != nil {
		l.Error("creating the vault", zap.Error(err))
		return nil, "", err
	}

	if err := r.tm.Encrypt(provider, oauthID, key); err != nil {
		l.Error("encrypting the database", zap.Error(err))
		if delErr := r.vaults.Delete(ctx, tenantID); delErr != nil {
			l.Error("deleting the vault of a failed encryption", zap.Error(delErr))
		}
		return nil, "", err
	}

	l.Info("encrypted the database", zap.String("tenant_id", tenantID))
	return key, recoveryCode, nil
}

// DisableEncryption writes the tenant's database back in plain text and
// deletes its vault. The vault is deleted last: a crash (or a failure) in
// between leaves a plain database encrypted again on its next open.
func (r *Repository) DisableEncryption(ctx context.Context, key []byte) error {
	l := logger.FromCtx(ctx)

	provider, oauthID, err := tenantFromCtx(ctx)
	if err != nil {
		return err
	}
	tenantID := models.TenantID(provider, oauthID)
	if !r.vaults.IsEncrypted(tenantID) {
		return nil
	}

	deleteVault := func() error { return r.vaults.Delete(ctx, tenantID) }
	if err := r.tm.Decrypt(provider, oauthID, key, deleteVault); err != nil {
		l.Error("decrypting the database", zap.Error(err))
		return err
	}

	l.Info("decrypted the database", zap.String("tenant_id", tenantID))
	return nil
}
