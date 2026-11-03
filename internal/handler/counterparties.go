package handler

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

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

	return views.Counterparties(getClaims(r.Context()), b, h.mapCounterpartyToFD(dfltCounterparty(), &errs.ValidationError{}, models.CounterpartyLabels(cp)), cp, cats), nil
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

// counterpartyRecentTxs is how many transactions a counterparty's details
// list, the rest a link away.
const counterpartyRecentTxs = 8

// CounterpartyDetails is the counterparty in full, in a drawer: its rating,
// note, locations, what was spent with it and its last transactions.
func (h Handler) CounterpartyDetails(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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

	d := &views.CounterpartyDetailsData{Counterparty: cp, Def: &getClaims(r.Context()).DefaultCurrency}
	f := models.TxFilter{CounterpartyID: id, Status: models.TxStatusFulfilled}
	if d.AllTime, err = h.repo.GetTransactionsSummary(r.Context(), f, d.Def); err != nil {
		l.Error("summing counterparty transactions", zap.Error(err))
		return nil, err
	}
	now := time.Now()
	f.From = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if d.ThisMonth, err = h.repo.GetTransactionsSummary(r.Context(), f, d.Def); err != nil {
		l.Error("summing counterparty transactions of the month", zap.Error(err))
		return nil, err
	}
	if d.Recent, err = h.repo.GetCounterpartyRecentTransactions(r.Context(), id, counterpartyRecentTxs); err != nil {
		l.Error("fetching counterparty transactions", zap.Error(err))
		return nil, err
	}

	return views.CounterpartyDetails(d), nil
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
	labels, err := h.counterpartyLabels(r)
	if err != nil {
		l.Error("fetching counterparty labels", zap.Error(err))
		return nil, err
	}
	cp.Labels = models.MatchLabelCasing(cp.Labels, labels)

	if err := cp.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating counterparty", zap.Error(err), zap.Any("errors", valerr.Errors))
			return views.CreateCounterpartyForm(h.mapCounterpartyToFD(cp, &valerr, labels), cats), nil
		}
		l.Error("sanitizing counterparty", zap.Error(err))
		return nil, err
	}

	clearNonMerchantFields(cp)

	if valerr := resolveCounterpartyDefaultCategory(cp, cats); valerr != nil {
		return views.CreateCounterpartyForm(h.mapCounterpartyToFD(cp, valerr, labels), cats), nil
	}

	if cp.Icon == "" {
		if cp.IsMerchant {
			cp.Icon = brandicons.Default().Detect(cp.Name, cp.Aliases)
		}
	} else if valerr := validateCounterpartyIcon(cp); valerr != nil {
		return views.CreateCounterpartyForm(h.mapCounterpartyToFD(cp, valerr, labels), cats), nil
	}
	cp.Color = brandicons.Default().ColorFor(cp.Icon, "")

	if err := h.repo.CreateCounterparty(r.Context(), cp); err != nil {
		l.Error("creating counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("created counterparty")
	drawerCreated(w)

	if err = renderToBody(w, r, views.CounterpartyOOB(cp)); err != nil {
		l.Error("rendering counterparty oob", zap.Error(err))
		return nil, err
	}
	l.Debug("rendered counterparty oob")

	labels = models.CounterpartyLabels([]models.Counterparty{{Labels: labels}, *cp})
	return views.CreateCounterpartyForm(h.mapCounterpartyToFD(dfltCounterparty(), &errs.ValidationError{}, labels), cats), nil
}

// QuickCreateCounterparty creates a merchant (a person with the "person"
// query, e.g. for debts) from a combobox's search and responds with its
// combobox option. An existing counterparty with that name or alias is reused.
func (h Handler) QuickCreateCounterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())

	cp := dfltCounterparty()
	cp.ID = idgen.New()
	cp.Name = r.FormValue("name")
	cp.IsMerchant = !r.URL.Query().Has("person")
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
		// Picked by name again, so it's in use: back in the pickers.
		if existing.IsArchived {
			if err := h.repo.SetCounterpartyArchived(r.Context(), existing.ID, false); err != nil {
				l.Error("unarchiving counterparty", zap.Error(err))
				return nil, err
			}
			l.Info("unarchived quick created counterparty", zap.String("id", existing.ID))
		}
		return views.CounterpartyComboOption(existing), nil
	}

	if cp.IsMerchant {
		cp.Icon = brandicons.Default().Detect(cp.Name, cp.Aliases)
	}
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

	labels, err := h.counterpartyLabels(r)
	if err != nil {
		l.Error("fetching counterparty labels", zap.Error(err))
		return nil, err
	}

	return views.EditCounterparty(id, h.mapCounterpartyToFD(cp, &errs.ValidationError{}, labels), cats), nil
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
	labels, err := h.counterpartyLabels(r)
	if err != nil {
		l.Error("fetching counterparty labels", zap.Error(err))
		return nil, err
	}
	cp.Labels = models.MatchLabelCasing(cp.Labels, labels)

	if err := cp.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if errors.As(err, &valerr) {
			l.Error("validating counterparty", zap.Error(err), zap.Any("errors", valerr.Errors))
			return views.EditCounterpartyForm(id, h.mapCounterpartyToFD(cp, &valerr, labels), cats), nil
		}
		l.Error("sanitizing counterparty", zap.Error(err))
		return nil, err
	}

	clearNonMerchantFields(cp)

	if valerr := resolveCounterpartyDefaultCategory(cp, cats); valerr != nil {
		return views.EditCounterpartyForm(id, h.mapCounterpartyToFD(cp, valerr, labels), cats), nil
	}

	if valerr := validateCounterpartyIcon(cp); valerr != nil {
		return views.EditCounterpartyForm(id, h.mapCounterpartyToFD(cp, valerr, labels), cats), nil
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

	if views.CounterpartiesListID(cp) == views.CounterpartiesListID(prev) {
		drawerSaved(w, "#"+views.CounterpartyWrapperID(id), "innerHTML")
		return views.Counterparty(cp), nil
	}

	// (Un)archived or (un)marked as a merchant: move the card to its list.
	drawerSaved(w, "#"+views.CounterpartyWrapperID(id), "none")
	if err := renderToBody(w, r, views.CounterpartyDeleteOOB(id)); err != nil {
		l.Error("rendering counterparty delete oob", zap.Error(err))
		return nil, err
	}
	return views.CounterpartyOOB(cp), nil
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

// ToggleArchiveCounterparty archives the counterparty, or unarchives an
// archived one, moving its card to the other section of the list.
func (h Handler) ToggleArchiveCounterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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

	cp.IsArchived = !cp.IsArchived
	if err := h.repo.SetCounterpartyArchived(r.Context(), id, cp.IsArchived); err != nil {
		l.Error("toggling counterparty archive", zap.Error(err))
		return nil, err
	}
	l.Info("toggled counterparty archive", zap.String("id", id), zap.Bool("is_archived", cp.IsArchived))

	w.Header().Set("HX-Reswap", "none")
	if err := renderToBody(w, r, views.CounterpartyDeleteOOB(id)); err != nil {
		l.Error("rendering counterparty delete oob", zap.Error(err))
		return nil, err
	}
	return views.CounterpartyOOB(cp), nil
}

// RateCounterparty sets a merchant's rating from its card's stars (0 clears
// it) and answers the card.
func (h Handler) RateCounterparty(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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
	if !cp.IsMerchant {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Only merchants can be rated")
	}

	cp.Rating = parseRating(r.FormValue("rating"))
	if err := h.repo.SetCounterpartyRating(r.Context(), id, cp.Rating); err != nil {
		l.Error("rating counterparty", zap.Error(err))
		return nil, err
	}
	l.Info("rated counterparty", zap.String("id", id), zap.Int("rating", cp.Rating))

	// The details' stars keep their state, only the card is updated.
	if r.FormValue("from") == views.CounterpartyFromDetails {
		w.Header().Set("HX-Reswap", "none")
		return views.CounterpartyReplaceOOB(cp), nil
	}
	return views.Counterparty(cp), nil
}

// AddCounterpartyLocation adds a location to a merchant from its details,
// answering their locations (with the form's errors, if any) and its card out
// of band.
func (h Handler) AddCounterpartyLocation(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
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
	if !cp.IsMerchant {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Only merchants have locations")
	}

	loc := &models.CounterpartyLocation{
		Label:   strings.TrimSpace(r.FormValue("label")),
		Address: strings.TrimSpace(r.FormValue("address")),
		MapsURL: strings.TrimSpace(r.FormValue("maps_url")),
	}
	if valerr := h.valid.Validate(loc); valerr != nil {
		l.Info("invalid counterparty location", zap.Any("errors", valerr.Errors))
		return views.CounterpartyDetailsLocations(cp, &mapLocationsToFD([]models.CounterpartyLocation{*loc}, valerr, "")[0]), nil
	}

	if err := h.repo.AddCounterpartyLocation(r.Context(), id, loc); err != nil {
		l.Error("adding counterparty location", zap.Error(err))
		return nil, err
	}
	l.Info("added counterparty location", zap.String("id", id))
	cp.Locations = append(cp.Locations, *loc)

	if err := renderToBody(w, r, views.CounterpartyReplaceOOB(cp)); err != nil {
		l.Error("rendering counterparty oob", zap.Error(err))
		return nil, err
	}
	return views.CounterpartyDetailsLocations(cp, nil), nil
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
		return views.MergeCounterpartyForm(from, fd, cps), nil
	}

	into, err := h.repo.MergeCounterparties(r.Context(), id, intoID)
	if err != nil {
		l.Error("merging counterparties", zap.Error(err), zap.String("from", id), zap.String("into", intoID))
		return nil, err
	}
	l.Info("merged counterparties", zap.String("from", id), zap.String("into", intoID))

	// Drop the merged counterparty's card, and update the one merged into.
	drawerSaved(w, "#"+views.CounterpartyWrapperID(id), "outerHTML")
	return views.CounterpartyReplaceOOB(into), nil
}

// counterpartyAndMergeTargets returns the counterparty and the others it can
// be merged into, archived ones left out.
func (h Handler) counterpartyAndMergeTargets(r *http.Request, id string) (*models.Counterparty, []models.Counterparty, error) {
	from, err := h.repo.GetCounterparty(r.Context(), id)
	if err != nil {
		return nil, nil, err
	}
	cps, err := h.repo.GetCounterparties(r.Context())
	if err != nil {
		return nil, nil, err
	}
	cps = slices.DeleteFunc(cps, func(cp models.Counterparty) bool { return cp.ID == id || cp.IsArchived })
	return from, cps, nil
}

// counterpartyLabels are the labels in use by the counterparties.
func (h Handler) counterpartyLabels(r *http.Request) ([]string, error) {
	cps, err := h.repo.GetCounterparties(r.Context())
	if err != nil {
		return nil, err
	}
	return models.CounterpartyLabels(cps), nil
}

// mapCounterpartyToFD maps the counterparty, its errors and the labels in
// use (suggested in the form).
func (h Handler) mapCounterpartyToFD(cp *models.Counterparty, err *errs.ValidationError, labels []string) *views.CounterpartyFD {
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
		IsArchived:        form.FDInput{Value: formatBooleanIfNonZero(cp.IsArchived), Error: err.Errors.MsgFor("IsArchived")},
		Rating:            form.FDInput{Value: formatIntIfNonZero(cp.Rating), Error: err.Errors.MsgFor("Rating")},
		Note:              form.FDInput{Value: cp.Note, Error: err.Errors.MsgFor("Note")},
		Labels:            form.FDInput{Value: strings.Join(cp.Labels, ", "), Error: labelsError(err)},
		LabelsInUse:       labels,
		Locations:         mapLocationsToFD(cp.Locations, err, "Locations"),
	}
}

// mapLocationsToFD maps the locations and their errors, keyed by prefix and
// index (e.g. "Locations[0]."), or by field alone for an empty prefix.
func mapLocationsToFD(locs []models.CounterpartyLocation, err *errs.ValidationError, prefix string) []views.CounterpartyLocationFD {
	fds := make([]views.CounterpartyLocationFD, len(locs))
	for i, loc := range locs {
		key := ""
		if prefix != "" {
			key = fmt.Sprintf("%s[%d].", prefix, i)
		}
		fds[i] = views.CounterpartyLocationFD{
			ID:      loc.ID,
			Label:   form.FDInput{Value: loc.Label, Error: err.Errors.MsgFor(key + "Label")},
			Address: form.FDInput{Value: loc.Address, Error: err.Errors.MsgFor(key + "Address")},
			MapsURL: form.FDInput{Value: loc.MapsURL, Error: err.Errors.MsgFor(key + "MapsURL")},
		}
	}
	return fds
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
		IsArchived:      parseBoolean(r.FormValue("is_archived")),
		Rating:          parseRating(r.FormValue("rating")),
		Note:            r.FormValue("note"),
		Labels:          parseAliases(r.FormValue("labels")),
		Locations:       parseCounterpartyLocations(r),
	}, nil
}

// labelsError is the first error of the labels, each label's included.
func labelsError(err *errs.ValidationError) string {
	if msg := err.Errors.MsgFor("Labels"); msg != "" {
		return msg
	}
	for k, f := range err.Errors {
		if strings.HasPrefix(k, "Labels[") {
			return f.String()
		}
	}
	return ""
}

// parseRating reads a rating, 0 (unrated) when blank or out of range.
func parseRating(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 || n > models.MaxRating {
		return 0
	}
	return n
}

// parseCounterpartyLocations reads the location rows (parallel location_*
// fields, one value per row), trimmed, dropping blank rows.
func parseCounterpartyLocations(r *http.Request) []models.CounterpartyLocation {
	_ = r.ParseForm()
	ids, labels := r.PostForm["location_id"], r.PostForm["location_label"]
	addrs, urls := r.PostForm["location_address"], r.PostForm["location_maps_url"]
	at := func(vs []string, i int) string {
		if i < len(vs) {
			return strings.TrimSpace(vs[i])
		}
		return ""
	}
	locs := []models.CounterpartyLocation{}
	for i := range max(len(labels), len(addrs), len(urls)) {
		loc := models.CounterpartyLocation{
			ID: at(ids, i), Label: at(labels, i), Address: at(addrs, i), MapsURL: at(urls, i),
		}
		if loc.Label == "" && loc.Address == "" && loc.MapsURL == "" {
			continue
		}
		locs = append(locs, loc)
	}
	return locs
}

// clearNonMerchantFields drops the merchant only fields, as hidden form inputs
// are still submitted.
func clearNonMerchantFields(cp *models.Counterparty) {
	if cp.IsMerchant {
		return
	}
	cp.Icon = ""
	cp.DefaultCategory = nil
	cp.Rating = 0
	cp.Locations = nil
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

// parseAliases splits a comma separated list (aliases, labels), dropping blanks and
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
