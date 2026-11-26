package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
	"github.com/omareloui/tyche/web/views/components"
	"github.com/omareloui/tyche/web/views/form"
	"go.uber.org/zap"
)

// tagSummaries are every tag's summaries (live then archived), or the one
// tag's when id is set.
func (h Handler) tagSummaries(ctx context.Context, tags []models.Tag, id string) (map[string]*models.TagSummary, error) {
	spends, err := h.repo.GetTagSpending(ctx, id)
	if err != nil {
		return nil, err
	}
	cats, err := h.repo.GetCategories(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]models.Category, len(cats))
	for _, c := range cats {
		byID[c.ID] = c
	}
	return models.NewTagSummaries(tags, spends, byID, getClaims(ctx).DefaultCurrency), nil
}

func (h Handler) tagsData(ctx context.Context) (*views.TagsData, error) {
	tags, err := h.repo.GetTags(ctx)
	if err != nil {
		return nil, err
	}
	sums, err := h.tagSummaries(ctx, tags, "")
	if err != nil {
		return nil, err
	}
	d := &views.TagsData{Def: &getClaims(ctx).DefaultCurrency}
	for _, t := range tags {
		if t.IsArchived() {
			d.Archived = append(d.Archived, sums[t.ID])
		} else {
			d.Live = append(d.Live, sums[t.ID])
		}
	}
	return d, nil
}

// Tags is the tags page: every tag with what it spent.
func (h Handler) Tags(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	b, err := h.repo.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.tagsData(ctx)
	if err != nil {
		logger.FromCtx(ctx).Error("fetching tags", zap.Error(err))
		return nil, err
	}
	curs, err := h.repo.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	// ?tag= opens the tag's details, e.g. when reloading the page.
	openID := r.URL.Query().Get(views.TagDetailsParam)
	return views.Tags(getClaims(ctx), b, mapTagToFD(&models.Tag{}, nil), curs, d, openID), nil
}

// TagsList is the tags' cards alone (views.TagsChangedEvent).
func (h Handler) TagsList(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d, err := h.tagsData(r.Context())
	if err != nil {
		return nil, err
	}
	return views.TagsList(d), nil
}

// tagOf is the path's tag, 404 when gone.
func (h Handler) tagOf(r *http.Request) (*models.Tag, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	t, err := h.repo.GetTag(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, resperr.New(http.StatusNotFound, "Tag not found")
	}
	return t, err
}

// TagDetails is the tag in full, in a drawer.
func (h Handler) TagDetails(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	t, err := h.tagOf(r)
	if err != nil {
		return nil, err
	}
	return h.tagDetails(r.Context(), t)
}

func (h Handler) tagDetails(ctx context.Context, t *models.Tag) (templ.Component, error) {
	sums, err := h.tagSummaries(ctx, []models.Tag{*t}, t.ID)
	if err != nil {
		return nil, err
	}
	s := sums[t.ID]
	m := s.Month(s.DefaultMonth(getClaims(ctx).MonthStart, h.userToday(ctx)), time.Time{}, time.Time{})
	return views.TagDetails(s, m), nil
}

// TagDays is the details' day chart of one of the user's months (?month=,
// else the default one), with the days ?from= to ?to= picked.
func (h Handler) TagDays(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	t, err := h.tagOf(r)
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	sums, err := h.tagSummaries(ctx, []models.Tag{*t}, t.ID)
	if err != nil {
		return nil, err
	}
	s := sums[t.ID]
	start := getClaims(ctx).MonthStart
	q := r.URL.Query()
	p := s.DefaultMonth(start, h.userToday(ctx))
	if month, err := time.Parse("2006-01", q.Get("month")); err == nil {
		p = start.For(month)
	}
	from, _ := time.Parse(time.DateOnly, q.Get("from"))
	to, _ := time.Parse(time.DateOnly, q.Get("to"))
	return views.TagDaysSection(s, s.Month(p, from, to)), nil
}

// tagAnswer answers a tag's action from its details with them again, in the
// drawer, the list re-rendered.
func (h Handler) tagAnswer(w http.ResponseWriter, r *http.Request, id string) (templ.Component, error) {
	w.Header().Set("HX-Retarget", "#"+components.DrawerSlot)
	w.Header().Set("HX-Reswap", "innerHTML")
	addHXTrigger(w, views.TagsChangedEvent)
	t, err := h.repo.GetTag(r.Context(), id)
	if err != nil {
		return nil, err
	}
	return h.tagDetails(r.Context(), t)
}

// mapFDToTag reads the tag form; fd has what couldn't be read.
func mapFDToTag(r *http.Request) (*models.Tag, *views.TagFD) {
	t := &models.Tag{
		Name:    r.FormValue("name"),
		Color:   r.FormValue("color"),
		AutoTag: r.FormValue("auto_tag") != "",
		Note:    strings.TrimSpace(r.FormValue("note")),
	}
	fd := mapTagToFD(t, nil)
	fd.StartsOn.Value = strings.TrimSpace(r.FormValue("starts_on"))
	fd.EndsOn.Value = strings.TrimSpace(r.FormValue("ends_on"))
	fd.Budget.Amount.Value = r.FormValue("amount")
	fd.Budget.CurrencyID.Value = r.FormValue("currency_id")
	bad := false
	parseDay := func(in *form.FDInput) *time.Time {
		if in.Value == "" {
			return nil
		}
		d, err := time.Parse(time.DateOnly, in.Value)
		if err != nil {
			in.Error = "Invalid date."
			bad = true
			return nil
		}
		return &d
	}
	t.StartsOn, t.EndsOn = parseDay(&fd.StartsOn), parseDay(&fd.EndsOn)
	budget, err := parseFloatIfExists(fd.Budget.Amount.Value)
	if err != nil {
		fd.Budget.Amount.Error = "Invalid amount."
		bad = true
	} else if budget > 0 {
		t.Budget = &budget
		t.BudgetCurrency = &models.Currency{ID: fd.Budget.CurrencyID.Value}
	}
	if !bad {
		return t, nil
	}
	return t, fd
}

func formatDayInput(d *time.Time) string {
	if d == nil {
		return ""
	}
	return d.Format(time.DateOnly)
}

// mapTagToFD is the tag's form, with valerr's errors.
func mapTagToFD(t *models.Tag, valerr *errs.ValidationError) *views.TagFD {
	if valerr == nil {
		valerr = &errs.ValidationError{}
	}
	fd := &views.TagFD{
		Name:     form.FDInput{Value: t.Name, Error: valerr.Errors.MsgFor("Name")},
		Color:    form.FDInput{Value: t.Color},
		StartsOn: form.FDInput{Value: formatDayInput(t.StartsOn)},
		EndsOn:   form.FDInput{Value: formatDayInput(t.EndsOn)},
		Note:     form.FDInput{Value: t.Note, Error: valerr.Errors.MsgFor("Note")},
		Budget:   &form.AmountFD{Amount: form.FDInput{Error: valerr.Errors.MsgFor("Budget")}},
	}
	if t.AutoTag {
		fd.AutoTag.Value = "on"
	}
	if t.Budget != nil {
		fd.Budget.Amount.Value = formatFloatIfNonZero(*t.Budget)
	}
	if t.BudgetCurrency != nil {
		fd.Budget.CurrencyID.Value = t.BudgetCurrency.ID
	}
	return fd
}

// saveTagForm validates and saves the form's tag (id empty creates it),
// answering nil when saved, else the form data with what went wrong.
func (h Handler) saveTagForm(r *http.Request, id string) (*models.Tag, *views.TagFD, error) {
	t, bad := mapFDToTag(r)
	if bad != nil {
		return nil, bad, nil
	}
	t.ID = id
	keep := func(fd *views.TagFD) *views.TagFD {
		fd.StartsOn.Value, fd.EndsOn.Value = r.FormValue("starts_on"), r.FormValue("ends_on")
		fd.Budget.Amount.Value, fd.Budget.CurrencyID.Value = r.FormValue("amount"), r.FormValue("currency_id")
		return fd
	}
	if err := t.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		switch {
		case errors.As(err, &valerr):
			return nil, keep(mapTagToFD(t, &valerr)), nil
		case errors.Is(err, models.ErrTagDays):
			fd := keep(mapTagToFD(t, nil))
			fd.EndsOn.Error = "A trip needs both days, the last on or after the first."
			return nil, fd, nil
		}
		return nil, nil, err
	}
	err := h.repo.SaveTag(r.Context(), t)
	if errors.Is(err, repository.ErrTagTaken) {
		fd := keep(mapTagToFD(t, nil))
		fd.Name.Error = "Another tag has this name."
		return nil, fd, nil
	}
	if err != nil {
		logger.FromCtx(r.Context()).Error("saving tag", zap.Error(err))
		return nil, nil, err
	}
	return t, nil, nil
}

// CreateTag creates a tag from the page's create form, answering the fresh
// form (the list re-rendered).
func (h Handler) CreateTag(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	curs, err := h.repo.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	def := &getClaims(ctx).DefaultCurrency
	t, fd, err := h.saveTagForm(r, "")
	if err != nil {
		return nil, err
	}
	if fd != nil {
		return views.CreateTagForm(fd, curs, def), nil
	}
	logger.FromCtx(ctx).Info("created tag", zap.String("tag_id", t.ID))
	drawerCreated(w)
	addHXTrigger(w, views.TagsChangedEvent)
	return views.CreateTagForm(mapTagToFD(&models.Tag{}, nil), curs, def), nil
}

// GetEditTag is the tag's edit form, in a drawer.
func (h Handler) GetEditTag(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	t, err := h.tagOf(r)
	if err != nil {
		return nil, err
	}
	curs, err := h.repo.GetCurrencies(r.Context())
	if err != nil {
		return nil, err
	}
	return views.EditTag(t.ID, mapTagToFD(t, nil), curs, &getClaims(r.Context()).DefaultCurrency), nil
}

// EditTag saves the tag's edit form, answering its details.
func (h Handler) EditTag(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	old, err := h.tagOf(r)
	if err != nil {
		return nil, err
	}
	_, fd, err := h.saveTagForm(r, old.ID)
	if err != nil {
		return nil, err
	}
	if fd != nil {
		curs, err := h.repo.GetCurrencies(r.Context())
		if err != nil {
			return nil, err
		}
		return views.EditTagForm(old.ID, fd, curs, &getClaims(r.Context()).DefaultCurrency), nil
	}
	logger.FromCtx(r.Context()).Info("edited tag", zap.String("tag_id", old.ID))
	return h.tagAnswer(w, r, old.ID)
}

// ArchiveTag archives the tag (?archived=true) or brings it back.
func (h Handler) ArchiveTag(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	t, err := h.tagOf(r)
	if err != nil {
		return nil, err
	}
	if err := h.repo.SetTagArchived(r.Context(), t.ID, r.URL.Query().Get("archived") == "true"); err != nil {
		return nil, err
	}
	return h.tagAnswer(w, r, t.ID)
}

// DeleteTag deletes the tag (its transactions stay, untagged), closing its
// details.
func (h Handler) DeleteTag(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	t, err := h.tagOf(r)
	if err != nil {
		return nil, err
	}
	if err := h.repo.DeleteTag(r.Context(), t.ID); err != nil {
		logger.FromCtx(r.Context()).Error("deleting tag", zap.Error(err), zap.String("tag_id", t.ID))
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("deleted tag", zap.String("tag_id", t.ID))
	addHXTrigger(w, drawerSavedEvent)
	addHXTrigger(w, views.TagsChangedEvent)
	return templ.NopComponent, nil
}

// TagRange previews tagging everything in a trip's days.
func (h Handler) TagRange(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	t, err := h.tagOf(r)
	if err != nil {
		return nil, err
	}
	if !t.IsTrip() {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Only a trip (a tag with days) can tag its days.")
	}
	c, err := h.repo.GetTagRangeCandidates(r.Context(), t)
	if err != nil {
		return nil, err
	}
	return views.TagRange(t, len(c.IDs), c.Totals), nil
}

// ApplyTagRange tags everything in a trip's days (the candidates worked out
// again), answering its details.
func (h Handler) ApplyTagRange(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	t, err := h.tagOf(r)
	if err != nil {
		return nil, err
	}
	if !t.IsTrip() || t.IsArchived() {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Only a live trip (a tag with days) can tag its days.")
	}
	c, err := h.repo.GetTagRangeCandidates(r.Context(), t)
	if err != nil {
		return nil, err
	}
	n, err := h.repo.TagTransactions(r.Context(), t.ID, c.IDs)
	if err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("tagged a trip's days", zap.String("tag_id", t.ID), zap.Int("count", n))
	return h.tagAnswer(w, r, t.ID)
}
