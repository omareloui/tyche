// Package smstemplates keeps the shared sms templates (in the shared
// database) in memory for every user, and converts the stored templates of
// both databases.
package smstemplates

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sharedqueries"
	"github.com/omareloui/tyche/internal/smsparser"
	"go.uber.org/zap"
)

const seedVersionKey = "sms_templates_seed_version"

// Store holds the shared templates, writes go to the database and reload them.
type Store struct {
	q       *sharedqueries.Queries
	mu      sync.Mutex
	current atomic.Pointer[[]models.SMSTemplate]
}

func New(q *sharedqueries.Queries) *Store {
	s := &Store{q: q}
	s.current.Store(&[]models.SMSTemplate{})
	return s
}

// Seed adds the templates of smsparser.Seeds missing by seed key when the
// seeds are newer than the database's. Edited seeded templates are kept, the
// deleted ones come back only on a new seed version. A template written by an
// admin with the body of a new seed becomes that seed instead of a duplicate.
func (s *Store) Seed(ctx context.Context) error {
	l := logger.FromCtx(ctx)

	version := 0
	v, err := s.q.GetMeta(ctx, seedVersionKey)
	switch {
	case err == nil:
		version, _ = strconv.Atoi(v)
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("getting the sms templates seed version: %w", err)
	}
	if version >= smsparser.SeedVersion {
		return nil
	}

	for _, t := range smsparser.Seeds() {
		adopted, err := s.q.AdoptSMSTemplateSeed(ctx, sharedqueries.AdoptSMSTemplateSeedParams{
			SeedKey: sql.NullString{String: t.SeedKey, Valid: true}, Body: t.Body,
		})
		if err != nil {
			return fmt.Errorf("adopting sms template %q: %w", t.SeedKey, err)
		}
		if adopted > 0 {
			continue
		}

		t.ID = idgen.New()
		p := ParamsOf(&t)
		if err := s.q.CreateSMSTemplate(ctx, sharedqueries.CreateSMSTemplateParams{
			ID: p.ID, Name: p.Name, Bank: p.Bank, Senders: p.Senders, Action: p.Action, TxKind: p.TxKind,
			Body: p.Body, Fixed: p.Fixed, Sample: p.Sample, Priority: p.Priority, IsEnabled: p.IsEnabled,
			SeedKey: sql.NullString{String: t.SeedKey, Valid: t.SeedKey != ""},
		}); err != nil {
			return fmt.Errorf("seeding sms template %q: %w", t.SeedKey, err)
		}
	}
	if err := s.q.SetMeta(ctx, sharedqueries.SetMetaParams{Key: seedVersionKey, Value: strconv.Itoa(smsparser.SeedVersion)}); err != nil {
		return fmt.Errorf("setting the sms templates seed version: %w", err)
	}
	l.Info("seeded the sms templates", zap.Int("from_version", version), zap.Int("to_version", smsparser.SeedVersion))
	return nil
}

// Load reads the templates from the database into memory.
func (s *Store) Load(ctx context.Context) error {
	rows, err := s.q.ListSMSTemplates(ctx)
	if err != nil {
		return fmt.Errorf("listing the shared sms templates: %w", err)
	}
	ts := make([]models.SMSTemplate, len(rows))
	for i, r := range rows {
		ts[i] = sharedModel(r.SmsTemplate)
	}
	s.current.Store(&ts)
	return nil
}

// List returns the in-memory templates, it mustn't be modified.
func (s *Store) List() []models.SMSTemplate {
	return *s.current.Load()
}

func (s *Store) Get(id string) (*models.SMSTemplate, error) {
	for _, t := range s.List() {
		if t.ID == id {
			return &t, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (s *Store) Create(ctx context.Context, t *models.SMSTemplate) error {
	return s.write(ctx, func() error {
		p := ParamsOf(t)
		return s.q.CreateSMSTemplate(ctx, sharedqueries.CreateSMSTemplateParams{
			ID: p.ID, Name: p.Name, Bank: p.Bank, Senders: p.Senders, Action: p.Action, TxKind: p.TxKind,
			Body: p.Body, Fixed: p.Fixed, Sample: p.Sample, Priority: p.Priority, IsEnabled: p.IsEnabled,
		})
	})
}

func (s *Store) Update(ctx context.Context, t *models.SMSTemplate) error {
	return s.write(ctx, func() error {
		p := ParamsOf(t)
		return s.q.UpdateSMSTemplate(ctx, sharedqueries.UpdateSMSTemplateParams{
			ID: p.ID, Name: p.Name, Bank: p.Bank, Senders: p.Senders, Action: p.Action, TxKind: p.TxKind,
			Body: p.Body, Fixed: p.Fixed, Sample: p.Sample, Priority: p.Priority, IsEnabled: p.IsEnabled,
		})
	})
}

func (s *Store) Delete(ctx context.Context, id string) error {
	return s.write(ctx, func() error {
		return s.q.DeleteSMSTemplate(ctx, id)
	})
}

// write runs the change and reloads the templates, one at a time so a
// reload never misses a concurrent change.
func (s *Store) write(ctx context.Context, change func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := change(); err != nil {
		return err
	}
	return s.Load(ctx)
}

func sharedModel(r sharedqueries.SmsTemplate) models.SMSTemplate {
	t := Row{
		ID: r.ID, Name: r.Name, Bank: r.Bank, Senders: r.Senders, Action: r.Action, TxKind: r.TxKind,
		Body: r.Body, Fixed: r.Fixed, Sample: r.Sample, Priority: r.Priority, IsEnabled: r.IsEnabled,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}.Model(models.SMSTemplateShared)
	t.SeedKey = r.SeedKey.String
	return t
}
