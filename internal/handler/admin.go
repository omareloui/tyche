package handler

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

func (h Handler) AdminFxRates(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	currs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}

	status, err := h.fxRatesStatus(r)
	if err != nil {
		return nil, err
	}

	return views.AdminFxRates(getClaims(r.Context()), b, status, currs, h.colStatus(r)), nil
}

func (h Handler) colStatus(r *http.Request) views.ColStatusData {
	if h.colJob == nil {
		return views.ColStatusData{}
	}
	return views.ColStatusData{
		Status:    h.colJob.Status(),
		FetchedAt: h.colJob.FetchedAt(r.Context()),
		Interval:  h.colJob.Interval(),
	}
}

func (h Handler) AdminColStatus(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return views.ColStatus(h.colStatus(r)), nil
}

func (h Handler) RefreshCostOfLiving(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if h.colJob == nil {
		return nil, resperr.New(http.StatusNotFound, "The cost of living job is off")
	}
	queued := h.colJob.Trigger()
	logger.FromCtx(r.Context()).Info("triggered the cost of living job", zap.Bool("queued", queued))
	s := h.colStatus(r)
	s.Pending = queued
	return views.ColStatus(s), nil
}

func (h Handler) AdminFxRatesStatus(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	status, err := h.fxRatesStatus(r)
	if err != nil {
		return nil, err
	}
	return views.FxRatesStatus(status), nil
}

func (h Handler) RefreshFxRates(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	queued := h.fxJob.Trigger()
	l.Info("triggered the fx rates job", zap.Bool("queued", queued))

	status, err := h.fxRatesStatus(r)
	if err != nil {
		return nil, err
	}
	// The run starts in the background; show it as pending right away so the
	// status keeps polling until it's done.
	status.Pending = queued
	return views.FxRatesStatus(status), nil
}

func (h Handler) fxRatesStatus(r *http.Request) (views.FxRatesStatusData, error) {
	info, err := h.fxJob.Info(r.Context())
	if err != nil {
		logger.FromCtx(r.Context()).Error("fetching the fx rates info", zap.Error(err))
		return views.FxRatesStatusData{}, err
	}
	return views.FxRatesStatusData{
		Info:     info,
		Status:   h.fxJob.Status(),
		Interval: h.fxJob.Interval(),
	}, nil
}

func (h Handler) AdminTenants(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	data, err := h.adminTenantsData(r)
	if err != nil {
		return nil, err
	}
	return views.AdminTenants(getClaims(r.Context()), b, data), nil
}

// DeleteTenant deletes a user's account, e.g. when they forgot the password
// of their encrypted database (and its recovery code) or it's broken: they
// start over by signing in again.
func (h Handler) DeleteTenant(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	id := r.PathValue("id")
	if !repository.ValidTenantID(id) {
		return nil, resperr.New(http.StatusNotFound, "No such account.")
	}

	data, err := h.adminTenantsData(r)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(data.Tenants, func(t models.TenantInfo) bool { return t.ID == id }) {
		return nil, resperr.New(http.StatusNotFound, "No such account.")
	}
	if id == data.Self {
		return nil, resperr.New(http.StatusUnprocessableEntity, "That's your own account, use Reset account instead.")
	}
	if strings.TrimSpace(r.FormValue("confirm")) != id {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Type the account ID to confirm.")
	}

	if err := h.repo.DeleteTenant(ctx, id); err != nil {
		logger.FromCtx(ctx).Error("deleting a tenant", zap.String("tenant_id", id), zap.Error(err))
		return nil, err
	}
	forgetUnlocks(id)
	if err := h.badges.Forget(ctx, id); err != nil {
		logger.FromCtx(ctx).Error("forgetting a tenant's push subscriptions", zap.String("tenant_id", id), zap.Error(err))
	}

	data, err = h.adminTenantsData(r)
	if err != nil {
		return nil, err
	}
	data.Deleted = id
	// The form targets its error message, the list replaces itself.
	w.Header().Set("HX-Retarget", "#admin-tenants")
	w.Header().Set("HX-Reswap", "outerHTML")
	return views.AdminTenantsList(data), nil
}

func (h Handler) adminTenantsData(r *http.Request) (views.AdminTenantsData, error) {
	tenants, err := h.repo.ListTenants(r.Context())
	if err != nil {
		logger.FromCtx(r.Context()).Error("listing the tenants", zap.Error(err))
		return views.AdminTenantsData{}, err
	}
	claims := getClaims(r.Context())
	return views.AdminTenantsData{
		Tenants: tenants,
		Self:    models.TenantID(claims.OAuthProvider, claims.OAuthID),
	}, nil
}

// adminAIDays is how many days the AI usage page's daily chart shows.
const adminAIDays = 90

// aiLimits are a tenant's AI calls a calendar month of each kind, unless an
// admin set its own.
func (h Handler) aiLimits() map[models.ScanKind]int {
	return map[models.ScanKind]int{
		models.ScanReceipt:     h.ocrCfg.ScansPerMonth,
		models.ScanSMSGuess:    h.ocrCfg.SMSGuessesPerMonth,
		models.ScanColEstimate: h.ocrCfg.ColEstimatesPerMonth,
		models.ScanTaxRules:    h.ocrCfg.TaxRulesPerMonth,
		models.ScanVisaRules:   h.ocrCfg.VisaRulesPerMonth,
		models.ScanMerchant:    h.ocrCfg.MerchantGuessesPerMonth,
		models.ScanQuickAdd:    h.ocrCfg.QuickAddsPerMonth,
		models.ScanSplit:       h.ocrCfg.SplitGuessesPerMonth,
	}
}

// AdminAI is every AI call's usage, estimated: totals, a projection,
// charts, top accounts.
func (h Handler) AdminAI(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	d, err := h.adminAIData(r)
	if err != nil {
		return nil, err
	}
	return views.AdminAI(getClaims(r.Context()), b, d), nil
}

// AdminAILimits is every tenant's usage with its monthly limits of each
// kind.
func (h Handler) AdminAILimits(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	d, err := h.adminAIData(r)
	if err != nil {
		return nil, err
	}
	return views.AdminAILimits(getClaims(r.Context()), b, d), nil
}

func (h Handler) adminAIData(r *http.Request) (views.AdminAIData, error) {
	ctx := r.Context()
	now := time.Now().UTC()
	rng := r.URL.Query().Get("range")
	if rng != "all" {
		rng = "12m"
	}
	scans, err := h.repo.ReceiptScansSince(ctx, time.Time{})
	if err != nil {
		return views.AdminAIData{}, err
	}
	limits, err := h.repo.AILimits(ctx)
	if err != nil {
		return views.AdminAIData{}, err
	}
	var monthlyFrom time.Time
	if rng == "12m" {
		monthlyFrom, _ = models.CalendarMonth(now)
		monthlyFrom = monthlyFrom.AddDate(0, -11, 0)
	}
	defs := h.aiLimits()
	return views.AdminAIData{
		Report: models.NewAIUsageReport(scans, now, now.AddDate(0, 0, -(adminAIDays-1)), monthlyFrom, limits, defs),
		Range:  rng,
		Enabled: map[models.ScanKind]bool{
			models.ScanReceipt:     h.ocr != nil,
			models.ScanSMSGuess:    h.smsGuess != nil,
			models.ScanColEstimate: h.colGuess != nil,
			models.ScanTaxRules:    h.taxGuess != nil,
			models.ScanVisaRules:   h.visaGuess != nil,
			models.ScanMerchant:    h.merchantGuess != nil,
			models.ScanQuickAdd:    h.txGuess != nil,
			models.ScanSplit:       h.splitGuess != nil,
		},
		Model:         h.ocrCfg.Model,
		Prices:        h.ocrCfg.Prices(),
		DefaultLimits: defs,
		Now:           now,
	}, nil
}

// maxAILimit is the most calls of a kind a month an admin can give a
// tenant.
const maxAILimit = 100000

// SetAILimits sets a tenant's calls a calendar month of every kind sent
// (limit_<kind>, empty: back to the default), answering its row.
func (h Handler) SetAILimits(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	tenantID := r.PathValue("tenant")
	if !repository.ValidTenantID(tenantID) {
		return nil, resperr.New(http.StatusNotFound, "No such account")
	}
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "Bad form")
	}
	limits := map[models.ScanKind]*int{}
	for _, k := range models.ScanKinds {
		vs, ok := r.Form["limit_"+string(k)]
		if !ok {
			continue
		}
		var limit *int
		if v := strings.TrimSpace(vs[0]); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 || n > maxAILimit {
				return nil, resperr.New(http.StatusUnprocessableEntity, fmt.Sprintf("%s: the limit is a whole number from 0 to %d.", k.Label(), maxAILimit))
			}
			limit = &n
		}
		limits[k] = limit
	}
	for k, limit := range limits {
		if err := h.repo.SetAILimit(ctx, tenantID, k, limit); err != nil {
			return nil, err
		}
		l := logger.FromCtx(ctx).With(zap.String("tenant_id", tenantID), zap.String("kind", string(k)))
		if limit == nil {
			l.Info("reset an AI limit to the default")
		} else {
			l.Info("set an AI limit", zap.Int("limit", *limit))
		}
	}

	d, err := h.adminAIData(r)
	if err != nil {
		return nil, err
	}
	row := models.TenantAIUsage{TenantID: tenantID, Limits: d.DefaultLimits}
	for _, t := range d.Report.Tenants {
		if t.TenantID == tenantID {
			row = t
		}
	}
	return views.AdminAITenantRow(row, d, "Saved"), nil
}
