package handler

import (
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/countries"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/payroll"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// salaryRulesChanged re-renders the calculator, picking the rules id (none
// when deleted) once the drawer closes.
func salaryRulesChanged(w http.ResponseWriter, id, country string) {
	b, _ := json.Marshal(map[string]any{
		drawerSavedEvent:       true,
		"salary-rules-changed": map[string]string{"id": id, "country": country},
	})
	w.Header().Set("HX-Trigger", string(b))
}

// SalaryRules lists the user's own tax rules in a drawer.
func (h Handler) SalaryRules(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	rules, err := h.repo.GetSalaryRules(r.Context())
	if err != nil {
		return nil, err
	}
	return views.SalaryRulesDrawer(rules), nil
}

// NewSalaryRule is the form of new rules, a copy of the ones the query
// string picks (as the calculator's form), else empty for its country.
func (h Handler) NewSalaryRule(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	v := r.URL.Query()
	f := parseSalaryForm(v)
	c, ok := countries.ByISO2(f.Country)
	if !ok {
		c, _ = countries.FromTimezone(getClaims(ctx).Timezone)
	}
	rules := &payroll.Rules{Currency: c.Currency, Brackets: []payroll.Bracket{{UpTo: 0, Rate: 0}}}
	name := c.Name + " (my rules)"
	if v.Get("from") != "" && c.ISO2 != "" {
		from, fromName, _, err := h.salaryRulesFor(ctx, c, f.Rules, f.Region, f.Opts)
		if err != nil {
			return nil, err
		}
		if from != nil {
			rules = from.Clone()
		}
		if fromName != "" {
			name = fromName + " (copy)"
		} else if f.Region != "" {
			name = f.Region + ", " + c.Name + " (my rules)"
		}
	}
	fd := salaryRuleFD(name, c.ISO2, rules)
	return views.SalaryRuleDrawer("", fd), nil
}

// EditSalaryRule is the form of one of the user's rules.
func (h Handler) EditSalaryRule(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	s, err := h.repo.GetSalaryRule(r.Context(), r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, resperr.New(http.StatusNotFound, "Rules not found")
	}
	return views.SalaryRuleDrawer(s.ID, salaryRuleFD(s.Name, s.Country, &s.Rules)), nil
}

// CreateSalaryRule saves new rules of the user's.
func (h Handler) CreateSalaryRule(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.saveSalaryRule(w, r, "")
}

// UpdateSalaryRule saves the user's changed rules.
func (h Handler) UpdateSalaryRule(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	s, err := h.repo.GetSalaryRule(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, resperr.New(http.StatusNotFound, "Rules not found")
	}
	return h.saveSalaryRule(w, r, id)
}

func (h Handler) saveSalaryRule(w http.ResponseWriter, r *http.Request, id string) (templ.Component, error) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "Invalid form")
	}
	fd, s := parseSalaryRuleForm(r.Form)
	if s == nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return views.SalaryRuleForm(id, fd), nil
	}
	s.ID = id
	if err := h.repo.SaveSalaryRule(ctx, s); err != nil {
		return nil, err
	}
	logger.FromCtx(ctx).Info("saved salary rules", zap.String("id", s.ID))
	salaryRulesChanged(w, s.ID, s.Country)
	return views.SalaryRuleForm(s.ID, fd), nil
}

// DeleteSalaryRule deletes rules of the user's.
func (h Handler) DeleteSalaryRule(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	id := r.PathValue("id")
	if err := h.repo.DeleteSalaryRule(ctx, id); err != nil {
		return nil, err
	}
	logger.FromCtx(ctx).Info("deleted salary rules", zap.String("id", id))
	salaryRulesChanged(w, "", "")
	return templ.NopComponent, nil
}

// salaryRuleExtra are the parts of rules the form keeps as they are.
type salaryRuleExtra struct {
	TaxName string           `json:"tax_name,omitempty"`
	Credits []payroll.Credit `json:"credits,omitempty"`
	Layers  []payroll.Layer  `json:"layers,omitempty"`
}

func fmtNum(v float64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func fmtPct(r float64) string {
	return strconv.FormatFloat(math.Round(r*1e6)/1e4, 'f', -1, 64)
}

// salaryRuleFD is the form of rules.
func salaryRuleFD(name, country string, r *payroll.Rules) *views.SalaryRuleFD {
	fd := &views.SalaryRuleFD{
		Countries:  countries.All(),
		Name:       name,
		Country:    country,
		Currency:   r.Currency,
		Pays:       strconv.Itoa(r.Pays()),
		Allowance:  fmtNum(r.Allowance.Amount),
		TaperFrom:  fmtNum(r.Allowance.TaperFrom),
		TaperRate:  fmtNum(math.Round(r.Allowance.TaperRate*1e6) / 1e4),
		AsCredit:   r.Allowance.AsCredit,
		Tables:     payroll.FormatTables(r.Tables),
		Steps:      payroll.FormatSteps(r.WorkReduction),
		Notes:      r.Notes,
		Year:       r.Year,
		SourceURLs: r.Sources,
	}
	if len(r.Credits) > 0 || len(r.Layers) > 0 {
		b, _ := json.Marshal(salaryRuleExtra{TaxName: r.TaxName, Credits: r.Credits, Layers: r.Layers})
		fd.Extra = string(b)
		for _, c := range r.Credits {
			fd.ExtraNames = append(fd.ExtraNames, c.Name+" (credit)")
		}
		for _, l := range r.Layers {
			fd.ExtraNames = append(fd.ExtraNames, l.Name+" tax")
		}
	}
	for _, b := range r.Brackets {
		fd.Brackets = append(fd.Brackets, views.SalaryBracketRow{UpTo: fmtNum(b.UpTo), Rate: fmtPct(b.Rate)})
	}
	for _, d := range r.Deductions {
		fd.Deductions = append(fd.Deductions, views.SalaryDeductionRow{Name: d.Name, Amount: fmtNum(d.Amount)})
	}
	for _, c := range r.Contributions {
		row := views.SalaryContribRow{
			Name: c.Name, Floor: fmtNum(c.Floor), Cap: fmtNum(c.Cap),
			Monthly: c.Monthly, Deductible: c.Deductible, Credit: c.Credit, Bands: payroll.FormatBands(c.Bands),
		}
		if len(c.Bands) == 0 {
			row.Rate = fmtPct(c.Rate)
		}
		fd.Contributions = append(fd.Contributions, row)
	}
	return fd
}

func formNum(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return 0, true
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

func at(vs []string, i int) string {
	if i < len(vs) {
		return vs[i]
	}
	return ""
}

// parseSalaryRuleForm reads the rules' form, the rule nil when invalid (the
// errors in the form data).
func parseSalaryRuleForm(v url.Values) (*views.SalaryRuleFD, *models.SalaryRule) {
	fd := &views.SalaryRuleFD{
		Countries: countries.All(),
		Name:      strings.TrimSpace(v.Get("name")),
		Country:   strings.ToUpper(strings.TrimSpace(v.Get("country"))),
		Currency:  strings.ToUpper(strings.TrimSpace(v.Get("currency"))),
		Pays:      strings.TrimSpace(v.Get("pays")),
		Allowance: strings.TrimSpace(v.Get("allowance")),
		TaperFrom: strings.TrimSpace(v.Get("taper_from")),
		TaperRate: strings.TrimSpace(v.Get("taper_rate")),
		AsCredit:  v.Get("as_credit") != "",
		Tables:    v.Get("tables"),
		Steps:     v.Get("steps"),
		Notes:     strings.TrimSpace(v.Get("notes")),
	}
	fd.Year, _ = strconv.Atoi(v.Get("year"))
	fd.SourceURLs = v["source_url"]
	ups, rates := v["bracket_up_to"], v["bracket_rate"]
	for i := range max(len(ups), len(rates)) {
		row := views.SalaryBracketRow{UpTo: strings.TrimSpace(at(ups, i)), Rate: strings.TrimSpace(at(rates, i))}
		if row.UpTo != "" || row.Rate != "" {
			fd.Brackets = append(fd.Brackets, row)
		}
	}
	names, amounts := v["deduction_name"], v["deduction_amount"]
	for i := range max(len(names), len(amounts)) {
		row := views.SalaryDeductionRow{Name: strings.TrimSpace(at(names, i)), Amount: strings.TrimSpace(at(amounts, i))}
		if row.Name != "" || row.Amount != "" {
			fd.Deductions = append(fd.Deductions, row)
		}
	}
	cn := v["contrib_name"]
	for i := range cn {
		row := views.SalaryContribRow{
			Name:       strings.TrimSpace(cn[i]),
			Rate:       strings.TrimSpace(at(v["contrib_rate"], i)),
			Floor:      strings.TrimSpace(at(v["contrib_floor"], i)),
			Cap:        strings.TrimSpace(at(v["contrib_cap"], i)),
			Monthly:    at(v["contrib_monthly"], i) != "",
			Deductible: at(v["contrib_deductible"], i) != "",
			Credit:     at(v["contrib_credit"], i) != "",
			Bands:      strings.TrimSpace(at(v["contrib_bands"], i)),
		}
		if row.Name != "" || row.Rate != "" || row.Bands != "" {
			fd.Contributions = append(fd.Contributions, row)
		}
	}

	fail := func(msg string) (*views.SalaryRuleFD, *models.SalaryRule) {
		fd.Error = msg
		return fd, nil
	}
	if fd.Name == "" {
		return fail("Give the rules a name.")
	}
	if len(fd.Name) > 80 {
		return fail("The name is too long.")
	}
	if _, ok := countries.ByISO2(fd.Country); !ok {
		return fail("Pick a country.")
	}
	if len(fd.Notes) > 2000 {
		return fail("The notes are too long.")
	}

	r := payroll.Rules{Currency: fd.Currency, Notes: fd.Notes, Year: fd.Year}
	for _, u := range fd.SourceURLs {
		if strings.HasPrefix(u, "https://") && len(r.Sources) < 10 && len(u) < 500 {
			r.Sources = append(r.Sources, u)
		}
	}
	if p, err := strconv.Atoi(fd.Pays); err == nil {
		r.PaysPerYear = p
	} else if fd.Pays != "" {
		return fail("Pays a year must be a whole number.")
	}
	var ok bool
	if r.Allowance.Amount, ok = formNum(fd.Allowance); !ok {
		return fail("The allowance must be a number.")
	}
	if r.Allowance.TaperFrom, ok = formNum(fd.TaperFrom); !ok {
		return fail("The allowance's taper must be a number.")
	}
	taper, ok := formNum(fd.TaperRate)
	if !ok {
		return fail("The allowance's taper rate must be a number.")
	}
	r.Allowance.TaperRate = taper / 100
	r.Allowance.AsCredit = fd.AsCredit
	for i, b := range fd.Brackets {
		up, ok1 := formNum(b.UpTo)
		rate, ok2 := formNum(b.Rate)
		if !ok1 || !ok2 {
			return fail("Bracket " + strconv.Itoa(i+1) + ": the limit and the rate must be numbers.")
		}
		r.Brackets = append(r.Brackets, payroll.Bracket{UpTo: up, Rate: rate / 100})
	}
	for _, d := range fd.Deductions {
		a, ok := formNum(d.Amount)
		if !ok || d.Name == "" {
			return fail("Every deduction needs a name and an amount.")
		}
		r.Deductions = append(r.Deductions, payroll.Deduction{Name: d.Name, Amount: a})
	}
	for _, c := range fd.Contributions {
		rate, ok1 := formNum(c.Rate)
		floor, ok2 := formNum(c.Floor)
		cp, ok3 := formNum(c.Cap)
		if !ok1 || !ok2 || !ok3 {
			return fail(c.Name + ": the rate, floor and cap must be numbers.")
		}
		bands, err := payroll.ParseBands(c.Bands)
		if err != nil {
			return fail(c.Name + "'s bands: " + err.Error() + ".")
		}
		r.Contributions = append(r.Contributions, payroll.Contribution{
			Name: c.Name, Rate: rate / 100, Floor: floor, Cap: cp, Monthly: c.Monthly, Deductible: c.Deductible, Credit: c.Credit, Bands: bands,
		})
	}
	if extra := v.Get("extra"); extra != "" && v.Get("drop_extra") == "" {
		var e salaryRuleExtra
		if err := json.Unmarshal([]byte(extra), &e); err != nil {
			return fail("The rules' other taxes can't be read.")
		}
		r.TaxName, r.Credits, r.Layers = e.TaxName, e.Credits, e.Layers
		fd.Extra = extra
		for _, c := range e.Credits {
			fd.ExtraNames = append(fd.ExtraNames, c.Name+" (credit)")
		}
		for _, l := range e.Layers {
			fd.ExtraNames = append(fd.ExtraNames, l.Name+" tax")
		}
	}
	var err error
	if r.Tables, err = payroll.ParseTables(fd.Tables); err != nil {
		return fail("Other scales: " + err.Error() + ".")
	}
	if r.WorkReduction, err = payroll.ParseSteps(fd.Steps); err != nil {
		return fail("Reduction: " + err.Error() + ".")
	}
	if err := r.Validate(); err != nil {
		return fail("These rules don't add up: " + strings.TrimPrefix(err.Error(), payroll.ErrInvalid.Error()+": ") + ".")
	}
	return fd, &models.SalaryRule{Name: fd.Name, Country: fd.Country, Rules: r}
}
