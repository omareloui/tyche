package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

// maxNameLen is the longest first or last name.
const maxNameLen = 50

// Settings is the page of the user's settings.
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d, err := h.settingsData(r.Context())
	if err != nil {
		return nil, err
	}
	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	return views.Settings(getClaims(r.Context()), b, d), nil
}

// settingsData fills every card with the saved settings.
func (h *Handler) settingsData(ctx context.Context) (*views.SettingsData, error) {
	claims := getClaims(ctx)
	usr, err := h.repo.GetUserByOAuthID(ctx, claims.OAuthID)
	if err != nil {
		return nil, err
	}
	loc, err := h.repo.GetTimezone(ctx)
	if err != nil {
		return nil, err
	}
	ws, err := h.repo.GetWorkSettings(ctx)
	if err != nil {
		return nil, err
	}
	all, err := h.repo.GetAllCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	shown, err := h.repo.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	accs, err := h.defaultAccountOptions(ctx)
	if err != nil {
		return nil, err
	}

	d := &views.SettingsData{
		Profile: views.SettingsProfileFD{
			FirstName: form.FDInput{Value: usr.FirstName},
			LastName:  form.FDInput{Value: usr.LastName},
		},
		Region: views.SettingsRegionFD{
			CurrencyID: form.FDInput{Value: usr.DefaultCurrency.ID},
			Timezone:   form.FDInput{Value: loc.String()},
		},
		Month:           views.SettingsMonthFD{Day: form.FDInput{Value: fmt.Sprint(models.MonthStart(usr.StartOfPeriod).Day())}},
		Work:            views.NewWorkSettingsFD(ws),
		WorkSettings:    ws,
		Currencies:      slicesutils.Filter(all, func(c models.Currency) bool { return c.Kind == models.CurrencyKindFiat }),
		ShownCurrencies: shown,
		DefaultCurrency: &usr.DefaultCurrency,
		Accounts:        accs,
	}
	if usr.DefaultAccount != nil {
		d.Account.AccountID.Value = usr.DefaultAccount.ID
	}
	return d, nil
}

// defaultAccountOptions are the accounts that can be the default one: the
// active ones, but gam'yaas (only transfers use them).
func (h *Handler) defaultAccountOptions(ctx context.Context) ([]models.Account, error) {
	accs, err := h.activeAccounts(ctx)
	if err != nil {
		return nil, err
	}
	return slicesutils.Filter(accs, func(a models.Account) bool { return a.Kind.Enum != models.AccKindGamyaa }), nil
}

// refreshClaims reissues the cookies, as the claims have some settings.
func (h *Handler) refreshClaims(w http.ResponseWriter, ctx context.Context) error {
	usr, err := h.repo.GetUserByOAuthID(ctx, getClaims(ctx).OAuthID)
	if err != nil {
		return err
	}
	if _, err := setJWTCookies(w, usr); err != nil {
		logger.FromCtx(ctx).Error("refreshing the cookies after saving settings", zap.Error(err))
		return err
	}
	return nil
}

// settingsSaved answers card saved, with the claims (and the page's data)
// refreshed.
func (h *Handler) settingsSaved(w http.ResponseWriter, r *http.Request, card string, render func(*views.SettingsData) templ.Component) (templ.Component, error) {
	if err := h.refreshClaims(w, r.Context()); err != nil {
		return nil, err
	}
	d, err := h.settingsData(r.Context())
	if err != nil {
		return nil, err
	}
	d.Saved = card
	return render(d), nil
}

// SetProfile saves the user's name.
func (h *Handler) SetProfile(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	fd := views.SettingsProfileFD{
		FirstName: form.FDInput{Value: strings.TrimSpace(r.FormValue("first_name"))},
		LastName:  form.FDInput{Value: strings.TrimSpace(r.FormValue("last_name"))},
	}
	for _, in := range []*form.FDInput{&fd.FirstName, &fd.LastName} {
		switch n := utf8.RuneCountInString(in.Value); {
		case n == 0:
			in.Error = "Enter a name"
		case n > maxNameLen:
			in.Error = fmt.Sprintf("At most %d characters", maxNameLen)
		}
	}
	if fd.FirstName.Error != "" || fd.LastName.Error != "" {
		d, err := h.settingsData(r.Context())
		if err != nil {
			return nil, err
		}
		d.Profile = fd
		return views.SettingsProfileCard(d), nil
	}

	if err := h.repo.SetUserName(r.Context(), fd.FirstName.Value, fd.LastName.Value); err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("set the user's name")
	return h.settingsSaved(w, r, views.SettingsProfile, views.SettingsProfileCard)
}

// SetRegionSettings saves the main currency and the time zone. Changing the
// currency reloads the page, as every amount is shown in it.
func (h *Handler) SetRegionSettings(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()

	fd := views.SettingsRegionFD{
		CurrencyID: form.FDInput{Value: r.FormValue("currency_id")},
		Timezone:   form.FDInput{Value: strings.TrimSpace(r.FormValue("timezone"))},
	}
	cur, curErr := h.repo.GetCurrency(ctx, fd.CurrencyID.Value)
	if curErr == nil && cur.Kind != models.CurrencyKindFiat {
		curErr = errors.New("not a fiat currency")
	}
	if curErr != nil {
		fd.CurrencyID.Error = "Pick your main currency"
	}
	loc, err := models.LoadTimezone(fd.Timezone.Value)
	if err != nil {
		fd.Timezone.Error = "Unknown time zone, use a name like Africa/Cairo"
	}
	if fd.CurrencyID.Error != "" || fd.Timezone.Error != "" {
		d, err := h.settingsData(ctx)
		if err != nil {
			return nil, err
		}
		d.Region = fd
		return views.SettingsRegionCard(d), nil
	}

	changed := cur.ID != getClaims(ctx).DefaultCurrency.ID
	if err := h.repo.SetCurrencyAndTimezone(ctx, cur.ID, loc); err != nil {
		return nil, err
	}
	if changed {
		if err := h.refreshClaims(w, ctx); err != nil {
			return nil, err
		}
		hxRespondWithRefresh(w)
		return nil, nil
	}
	return h.settingsSaved(w, r, views.SettingsRegion, views.SettingsRegionCard)
}

// SetMonthStart saves the day the user's months start on.
func (h *Handler) SetMonthStart(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	fd := views.SettingsMonthFD{Day: form.FDInput{Value: r.FormValue("month_start")}}
	day, err := strconv.Atoi(fd.Day.Value)
	if err != nil || day < 1 || day > models.MaxMonthStart {
		fd.Day.Error = fmt.Sprintf("Pick a day from 1 to %d", models.MaxMonthStart)
		d, err := h.settingsData(r.Context())
		if err != nil {
			return nil, err
		}
		d.Month = fd
		return views.SettingsMonthCard(d), nil
	}

	if err := h.repo.SetMonthStart(r.Context(), models.MonthStart(day)); err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("set the month start", zap.Int("day", day))
	return h.settingsSaved(w, r, views.SettingsMonth, views.SettingsMonthCard)
}

// SetWorkSettings saves the salary and work hours prices are counted in.
func (h *Handler) SetWorkSettings(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()

	curs, err := h.repo.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	fd, ws := parseWorkSettings(r, curs)
	if ws == nil {
		d, err := h.settingsData(ctx)
		if err != nil {
			return nil, err
		}
		d.Work = fd
		return views.SettingsWorkCard(d), nil
	}

	if err := h.repo.SetWorkSettings(ctx, ws); err != nil {
		logger.FromCtx(ctx).Error("saving work settings", zap.Error(err))
		return nil, err
	}
	logger.FromCtx(ctx).Info("saved work settings")

	d, err := h.settingsData(ctx)
	if err != nil {
		return nil, err
	}
	d.Saved = views.SettingsWork
	return views.SettingsWorkCard(d), nil
}

// parseWorkSettings reads the salary form, the settings nil when invalid.
func parseWorkSettings(r *http.Request, curs []models.Currency) (*views.WorkSettingsFD, *models.WorkSettings) {
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
	if fd.Salary.Amount.Error != "" || fd.HoursPerWeek.Error != "" || cur == nil {
		return fd, nil
	}
	return fd, &models.WorkSettings{MonthlySalary: salary, Currency: *cur, HoursPerWeek: hours}
}

// SetDefaultAccountSettings saves the account new transactions start in.
func (h *Handler) SetDefaultAccountSettings(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()

	fd := views.SettingsAccountFD{AccountID: form.FDInput{Value: r.FormValue("account_id")}}
	accs, err := h.defaultAccountOptions(ctx)
	if err != nil {
		return nil, err
	}
	if slicesutils.Find(accs, func(a models.Account) bool { return a.ID == fd.AccountID.Value }) == nil {
		fd.AccountID.Error = "Pick an account"
		d, err := h.settingsData(ctx)
		if err != nil {
			return nil, err
		}
		d.Account = fd
		return views.SettingsAccountCard(d), nil
	}

	if err := h.repo.SetDefaultAccount(ctx, fd.AccountID.Value); err != nil {
		return nil, err
	}
	logger.FromCtx(ctx).Info("set the default account", zap.String("id", fd.AccountID.Value))
	return h.settingsSaved(w, r, views.SettingsAccount, views.SettingsAccountCard)
}
