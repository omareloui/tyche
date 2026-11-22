package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

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

// maxItemSuggestions are the items offered while typing a line's name.
const maxItemSuggestions = 8

var errItemNotFound = resperr.New(http.StatusNotFound, "Item not found")

// Items is the items page: the catalog of what was bought, with its stats.
func (h Handler) Items(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		l.Error("fetching balances", zap.Error(err))
		return nil, err
	}
	d, err := h.itemsList(r)
	if err != nil {
		return nil, err
	}
	// ?item= opens the item's details, e.g. when reloading the page.
	return views.Items(getClaims(r.Context()), b, d, r.URL.Query().Get(views.ItemDetailsParam)), nil
}

// ItemsList is the items page's body alone, re-rendered when items change
// or the range does (its url kept in the page's).
func (h Handler) ItemsList(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d, err := h.itemsList(r)
	if err != nil {
		return nil, err
	}
	page := "/items"
	if q := d.Range.Values().Encode(); q != "" {
		page += "?" + q
	}
	w.Header().Set("HX-Replace-Url", page)
	return views.ItemsList(d), nil
}

func (h Handler) itemsList(r *http.Request) (*views.ItemsData, error) {
	l := logger.FromCtx(r.Context())
	claims := getClaims(r.Context())
	def := claims.DefaultCurrency
	rng := models.ParseItemsRange(r.URL.Query(), h.userToday(r.Context()), claims.MonthStart)
	items, err := h.repo.GetItems(r.Context(), def, rng)
	if err != nil {
		l.Error("fetching items", zap.Error(err))
		return nil, err
	}
	d := &views.ItemsData{Items: items, Def: &def, Range: rng, Any: len(items) > 0}
	if !rng.IsAll() {
		// Only the ones bought in it.
		d.Items = slices.DeleteFunc(items, func(s models.ItemSummary) bool { return s.Stats.Times == 0 })
	}
	return d, nil
}

// item is the item of the path.
func (h Handler) item(r *http.Request) (*models.ItemSummary, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	it, err := h.repo.GetItem(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errItemNotFound
	}
	if err != nil {
		logger.FromCtx(r.Context()).Error("fetching item", zap.Error(err), zap.String("id", id))
		return nil, err
	}
	return it, nil
}

// ItemDetails is an item in a drawer: its prices over time, per merchant,
// and its purchases.
func (h Handler) ItemDetails(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	it, err := h.item(r)
	if err != nil {
		return nil, err
	}
	return h.itemDetails(r, it)
}

func (h Handler) itemDetails(r *http.Request, it *models.ItemSummary) (templ.Component, error) {
	def := getClaims(r.Context()).DefaultCurrency
	stats, err := h.repo.GetItemStats(r.Context(), it.Item.ID, def)
	if err != nil {
		logger.FromCtx(r.Context()).Error("fetching item stats", zap.Error(err), zap.String("id", it.Item.ID))
		return nil, err
	}
	it.Stats = stats
	return views.ItemDetails(it), nil
}

// itemCategories are the categories an item can be in.
func (h Handler) itemCategories(r *http.Request) ([]models.ItemCategory, error) {
	cats, err := h.repo.GetItemCategories(r.Context())
	if err != nil {
		logger.FromCtx(r.Context()).Error("fetching item categories", zap.Error(err))
	}
	return cats, err
}

func itemToFD(it *models.Item) *views.ItemFD {
	fd := &views.ItemFD{
		Name:    form.FDInput{Value: it.Name},
		Aliases: form.FDInput{Value: strings.Join(it.Aliases, ",")},
	}
	if it.Category != nil {
		fd.CategoryID.Value = it.Category.ID
	}
	return fd
}

// GetEditItem is the item's edit form, in a drawer.
func (h Handler) GetEditItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	it, err := h.item(r)
	if err != nil {
		return nil, err
	}
	cats, err := h.itemCategories(r)
	if err != nil {
		return nil, err
	}
	return views.EditItem(it.Item.ID, itemToFD(&it.Item), cats), nil
}

// EditItem saves the item's name, aliases and category, answering its
// details.
func (h Handler) EditItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	cur, err := h.item(r)
	if err != nil {
		return nil, err
	}
	cats, err := h.itemCategories(r)
	if err != nil {
		return nil, err
	}
	it := &models.Item{
		ID:      cur.Item.ID,
		Name:    r.FormValue("name"),
		Aliases: parseAliases(r.FormValue("aliases")),
	}
	fd := &views.ItemFD{
		Name:       form.FDInput{Value: r.FormValue("name")},
		Aliases:    form.FDInput{Value: r.FormValue("aliases")},
		CategoryID: form.FDInput{Value: r.FormValue("category_id")},
	}
	if id := fd.CategoryID.Value; id != "" {
		i := slices.IndexFunc(cats, func(c models.ItemCategory) bool { return c.ID == id })
		if i < 0 {
			fd.CategoryID.Error = "Pick a category."
			return views.EditItemForm(it.ID, fd, cats), nil
		}
		it.Category = &cats[i]
	}

	if err := it.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if !errors.As(err, &valerr) {
			l.Error("sanitizing item", zap.Error(err))
			return nil, err
		}
		fd.Name.Error = valerr.Errors.MsgFor("Name")
		fd.Aliases.Error = valerr.Errors.MsgFor("Aliases")
		if fd.Name.Error == "" && fd.Aliases.Error == "" {
			fd.Aliases.Error = "Check the aliases (at most 50, each up to 200 characters)."
		}
		return views.EditItemForm(it.ID, fd, cats), nil
	}

	if err := h.repo.UpdateItem(r.Context(), it); err != nil {
		if errors.Is(err, repository.ErrItemTaken) {
			fd.Name.Error = "Another item has this name or one of these aliases: merge them instead."
			return views.EditItemForm(it.ID, fd, cats), nil
		}
		l.Error("updating item", zap.Error(err), zap.String("id", it.ID))
		return nil, err
	}
	l.Info("updated item", zap.String("id", it.ID))

	addHXTrigger(w, views.ItemsChangedEvent)
	return h.answerItemDetails(w, r, it.ID)
}

// answerItemDetails answers the item's details into the drawer, in place of
// the form it was opened from.
func (h Handler) answerItemDetails(w http.ResponseWriter, r *http.Request, id string) (templ.Component, error) {
	it, err := h.repo.GetItem(r.Context(), id)
	if err != nil {
		return nil, err
	}
	w.Header().Set("HX-Retarget", "#"+components.DrawerSlot)
	w.Header().Set("HX-Reswap", "innerHTML")
	return h.itemDetails(r, it)
}

// itemMergeTargets are the items it can be merged into, by name.
func (h Handler) itemMergeTargets(r *http.Request, id string) ([]models.Item, error) {
	def := getClaims(r.Context()).DefaultCurrency
	all, err := h.repo.GetItems(r.Context(), def, models.ItemsRange{})
	if err != nil {
		return nil, err
	}
	out := make([]models.Item, 0, len(all))
	for _, s := range all {
		if s.Item.ID != id {
			out = append(out, s.Item)
		}
	}
	return out, nil
}

// GetMergeItem is the form merging the item into another, in a drawer.
func (h Handler) GetMergeItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	it, err := h.item(r)
	if err != nil {
		return nil, err
	}
	targets, err := h.itemMergeTargets(r, it.Item.ID)
	if err != nil {
		return nil, err
	}
	return views.MergeItem(&it.Item, form.FDInput{}, targets), nil
}

// MergeItem merges the item into the picked one, answering that one's
// details.
func (h Handler) MergeItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	it, err := h.item(r)
	if err != nil {
		return nil, err
	}
	targets, err := h.itemMergeTargets(r, it.Item.ID)
	if err != nil {
		return nil, err
	}
	intoID := r.FormValue("into_id")
	if !slices.ContainsFunc(targets, func(t models.Item) bool { return t.ID == intoID }) {
		return views.MergeItemForm(&it.Item, form.FDInput{Value: intoID, Error: "Select an item to merge into"}, targets), nil
	}
	if _, err := h.repo.MergeItems(r.Context(), it.Item.ID, intoID); err != nil {
		if errors.Is(err, repository.ErrItemTaken) {
			return views.MergeItemForm(&it.Item, form.FDInput{Value: intoID, Error: "Their names clash with another item's: rename one first."}, targets), nil
		}
		return nil, err
	}
	l.Info("merged items", zap.String("from", it.Item.ID), zap.String("into", intoID))
	addHXTrigger(w, views.ItemsChangedEvent)
	return h.answerItemDetails(w, r, intoID)
}

// DeleteItem deletes an item no line is of, closing its details.
func (h Handler) DeleteItem(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	it, err := h.item(r)
	if err != nil {
		return nil, err
	}
	if err := h.repo.DeleteItem(r.Context(), it.Item.ID); err != nil {
		if errors.Is(err, repository.ErrItemInUse) {
			return nil, resperr.New(http.StatusConflict, "It's on transactions: merge it into another item instead.")
		}
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("deleted item", zap.String("id", it.Item.ID))
	addHXTrigger(w, views.ItemsChangedEvent)
	drawerSaved(w, "#"+views.ItemRowID(it.Item.ID), "outerHTML")
	return templ.NopComponent, nil
}

// ItemSuggestions are the items offered while typing a line's name (q),
// the ones bought at the form's counterparty (cp) first.
func (h Handler) ItemSuggestions(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	w.Header().Set("Cache-Control", "no-store")
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		return []byte("[]"), nil
	}
	limit := maxItemSuggestions
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n < limit {
		limit = n
	}
	items, err := h.repo.SearchItems(r.Context(), q, r.URL.Query().Get("cp"), limit)
	if err != nil {
		return nil, err
	}
	return json.Marshal(items)
}

// ItemCategories is the item categories' drawer: each opening its edit
// form, and a new one's.
func (h Handler) ItemCategories(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	cats, err := h.itemCategories(r)
	if err != nil {
		return nil, err
	}
	return views.ItemCategories(cats), nil
}

// itemCategoryOf is the item category of the path.
func (h Handler) itemCategoryOf(r *http.Request) (*models.ItemCategory, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	c, err := h.repo.GetItemCategory(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, resperr.New(http.StatusNotFound, "Category not found")
	}
	return c, err
}

// GetNewItemCategory is a new item category's form, in the drawer.
func (h Handler) GetNewItemCategory(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return views.ItemCategoryDrawer("", &views.ItemCategoryFD{Color: form.FDInput{Value: models.GetRandomCategoryColor()}}), nil
}

// GetEditItemCategory is the item category's edit form, in the drawer.
func (h Handler) GetEditItemCategory(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	c, err := h.itemCategoryOf(r)
	if err != nil {
		return nil, err
	}
	return views.ItemCategoryDrawer(c.ID, &views.ItemCategoryFD{
		Name: form.FDInput{Value: c.Name}, Color: form.FDInput{Value: c.Color},
	}), nil
}

// CreateItemCategory saves a new item category, answering the categories.
func (h Handler) CreateItemCategory(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	return h.saveItemCategory(w, r, &models.ItemCategory{})
}

// EditItemCategory saves the item category, answering the categories.
func (h Handler) EditItemCategory(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	c, err := h.itemCategoryOf(r)
	if err != nil {
		return nil, err
	}
	return h.saveItemCategory(w, r, c)
}

func (h Handler) saveItemCategory(w http.ResponseWriter, r *http.Request, c *models.ItemCategory) (templ.Component, error) {
	l := logger.FromCtx(r.Context())
	c.Name, c.Color = r.FormValue("name"), r.FormValue("color")
	fd := &views.ItemCategoryFD{Name: form.FDInput{Value: c.Name}, Color: form.FDInput{Value: c.Color}}
	if err := c.SanitizeAndValidate(h.sani, h.valid); err != nil {
		var valerr errs.ValidationError
		if !errors.As(err, &valerr) {
			return nil, err
		}
		fd.Name.Error = valerr.Errors.MsgFor("Name")
		return views.ItemCategoryForm(c.ID, fd), nil
	}
	if err := h.repo.SaveItemCategory(r.Context(), c); err != nil {
		if errors.Is(err, repository.ErrItemCategoryTaken) {
			fd.Name.Error = "There's already a category with this name."
			return views.ItemCategoryForm(c.ID, fd), nil
		}
		l.Error("saving item category", zap.Error(err))
		return nil, err
	}
	l.Info("saved item category", zap.String("id", c.ID))
	return h.answerItemCategories(w, r)
}

// DeleteItemCategory deletes the item category (its items keep going,
// without one), answering the categories.
func (h Handler) DeleteItemCategory(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	c, err := h.itemCategoryOf(r)
	if err != nil {
		return nil, err
	}
	if err := h.repo.DeleteItemCategory(r.Context(), c.ID); err != nil {
		return nil, err
	}
	logger.FromCtx(r.Context()).Info("deleted item category", zap.String("id", c.ID))
	return h.answerItemCategories(w, r)
}

// answerItemCategories answers the categories' drawer in place of the form,
// the items page's list re-rendered.
func (h Handler) answerItemCategories(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	addHXTrigger(w, views.ItemsChangedEvent)
	w.Header().Set("HX-Retarget", "#"+components.DrawerSlot)
	w.Header().Set("HX-Reswap", "innerHTML")
	return h.ItemCategories(w, r)
}

// QuickCreateItemCategory answers the item form's category combobox
// (CreateURL): the category named name, created when there's none.
func (h Handler) QuickCreateItemCategory(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return nil, resperr.New(http.StatusBadRequest, "name is required")
	}
	c, err := h.repo.FindItemCategory(r.Context(), name)
	if err != nil {
		return nil, err
	}
	if c == nil {
		c = &models.ItemCategory{Name: name, Color: models.GetRandomCategoryColor()}
		if err := c.SanitizeAndValidate(h.sani, h.valid); err != nil {
			return nil, resperr.New(http.StatusBadRequest, "Name it in at most 60 characters.")
		}
		if err := h.repo.SaveItemCategory(r.Context(), c); err != nil {
			return nil, err
		}
		logger.FromCtx(r.Context()).Info("quick created item category", zap.String("id", c.ID))
	}
	return views.ItemCategoryComboOption(c), nil
}
