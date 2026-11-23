package handler

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/countries"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/visa"
	"github.com/omareloui/tyche/internal/visaguess"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

const visaGuessTimeout = 90 * time.Second

// visaMaxDependents caps the dependents asked.
const visaMaxDependents = 10

// SetVisaGuess turns the AI's visa requirements lookups on (nil keeps them
// off).
func (h *Handler) SetVisaGuess(g visaguess.Guesser) {
	h.visaGuess = g
}

// VisaCalc is the visa readiness tool, its form filled from the query
// string, else the time zone's country and the user's own money.
func (h Handler) VisaCalc(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	b, err := h.repo.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.visaData(ctx, r.URL.Query())
	if err != nil {
		return nil, err
	}
	return views.VisaCalc(getClaims(ctx), b, d), nil
}

// VisaCalcBody is the form and result, when the country or the visa change
// what the form asks.
func (h Handler) VisaCalcBody(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d, err := h.visaData(r.Context(), r.URL.Query())
	if err != nil {
		return nil, err
	}
	w.Header().Set("HX-Replace-Url", views.VisaURL(d.Form))
	return views.VisaBody(d), nil
}

// VisaCalcResult is the result, on every other change of the form.
func (h Handler) VisaCalcResult(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d, err := h.visaData(r.Context(), r.URL.Query())
	if err != nil {
		return nil, err
	}
	w.Header().Set("HX-Replace-Url", views.VisaURL(d.Form))
	return views.VisaResult(d), nil
}

// parseVisaPick reads a visa's pick of prefix p (empty for the page's).
func parseVisaPick(v url.Values, p string) views.VisaPickForm {
	f := views.VisaPickForm{
		Country: strings.ToUpper(strings.TrimSpace(v.Get(p + "country"))),
		Visa:    strings.TrimSpace(v.Get(p + "visa")),
		Name:    strings.TrimSpace(v.Get(p + "name")),
		Offer:   strings.TrimSpace(v.Get(p + "offer")),
		Opts:    map[string]string{},
	}
	if len(f.Name) > 80 {
		f.Name = f.Name[:80]
	}
	for k, vs := range v {
		if key, ok := strings.CutPrefix(k, p+"opt_"); ok && len(vs) > 0 {
			f.Opts[key] = vs[0]
		}
	}
	return f
}

// parseVisaMoneyForm reads the user's numbers shared by every visa.
func parseVisaMoneyForm(v url.Values) views.VisaMoneyForm {
	f := views.VisaMoneyForm{
		Dependents: strings.TrimSpace(v.Get("dependents")),
		Income:     strings.TrimSpace(v.Get("income")),
		Savings:    strings.TrimSpace(v.Get("savings")),
	}
	if n, err := strconv.Atoi(f.Dependents); err != nil || n <= 0 {
		f.Dependents = ""
	} else if n > visaMaxDependents {
		f.Dependents = strconv.Itoa(visaMaxDependents)
	}
	return f
}

func parseVisaForm(v url.Values) views.VisaForm {
	f := views.VisaForm{VisaPickForm: parseVisaPick(v, ""), Money: parseVisaMoneyForm(v), Checked: map[int]bool{}}
	for k := range v {
		if s, ok := strings.CutPrefix(k, "check_"); ok {
			if i, err := strconv.Atoi(s); err == nil && i >= 0 && i < visa.MaxRequirements && v.Get(k) != "" {
				f.Checked[i] = true
			}
		}
	}
	return f
}

// typedAmount reads an amount the user typed, ok false when nothing was.
func typedAmount(s string) (float64, bool) {
	if strings.TrimSpace(s) == "" {
		return 0, false
	}
	v, _ := parseColAmount(s)
	return v, true
}

// visaCountry is the picked country, else the time zone's.
func visaCountry(ctx context.Context, iso2 string) countries.Country {
	if c, ok := countries.ByISO2(iso2); ok {
		return c
	}
	tz := ""
	if claims := getClaims(ctx); claims != nil {
		tz = claims.Timezone
	}
	if c, ok := countries.FromTimezone(tz); ok {
		return c
	}
	c, _ := countries.FromTimezone(models.DefaultTimezone)
	return c
}

// resolveVisa picks a visa's rule: one of the country's built in visas
// (visa = its key), the AI's kept requirements (visa = ai, of the typed
// name), or one of the user's own (visa = its id, which picks its country).
func (h Handler) resolveVisa(ctx context.Context, f *views.VisaPickForm, mine []models.VisaRule) (*views.VisaPick, error) {
	p := &views.VisaPick{AIOn: h.visaGuess != nil}
	for _, m := range mine {
		if m.ID == f.Visa && m.Country != f.Country {
			f.Country = m.Country
		}
	}
	p.Country = visaCountry(ctx, f.Country)
	f.Country = p.Country.ISO2
	p.Visas = visa.BuiltinVisas(p.Country.ISO2)
	for _, m := range mine {
		if m.Country == p.Country.ISO2 {
			p.MyRules = append(p.MyRules, m)
		}
	}

	own := func(id string) *models.VisaRule {
		for i := range p.MyRules {
			if p.MyRules[i].ID == id {
				return &p.MyRules[i]
			}
		}
		return nil
	}
	if f.Visa == "" || (f.Visa != views.VisaPickAI && !visa.IsBuiltin(p.Country.ISO2, f.Visa) && own(f.Visa) == nil) {
		switch {
		case len(p.Visas) > 0:
			f.Visa = p.Visas[0].Key
		case len(p.MyRules) > 0:
			f.Visa = p.MyRules[0].ID
		default:
			f.Visa = views.VisaPickAI
		}
	}

	switch {
	case f.Visa == views.VisaPickAI:
		if f.Name == "" {
			p.NeedName = true
			break
		}
		rule, err := h.repo.GetVisaRulesCache(ctx, visaguess.Key(p.Country.ISO2, f.Name, visaguess.Year(time.Now())))
		if err != nil {
			return nil, err
		}
		p.Rule, p.NeedAI = rule, rule == nil
		if tenantID, err := repository.TenantIDFromCtx(ctx); err == nil {
			p.AILeft, p.AILimit, _ = h.repo.AICallsLeft(ctx, tenantID, models.ScanVisaRules, h.ocrCfg.VisaRulesPerMonth, time.Now())
		}
	case visa.IsBuiltin(p.Country.ISO2, f.Visa):
		p.Options = visa.BuiltinOptions(p.Country.ISO2, f.Visa)
		f.Opts = visa.PickOptions(p.Options, f.Opts)
		p.Rule, _ = visa.Builtin(p.Country.ISO2, f.Visa, f.Opts)
	default:
		m := own(f.Visa)
		rule := m.Rule
		p.Rule, p.RuleName = &rule, m.Name
	}
	if f.Visa != views.VisaPickAI {
		f.Name = ""
	}
	if p.Options == nil {
		f.Opts = map[string]string{}
	}
	return p, nil
}

// visaRate is what one of the default currency is in cur, false without a
// rate.
func visaRate(def, cur string) (float64, bool) {
	if def == cur {
		return 1, true
	}
	rate, err := fxrate.GetFxRate(def, cur)
	if err != nil || rate <= 0 {
		return 0, false
	}
	return rate, true
}

// visaInput is the user's money in the rule's currency: the income and
// savings typed (in the default currency), else their transactions' (else
// the salary on settings), the offer typed (in the rule's).
func visaInput(claims *jwt.AccessClaims, money *models.VisaMoney, mf views.VisaMoneyForm, offer string, rule *visa.Rule, rate float64) (visa.Input, string) {
	in := visa.Input{}
	in.Dependents, _ = strconv.Atoi(mf.Dependents)
	from := views.VisaFromTxs
	if v, ok := typedAmount(mf.Income); ok {
		in.Income, in.IncomeTyped, from = v*rate, true, views.VisaFromTyped
	} else if money.HasIncome() {
		in.Income = money.Average() * rate
	} else if claims != nil && claims.Salary.Amount > 0 {
		if r, ok := visaRate(claims.Salary.Currency, rule.Currency); ok {
			in.Income, in.IncomeTyped, from = claims.Salary.Amount*r, true, views.VisaFromSalary
		}
	} else {
		from = ""
	}
	if !in.IncomeTyped {
		in.MonthlyIncomes = make([]float64, len(money.Months))
		for i, m := range money.Months {
			in.MonthlyIncomes[i] = m * rate
		}
	}
	if v, ok := typedAmount(mf.Savings); ok {
		in.Savings = v * rate
	} else {
		in.Savings = money.Savings * rate
	}
	in.Offer, _ = typedAmount(offer)
	return in, from
}

// visaPickResult checks a picked visa's rule against the user's money.
func (h Handler) visaPickResult(ctx context.Context, p *views.VisaPick, money *models.VisaMoney, mf views.VisaMoneyForm, offer string, checked map[int]bool) {
	if p.Rule == nil {
		return
	}
	claims := getClaims(ctx)
	def := claims.DefaultCurrency.String
	rate, ok := visaRate(def, p.Rule.Currency)
	if !ok && visaAsksMoney(p.Rule) {
		p.Error = "There's no exchange rate from " + def + " to " + p.Rule.Currency + " to compare your money with."
		return
	}
	p.Rate = rate
	p.Input, p.IncomeFrom = visaInput(claims, money, mf, offer, p.Rule, rate)
	p.Input.Checked = checked
	res := visa.Check(p.Rule, p.Input)
	p.Result = &res
	if curs, err := h.repo.GetCurrencies(ctx); err == nil {
		for _, c := range curs {
			if c.String == p.Rule.Currency {
				p.CurrencyID = c.ID
			}
		}
	}
}

func (h Handler) visaData(ctx context.Context, v url.Values) (*views.VisaData, error) {
	claims := getClaims(ctx)
	d := &views.VisaData{Form: parseVisaForm(v), Countries: countries.All(), Default: claims.DefaultCurrency}
	mine, err := h.repo.GetVisaRules(ctx)
	if err != nil {
		return nil, err
	}
	d.Pick, err = h.resolveVisa(ctx, &d.Form.VisaPickForm, mine)
	if err != nil {
		return nil, err
	}
	d.Money, err = h.repo.GetVisaMoney(ctx, &claims.DefaultCurrency, time.Now())
	if err != nil {
		return nil, err
	}
	h.visaPickResult(ctx, d.Pick, d.Money, d.Form.Money, d.Form.Offer, d.Form.Checked)
	return d, nil
}

// errVisa is a lookup's problem to tell the user.
type errVisa struct{ msg string }

func (e errVisa) Error() string { return e.msg }

// FetchVisaRules has the AI look up the form's visa (country + typed name),
// within the user's monthly lookups: kept ones are reused, unless refresh
// asks for them again (counted). Answers the result, or the comparison
// when view=compare (fetch = the visa's index).
func (h Handler) FetchVisaRules(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	if h.visaGuess == nil {
		return nil, resperr.New(http.StatusNotFound, "Visa lookups are off")
	}
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "Invalid form")
	}
	compare := r.Form.Get("view") == "compare"
	prefix := ""
	if compare {
		prefix = "v" + r.Form.Get("fetch") + "_"
	}
	pick := parseVisaPick(r.Form, prefix)
	c, ok := countries.ByISO2(pick.Country)
	if !ok {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Pick a country")
	}
	if pick.Name == "" {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Type the visa's name")
	}
	in := visaguess.In{Country: c.ISO2, Name: c.Name, Currency: c.Currency, Visa: pick.Name, Year: visaguess.Year(time.Now())}

	var msg string
	cached, err := h.repo.GetVisaRulesCache(ctx, in.Key())
	if err != nil {
		return nil, err
	}
	if cached == nil || r.Form.Get("refresh") != "" {
		_, err := h.lookUpVisaRules(ctx, in)
		var e errVisa
		if errors.As(err, &e) {
			msg = e.msg
		} else if err != nil {
			return nil, err
		}
	}

	if compare {
		d, err := h.visaCompareData(ctx, r.Form)
		if err != nil {
			return nil, err
		}
		d.Error = msg
		w.Header().Set("HX-Replace-Url", views.VisaCompareURL(d))
		return views.VisaCompareBody(d), nil
	}
	d, err := h.visaData(ctx, r.Form)
	if err != nil {
		return nil, err
	}
	if msg != "" {
		d.Pick.Error = msg
	}
	w.Header().Set("HX-Replace-Url", views.VisaURL(d.Form))
	return views.VisaResult(d), nil
}

// lookUpVisaRules asks the AI, within the tenant's monthly lookups, keeping
// the answer for everyone under the typed name and the visa's own.
func (h Handler) lookUpVisaRules(ctx context.Context, in visaguess.In) (*visa.Rule, error) {
	l := logger.FromCtx(ctx)
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	left, limit, err := h.repo.AICallsLeft(ctx, tenantID, models.ScanVisaRules, h.ocrCfg.VisaRulesPerMonth, time.Now())
	if err != nil {
		return nil, err
	}
	if left <= 0 {
		return nil, errVisa{"You've had visas looked up " + strconv.Itoa(limit) + " times this month, the most you can. It starts again on the 1st."}
	}

	callCtx, cancel := context.WithTimeout(ctx, visaGuessTimeout)
	defer cancel()
	rule, usage, callErr := h.visaGuess.Guess(callCtx, in)
	scan := models.ReceiptScan{
		TenantID:      tenantID,
		Kind:          models.ScanVisaRules,
		Model:         h.visaGuess.Model(),
		InputTokens:   usage.InputTokens,
		OutputTokens:  usage.OutputTokens,
		SearchQueries: usage.SearchQueries,
		CostMicros:    usage.CostMicros(h.ocrCfg.Prices()),
		OK:            callErr == nil,
	}
	if callErr != nil {
		scan.Error = callErr.Error()
	}
	if err := h.repo.RecordReceiptScan(ctx, scan); err != nil {
		l.Error("recording a visa lookup", zap.Error(err))
	}
	if errors.Is(callErr, visaguess.ErrNoRules) {
		l.Info("no visa requirements", zap.Error(callErr))
		return nil, errVisa{"Couldn't find this visa's requirements. Check its name, or type them yourself."}
	}
	if callErr != nil {
		l.Error("looking up a visa", zap.Error(callErr))
		return nil, errVisa{"Couldn't look the visa up right now, try again in a bit."}
	}
	keys := []string{in.Key()}
	if k := visaguess.Key(in.Country, rule.Name, in.Year); k != in.Key() {
		keys = append(keys, k)
	}
	if err := h.repo.SetVisaRulesCache(ctx, keys, h.visaGuess.Model(), rule); err != nil {
		l.Error("keeping visa requirements", zap.Error(err))
	}
	l.Info("looked up a visa", zap.Int("requirements", len(rule.Requirements)))
	return rule, nil
}

// visaRound rounds an amount shown in a form.
func visaRound(v float64) string {
	return strconv.FormatFloat(math.Round(v), 'f', -1, 64)
}

// visaYear is the year requirements are of now.
func visaYear() int { return visaguess.Year(time.Now()) }

// visaAsksMoney tells the rule has a money requirement (one of conditions
// only needs no exchange rate).
func visaAsksMoney(r *visa.Rule) bool {
	for _, q := range r.Requirements {
		if q.Kind.Money() {
			return true
		}
	}
	return false
}
