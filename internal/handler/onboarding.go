package handler

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

// welcomeAccKinds are the kinds of accounts the onboarding adds, the others
// (debts, gam'yaas) need more than a balance.
var welcomeAccKinds = []models.AccKind{models.AccKindBankAcc, models.AccKindCreditCard, models.AccKindCash}

// Welcome is the onboarding, shown on signing in until it's finished or
// skipped. ?step= is the step, the region one by default.
func (h *Handler) Welcome(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d, err := h.welcomeData(r, models.ParseOnboardingStep(r.URL.Query().Get("step")))
	if err != nil {
		return nil, err
	}
	if r.Header.Get("HX-Target") == views.WelcomeTarget {
		return views.WelcomeBody(d), nil
	}
	balances, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	return views.Welcome(getClaims(r.Context()), balances, d), nil
}

// welcomeStep answers a step's form with the step to show (the next one, or
// the same with its errors).
func welcomeStep(w http.ResponseWriter, d *views.WelcomeData) templ.Component {
	w.Header().Set("HX-Retarget", "#"+views.WelcomeTarget)
	w.Header().Set("HX-Reswap", "innerHTML show:window:top")
	w.Header().Set("HX-Push-Url", "/welcome?step="+string(d.Step))
	return views.WelcomeBody(d)
}

func (h *Handler) welcomeData(r *http.Request, step models.OnboardingStep) (*views.WelcomeData, error) {
	ctx := r.Context()
	d := &views.WelcomeData{Step: step}
	switch step {
	case models.OnboardingRegion:
		return d, h.welcomeRegionData(r, d)
	case models.OnboardingAccounts:
		return d, h.welcomeAccountsData(ctx, d)
	case models.OnboardingSecurity:
		tenantID, err := repository.TenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		d.Encrypted = h.vaults.IsEncrypted(tenantID)
	case models.OnboardingBadge:
		// Nothing to offer without the VAPID keys.
		if !h.badges.Enabled() {
			d.Step = models.OnboardingDone
		}
	}
	return d, nil
}

// welcomeRegionData fills the region step with the user's currency and time
// zone. While they're the defaults of a new user, it's rendered again with
// the browser's time zone and languages (?tz=&langs=) to suggest them.
func (h *Handler) welcomeRegionData(r *http.Request, d *views.WelcomeData) error {
	ctx := r.Context()
	curs, err := h.repo.GetAllCurrencies(ctx)
	if err != nil {
		return err
	}
	d.Currencies = slicesutils.Filter(curs, func(c models.Currency) bool { return c.Kind == models.CurrencyKindFiat })

	cur, err := h.repo.GetDefaultCurrency(ctx)
	if err != nil {
		return err
	}
	loc, err := h.repo.GetTimezone(ctx)
	if err != nil {
		return err
	}
	d.CurrencyID, d.Timezone = cur.ID, loc.String()

	q := r.URL.Query()
	if !q.Has("tz") {
		d.Suggest = cur.String == models.GetDefaultCurrency().String && d.Timezone == models.DefaultTimezone
		return nil
	}
	tz := q.Get("tz")
	if _, err := models.LoadTimezone(tz); err == nil {
		d.Timezone = tz
	}
	if code := models.SuggestCurrency(tz, strings.Split(q.Get("langs"), ",")); code != "" {
		if c, err := h.repo.GetCurrencyByString(ctx, code); err == nil {
			d.CurrencyID = c.ID
		}
	}
	return nil
}

func (h *Handler) welcomeAccountsData(ctx context.Context, d *views.WelcomeData) error {
	accs, err := h.activeAccounts(ctx)
	if err != nil {
		return err
	}
	d.Accounts = slicesutils.Filter(accs, func(a models.Account) bool { return slices.Contains(welcomeAccKinds, a.Kind.Enum) })

	kinds, err := h.repo.GetAccountKinds(ctx)
	if err != nil {
		return err
	}
	d.Kinds = slicesutils.Filter(kinds, func(k models.AccountKind) bool { return slices.Contains(welcomeAccKinds, k.Enum) })

	if d.ShownCurrencies, err = h.repo.GetCurrencies(ctx); err != nil {
		return err
	}
	if d.DefaultCurrency, err = h.repo.GetDefaultCurrency(ctx); err != nil {
		return err
	}

	if d.NewAccount.KindID.Value == "" {
		if k := slicesutils.Find(d.Kinds, func(k models.AccountKind) bool { return k.Enum == models.DfltAccKind }); k != nil {
			d.NewAccount.KindID.Value = k.ID
		}
	}
	if d.NewAccount.CurrencyID.Value == "" {
		d.NewAccount.CurrencyID.Value = d.DefaultCurrency.ID
	}
	return nil
}

// WelcomeRegion saves the region step: the default currency and time zone.
func (h *Handler) WelcomeRegion(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	l := logger.FromCtx(ctx)

	d := &views.WelcomeData{Step: models.OnboardingRegion}
	cur, curErr := h.repo.GetCurrency(ctx, r.FormValue("currency_id"))
	loc, tzErr := models.LoadTimezone(r.FormValue("timezone"))
	if curErr == nil && cur.Kind != models.CurrencyKindFiat {
		curErr = errors.New("not a fiat currency")
	}
	if curErr != nil || tzErr != nil {
		if err := h.welcomeRegionData(r, d); err != nil {
			return nil, err
		}
		d.Suggest = false
		d.CurrencyID, d.Timezone = r.FormValue("currency_id"), r.FormValue("timezone")
		if curErr != nil {
			d.RegionError = "Pick your main currency."
		} else {
			d.RegionError = "Unknown time zone, use a name like Africa/Cairo."
		}
		return welcomeStep(w, d), nil
	}

	if err := h.repo.SetRegion(ctx, cur.ID, loc); err != nil {
		return nil, err
	}

	// The claims have the default currency.
	usr, err := h.repo.GetUserByOAuthID(ctx, getClaims(ctx).OAuthID)
	if err != nil {
		return nil, err
	}
	if _, err := setJWTCookies(w, usr); err != nil {
		l.Error("refreshing the cookies after setting the currency", zap.Error(err))
		return nil, err
	}

	d.Step = d.Step.Next()
	if err := h.welcomeAccountsData(ctx, d); err != nil {
		return nil, err
	}
	return welcomeStep(w, d), nil
}

// WelcomeAddAccount adds an account on the accounts step, saving the
// balances typed so far too.
func (h *Handler) WelcomeAddAccount(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	l := logger.FromCtx(ctx)

	d := &views.WelcomeData{Step: models.OnboardingAccounts}
	if err := h.welcomeAccountsData(ctx, d); err != nil {
		return nil, err
	}
	if msg, err := h.saveWelcomeBalances(ctx, r, d.Accounts); err != nil {
		return nil, err
	} else if msg != "" {
		d.AccountsError = msg
		return welcomeStep(w, d), nil
	}
	// With the balances just saved.
	d = &views.WelcomeData{Step: models.OnboardingAccounts}
	if err := h.welcomeAccountsData(ctx, d); err != nil {
		return nil, err
	}

	fd := views.WelcomeAccountFD{
		KindID:     form.FDInput{Value: r.FormValue("new_kind_id")},
		Name:       form.FDInput{Value: strings.TrimSpace(r.FormValue("new_name"))},
		CurrencyID: form.FDInput{Value: r.FormValue("new_currency_id")},
		Amount:     form.FDInput{Value: r.FormValue("new_amount")},
	}
	kind := slicesutils.Find(d.Kinds, func(k models.AccountKind) bool { return k.ID == fd.KindID.Value })
	cur := slicesutils.Find(d.ShownCurrencies, func(c models.Currency) bool { return c.ID == fd.CurrencyID.Value })
	amount, amountErr := parseFloatIfExists(fd.Amount.Value)
	if kind == nil {
		fd.KindID.Error = "Pick a kind."
	}
	if cur == nil {
		fd.CurrencyID.Error = "Pick a currency."
	}
	if amountErr != nil {
		fd.Amount.Error = "Invalid balance."
	}
	if fd.Name.Value == "" {
		fd.Name.Error = "Name the account."
	}

	if fd.KindID.Error == "" && fd.CurrencyID.Error == "" && fd.Amount.Error == "" && fd.Name.Error == "" {
		a := &models.Account{ID: idgen.New(), Name: fd.Name.Value, Kind: *kind, Currency: *cur, Amount: amount}
		if kind.Enum == models.AccKindCreditCard {
			a.Amount = -amount
		}
		err := h.validateAccount(a)
		if err == nil {
			err = h.repo.CreateAccount(ctx, a)
		}
		var valerr errs.ValidationError
		var dupErr *errs.ValidationError
		switch {
		case errors.As(err, &dupErr):
			fd.Name.Error = "You already have an account with this name."
		case errors.As(err, &valerr):
			fd.Name.Error = valerr.Errors.MsgFor("Name")
			if fd.Name.Error == "" {
				fd.Name.Error = "Invalid account."
			}
		case err != nil:
			return nil, err
		default:
			l.Info("added an account on the onboarding", zap.String("account_id", a.ID))
		}
	}
	if fd.Name.Error != "" || fd.KindID.Error != "" || fd.CurrencyID.Error != "" || fd.Amount.Error != "" {
		d.NewAccount = fd
		return welcomeStep(w, d), nil
	}

	d = &views.WelcomeData{Step: models.OnboardingAccounts}
	if err := h.welcomeAccountsData(ctx, d); err != nil {
		return nil, err
	}
	return welcomeStep(w, d), nil
}

// WelcomeBalances saves the accounts step's balances, moving on.
func (h *Handler) WelcomeBalances(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	d := &views.WelcomeData{Step: models.OnboardingAccounts}
	if err := h.welcomeAccountsData(ctx, d); err != nil {
		return nil, err
	}
	if msg, err := h.saveWelcomeBalances(ctx, r, d.Accounts); err != nil {
		return nil, err
	} else if msg != "" {
		d.AccountsError = msg
		return welcomeStep(w, d), nil
	}

	d, err := h.welcomeData(r, models.OnboardingAccounts.Next())
	if err != nil {
		return nil, err
	}
	return welcomeStep(w, d), nil
}

// saveWelcomeBalances writes the balances of the accounts step as is (no
// adjustment transactions: they're where the accounts start), what's owed for
// credit cards. msg is the error to show for an invalid one.
func (h *Handler) saveWelcomeBalances(ctx context.Context, r *http.Request, accs []models.Account) (msg string, err error) {
	var fixes []models.BalanceFix
	for _, a := range accs {
		v := strings.TrimSpace(r.FormValue("amount-" + a.ID))
		if v == "" {
			continue
		}
		amount, err := parseFloatIfExists(v)
		if err != nil {
			return "Invalid balance for " + a.Name + ".", nil
		}
		if a.Kind.Enum == models.AccKindCreditCard {
			amount = -amount
		}
		fixes = append(fixes, models.BalanceFix{AccountID: a.ID, Amount: amount})
	}
	if len(fixes) == 0 {
		return "", nil
	}
	changed, err := h.repo.FixBalances(ctx, fixes, false, time.Now())
	if err != nil {
		return "", err
	}
	logger.FromCtx(ctx).Info("set the starting balances", zap.Int("changed", changed))
	return "", nil
}

// FinishWelcome ends the onboarding (finished or skipped), sending to ?next=
// or the dashboard.
func (h *Handler) FinishWelcome(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	if err := h.repo.SetOnboarded(r.Context()); err != nil {
		return nil, err
	}
	next := "/"
	if n := r.FormValue("next"); n != "" {
		next = safeNext(n)
	}
	w.Header().Set("HX-Redirect", next)
	return nil, nil
}
