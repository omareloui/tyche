package handler

import (
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/numsutils"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

func (h Handler) Decide(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		l.Error("fetching currencies", zap.Error(err))
		return nil, err
	}

	ws, err := h.repo.GetWorkSettings(r.Context())
	if err != nil {
		return nil, err
	}

	items, err := h.repo.GetWishlistItems(r.Context())
	if err != nil {
		return nil, err
	}

	claims := getClaims(r.Context())
	return views.Decide(claims, balances, curs, ws, items,
		mapWishlistItemToFD(dfltWishlistItem(&claims.DefaultCurrency), &errs.ValidationError{}),
		mapPurchaseToFD(&models.Purchase{Currency: claims.DefaultCurrency})), nil
}

// Afford checks a purchase from the afford form.
func (h Handler) Afford(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		return nil, err
	}

	claims := getClaims(r.Context())
	p, err := mapFDToPurchase(r, curs, &claims.DefaultCurrency)
	if errors.Is(err, errNoInstallments) {
		return views.AffordHint("Fill in the months or the amount per month."), nil
	}
	if err != nil {
		return views.AffordResult(nil, nil), nil
	}

	return h.affordResult(r, p)
}

// AffordSection renders the afford form, prefilled from a wishlist item if
// asked, with its result.
func (h Handler) AffordSection(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		return nil, err
	}

	claims := getClaims(r.Context())
	p := &models.Purchase{Currency: claims.DefaultCurrency}
	name := ""

	if id := r.URL.Query().Get("wishlist_id"); id != "" {
		item, err := h.repo.GetWishlistItem(r.Context(), id)
		if err != nil {
			return nil, err
		}
		name = item.Name
		p = &models.Purchase{
			Price:             item.Price,
			Currency:          item.Currency,
			Installments:      item.Installments,
			InstallmentAmount: item.InstallmentAmount,
		}
	}

	result, err := h.affordResult(r, p)
	if err != nil {
		return nil, err
	}
	return views.AffordSection(mapPurchaseToFD(p), name, curs, &claims.DefaultCurrency, result), nil
}

func (h Handler) affordResult(r *http.Request, p *models.Purchase) (templ.Component, error) {
	if p.Price <= 0 && p.TotalCost() <= 0 {
		return views.AffordResult(nil, nil), nil
	}

	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}

	goals, err := h.repo.GetUnfulfilledGoals(r.Context())
	if err != nil {
		return nil, err
	}

	paces, err := h.repo.GetGoalPaces(r.Context(), goals)
	if err != nil {
		return nil, err
	}

	ws, err := h.repo.GetWorkSettings(r.Context())
	if err != nil {
		return nil, err
	}

	claims := getClaims(r.Context())
	a := models.CheckAffordability(*p, balances, goals, paces, ws, &claims.DefaultCurrency)
	return views.AffordResult(&a, ws), nil
}

func (h Handler) EditWorkSettings(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		return nil, err
	}

	fd := &views.WorkSettingsFD{
		Salary: &form.AmountFD{
			Amount:     form.FDInput{Value: r.FormValue("amount")},
			CurrencyID: form.FDInput{Value: r.FormValue("currency_id")},
		},
		HoursPerWeek: form.FDInput{Value: r.FormValue("work_hours_per_week")},
	}

	salary, err := parseFloatIfExists(fd.Salary.Amount.Value)
	if err != nil || salary <= 0 {
		fd.Salary.Amount.Error = "Enter your monthly salary or income"
	}
	hours, err := strconv.ParseFloat(fd.HoursPerWeek.Value, 64)
	if err != nil || hours <= 0 || hours > 168 {
		fd.HoursPerWeek.Error = "Between 1 and 168 hours"
	}
	cur := slicesutils.Find(curs, func(c models.Currency) bool { return c.ID == fd.Salary.CurrencyID.Value })
	if cur == nil {
		fd.Salary.CurrencyID.Error = "Pick a currency"
	}

	claims := getClaims(r.Context())
	if fd.Salary.Amount.Error != "" || fd.HoursPerWeek.Error != "" || cur == nil {
		ws, err := h.repo.GetWorkSettings(r.Context())
		if err != nil {
			return nil, err
		}
		return views.WorkSettingsCard(ws, fd, curs, &claims.DefaultCurrency, true), nil
	}

	ws := &models.WorkSettings{MonthlySalary: salary, Currency: *cur, HoursPerWeek: hours}
	if err := h.repo.SetWorkSettings(r.Context(), ws); err != nil {
		l.Error("saving work settings", zap.Error(err))
		return nil, err
	}
	l.Info("saved work settings")

	w.Header().Set("HX-Trigger", views.WorkSettingsChangedEvent)
	return views.WorkSettingsCard(ws, views.NewWorkSettingsFD(ws), curs, &claims.DefaultCurrency, false), nil
}

func mapPurchaseToFD(p *models.Purchase) *views.PurchaseFD {
	mode := views.PayCash
	if p.IsInstallments() {
		mode = views.PayInstallments
	}
	return &views.PurchaseFD{
		AmountFD: &form.AmountFD{
			Amount:     form.FDInput{Value: formatFloatIfNonZero(p.Price)},
			CurrencyID: form.FDInput{Value: p.Currency.ID},
		},
		Mode:              form.FDInput{Value: mode},
		Installments:      form.FDInput{Value: formatIntIfNonZero(p.Installments)},
		InstallmentAmount: form.FDInput{Value: formatFloatIfNonZero(p.InstallmentAmount)},
	}
}

var (
	errInvalidPurchase = errors.New("invalid purchase")
	// errNoInstallments is paying in installments with neither their count
	// nor their amount. The purchase is still returned, as cash.
	errNoInstallments = errors.New("missing the installments")
)

// mapFDToPurchase reads a price, its currency and, when paying in
// installments, either their count or their amount (working out the other
// from the price) or both, when the installments cost more than the price.
func mapFDToPurchase(r *http.Request, curs []models.Currency, dflt *models.Currency) (*models.Purchase, error) {
	price, err := parseFloatIfExists(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}

	p := &models.Purchase{Price: price, Currency: *dflt}
	if cur := slicesutils.Find(curs, func(c models.Currency) bool { return c.ID == r.FormValue("currency_id") }); cur != nil {
		p.Currency = *cur
	}

	if r.FormValue("mode") == views.PayInstallments {
		n, err := parseIntIfExists(r.FormValue("installments"))
		if err != nil {
			return nil, err
		}
		amt, err := parseFloatIfExists(r.FormValue("installment_amount"))
		if err != nil {
			return nil, err
		}
		if n < 0 || amt < 0 {
			return nil, errInvalidPurchase
		}
		switch {
		case n == 0 && amt == 0:
			return p, errNoInstallments
		case n == 0 && price > 0:
			// Enough months to pay the price, split evenly over them.
			n = int(math.Ceil(numsutils.Round(price / amt)))
			amt = numsutils.Round(price / float64(n))
		case amt == 0:
			amt = numsutils.Round(price / float64(n))
		}
		p.Installments, p.InstallmentAmount = n, amt
	}

	return p, nil
}
