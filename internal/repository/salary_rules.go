package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/payroll"
	"github.com/omareloui/tyche/internal/queries"
	"github.com/omareloui/tyche/internal/sharedqueries"
)

func toSalaryRule(row queries.SalaryRule) (models.SalaryRule, error) {
	s := models.SalaryRule{ID: row.ID, Name: row.Name, Country: row.Country}
	if err := json.Unmarshal([]byte(row.Rules), &s.Rules); err != nil {
		return s, err
	}
	s.Rules.Source = payroll.SourceManual
	s.Rules.Country = row.Country
	return s, nil
}

// GetSalaryRules are the user's own tax rules, by name (unreadable ones left
// out).
func (r *Repository) GetSalaryRules(ctx context.Context) ([]models.SalaryRule, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetSalaryRules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.SalaryRule, 0, len(rows))
	for _, row := range rows {
		if s, err := toSalaryRule(row); err == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

// GetSalaryRule is one of the user's tax rules, nil when there's none.
func (r *Repository) GetSalaryRule(ctx context.Context, id string) (*models.SalaryRule, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := q.GetSalaryRule(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s, err := toSalaryRule(row)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// SaveSalaryRule creates (empty ID, set on it) or updates the user's tax
// rules.
func (r *Repository) SaveSalaryRule(ctx context.Context, s *models.SalaryRule) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	s.Rules.Source = payroll.SourceManual
	s.Rules.Country = s.Country
	b, err := json.Marshal(s.Rules)
	if err != nil {
		return err
	}
	if s.ID == "" {
		s.ID = idgen.New()
		return q.CreateSalaryRule(ctx, queries.CreateSalaryRuleParams{ID: s.ID, Name: s.Name, Country: s.Country, Rules: string(b)})
	}
	return q.UpdateSalaryRule(ctx, queries.UpdateSalaryRuleParams{ID: s.ID, Name: s.Name, Country: s.Country, Rules: string(b)})
}

// DeleteSalaryRule deletes one of the user's tax rules.
func (r *Repository) DeleteSalaryRule(ctx context.Context, id string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.DeleteSalaryRule(ctx, id)
}

// GetTaxRules are the AI's rules kept under key, nil when there are none.
func (r *Repository) GetTaxRules(ctx context.Context, key string) (*payroll.Rules, error) {
	row, err := r.shared.GetTaxRules(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rules payroll.Rules
	if err := json.Unmarshal([]byte(row.Rules), &rules); err != nil {
		return nil, nil
	}
	rules.Source = payroll.SourceAI
	if rules.FetchedAt.IsZero() {
		rules.FetchedAt = row.CreatedAt
	}
	return &rules, nil
}

// SetTaxRules keeps the AI's rules under key, replacing any.
func (r *Repository) SetTaxRules(ctx context.Context, key, model string, rules *payroll.Rules) error {
	now := time.Now().UTC()
	rules.FetchedAt = now
	b, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	return r.shared.SetTaxRules(ctx, sharedqueries.SetTaxRulesParams{Key: key, Rules: string(b), Model: model, CreatedAt: now})
}
