package repository

import (
	"context"
	"database/sql"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) GetSMSMessageByHash(ctx context.Context, hash string) (*models.SMSMessage, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbMsg, err := q.GetSMSMessageByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	return dbSMSMessageToSMSMessage(&dbMsg.SmsMessage), nil
}

func (r *Repository) GetSMSMessage(ctx context.Context, id string) (*models.SMSMessage, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbMsg, err := q.GetSMSMessage(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbSMSMessageToSMSMessage(&dbMsg.SmsMessage), nil
}

func (r *Repository) GetSMSMessagesByStatus(ctx context.Context, statuses ...models.SMSStatus) ([]models.SMSMessage, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	sts := make([]string, len(statuses))
	for i, s := range statuses {
		sts[i] = string(s)
	}

	dbMsgs, err := q.GetSMSMessagesByStatus(ctx, sts)
	if err != nil {
		l.Error("fetching sms messages", zap.Error(err))
		return nil, err
	}

	msgs := make([]models.SMSMessage, len(dbMsgs))
	for i, m := range dbMsgs {
		msgs[i] = *dbSMSMessageToSMSMessage(&m.SmsMessage)
	}
	return msgs, nil
}

// CreateSMSMessage stores the sms and, if passed, creates its transaction in
// the same database transaction.
func (r *Repository) CreateSMSMessage(ctx context.Context, msg *models.SMSMessage, tx *models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		if tx != nil {
			if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, tx); err != nil {
				return err
			}
			msg.TransactionID = tx.ID
		}

		if err := qtx.CreateSMSMessage(ctx, queries.CreateSMSMessageParams{
			ID:            msg.ID,
			Sender:        msg.Sender,
			Body:          msg.Body,
			ReceivedAt:    msg.ReceivedAt,
			Hash:          msg.Hash,
			Status:        string(msg.Status),
			Error:         nullString(msg.Error),
			TransactionID: nullString(msg.TransactionID),
			DeviceID:      nullString(msg.DeviceID),
		}); err != nil {
			l.Error("creating sms message", zap.Error(err), zap.String("hash", msg.Hash))
			return err
		}
		l.Debug("created sms message", zap.String("id", msg.ID), zap.String("status", string(msg.Status)))
		return nil
	})
}

// UpdateSMSMessageResult updates the sms status and, if passed, creates its
// transaction in the same database transaction.
func (r *Repository) UpdateSMSMessageResult(ctx context.Context, msg *models.SMSMessage, tx *models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		if tx != nil {
			if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, tx); err != nil {
				return err
			}
			msg.TransactionID = tx.ID
		}

		if err := qtx.UpdateSMSMessageResult(ctx, queries.UpdateSMSMessageResultParams{
			ID:            msg.ID,
			Status:        string(msg.Status),
			Error:         nullString(msg.Error),
			TransactionID: nullString(msg.TransactionID),
		}); err != nil {
			l.Error("updating sms message", zap.Error(err), zap.String("id", msg.ID))
			return err
		}
		l.Debug("updated sms message", zap.String("id", msg.ID), zap.String("status", string(msg.Status)))
		return nil
	})
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func dbSMSMessageToSMSMessage(m *queries.SmsMessage) *models.SMSMessage {
	return &models.SMSMessage{
		ID:            m.ID,
		Sender:        m.Sender,
		Body:          m.Body,
		ReceivedAt:    m.ReceivedAt,
		Hash:          m.Hash,
		Status:        models.SMSStatus(m.Status),
		Error:         m.Error.String,
		TransactionID: m.TransactionID.String,
		DeviceID:      m.DeviceID.String,
		CreatedAt:     m.CreatedAt,
	}
}
