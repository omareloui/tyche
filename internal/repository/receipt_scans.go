package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sharedqueries"
)

// RecordReceiptScan keeps a call reading a receipt, for the usage and the
// tenant's monthly limit.
func (r *Repository) RecordReceiptScan(ctx context.Context, s models.ReceiptScan) error {
	if s.ScannedAt.IsZero() {
		s.ScannedAt = time.Now()
	}
	return r.shared.AddReceiptScan(ctx, sharedqueries.AddReceiptScanParams{
		ID:           idgen.New(),
		TenantID:     s.TenantID,
		ScannedAt:    s.ScannedAt.UTC(),
		Model:        s.Model,
		InputTokens:  int64(s.InputTokens),
		OutputTokens: int64(s.OutputTokens),
		CostMicros:   s.CostMicros,
		Ok:           s.OK,
		Error:        s.Error,
	})
}

// ReceiptScansLeft is how many receipts the tenant may still scan this
// calendar month (UTC), and its limit (def unless set for it).
func (r *Repository) ReceiptScansLeft(ctx context.Context, tenantID string, def int, now time.Time) (left, limit int, err error) {
	limit, err = r.ReceiptLimit(ctx, tenantID, def)
	if err != nil {
		return 0, 0, err
	}
	from, to := models.CalendarMonth(now)
	n, err := r.shared.CountReceiptScans(ctx, sharedqueries.CountReceiptScansParams{
		TenantID: tenantID, FromAt: from, ToAt: to,
	})
	if err != nil {
		return 0, 0, err
	}
	return max(limit-int(n), 0), limit, nil
}

// ReceiptLimit is the tenant's scans a calendar month, def unless an admin
// set it.
func (r *Repository) ReceiptLimit(ctx context.Context, tenantID string, def int) (int, error) {
	n, err := r.shared.GetReceiptLimit(ctx, tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return def, nil
	}
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// ReceiptLimits are the tenants' limits set by an admin.
func (r *Repository) ReceiptLimits(ctx context.Context) (map[string]int, error) {
	rows, err := r.shared.ListReceiptLimits(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.TenantID] = int(row.MonthlyLimit)
	}
	return out, nil
}

// SetReceiptLimit sets the tenant's scans a calendar month, nil going back
// to the default.
func (r *Repository) SetReceiptLimit(ctx context.Context, tenantID string, limit *int) error {
	if limit == nil {
		return r.shared.DeleteReceiptLimit(ctx, tenantID)
	}
	return r.shared.SetReceiptLimit(ctx, sharedqueries.SetReceiptLimitParams{
		TenantID: tenantID, MonthlyLimit: int64(*limit), UpdatedAt: time.Now().UTC(),
	})
}

// ReceiptScansSince is every scan since from (zero for all), oldest first.
func (r *Repository) ReceiptScansSince(ctx context.Context, from time.Time) ([]models.ReceiptScan, error) {
	rows, err := r.shared.ListReceiptScansSince(ctx, from.UTC())
	if err != nil {
		return nil, err
	}
	out := make([]models.ReceiptScan, len(rows))
	for i, row := range rows {
		out[i] = models.ReceiptScan{
			TenantID:     row.TenantID,
			ScannedAt:    row.ScannedAt,
			Model:        row.Model,
			InputTokens:  int(row.InputTokens),
			OutputTokens: int(row.OutputTokens),
			CostMicros:   row.CostMicros,
			OK:           row.Ok,
			Error:        row.Error,
		}
	}
	return out, nil
}
