package repository

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sharedqueries"
)

// RecordReceiptScan keeps an AI call (any kind), for the usage and the
// tenant's monthly limit.
func (r *Repository) RecordReceiptScan(ctx context.Context, s models.ReceiptScan) error {
	if s.ScannedAt.IsZero() {
		s.ScannedAt = time.Now()
	}
	return r.shared.AddReceiptScan(ctx, sharedqueries.AddReceiptScanParams{
		ID:            idgen.New(),
		TenantID:      s.TenantID,
		ScannedAt:     s.ScannedAt.UTC(),
		Model:         s.Model,
		InputTokens:   int64(s.InputTokens),
		OutputTokens:  int64(s.OutputTokens),
		CostMicros:    s.CostMicros,
		Ok:            s.OK,
		Error:         s.Error,
		Cached:        s.Cached,
		Kind:          string(cmp.Or(s.Kind, models.ScanReceipt)),
		SearchQueries: int64(s.SearchQueries),
	})
}

// AICallsLeft is how many calls of the kind the tenant may still make this
// calendar month (UTC), and its limit (def unless an admin set its own).
func (r *Repository) AICallsLeft(ctx context.Context, tenantID string, kind models.ScanKind, def int, now time.Time) (left, limit int, err error) {
	limit, err = r.AILimit(ctx, tenantID, kind, def)
	if err != nil {
		return 0, 0, err
	}
	from, to := models.CalendarMonth(now)
	n, err := r.shared.CountReceiptScans(ctx, sharedqueries.CountReceiptScansParams{
		TenantID: tenantID, Kind: string(kind), FromAt: from, ToAt: to,
	})
	if err != nil {
		return 0, 0, err
	}
	return max(limit-int(n), 0), limit, nil
}

// AILimit is the tenant's calls of the kind a calendar month, def unless an
// admin set it.
func (r *Repository) AILimit(ctx context.Context, tenantID string, kind models.ScanKind, def int) (int, error) {
	n, err := r.shared.GetAILimit(ctx, sharedqueries.GetAILimitParams{TenantID: tenantID, Kind: string(kind)})
	if errors.Is(err, sql.ErrNoRows) {
		return def, nil
	}
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// AILimits are the tenants' limits set by an admin, by tenant then kind.
func (r *Repository) AILimits(ctx context.Context) (map[string]map[models.ScanKind]int, error) {
	rows, err := r.shared.ListAILimits(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]map[models.ScanKind]int{}
	for _, row := range rows {
		if out[row.TenantID] == nil {
			out[row.TenantID] = map[models.ScanKind]int{}
		}
		out[row.TenantID][models.ScanKind(row.Kind)] = int(row.MonthlyLimit)
	}
	return out, nil
}

// SetAILimit sets the tenant's calls of the kind a calendar month, nil going
// back to the default.
func (r *Repository) SetAILimit(ctx context.Context, tenantID string, kind models.ScanKind, limit *int) error {
	if limit == nil {
		return r.shared.DeleteAILimit(ctx, sharedqueries.DeleteAILimitParams{TenantID: tenantID, Kind: string(kind)})
	}
	return r.shared.SetAILimit(ctx, sharedqueries.SetAILimitParams{
		TenantID: tenantID, Kind: string(kind), MonthlyLimit: int64(*limit), UpdatedAt: time.Now().UTC(),
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
			TenantID:      row.TenantID,
			ScannedAt:     row.ScannedAt,
			Model:         row.Model,
			InputTokens:   int(row.InputTokens),
			OutputTokens:  int(row.OutputTokens),
			CostMicros:    row.CostMicros,
			OK:            row.Ok,
			Error:         row.Error,
			Cached:        row.Cached,
			Kind:          models.ScanKind(row.Kind),
			SearchQueries: int(row.SearchQueries),
		}
	}
	return out, nil
}
