package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"github.com/omareloui/tyche/internal/sharedqueries"
)

// ColBasketMonths is how many of the user's months the cost of living
// basket is read from.
const ColBasketMonths = 12

// ColBasket is the user's spending per expense category, a month on average.
type ColBasket struct {
	Categories []models.ColCategory
	// Months are the months with spending the average is over (≤
	// ColBasketMonths).
	Months int
	// Missing tells some spending had no exchange rate to home's currency
	// and was left out.
	Missing bool
}

// GetColBasket is the user's monthly spending per expense category over
// their last full months (ColBasketMonths; the current one too when they
// have no full month yet), in home's currency, each with its price group:
// the one picked, else the one its name suggests.
func (r *Repository) GetColBasket(ctx context.Context, home string, now time.Time) (*ColBasket, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	ms := monthStartFromCtx(ctx)
	cur := ms.Of(now)
	from := ms.Plus(now, -ColBasketMonths).From
	rows, err := q.GetDailyCategorySpending(ctx, queries.GetDailyCategorySpendingParams{FromDate: from, ToDate: cur.From})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		if rows, err = q.GetDailyCategorySpending(ctx, queries.GetDailyCategorySpendingParams{FromDate: cur.From, ToDate: cur.To}); err != nil {
			return nil, err
		}
	}

	b := &ColBasket{}
	totals := map[string]float64{}
	months := map[time.Time]bool{}
	for _, row := range rows {
		c := dbCurrencyToCurrency(&row.Currency, false)
		v, ok := models.Convert(row.Total, c, home)
		if !ok {
			b.Missing = true
			continue
		}
		totals[row.CategoryID] += v
		if day, err := time.Parse(time.DateOnly, row.Day); err == nil {
			months[ms.Of(day).Month] = true
		}
	}
	b.Months = max(len(months), 1)

	cats, err := r.GetCategories(ctx)
	if err != nil {
		return nil, err
	}
	picked, err := r.getCategoryColGroups(ctx, q)
	if err != nil {
		return nil, err
	}

	for _, c := range cats {
		total := totals[c.ID]
		if total <= 0 && !c.HasKind(models.ExpenseTxKind) {
			continue
		}
		cc := models.ColCategory{ID: c.ID, Name: c.Name, Color: c.Color, Monthly: max(total, 0) / float64(b.Months)}
		if g, ok := picked[c.ID]; ok {
			cc.Group = g
		} else {
			cc.Group, cc.Guessed = models.GuessColGroup(c.Name), true
		}
		b.Categories = append(b.Categories, cc)
	}
	if v := totals[""]; v > 0 {
		b.Categories = append(b.Categories, models.ColCategory{
			ID: models.UncategorizedKey, Name: "Uncategorized", Group: models.ColAverage, Monthly: v / float64(b.Months),
		})
	}
	slices.SortStableFunc(b.Categories, func(a, b models.ColCategory) int {
		switch {
		case a.Monthly > b.Monthly:
			return -1
		case a.Monthly < b.Monthly:
			return 1
		}
		return 0
	})
	return b, nil
}

func (r *Repository) getCategoryColGroups(ctx context.Context, q *queries.Queries) (map[string]models.ColGroup, error) {
	rows, err := q.GetCategoryColGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]models.ColGroup, len(rows))
	for _, row := range rows {
		if g := models.ColGroup(row.ColGroup); models.ValidColGroup(g) {
			out[row.CategoryID] = g
		}
	}
	return out, nil
}

// SetCategoryColGroup picks the price group of a category ("" goes back to
// the guess).
func (r *Repository) SetCategoryColGroup(ctx context.Context, categoryID string, g models.ColGroup) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	if g == "" {
		return q.DeleteCategoryColGroup(ctx, categoryID)
	}
	return q.SetCategoryColGroup(ctx, queries.SetCategoryColGroupParams{CategoryID: categoryID, ColGroup: string(g)})
}

// GetColCityEstimate is the estimate kept under key, nil when none.
func (r *Repository) GetColCityEstimate(ctx context.Context, key string) (*models.ColCityEstimate, error) {
	row, err := r.shared.GetColCityEstimate(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var e models.ColCityEstimate
	if err := json.Unmarshal([]byte(row.Estimate), &e); err != nil {
		return nil, nil
	}
	e.Model = row.Model
	return &e, nil
}

// SetColCityEstimate keeps an estimate under key.
func (r *Repository) SetColCityEstimate(ctx context.Context, key string, e *models.ColCityEstimate) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return r.shared.SetColCityEstimate(ctx, sharedqueries.SetColCityEstimateParams{
		Key: key, Estimate: string(b), Model: e.Model, CreatedAt: time.Now().UTC(),
	})
}
