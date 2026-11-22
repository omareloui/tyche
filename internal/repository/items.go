package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

var (
	// ErrItemTaken is a name or alias already another item's.
	ErrItemTaken = errors.New("another item has this name")
	// ErrItemInUse is an item still on lines, merged rather than deleted.
	ErrItemInUse = errors.New("the item is on transactions")
	// ErrItemSelfMerge is an item merged into itself.
	ErrItemSelfMerge = errors.New("an item can't be merged into itself")
	// ErrItemCategoryTaken is a name another item category has.
	ErrItemCategoryTaken = errors.New("another item category has this name")
)

// GetItemCategories is every item category, by name, with its items count.
func (r *Repository) GetItemCategories(ctx context.Context) ([]models.ItemCategory, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetItemCategories(ctx)
	if err != nil {
		logger.FromCtx(ctx).Error("querying item categories", zap.Error(err))
		return nil, err
	}
	out := make([]models.ItemCategory, len(rows))
	for i, row := range rows {
		c := row.ItemCategory
		out[i] = models.ItemCategory{ID: c.ID, Name: c.Name, Color: c.Color, Items: int(row.Items)}
	}
	return out, nil
}

// GetItemCategory is one item category.
func (r *Repository) GetItemCategory(ctx context.Context, id string) (*models.ItemCategory, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	c, err := q.GetItemCategory(ctx, id)
	if err != nil {
		return nil, err
	}
	return &models.ItemCategory{ID: c.ID, Name: c.Name, Color: c.Color}, nil
}

// FindItemCategory is the item category named name (any case), nil when
// none.
func (r *Repository) FindItemCategory(ctx context.Context, name string) (*models.ItemCategory, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	c, err := q.GetItemCategoryByName(ctx, strings.TrimSpace(name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &models.ItemCategory{ID: c.ID, Name: c.Name, Color: c.Color}, nil
}

// SaveItemCategory creates the item category (no ID yet) or updates it; its
// name can't be another one's.
func (r *Repository) SaveItemCategory(ctx context.Context, c *models.ItemCategory) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		other, err := qtx.GetItemCategoryByName(ctx, c.Name)
		if err == nil && other.ID != c.ID {
			return ErrItemCategoryTaken
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if c.ID == "" {
			c.ID = idgen.New()
			return qtx.CreateItemCategory(ctx, queries.CreateItemCategoryParams{ID: c.ID, Name: c.Name, Color: c.Color})
		}
		return qtx.UpdateItemCategory(ctx, queries.UpdateItemCategoryParams{ID: c.ID, Name: c.Name, Color: c.Color})
	})
}

// DeleteItemCategory deletes the item category, its items left without one.
func (r *Repository) DeleteItemCategory(ctx context.Context, id string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.DeleteItemCategory(ctx, id)
}

func dbItemToItem(it queries.Item, catName, catColor sql.NullString) models.Item {
	aliases, _ := unmarshalStrings(it.Aliases)
	item := models.Item{ID: it.ID, Name: it.Name, Aliases: aliases, CreatedAt: it.CreatedAt}
	if it.CategoryID.Valid {
		item.Category = &models.ItemCategory{ID: it.CategoryID.String, Name: catName.String, Color: catColor.String}
	}
	return item
}

// GetItems is every item with its stats of the range (every day when zero),
// by name.
func (r *Repository) GetItems(ctx context.Context, def models.Currency, rng models.ItemsRange) ([]models.ItemSummary, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetItems(ctx)
	if err != nil {
		logger.FromCtx(ctx).Error("querying items", zap.Error(err))
		return nil, err
	}
	items := make([]models.ItemSummary, len(rows))
	for i, row := range rows {
		items[i] = models.ItemSummary{
			Item:      dbItemToItem(row.Item, row.CategoryName, row.CategoryColor),
			Merchants: int(row.Merchants),
		}
	}
	lines, err := r.itemLines(ctx, q, "", rng)
	if err != nil {
		return nil, err
	}
	return models.NewItemSummaries(items, lines, def), nil
}

// GetItem is one item, without its stats.
func (r *Repository) GetItem(ctx context.Context, id string) (*models.ItemSummary, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := q.GetItem(ctx, id)
	if err != nil {
		return nil, err
	}
	return &models.ItemSummary{
		Item:      dbItemToItem(row.Item, row.CategoryName, row.CategoryColor),
		Merchants: int(row.Merchants),
	}, nil
}

// GetItemStats is what was bought of the item, in def at today's rates.
func (r *Repository) GetItemStats(ctx context.Context, id string, def models.Currency) (*models.ItemStats, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	lines, err := r.itemLines(ctx, q, id, models.ItemsRange{})
	if err != nil {
		return nil, err
	}
	return models.NewItemStats(lines, def), nil
}

// itemLines are the item lines of fulfilled expenses, of the item or every
// one (""), dated in the range.
func (r *Repository) itemLines(ctx context.Context, q *queries.Queries, itemID string, rng models.ItemsRange) ([]models.ItemLine, error) {
	p := queries.GetItemLinesParams{}
	if itemID != "" {
		p.ItemID = itemID
	}
	if !rng.From.IsZero() {
		p.FromDate = rng.From
	}
	if !rng.To.IsZero() {
		p.ToDate = rng.To
	}
	rows, err := q.GetItemLines(ctx, p)
	if err != nil {
		logger.FromCtx(ctx).Error("querying item lines", zap.Error(err))
		return nil, err
	}
	lines := make([]models.ItemLine, len(rows))
	for i, row := range rows {
		cur := dbCurrencyToCurrency(&row.Currency, false)
		lines[i] = models.ItemLine{
			ItemID: row.ItemID.String, TxID: row.TransactionID, Day: row.ScheduledTo,
			Quantity: row.Quantity, Total: row.Total,
			Foreign:    row.LinesCurrencyID.Valid && row.LinesCurrencyID.String != row.Currency.ID,
			LinesTotal: row.LinesTotal, Amount: row.Amount, Currency: *cur,
		}
		if row.CounterpartyID.Valid {
			lines[i].Merchant = &models.Counterparty{
				ID: row.CounterpartyID.String, Name: row.CounterpartyName.String,
				Icon: row.CounterpartyIcon.String, Color: row.CounterpartyColor.String,
			}
		}
	}
	return lines, nil
}

// checkItemKeysFree refuses keys another item than id has as its name or an
// alias.
func checkItemKeysFree(ctx context.Context, qtx *queries.Queries, id string, keys []string) error {
	rows, err := qtx.GetItemCandidates(ctx, sql.NullString{})
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ID == id {
			continue
		}
		aliases, _ := unmarshalStrings(row.Aliases)
		other := models.Item{Name: row.NameKey, Aliases: aliases}
		for _, k := range other.Keys() {
			for _, mine := range keys {
				if k == mine {
					return ErrItemTaken
				}
			}
		}
	}
	return nil
}

// UpdateItem saves the item's name, aliases and category; none of them may
// be another item's name or alias.
func (r *Repository) UpdateItem(ctx context.Context, it *models.Item) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		return updateItemWithTxn(ctx, qtx, it)
	})
}

func updateItemWithTxn(ctx context.Context, qtx *queries.Queries, it *models.Item) error {
	it.CleanAliases()
	if err := checkItemKeysFree(ctx, qtx, it.ID, it.Keys()); err != nil {
		return err
	}
	aliases, err := marshalStrings(it.Aliases)
	if err != nil {
		return err
	}
	var cat sql.NullString
	if it.Category != nil && it.Category.ID != "" {
		cat = sql.NullString{String: it.Category.ID, Valid: true}
	}
	if err := qtx.UpdateItem(ctx, queries.UpdateItemParams{
		ID: it.ID, Name: strings.TrimSpace(it.Name), NameKey: it.Key(), Aliases: aliases, CategoryID: cat,
	}); err != nil {
		logger.FromCtx(ctx).Error("updating item", zap.Error(err), zap.String("id", it.ID))
		return err
	}
	return nil
}

// CreateItem adds an item to the catalog (lines create theirs when saved,
// this is for tests and imports).
func (r *Repository) CreateItem(ctx context.Context, it *models.Item) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		it.CleanAliases()
		if it.ID == "" {
			it.ID = idgen.New()
		}
		if err := checkItemKeysFree(ctx, qtx, it.ID, it.Keys()); err != nil {
			return err
		}
		aliases, err := marshalStrings(it.Aliases)
		if err != nil {
			return err
		}
		var cat sql.NullString
		if it.Category != nil && it.Category.ID != "" {
			cat = sql.NullString{String: it.Category.ID, Valid: true}
		}
		return qtx.CreateItem(ctx, queries.CreateItemParams{
			ID: it.ID, Name: strings.TrimSpace(it.Name), NameKey: it.Key(), Aliases: aliases, CategoryID: cat,
		})
	})
}

// DeleteItem deletes an item no line is of.
func (r *Repository) DeleteItem(ctx context.Context, id string) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		n, err := qtx.CountItemLines(ctx, sql.NullString{String: id, Valid: true})
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrItemInUse
		}
		return qtx.DeleteItem(ctx, id)
	})
}

// MergeItems makes fromID's lines and merchants intoID's, adds its name and
// aliases to intoID's aliases and deletes it.
func (r *Repository) MergeItems(ctx context.Context, fromID, intoID string) (*models.Item, error) {
	if fromID == intoID {
		return nil, ErrItemSelfMerge
	}
	var into models.Item
	err := r.transact(ctx, func(qtx *queries.Queries) error {
		from, err := qtx.GetItem(ctx, fromID)
		if err != nil {
			return err
		}
		row, err := qtx.GetItem(ctx, intoID)
		if err != nil {
			return err
		}
		into = dbItemToItem(row.Item, row.CategoryName, row.CategoryColor)
		fromItem := dbItemToItem(from.Item, from.CategoryName, from.CategoryColor)
		into.Absorb(&fromItem)

		if err := qtx.MoveItemLines(ctx, queries.MoveItemLinesParams{
			IntoID: sql.NullString{String: intoID, Valid: true}, FromID: sql.NullString{String: fromID, Valid: true},
		}); err != nil {
			return err
		}
		if err := qtx.MoveItemMerchants(ctx, queries.MoveItemMerchantsParams{IntoID: intoID, FromID: fromID}); err != nil {
			return err
		}
		if err := qtx.DeleteItem(ctx, fromID); err != nil {
			return err
		}
		return updateItemWithTxn(ctx, qtx, &into)
	})
	if err != nil {
		logger.FromCtx(ctx).Error("merging items", zap.Error(err), zap.String("from", fromID), zap.String("into", intoID))
		return nil, err
	}
	return &into, nil
}

// ItemSuggestion is an item offered while typing a line's name.
type ItemSuggestion struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	AtMerchant bool    `json:"at_merchant"`
	LastPrice  float64 `json:"last_price"`
	Currency   string  `json:"currency"`
}

// SearchItems are the items whose name or an alias holds q, the ones bought
// at the counterparty (cpID, "" for none) first.
func (r *Repository) SearchItems(ctx context.Context, query, cpID string, limit int) ([]ItemSuggestion, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.SearchItems(ctx, queries.SearchItemsParams{
		Q:              strings.ToLower(strings.TrimSpace(query)),
		CounterpartyID: sql.NullString{String: cpID, Valid: cpID != ""},
		Lim:            int64(limit),
	})
	if err != nil {
		logger.FromCtx(ctx).Error("searching items", zap.Error(err))
		return nil, err
	}
	out := make([]ItemSuggestion, len(rows))
	for i, row := range rows {
		out[i] = ItemSuggestion{ID: row.ID, Name: row.Name, AtMerchant: row.AtMerchant, LastPrice: row.LastPrice, Currency: row.LastCurrency}
	}
	return out, nil
}

// MatchItems are the items the names would be linked to when saved (none
// created), "" for a name matching none: a scanned receipt's lines.
func (r *Repository) MatchItems(ctx context.Context, cpID string, names []string) ([]string, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetItemCandidates(ctx, sql.NullString{String: cpID, Valid: cpID != ""})
	if err != nil {
		return nil, err
	}
	cands := make([]models.ItemCandidate, len(rows))
	for i, row := range rows {
		aliases, _ := unmarshalStrings(row.Aliases)
		cands[i] = models.ItemCandidate{ID: row.ID, NameKey: row.NameKey, Aliases: aliases, AtMerchant: row.AtMerchant, CreatedAt: row.CreatedAt}
	}
	ids := make([]string, len(names))
	for i, n := range names {
		if c := models.MatchItem(n, cands); c != nil {
			ids[i] = c.ID
		}
	}
	return ids, nil
}
