package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/sharedqueries"
)

// prfLen is the length of a WebAuthn PRF output (and of the salts it's
// evaluated with).
const prfLen = 32

// Passkey is a passkey unlocking the database (fingerprint, face or the
// device's PIN): its key is wrapped with the PRF output the authenticator
// derives from the credential and PRFSalt, which the server never keeps.
type Passkey struct {
	ID           string
	CredentialID []byte
	// Credential is the WebAuthn credential (public key, sign count…) as
	// JSON, opaque to the vault.
	Credential string
	PRFSalt    []byte
	Name       string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

// NewPRFSalt generates the input a new passkey's PRF is evaluated with.
func NewPRFSalt() ([]byte, error) {
	return randomBytes(prfLen)
}

// AddPasskey wraps the key with a passkey's PRF output.
func (s *Store) AddPasskey(ctx context.Context, tenantID string, key []byte, p *Passkey, prfOutput []byte) error {
	if !s.IsEncrypted(tenantID) {
		return ErrNoVault
	}
	if len(prfOutput) != prfLen || len(p.PRFSalt) != prfLen || len(p.CredentialID) == 0 {
		return ErrInvalidSlot
	}
	id := idgen.New()
	w, err := wrapWithSecret(slotAAD(tenantID, KindPasskey, id), prfOutput, key)
	if err != nil {
		return err
	}

	err = s.transact(ctx, func(q *sharedqueries.Queries) error {
		if err := s.insertSlot(ctx, q, tenantID, KindPasskey, id, w, slotOpts{userAgent: truncate(p.Name, 256)}); err != nil {
			return err
		}
		return q.CreateVaultPasskey(ctx, sharedqueries.CreateVaultPasskeyParams{
			SlotID:       id,
			TenantID:     tenantID,
			CredentialID: p.CredentialID,
			Credential:   p.Credential,
			PrfSalt:      p.PRFSalt,
			Name:         truncate(p.Name, 64),
		})
	})
	if err != nil {
		return fmt.Errorf("vault: adding a passkey: %w", err)
	}
	p.ID = id
	return nil
}

// Passkeys lists the tenant's passkeys, newest first.
func (s *Store) Passkeys(ctx context.Context, tenantID string) ([]Passkey, error) {
	rows, err := s.q.ListVaultPasskeys(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("vault: listing the passkeys: %w", err)
	}
	passkeys := make([]Passkey, len(rows))
	for i, r := range rows {
		passkeys[i] = Passkey{
			ID:           r.SlotID,
			CredentialID: r.CredentialID,
			Credential:   r.Credential,
			PRFSalt:      r.PrfSalt,
			Name:         r.Name,
			CreatedAt:    r.CreatedAt,
			LastUsedAt:   r.LastUsedAt.Time,
		}
	}
	return passkeys, nil
}

// OpenPasskey gets the key with a passkey's PRF output, once its assertion
// is verified, saving its updated credential (sign count).
func (s *Store) OpenPasskey(ctx context.Context, tenantID string, credentialID, prfOutput []byte, credential string) ([]byte, error) {
	if len(prfOutput) != prfLen {
		return nil, ErrWrongSecret
	}
	row, err := s.q.GetVaultPasskeySlot(ctx, sharedqueries.GetVaultPasskeySlotParams{TenantID: tenantID, CredentialID: credentialID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidSlot
	}
	if err != nil {
		return nil, fmt.Errorf("vault: getting the passkey: %w", err)
	}
	slot := sharedqueries.VaultSlot{
		ID: row.ID, TenantID: row.TenantID, Kind: row.Kind, KdfParams: row.KdfParams,
		Salt: row.Salt, WrappedKey: row.WrappedKey, CreatedAt: row.CreatedAt,
	}
	key, err := unwrapSlot(&slot, prfOutput)
	if errors.Is(err, errDecrypt) {
		return nil, ErrWrongSecret
	}
	if err != nil {
		return nil, err
	}

	err = s.transact(ctx, func(q *sharedqueries.Queries) error {
		if err := q.TouchVaultSlot(ctx, sharedqueries.TouchVaultSlotParams{ID: row.ID, Now: sql.NullTime{Time: time.Now().UTC(), Valid: true}}); err != nil {
			return err
		}
		return q.TouchVaultPasskey(ctx, sharedqueries.TouchVaultPasskeyParams{SlotID: row.ID, Credential: credential})
	})
	if err != nil {
		return nil, fmt.Errorf("vault: updating the passkey: %w", err)
	}
	return key, nil
}

// DeletePasskey removes a passkey, it doesn't unlock the database anymore.
func (s *Store) DeletePasskey(ctx context.Context, tenantID, id string) error {
	if _, err := s.q.DeleteVaultPasskey(ctx, sharedqueries.DeleteVaultPasskeyParams{SlotID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("vault: deleting a passkey: %w", err)
	}
	return nil
}
