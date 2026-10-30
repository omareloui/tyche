package handler

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/brandicons"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

func (h Handler) Counterparties(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}

	cp, err := h.repo.GetCounterparties(r.Context())
	if err != nil {
		l.Error("fetching counter parties", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched counter parties")

	cats, err := h.repo.GetCategories(r.Context())
	if err != nil {
		l.Error("fetching categories", zap.Error(err))
		return nil, err
	}

	return views.Counterparties(getClaims(r.Context()), b, h.mapCounterpartyToFD(dfltCounterparty(), &errs.ValidationError{}), cp, cats), nil
}

func (h Handler) Counterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	cp, err := h.repo.GetCounterparty(r.Context(), id)
	if err != nil {
		l.Error("fetching counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("fetched counterparty", zap.Any("counterparty", cp))

	return views.Counterparty(cp), nil
}

func (h Handler) CreateCounterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	cp, err := h.mapFDToCounterparty(r)
	if err != nil {
		l.Error("parsing the form", zap.Error(err))
		return nil, err
	}
	cp.ID = idgen.New()

	cats, err := h.repo.GetCategories(r.Context())
	if err != nil {
		l.Error("fetching categories", zap.Error(err))
		return nil, err
	}

	if err := cp.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating counterparty", zap.Error(err), zap.Any("errors", valerr.Errors))
			return views.CreateCounterpartyForm(h.mapCounterpartyToFD(cp, &valerr), cats), nil
		}
		l.Error("sanitizing counterparty", zap.Error(err))
		return nil, err
	}

	clearNonMerchantFields(cp)

	if valerr := resolveCounterpartyDefaultCategory(cp, cats); valerr != nil {
		return views.CreateCounterpartyForm(h.mapCounterpartyToFD(cp, valerr), cats), nil
	}

	if cp.Icon == "" {
		if cp.IsMerchant {
			cp.Icon = brandicons.Default().Detect(cp.Name, cp.Aliases)
		}
	} else if valerr := validateCounterpartyIcon(cp); valerr != nil {
		return views.CreateCounterpartyForm(h.mapCounterpartyToFD(cp, valerr), cats), nil
	}
	cp.Color = brandicons.Default().ColorFor(cp.Icon, "")

	if err := h.repo.CreateCounterparty(r.Context(), cp); err != nil {
		l.Error("creating counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("created counterparty")

	if err = renderToBody(w, r, views.CounterpartyOOB(cp)); err != nil {
		l.Error("rendering counterparty oob", zap.Error(err))
		return nil, err
	}
	l.Debug("rendered counterparty oob")

	return views.CreateCounterpartyForm(h.mapCounterpartyToFD(dfltCounterparty(), &errs.ValidationError{}), cats), nil
}

// QuickCreateCounterparty creates a merchant from a combobox's search and
// responds with its combobox option. An existing counterparty with that name
// or alias is reused.
func (h Handler) QuickCreateCounterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	cp := dfltCounterparty()
	cp.ID = idgen.New()
	cp.Name = r.FormValue("name")
	if err := cp.SanitizeAndValidate(h.sani, h.valid); err != nil {
		l.Error("quick create counterparty validation", zap.Error(err))
		return nil, resperr.New(http.StatusBadRequest, err.Error())
	}

	cps, err := h.repo.GetCounterparties(r.Context())
	if err != nil {
		l.Error("fetching counterparties", zap.Error(err))
		return nil, err
	}
	if existing := slicesutils.Find(cps, func(c models.Counterparty) bool {
		return strings.EqualFold(c.Name, cp.Name) ||
			slices.ContainsFunc(c.Aliases, func(a string) bool { return strings.EqualFold(a, cp.Name) })
	}); existing != nil {
		return views.CounterpartyComboOption(existing), nil
	}

	cp.Icon = brandicons.Default().Detect(cp.Name, cp.Aliases)
	cp.Color = brandicons.Default().ColorFor(cp.Icon, "")

	if err := h.repo.CreateCounterparty(r.Context(), cp); err != nil {
		l.Error("creating counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("quick created counterparty")

	return views.CounterpartyComboOption(cp), nil
}

func (h Handler) GetEditCounterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	cp, err := h.repo.GetCounterparty(r.Context(), id)
	if err != nil {
		l.Error("fetching counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("fetched counterparty", zap.Any("counterparty", cp))

	cats, err := h.repo.GetCategories(r.Context())
	if err != nil {
		l.Error("fetching categories", zap.Error(err))
		return nil, err
	}

	return views.EditCounterparty(id, h.mapCounterpartyToFD(cp, &errs.ValidationError{}), cats), nil
}

func (h Handler) EditCounterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	cp, err := h.mapFDToCounterparty(r)
	if err != nil {
		l.Error("parsing the form", zap.Error(err))
		return nil, err
	}
	cp.ID = id

	cats, err := h.repo.GetCategories(r.Context())
	if err != nil {
		l.Error("fetching categories", zap.Error(err))
		return nil, err
	}

	if err := cp.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating counterparty", zap.Error(err), zap.Any("errors", valerr.Errors))
			return views.EditCounterparty(id, h.mapCounterpartyToFD(cp, &valerr), cats), nil
		}
		l.Error("sanitizing counterparty", zap.Error(err))
		return nil, err
	}

	clearNonMerchantFields(cp)

	if valerr := resolveCounterpartyDefaultCategory(cp, cats); valerr != nil {
		return views.EditCounterparty(id, h.mapCounterpartyToFD(cp, valerr), cats), nil
	}

	if valerr := validateCounterpartyIcon(cp); valerr != nil {
		return views.EditCounterparty(id, h.mapCounterpartyToFD(cp, valerr), cats), nil
	}

	prev, err := h.repo.GetCounterparty(r.Context(), id)
	if err != nil {
		l.Error("fetching counterparty", zap.Error(err))
		return nil, err
	}

	// Detect the icon of a counterparty just marked as a merchant.
	if cp.Icon == "" && cp.IsMerchant && !prev.IsMerchant {
		cp.Icon = brandicons.Default().Detect(cp.Name, cp.Aliases)
	}
	cp.Color = brandicons.Default().ColorFor(cp.Icon, prev.Color)

	l.Debug("update counterparty", zap.Any("counterparty", cp))

	if err := h.repo.UpdateCounterparty(r.Context(), cp); err != nil {
		l.Error("update counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("updated counterparty")

	return views.Counterparty(cp), nil
}

func (h Handler) DeleteCounterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())
	if err := h.repo.DeleteCounterparty(r.Context(), id); err != nil {
		l.Error("deleting counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("deleted counterparty")

	return nil, nil
}

func (h Handler) GetMergeCounterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	from, cps, err := h.counterpartyAndMergeTargets(r, id)
	if err != nil {
		l.Error("fetching counterparties to merge", zap.Error(err))
		return nil, err
	}

	return views.MergeCounterparty(from, &views.MergeCounterpartyFD{}, cps), nil
}

func (h Handler) MergeCounterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}

	l := logger.FromCtx(r.Context())

	from, cps, err := h.counterpartyAndMergeTargets(r, id)
	if err != nil {
		l.Error("fetching counterparties to merge", zap.Error(err))
		return nil, err
	}

	intoID := r.FormValue("into_id")
	if !slices.ContainsFunc(cps, func(cp models.Counterparty) bool { return cp.ID == intoID }) {
		fd := &views.MergeCounterpartyFD{IntoID: form.FDInput{Value: intoID, Error: "Select a counterparty to merge into"}}
		return views.MergeCounterparty(from, fd, cps), nil
	}

	into, err := h.repo.MergeCounterparties(r.Context(), id, intoID)
	if err != nil {
		l.Error("merging counterparties", zap.Error(err), zap.String("from", id), zap.String("into", intoID))
		return nil, err
	}
	l.Info("merged counterparties", zap.String("from", id), zap.String("into", intoID))

	// Drop the merged counterparty's card, and update the one merged into.
	w.Header().Set("HX-Retarget", "#"+views.CounterpartyWrapperID(id))
	w.Header().Set("HX-Reswap", "outerHTML")
	return views.CounterpartyReplaceOOB(into), nil
}

// counterpartyAndMergeTargets returns the counterparty and the others it can
// be merged into.
func (h Handler) counterpartyAndMergeTargets(r *http.Request, id string) (*models.Counterparty, []models.Counterparty, error) {
	from, err := h.repo.GetCounterparty(r.Context(), id)
	if err != nil {
		return nil, nil, err
	}
	cps, err := h.repo.GetCounterparties(r.Context())
	if err != nil {
		return nil, nil, err
	}
	cps = slices.DeleteFunc(cps, func(cp models.Counterparty) bool { return cp.ID == id })
	return from, cps, nil
}

func (h Handler) mapCounterpartyToFD(cp *models.Counterparty, err *errs.ValidationError) *views.CounterpartyFD {
	var defaultCategoryID string
	if cp.DefaultCategory != nil {
		defaultCategoryID = cp.DefaultCategory.ID
	}
	return &views.CounterpartyFD{
		Name:              form.FDInput{Value: cp.Name, Error: err.Errors.MsgFor("Name")},
		Aliases:           form.FDInput{Value: strings.Join(cp.Aliases, ", "), Error: err.Errors.MsgFor("Aliases")},
		Icon:              form.FDInput{Value: cp.Icon, Error: err.Errors.MsgFor("Icon")},
		IsMerchant:        form.FDInput{Value: formatBooleanIfNonZero(cp.IsMerchant), Error: err.Errors.MsgFor("IsMerchant")},
		DefaultCategoryID: form.FDInput{Value: defaultCategoryID, Error: err.Errors.MsgFor("DefaultCategoryID")},
	}
}

func (h Handler) mapFDToCounterparty(r *http.Request) (*models.Counterparty, error) {
	var defaultCategory *models.Category
	if catID := r.FormValue("default_category_id"); catID != "" {
		defaultCategory = &models.Category{ID: catID}
	}
	return &models.Counterparty{
		Name:            r.FormValue("name"),
		Aliases:         parseAliases(r.FormValue("aliases")),
		Icon:            r.FormValue("icon"),
		IsMerchant:      parseBoolean(r.FormValue("is_merchant")),
		DefaultCategory: defaultCategory,
	}, nil
}

// clearNonMerchantFields drops the merchant only fields, as hidden form inputs
// are still submitted.
func clearNonMerchantFields(cp *models.Counterparty) {
	if cp.IsMerchant {
		return
	}
	cp.Icon = ""
	cp.DefaultCategory = nil
}

// resolveCounterpartyDefaultCategory replaces the default category with the
// full one from the list, erroring if it doesn't exist.
func resolveCounterpartyDefaultCategory(cp *models.Counterparty, cats []models.Category) *errs.ValidationError {
	if cp.DefaultCategory == nil {
		return nil
	}
	cat := slicesutils.Find(cats, func(c models.Category) bool { return c.ID == cp.DefaultCategory.ID })
	if cat == nil {
		return &errs.ValidationError{Errors: errs.ValidationErrors{"DefaultCategoryID": {Tag: "required"}}}
	}
	cp.DefaultCategory = cat
	return nil
}

// validateCounterpartyIcon expects a sanitized counterparty, where a blank
// icon means no icon.
func validateCounterpartyIcon(cp *models.Counterparty) *errs.ValidationError {
	if cp.Icon == "" {
		return nil
	}
	if _, ok := brandicons.Default().Get(cp.Icon); !ok {
		return &errs.ValidationError{Errors: errs.ValidationErrors{"Icon": {Tag: "brand_icon"}}}
	}
	return nil
}

// parseAliases splits a comma separated list, dropping blanks and
// case-insensitive duplicates.
func parseAliases(s string) []string {
	aliases := []string{}
	seen := map[string]bool{}
	for a := range strings.SplitSeq(s, ",") {
		a = strings.TrimSpace(a)
		key := strings.ToLower(a)
		if a == "" || seen[key] {
			continue
		}
		seen[key] = true
		aliases = append(aliases, a)
	}
	return aliases
}

func dfltCounterparty() *models.Counterparty {
	return &models.Counterparty{IsMerchant: true}
}
