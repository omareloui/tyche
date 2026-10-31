package repository

import (
	"context"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"github.com/omareloui/tyche/internal/smstemplates"
)

func (r *Repository) GetSMSTemplates(ctx context.Context) ([]models.SMSTemplate, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := q.ListSMSTemplates(ctx)
	if err != nil {
		return nil, err
	}
	ts := make([]models.SMSTemplate, len(rows))
	for i, row := range rows {
		ts[i] = smstemplates.Row(row.SmsTemplate).Model(models.SMSTemplateMine)
	}
	return ts, nil
}

func (r *Repository) GetSMSTemplate(ctx context.Context, id string) (*models.SMSTemplate, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	row, err := q.GetSMSTemplate(ctx, id)
	if err != nil {
		return nil, err
	}
	t := smstemplates.Row(row.SmsTemplate).Model(models.SMSTemplateMine)
	return &t, nil
}

func (r *Repository) CreateSMSTemplate(ctx context.Context, t *models.SMSTemplate) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.CreateSMSTemplate(ctx, queries.CreateSMSTemplateParams(smstemplates.ParamsOf(t)))
}

func (r *Repository) UpdateSMSTemplate(ctx context.Context, t *models.SMSTemplate) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	p := smstemplates.ParamsOf(t)
	return q.UpdateSMSTemplate(ctx, queries.UpdateSMSTemplateParams{
		ID: p.ID, Name: p.Name, Bank: p.Bank, Senders: p.Senders, Action: p.Action, TxKind: p.TxKind,
		Body: p.Body, Fixed: p.Fixed, Sample: p.Sample, Priority: p.Priority, IsEnabled: p.IsEnabled,
	})
}

func (r *Repository) DeleteSMSTemplate(ctx context.Context, id string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.DeleteSMSTemplate(ctx, id)
}
