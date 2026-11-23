package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// ErrTagTaken is a name another tag has (any case).
var ErrTagTaken = errors.New("another tag has this name")

func dbTagToTag(t queries.Tag, curs map[string]models.Currency) models.Tag {
	tag := models.Tag{
		ID:         t.ID,
		Name:       t.Name,
		Color:      t.Color,
		StartsOn:   dayPtr(t.StartsOn),
		EndsOn:     dayPtr(t.EndsOn),
		AutoTag:    t.AutoTag,
		ArchivedAt: timePtr(t.ArchivedAt),
		Note:       t.Note,
	}
	if t.Budget.Valid {
		b := t.Budget.Float64
		tag.Budget = &b
		if c, ok := curs[t.BudgetCurrencyID.String]; ok {
			tag.BudgetCurrency = &c
		} else if t.BudgetCurrencyID.Valid {
			tag.BudgetCurrency = &models.Currency{ID: t.BudgetCurrencyID.String}
		}
	}
	return tag
}

func timePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// currencyMap is the currencies by id.
func (r *Repository) currencyMap(ctx context.Context) (map[string]models.Currency, error) {
	curs, err := r.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]models.Currency, len(curs))
	for _, c := range curs {
		out[c.ID] = c
	}
	return out, nil
}

// GetTags is every tag with how many transactions carry it: the live ones
// first (trips by their first day, latest first, then by name), archived
// last.
func (r *Repository) GetTags(ctx context.Context) ([]models.Tag, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	curs, err := r.currencyMap(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetTags(ctx)
	if err != nil {
		logger.FromCtx(ctx).Error("querying tags", zap.Error(err))
		return nil, err
	}
	out := make([]models.Tag, len(rows))
	for i, row := range rows {
		out[i] = dbTagToTag(row.Tag, curs)
		out[i].Txs = int(row.Txs)
	}
	return out, nil
}

// GetTag is one tag, with how many transactions carry it.
func (r *Repository) GetTag(ctx context.Context, id string) (*models.Tag, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	curs, err := r.currencyMap(ctx)
	if err != nil {
		return nil, err
	}
	row, err := q.GetTag(ctx, id)
	if err != nil {
		return nil, err
	}
	t := dbTagToTag(row.Tag, curs)
	t.Txs = int(row.Txs)
	return &t, nil
}

func tagParams(t *models.Tag) (sql.NullFloat64, sql.NullString) {
	var budget sql.NullFloat64
	var cur sql.NullString
	if t.Budget != nil {
		budget = sql.NullFloat64{Float64: *t.Budget, Valid: true}
		if t.BudgetCurrency != nil && t.BudgetCurrency.ID != "" {
			cur = sql.NullString{String: t.BudgetCurrency.ID, Valid: true}
		}
	}
	return budget, cur
}

// SaveTag creates the tag (no ID yet) or updates it; its name can't be
// another one's.
func (r *Repository) SaveTag(ctx context.Context, t *models.Tag) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		other, err := qtx.GetTagByName(ctx, t.Name)
		if err == nil && other.ID != t.ID {
			return ErrTagTaken
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		budget, cur := tagParams(t)
		if t.ID == "" {
			t.ID = idgen.New()
			return qtx.CreateTag(ctx, queries.CreateTagParams{
				ID: t.ID, Name: t.Name, Color: t.Color,
				StartsOn: nullDay(t.StartsOn), EndsOn: nullDay(t.EndsOn), AutoTag: t.AutoTag,
				Budget: budget, BudgetCurrencyID: cur, Note: t.Note,
			})
		}
		return qtx.UpdateTag(ctx, queries.UpdateTagParams{
			ID: t.ID, Name: t.Name, Color: t.Color,
			StartsOn: nullDay(t.StartsOn), EndsOn: nullDay(t.EndsOn), AutoTag: t.AutoTag,
			Budget: budget, BudgetCurrencyID: cur, Note: t.Note,
		})
	})
}

// SetTagArchived archives the tag (out of the pickers, kept on its
// transactions) or brings it back.
func (r *Repository) SetTagArchived(ctx context.Context, id string, archived bool) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.SetTagArchived(ctx, queries.SetTagArchivedParams{
		ID:         id,
		ArchivedAt: sql.NullTime{Time: time.Now(), Valid: archived},
	})
}

// DeleteTag deletes the tag; its transactions stay, untagged.
func (r *Repository) DeleteTag(ctx context.Context, id string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.DeleteTag(ctx, id)
}

// findOrCreateTagWithTxn is the tag named name (any case), created (a random
// color) when none.
func findOrCreateTagWithTxn(ctx context.Context, qtx *queries.Queries, name string) (models.Tag, error) {
	name = strings.TrimSpace(name)
	t, err := qtx.GetTagByName(ctx, name)
	if err == nil {
		return dbTagToTag(t, nil), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return models.Tag{}, err
	}
	tag := models.Tag{ID: idgen.New(), Name: name, Color: models.GetRandomCategoryColor()}
	if err := qtx.CreateTag(ctx, queries.CreateTagParams{ID: tag.ID, Name: tag.Name, Color: tag.Color}); err != nil {
		return models.Tag{}, err
	}
	return tag, nil
}

// writeTxTagsWithTxn replaces a saved transaction's tags with tx.Tags (nil
// leaves them as they are): by id, else by name, created when new.
func writeTxTagsWithTxn(ctx context.Context, qtx *queries.Queries, tx *models.Transaction, created bool) error {
	if tx.Tags == nil {
		return nil
	}
	if !created {
		if err := qtx.DeleteTransactionTags(ctx, tx.ID); err != nil {
			return err
		}
	}
	for i := range tx.Tags {
		if tx.Tags[i].ID == "" {
			t, err := findOrCreateTagWithTxn(ctx, qtx, tx.Tags[i].Name)
			if err != nil {
				logger.FromCtx(ctx).Error("finding a tag", zap.Error(err), zap.String("transaction_id", tx.ID))
				return err
			}
			tx.Tags[i] = t
		}
		if err := qtx.AddTransactionTag(ctx, queries.AddTransactionTagParams{TransactionID: tx.ID, TagID: tx.Tags[i].ID}); err != nil {
			logger.FromCtx(ctx).Error("tagging a transaction", zap.Error(err), zap.String("transaction_id", tx.ID))
			return err
		}
	}
	return nil
}

// autoTagWithTxn adds the auto tagging trips covering a new transaction's day
// to its tags (see models.Transaction.AutoTaggable).
func autoTagWithTxn(ctx context.Context, qtx *queries.Queries, tx *models.Transaction) error {
	if !tx.AutoTaggable() {
		return nil
	}
	rows, err := qtx.GetAutoTags(ctx)
	if err != nil {
		logger.FromCtx(ctx).Error("querying auto tags", zap.Error(err))
		return err
	}
	for _, row := range rows {
		t := dbTagToTag(row, nil)
		if !t.AutoTags(tx.ScheduledTo) || models.HasTag(tx.Tags, t.ID) {
			continue
		}
		if err := qtx.AddTransactionTag(ctx, queries.AddTransactionTagParams{TransactionID: tx.ID, TagID: t.ID}); err != nil {
			logger.FromCtx(ctx).Error("auto tagging a transaction", zap.Error(err), zap.String("transaction_id", tx.ID))
			return err
		}
		tx.Tags = append(tx.Tags, t)
	}
	return nil
}

// loadTxTags sets each transaction's tags.
func loadTxTags(ctx context.Context, q *queries.Queries, lists ...[]models.Transaction) error {
	var ids []string
	type ref struct{ list, i int }
	at := map[string]ref{}
	for li, txs := range lists {
		for i := range txs {
			ids = append(ids, txs[i].ID)
			at[txs[i].ID] = ref{li, i}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.GetTagsOfTxs(ctx, ids)
	if err != nil {
		logger.FromCtx(ctx).Error("querying the transactions' tags", zap.Error(err))
		return err
	}
	for _, row := range rows {
		if r, ok := at[row.TransactionID]; ok {
			tx := &lists[r.list][r.i]
			tx.Tags = append(tx.Tags, dbTagToTag(row.Tag, nil))
		}
	}
	return nil
}

// SetTransactionTags replaces the transaction's tags (by id, else by name,
// created when new).
func (r *Repository) SetTransactionTags(ctx context.Context, txID string, tags []models.Tag) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		tx := &models.Transaction{ID: txID, Tags: tags}
		if tx.Tags == nil {
			tx.Tags = []models.Tag{}
		}
		return writeTxTagsWithTxn(ctx, qtx, tx, false)
	})
}

// TagRangeCandidates are the transactions a trip can tag at once (see
// GetTagRangeCandidates): their ids and what they moved per currency.
type TagRangeCandidates struct {
	IDs []string
	// Totals are what they moved (signed as the balances) per currency.
	Totals []models.CurrencyAmount
}

// GetTagRangeCandidates are the transactions dated in the trip's days it can
// tag at once.
func (r *Repository) GetTagRangeCandidates(ctx context.Context, t *models.Tag) (*TagRangeCandidates, error) {
	out := &TagRangeCandidates{}
	if !t.IsTrip() {
		return out, nil
	}
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetTagRangeCandidates(ctx, queries.GetTagRangeCandidatesParams{
		FromDay: t.StartsOn.Format(time.DateOnly),
		ToDay:   t.EndsOn.Format(time.DateOnly),
		TagID:   t.ID,
	})
	if err != nil {
		logger.FromCtx(ctx).Error("querying a trip's transactions", zap.Error(err), zap.String("tag_id", t.ID))
		return nil, err
	}
	at := map[string]int{}
	for _, row := range rows {
		out.IDs = append(out.IDs, row.ID)
		i, ok := at[row.Currency.ID]
		if !ok {
			i = len(out.Totals)
			at[row.Currency.ID] = i
			out.Totals = append(out.Totals, models.CurrencyAmount{Currency: *dbCurrencyToCurrency(&row.Currency, false)})
		}
		out.Totals[i].Amount += row.Amount
	}
	return out, nil
}

// TagTransactions tags the transactions with the tag, the ones already
// tagged left alone; how many it tagged.
func (r *Repository) TagTransactions(ctx context.Context, tagID string, ids []string) (int, error) {
	err := r.transact(ctx, func(qtx *queries.Queries) error {
		for _, id := range ids {
			if err := qtx.AddTransactionTag(ctx, queries.AddTransactionTagParams{TransactionID: id, TagID: tagID}); err != nil {
				logger.FromCtx(ctx).Error("tagging a transaction", zap.Error(err), zap.String("tag_id", tagID))
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

// GetTagSpending is what the tagged fulfilled expenses spent, per tag, day,
// category and currency (analysis rule); every tag's when tagID is empty.
func (r *Repository) GetTagSpending(ctx context.Context, tagID string) ([]models.TagSpend, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	var arg any
	if tagID != "" {
		arg = tagID
	}
	rows, err := q.GetTagSpending(ctx, arg)
	if err != nil {
		logger.FromCtx(ctx).Error("querying tags' spending", zap.Error(err))
		return nil, err
	}
	out := make([]models.TagSpend, len(rows))
	for i, row := range rows {
		out[i] = models.TagSpend{
			TagID:      row.TagID,
			Day:        row.Day,
			CategoryID: row.CategoryID,
			Currency:   *dbCurrencyToCurrency(&row.Currency, false),
			Amount:     row.Total,
		}
	}
	return out, nil
}
