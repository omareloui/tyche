package repository

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sharedqueries"
	"go.uber.org/zap"
)

// ErrTenantDeleted is returned for a tenant whose account an admin deleted,
// until they sign in again.
var ErrTenantDeleted = errors.New("the tenant's account was deleted")

// tenantIDPattern is what a tenant ID (models.TenantID) looks like, so it's
// safe as a file name.
var tenantIDPattern = regexp.MustCompile(`^[A-Z]{2}_[A-Za-z0-9_-]+$`)

// ValidTenantID reports whether id looks like a tenant ID.
func ValidTenantID(id string) bool {
	return tenantIDPattern.MatchString(id)
}

// LoadDeleted reads which tenants' accounts were deleted.
func (r *Repository) LoadDeleted(ctx context.Context) error {
	ids, err := r.shared.ListDeletedTenants(ctx)
	if err != nil {
		return fmt.Errorf("listing the deleted tenants: %w", err)
	}
	r.deletedMu.Lock()
	defer r.deletedMu.Unlock()
	r.deleted = make(map[string]bool, len(ids))
	for _, id := range ids {
		r.deleted[id] = true
	}
	return nil
}

// IsDeleted reports whether an admin deleted the tenant's account.
func (r *Repository) IsDeleted(tenantID string) bool {
	r.deletedMu.RLock()
	defer r.deletedMu.RUnlock()
	return r.deleted[tenantID]
}

// Undelete lets a deleted tenant have a database again, on a new sign in.
func (r *Repository) Undelete(ctx context.Context, tenantID string) error {
	if !r.IsDeleted(tenantID) {
		return nil
	}
	if err := r.shared.UnmarkTenantDeleted(ctx, tenantID); err != nil {
		return fmt.Errorf("undeleting the tenant: %w", err)
	}
	r.deletedMu.Lock()
	delete(r.deleted, tenantID)
	r.deletedMu.Unlock()
	return nil
}

// ListTenants lists every tenant database in the data directory, encrypted
// or not, plus the vaults whose database is missing, last changed first.
func (r *Repository) ListTenants(ctx context.Context) ([]models.TenantInfo, error) {
	entries, err := os.ReadDir(r.tm.baseDir)
	if err != nil {
		return nil, fmt.Errorf("listing the databases: %w", err)
	}

	r.tm.mu.RLock()
	open := make(map[string]bool, len(r.tm.connections))
	for id := range r.tm.connections {
		open[id] = true
	}
	r.tm.mu.RUnlock()

	tenants := make(map[string]*models.TenantInfo)
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".sqlite")
		if !ok || e.IsDir() || !ValidTenantID(id) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		_, bakErr := os.Stat(r.tm.backupPath(id))
		tenants[id] = &models.TenantInfo{
			ID:        id,
			Size:      fi.Size(),
			ModTime:   fi.ModTime(),
			HasBackup: bakErr == nil,
			Open:      open[id],
		}
	}

	vaults, err := r.vaults.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, v := range vaults {
		t, ok := tenants[v.TenantID]
		if !ok {
			t = &models.TenantInfo{ID: v.TenantID, FileMissing: true}
			tenants[v.TenantID] = t
		}
		t.Encrypted = true
		t.EncryptedAt = v.CreatedAt
		t.Sessions = v.Sessions
		t.Devices = v.Devices
	}

	list := make([]models.TenantInfo, 0, len(tenants))
	for _, t := range tenants {
		list = append(list, *t)
	}
	slices.SortFunc(list, func(a, b models.TenantInfo) int {
		if c := b.ModTime.Compare(a.ModTime); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return list, nil
}

// DeleteTenant deletes the tenant's account: their database (and its
// backup) and vault. Their old cookies and device tokens stop working, they
// start over by signing in again.
func (r *Repository) DeleteTenant(ctx context.Context, tenantID string) error {
	if !ValidTenantID(tenantID) {
		return fmt.Errorf("invalid tenant id %q", tenantID)
	}

	// Marked first, so nothing opens (creates) the database meanwhile.
	if err := r.shared.MarkTenantDeleted(ctx, sharedqueries.MarkTenantDeletedParams{TenantID: tenantID, DeletedAt: time.Now().UTC()}); err != nil {
		return fmt.Errorf("marking the tenant deleted: %w", err)
	}
	r.deletedMu.Lock()
	r.deleted[tenantID] = true
	r.deletedMu.Unlock()

	if err := r.tm.remove(tenantID); err != nil {
		return err
	}
	if err := r.vaults.Delete(ctx, tenantID); err != nil {
		return err
	}
	// Its scans stay for the usage history.
	if err := r.shared.DeleteReceiptLimit(ctx, tenantID); err != nil {
		return fmt.Errorf("deleting the tenant's receipt limit: %w", err)
	}
	logger.FromCtx(ctx).Warn("deleted a tenant", zap.String("tenant_id", tenantID))
	return nil
}

// remove closes the tenant's connection and deletes its database files.
func (tm *TenantManager) remove(tenantID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.closeTenant(tenantID)

	path := tm.dbPath(tenantID)
	for _, f := range []string{path, path + "-journal", path + "-wal", path + "-shm", tm.backupPath(tenantID), path + ".export", path + ".swap"} {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("deleting %s: %w", f, err)
		}
	}
	return syncFile(tm.baseDir)
}
