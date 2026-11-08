package repository

import (
	"context"
	"errors"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/tzctx"
	"go.uber.org/zap"
)

// RunDailyUpkeep runs, on the tenant's first request of the day (in the
// user's time zone), what's due once a day: TopUpRecurrentTxs, then purging
// the receipts scanned for a transaction never saved. What fails is logged:
// the request goes on without.
func (r *Repository) RunDailyUpkeep(ctx context.Context, tenantID string) {
	y, m, d := tzctx.In(ctx, time.Now()).Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)

	r.upkeepMu.Lock()
	if r.keptUp[tenantID] == day {
		r.upkeepMu.Unlock()
		return
	}
	r.keptUp[tenantID] = day
	r.upkeepMu.Unlock()

	if err := r.TopUpRecurrentTxs(ctx); err != nil {
		r.forgetDailyUpkeep(tenantID)
		if !errors.Is(err, ErrTenantLocked) && !errors.Is(err, ErrTenantDeleted) {
			logger.FromCtx(ctx).Error("writing the recurrent transactions' occurrences", zap.Error(err))
		}
		return
	}
	if _, q, err := r.dbFromCtx(ctx); err == nil {
		purgeOrphanReceipts(ctx, q, time.Now())
	}
}

// forgetDailyUpkeep makes the tenant's next request run its daily upkeep
// again, e.g. after its database got replaced.
func (r *Repository) forgetDailyUpkeep(tenantID string) {
	r.upkeepMu.Lock()
	delete(r.keptUp, tenantID)
	r.upkeepMu.Unlock()
}

// forgetDailyUpkeepOf is forgetDailyUpkeep of the tenant in ctx.
func (r *Repository) forgetDailyUpkeepOf(ctx context.Context) {
	if id, err := TenantIDFromCtx(ctx); err == nil {
		r.forgetDailyUpkeep(id)
	}
}
