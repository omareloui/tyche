package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/countries"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/visa"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// visaRulesChanged re-renders the tool, picking the requirements id (none
// when deleted) once the drawer closes.
func visaRulesChanged(w http.ResponseWriter, id, country string) {
	b, _ := json.Marshal(map[string]any{
		drawerSavedEvent:     true,
		"visa-rules-changed": map[string]string{"id": id, "country": country},
	})
	w.Header().Set("HX-Trigger", string(b))
}

// VisaRules lists the user's own visa requirements in a drawer.
func (h Handler) VisaRules(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	rules, err := h.repo.GetVisaRules(r.Context())
	if err != nil {
		return nil, err
	}
	return views.VisaRulesDrawer(rules), nil
}

// NewVisaRule is the form of new requirements, a copy of the visa the query
// string picks (as the tool's form) when from, else empty for its country.
func (h Handler) NewVisaRule(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	v := r.URL.Query()
	pick := parseVisaPick(v, "")
	c := visaCountry(ctx, pick.Country)
	rule := &visa.Rule{Currency: c.Currency, Category: visa.CategoryOther, Requirements: []visa.Requirement{{Kind: visa.KindIncome, Period: visa.PeriodMonth}}}
	name := ""
	if v.Get("from") != "" {
		mine, err := h.repo.GetVisaRules(ctx)
		if err != nil {
			return nil, err
		}
		p, err := h.resolveVisa(ctx, &pick, mine)
		if err != nil {
			return nil, err
		}
		if p.Rule != nil {
			rule = p.Rule.Clone()
			name = rule.Name + " (my copy)"
		}
	}
	return views.VisaRuleDrawer("", visaRuleFD(name, c.ISO2, rule)), nil
}

// EditVisaRule is the form of one of the user's requirements.
func (h Handler) EditVisaRule(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	v, err := h.repo.GetVisaRule(r.Context(), r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, resperr.New(http.StatusNotFound, "Requirements not found")
	}
	return views.VisaRuleDrawer(v.ID, visaRuleFD(v.Name, v.Country, &v.Rule)), nil
}

// CreateVisaRule saves new requirements of the user's.
func (h Handler) CreateVisaRule(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.saveVisaRule(w, r, "")
}

// UpdateVisaRule saves the user's changed requirements.
func (h Handler) UpdateVisaRule(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	v, err := h.repo.GetVisaRule(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, resperr.New(http.StatusNotFound, "Requirements not found")
	}
	return h.saveVisaRule(w, r, id)
}

func (h Handler) saveVisaRule(w http.ResponseWriter, r *http.Request, id string) (templ.Component, error) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "Invalid form")
	}
	fd, v := parseVisaRuleForm(r.Form)
	if v == nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return views.VisaRuleForm(id, fd), nil
	}
	v.ID = id
	if err := h.repo.SaveVisaRule(ctx, v); err != nil {
		return nil, err
	}
	logger.FromCtx(ctx).Info("saved visa rules", zap.String("id", v.ID))
	visaRulesChanged(w, v.ID, v.Country)
	return views.VisaRuleForm(v.ID, fd), nil
}

// DeleteVisaRule deletes requirements of the user's.
func (h Handler) DeleteVisaRule(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	id := r.PathValue("id")
	if err := h.repo.DeleteVisaRule(ctx, id); err != nil {
		return nil, err
	}
	logger.FromCtx(ctx).Info("deleted visa rules", zap.String("id", id))
	visaRulesChanged(w, "", "")
	return templ.NopComponent, nil
}

// fmtNums is amounts as the form's comma separated text.
func fmtNums(vs []float64) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return strings.Join(parts, ", ")
}

// visaRuleFD is the form of requirements.
func visaRuleFD(name, country string, r *visa.Rule) *views.VisaRuleFD {
	fd := &views.VisaRuleFD{
		Countries:  countries.All(),
		Name:       name,
		Country:    country,
		Currency:   r.Currency,
		Category:   string(r.Category),
		Notes:      r.Notes,
		Year:       r.Year,
		SourceURLs: r.Sources,
	}
	for _, q := range r.Requirements {
		row := views.VisaReqRow{
			Kind: string(q.Kind), Period: string(q.Period), Amount: fmtNum(q.Amount),
			PerDependent: fmtNums(q.PerDependent), Group: q.Group, Text: q.Text,
		}
		if q.MonthsProof > 0 {
			row.Months = strconv.Itoa(q.MonthsProof)
		}
		fd.Reqs = append(fd.Reqs, row)
	}
	return fd
}

// parseVisaRuleForm reads the requirements' form, the rule nil when invalid
// (the error in the form data).
func parseVisaRuleForm(v url.Values) (*views.VisaRuleFD, *models.VisaRule) {
	fd := &views.VisaRuleFD{
		Countries: countries.All(),
		Name:      strings.TrimSpace(v.Get("name")),
		Country:   strings.ToUpper(strings.TrimSpace(v.Get("country"))),
		Currency:  strings.ToUpper(strings.TrimSpace(v.Get("currency"))),
		Category:  string(visa.ParseCategory(v.Get("category"))),
		Notes:     strings.TrimSpace(v.Get("notes")),
	}
	fd.Year, _ = strconv.Atoi(v.Get("year"))
	fd.SourceURLs = v["source_url"]
	kinds := v["req_kind"]
	for i := range kinds {
		row := views.VisaReqRow{
			Kind:         strings.TrimSpace(kinds[i]),
			Period:       strings.TrimSpace(at(v["req_period"], i)),
			Amount:       strings.TrimSpace(at(v["req_amount"], i)),
			PerDependent: strings.TrimSpace(at(v["req_per_dep"], i)),
			Group:        strings.TrimSpace(at(v["req_group"], i)),
			Months:       strings.TrimSpace(at(v["req_months"], i)),
			Text:         strings.TrimSpace(at(v["req_text"], i)),
		}
		fd.Reqs = append(fd.Reqs, row)
	}
	fail := func(msg string) (*views.VisaRuleFD, *models.VisaRule) {
		fd.Error = msg
		return fd, nil
	}
	if fd.Name == "" || len(fd.Name) > 80 {
		return fail("Name the visa (80 characters at most).")
	}
	if _, ok := countries.ByISO2(fd.Country); !ok {
		return fail("Pick a country.")
	}
	if len(fd.Notes) > 2000 {
		return fail("Notes are 2,000 characters at most.")
	}
	rule := visa.Rule{Name: fd.Name, Country: fd.Country, Currency: fd.Currency, Category: visa.Category(fd.Category), Year: fd.Year, Notes: fd.Notes, Sources: fd.SourceURLs}
	if rule.Year <= 0 {
		rule.Year = visaYear()
	}
	for i, row := range fd.Reqs {
		q := visa.Requirement{Kind: visa.Kind(row.Kind), Period: visa.Period(row.Period), Group: row.Group, Text: row.Text}
		if q.Kind.Money() {
			amount, ok := formNum(row.Amount)
			if !ok {
				return fail("Requirement " + strconv.Itoa(i+1) + ": check its amount.")
			}
			q.Amount = amount
			if q.Kind == visa.KindIncome || q.Kind == visa.KindSalary {
				if q.Period != visa.PeriodYear {
					q.Period = visa.PeriodMonth
				}
			} else {
				q.Period = visa.PeriodOnce
			}
			for _, s := range strings.FieldsFunc(row.PerDependent, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
				d, ok := formNum(s)
				if !ok {
					return fail("Requirement " + strconv.Itoa(i+1) + ": check its amounts per dependent.")
				}
				q.PerDependent = append(q.PerDependent, d)
			}
			if q.Kind == visa.KindIncome && row.Months != "" {
				n, err := strconv.Atoi(row.Months)
				if err != nil {
					return fail("Requirement " + strconv.Itoa(i+1) + ": months of proof are a number.")
				}
				q.MonthsProof = n
			}
		} else {
			q.Period = ""
		}
		rule.Requirements = append(rule.Requirements, q)
	}
	if err := rule.Validate(); err != nil {
		return fail(strings.ToUpper(err.Error()[:1]) + err.Error()[1:] + ".")
	}
	return fd, &models.VisaRule{Name: fd.Name, Country: fd.Country, Rule: rule}
}
