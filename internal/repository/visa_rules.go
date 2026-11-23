package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"github.com/omareloui/tyche/internal/sharedqueries"
	"github.com/omareloui/tyche/internal/visa"
)

func toVisaRule(row queries.VisaRule) (models.VisaRule, error) {
	v := models.VisaRule{ID: row.ID, Name: row.Name, Country: row.Country}
	if err := json.Unmarshal([]byte(row.Rules), &v.Rule); err != nil {
		return v, err
	}
	v.Rule.Source = visa.SourceManual
	v.Rule.Country = row.Country
	v.Rule.Name = row.Name
	return v, nil
}

// GetVisaRules are the user's own visa requirements, by name (unreadable
// ones left out).
func (r *Repository) GetVisaRules(ctx context.Context) ([]models.VisaRule, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetVisaRules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.VisaRule, 0, len(rows))
	for _, row := range rows {
		if v, err := toVisaRule(row); err == nil {
			out = append(out, v)
		}
	}
	return out, nil
}

// GetVisaRule is one of the user's visa requirements, nil when there's
// none.
func (r *Repository) GetVisaRule(ctx context.Context, id string) (*models.VisaRule, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := q.GetVisaRule(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v, err := toVisaRule(row)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// SaveVisaRule creates (empty ID, set on it) or updates the user's visa
// requirements.
func (r *Repository) SaveVisaRule(ctx context.Context, v *models.VisaRule) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	v.Rule.Source = visa.SourceManual
	v.Rule.Country = v.Country
	v.Rule.Name = v.Name
	b, err := json.Marshal(v.Rule)
	if err != nil {
		return err
	}
	if v.ID == "" {
		v.ID = idgen.New()
		return q.CreateVisaRule(ctx, queries.CreateVisaRuleParams{ID: v.ID, Name: v.Name, Country: v.Country, Rules: string(b)})
	}
	return q.UpdateVisaRule(ctx, queries.UpdateVisaRuleParams{ID: v.ID, Name: v.Name, Country: v.Country, Rules: string(b)})
}

// DeleteVisaRule deletes one of the user's visa requirements.
func (r *Repository) DeleteVisaRule(ctx context.Context, id string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.DeleteVisaRule(ctx, id)
}

// GetVisaRulesCache are the AI's visa requirements kept under key, nil
// when there are none.
func (r *Repository) GetVisaRulesCache(ctx context.Context, key string) (*visa.Rule, error) {
	row, err := r.shared.GetVisaRules(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rule visa.Rule
	if err := json.Unmarshal([]byte(row.Rules), &rule); err != nil {
		return nil, nil
	}
	rule.Source = visa.SourceAI
	if rule.FetchedAt.IsZero() {
		rule.FetchedAt = row.CreatedAt
	}
	return &rule, nil
}

// SetVisaRulesCache keeps the AI's visa requirements under each key,
// replacing any.
func (r *Repository) SetVisaRulesCache(ctx context.Context, keys []string, model string, rule *visa.Rule) error {
	now := time.Now().UTC()
	rule.FetchedAt = now
	b, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := r.shared.SetVisaRules(ctx, sharedqueries.SetVisaRulesParams{Key: key, Rules: string(b), Model: model, CreatedAt: now}); err != nil {
			return err
		}
	}
	return nil
}
