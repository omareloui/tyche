package repository

import (
	"database/sql"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

func (r *Repository) createRecurrentTxWithTxn(qtx *queries.Queries, ctx context.Context, rt *models.RecurrentTx) error {
	l := logger.FromCtx(ctx)

	var catId, cpId, toAccId sql.NullString

	if rt.Category != nil {
		catId.Valid = rt.Category.ID != ""
		catId.String = rt.Category.ID
	}
	if rt.Counterparty != nil {
		cpId.Valid = rt.Counterparty.ID != ""
		cpId.String = rt.Counterparty.ID
	}
	if rt.ToAccount != nil {
		toAccId.Valid = rt.ToAccount.ID != ""
		toAccId.String = rt.ToAccount.ID
	}

	if err := qtx.CreateRecurrentTx(ctx, queries.CreateRecurrentTxParams{
		ID:             rt.ID,
		AccountID:      rt.Account.ID,
		Unit:           int64(rt.Unit),
		Every:          int64(rt.Every),
		Amount:         rt.Amount,
		Kind:           int64(rt.Kind),
		CategoryID:     catId,
		CounterpartyID: cpId,
		Description:    sql.NullString{String: rt.Description, Valid: rt.Description != ""},
		ToAccountID:    toAccId,
		ToAmount:       rt.ToAmount,
		StartsAt:       rt.StartsAt,
		StopsAt:        sql.NullTime{Time: rt.StopsAt, Valid: !rt.StopsAt.IsZero()},
		Anchor:         sql.NullTime{Time: rt.Anchor, Valid: true},
		NextIndex:      int64(rt.NextIndex),
		NextOn:         sql.NullTime{Time: rt.NextOn(), Valid: true},
		AutoFulfill:    rt.AutoFulfill,
	}); err != nil {
		l.Error("creating recurrent transaction", zap.Error(err), zap.Any("recurrent tx", rt))
		return err
	}
	l.Debug("created recurrent transaction", zap.Any("recurrent tx", rt))
	return writeRecurrentItemsWithTxn(ctx, qtx, rt, false)
}

func (r *Repository) GetRecurrentTxs(ctx context.Context) ([]models.RecurrentTx, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbRtx, err := q.GetRecurrentTxs(ctx)
	if err != nil {
		l.Error("querying recurrent transactions", zap.Error(err))
		return nil, err
	}
	l.Debug("queried recurrent transactions")

	rts := dbRecurrentTxsDetailsToRecurrentTxs(dbRtx)

	occs, err := q.GetRecurrentTxsOccurrences(ctx)
	if err != nil {
		l.Error("querying recurrent transactions occurrences", zap.Error(err))
		return nil, err
	}
	byID := make(map[string]*models.RecurrentTx, len(rts))
	for i := range rts {
		byID[rts[i].ID] = &rts[i]
	}
	for _, o := range occs {
		if rt := byID[o.RecurrentID.String]; rt != nil {
			rt.AddOccurrence(o.ScheduledTo, o.FulfilledAt.Valid)
		}
	}
	if err := loadRecurrentTxItems(ctx, q, rts); err != nil {
		return nil, err
	}

	return rts, nil
}

func (r *Repository) GetRecurrentTx(ctx context.Context, id string) (*models.RecurrentTx, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbRtx, err := q.GetRecurrentTx(ctx, id)
	if err != nil {
		l.Error("querying recurrent transactions", zap.Error(err))
		return nil, err
	}
	l.Debug("queried recurrent transactions")

	rt := dbRecurrentTxRowToRecurrentTx(queries.GetRecurrentTxsRow(dbRtx))

	occs, err := q.GetRecurrentTxOccurrences(ctx, sql.NullString{String: id, Valid: true})
	if err != nil {
		l.Error("querying recurrent transaction occurrences", zap.Error(err))
		return nil, err
	}
	for _, o := range occs {
		rt.AddOccurrence(o.ScheduledTo, o.FulfilledAt.Valid)
	}
	if rt.Items, err = recurrentTxItemsOf(ctx, q, id); err != nil {
		return nil, err
	}

	return rt, nil
}

// UpdateRecurrentTx saves the edited rt over old: its fields go to its
// scheduled occurrences (but the ones edited on their own); a changed
// schedule (every, unit, anchor) deletes the ones from today on and writes
// them again from rt's anchor; a sooner end deletes the ones after it.
func (r *Repository) UpdateRecurrentTx(ctx context.Context, old, rt *models.RecurrentTx) error {
	today := r.userToday(ctx)
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)
		rid := sql.NullString{String: rt.ID, Valid: true}

		if err := qtx.UpdateUnfulfilledOccurrences(ctx, queries.UpdateUnfulfilledOccurrencesParams{
			RecurrentID:    rid,
			AccountID:      rt.Account.ID,
			Amount:         rt.Amount,
			Kind:           int64(rt.Kind),
			CategoryID:     nullID(rt.Category),
			CounterpartyID: nullCpID(rt.Counterparty),
			Description:    sql.NullString{String: rt.Description, Valid: rt.Description != ""},
			ToAccountID:    nullAccID(rt.ToAccount),
			ToAmount:       rt.ToAmount,
		}); err != nil {
			l.Error("updating the scheduled occurrences", zap.Error(err), zap.String("id", rt.ID))
			return err
		}

		rescheduled := rt.Unit != old.Unit || rt.Every != old.Every || !rt.Anchor.Equal(old.Anchor)
		if rescheduled {
			if err := qtx.DeleteUnfulfilledOccurrencesFrom(ctx, queries.DeleteUnfulfilledOccurrencesFromParams{
				RecurrentID: rid, FromDate: today, KeepOverridden: true,
			}); err != nil {
				l.Error("deleting the scheduled occurrences", zap.Error(err), zap.String("id", rt.ID))
				return err
			}
			rt.NextIndex = rt.ResumeIndex(today)
		} else {
			rt.NextIndex = old.NextIndex
		}
		if !rt.StopsAt.IsZero() {
			if err := qtx.DeleteUnfulfilledOccurrencesFrom(ctx, queries.DeleteUnfulfilledOccurrencesFromParams{
				RecurrentID: rid, FromDate: rt.StopsAt.AddDate(0, 0, 1), KeepOverridden: false,
			}); err != nil {
				l.Error("deleting the occurrences after its end", zap.Error(err), zap.String("id", rt.ID))
				return err
			}
		}

		if err := qtx.UpdateRecurrentTx(ctx, queries.UpdateRecurrentTxParams{
			ID:             rt.ID,
			AccountID:      rt.Account.ID,
			Amount:         rt.Amount,
			Kind:           int64(rt.Kind),
			CategoryID:     nullID(rt.Category),
			CounterpartyID: nullCpID(rt.Counterparty),
			Unit:           int64(rt.Unit),
			Every:          int64(rt.Every),
			Description:    sql.NullString{String: rt.Description, Valid: rt.Description != ""},
			ToAccountID:    nullAccID(rt.ToAccount),
			ToAmount:       rt.ToAmount,
			StopsAt:        sql.NullTime{Time: rt.StopsAt, Valid: !rt.StopsAt.IsZero()},
			Anchor:         sql.NullTime{Time: rt.Anchor, Valid: true},
			NextIndex:      int64(rt.NextIndex),
			NextOn:         sql.NullTime{Time: rt.NextOn(), Valid: true},
			AutoFulfill:    rt.AutoFulfill,
		}); err != nil {
			l.Error("updating recurrent tx", zap.Any("recurrent tx", rt), zap.Error(err))
			return err
		}
		l.Debug("updated recurrent tx", zap.Any("recurrent tx", rt))

		if err := writeRecurrentItemsWithTxn(ctx, qtx, rt, true); err != nil {
			return err
		}
		if !rt.IsPaused() {
			return r.scheduleRecurrentTxWithTxn(ctx, qtx, rt, today)
		}
		return nil
	})
}

func nullID(c *models.Category) sql.NullString {
	if c == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: c.ID, Valid: c.ID != ""}
}

func nullCpID(c *models.Counterparty) sql.NullString {
	if c == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: c.ID, Valid: c.ID != ""}
}

func nullAccID(a *models.Account) sql.NullString {
	if a == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: a.ID, Valid: a.ID != ""}
}

func (r *Repository) DeleteRecurrentTx(ctx context.Context, id string) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		if err := qtx.DeleteScheduledTransactionsOfRecurrentTx(ctx, sql.NullString{String: id, Valid: id != ""}); err != nil {
			l.Error("deleting scheduled transaction of transaction", zap.Error(err))
			return err
		}
		l.Debug("deleted scheduled transaction of transaction")

		if err := qtx.DeleteRecurrentTx(ctx, id); err != nil {
			l.Error("deleting recurrent transaction", zap.Error(err))
			return err
		}
		l.Debug("deleted recurrent transaction")

		return nil
	})
}

// PauseRecurrentTx stops writing its occurrences, deleting the scheduled ones
// from today on.
func (r *Repository) PauseRecurrentTx(ctx context.Context, rt *models.RecurrentTx) error {
	today := r.userToday(ctx)
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := qtx.DeleteUnfulfilledOccurrencesFrom(ctx, queries.DeleteUnfulfilledOccurrencesFromParams{
			RecurrentID: sql.NullString{String: rt.ID, Valid: true}, FromDate: today, KeepOverridden: false,
		}); err != nil {
			return err
		}
		return qtx.SetRecurrentTxPaused(ctx, queries.SetRecurrentTxPausedParams{
			ID:        rt.ID,
			PausedAt:  sql.NullTime{Time: time.Now(), Valid: true},
			NextIndex: int64(rt.NextIndex),
			NextOn:    sql.NullTime{Time: rt.NextOn(), Valid: true},
		})
	})
}

// ResumeRecurrentTx writes its occurrences again, from today on: the ones
// while paused are skipped.
func (r *Repository) ResumeRecurrentTx(ctx context.Context, rt *models.RecurrentTx) error {
	today := r.userToday(ctx)
	return r.transact(ctx, func(qtx *queries.Queries) error {
		// Pausing deleted the ones from its day on.
		rt.PausedAt = time.Time{}
		rt.NextIndex = rt.ResumeIndex(today)
		if err := qtx.SetRecurrentTxPaused(ctx, queries.SetRecurrentTxPausedParams{
			ID:        rt.ID,
			NextIndex: int64(rt.NextIndex),
			NextOn:    sql.NullTime{Time: rt.NextOn(), Valid: true},
		}); err != nil {
			return err
		}
		return r.scheduleRecurrentTxWithTxn(ctx, qtx, rt, today)
	})
}

// TopUpRecurrentTxs writes the occurrences of every recurrent tx due through
// the schedule horizon (models.ScheduleHorizon) and fulfills the due ones of
// the ones fulfilling themselves.
func (r *Repository) TopUpRecurrentTxs(ctx context.Context) error {
	today := r.userToday(ctx)
	return r.transact(ctx, func(qtx *queries.Queries) error {
		rows, err := qtx.GetRecurrentTxsDue(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			rt := dbRecurrentTxRowToRecurrentTx(queries.GetRecurrentTxsRow(row))
			if rt.IsGamyaa() {
				continue
			}
			if err := r.scheduleRecurrentTxWithTxn(ctx, qtx, rt, today); err != nil {
				return err
			}
		}
		return r.autoFulfillWithTxn(ctx, qtx, today)
	})
}

// scheduleRecurrentTxWithTxn writes rt's occurrences due through the horizon
// (one migrated from its last scheduled date continuing from today), taking
// for one on or before today an SMS transaction already paying it, then
// fulfills the due ones when it fulfills them by itself.
func (r *Repository) scheduleRecurrentTxWithTxn(ctx context.Context, qtx *queries.Queries, rt *models.RecurrentTx, today time.Time) error {
	l := logger.FromCtx(ctx)
	if rt.Items == nil {
		items, err := recurrentTxItemsOf(ctx, qtx, rt.ID)
		if err != nil {
			return err
		}
		rt.Items = items
	}
	rt.ContinueFrom(today)
	ks := rt.ToSchedule(today, models.ScheduleHorizon(monthStartFromCtx(ctx), today))
	now := time.Now()
	for _, k := range ks {
		occ := rt.Occurrence(idgen.New(), k, now)
		if !occ.ScheduledTo.After(today) {
			paid, err := r.paidOccurrenceWithTxn(ctx, qtx, &occ)
			if err != nil {
				return err
			}
			if paid != nil {
				if err := qtx.SetTransactionRecurrent(ctx, queries.SetTransactionRecurrentParams{
					ID: paid.ID, RecurrentID: sql.NullString{String: rt.ID, Valid: true},
				}); err != nil {
					return err
				}
				l.Info("took an sms transaction for an occurrence", zap.String("recurrent_id", rt.ID), zap.String("id", paid.ID))
				continue
			}
		}
		if err := r.createTransactionWithTxn(qtx, ctx, &occ); err != nil {
			return err
		}
	}
	if len(ks) > 0 {
		rt.NextIndex = ks[len(ks)-1] + 1
	}
	if err := qtx.SetRecurrentTxNext(ctx, queries.SetRecurrentTxNextParams{
		ID:        rt.ID,
		NextIndex: int64(rt.NextIndex),
		NextOn:    sql.NullTime{Time: rt.NextOn(), Valid: true},
	}); err != nil {
		return err
	}
	if rt.AutoFulfill {
		return r.autoFulfillWithTxn(ctx, qtx, today)
	}
	return nil
}

// paidOccurrenceWithTxn is the SMS transaction, not taken by a recurrent tx
// yet, that likely paid occ, nil if none.
func (r *Repository) paidOccurrenceWithTxn(ctx context.Context, qtx *queries.Queries, occ *models.Transaction) (*models.Transaction, error) {
	rows, err := qtx.GetUnlinkedSMSTransactions(ctx, queries.GetUnlinkedSMSTransactionsParams{
		AccountID: occ.Account.ID,
		Since:     occ.ScheduledTo.Add(-models.RecurrentMatchWindow),
		Until:     occ.ScheduledTo.Add(models.RecurrentMatchWindow),
	})
	if err != nil {
		logger.FromCtx(ctx).Error("querying sms transactions paying an occurrence", zap.Error(err))
		return nil, err
	}
	var best *models.Transaction
	for i := range rows {
		sms := leanTransaction(&rows[i])
		if pick := models.PickRecurrentOccurrence(sms, []models.Transaction{*occ}); pick != nil {
			if best == nil || absDur(sms.ScheduledTo.Sub(occ.ScheduledTo)) < absDur(best.ScheduledTo.Sub(occ.ScheduledTo)) {
				best = sms
			}
		}
	}
	return best, nil
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// leanTransaction is a transactions row with only its references' ids.
func leanTransaction(t *queries.Transaction) *models.Transaction {
	tx := dbTransactionToTransaction(t, nil, nil, nil, nil, nil, nil, nil, nil)
	tx.Account.ID = t.AccountID
	return tx
}

// autoFulfillWithTxn fulfills, on their day, the occurrences due by today of
// the recurrent txs fulfilling themselves, flagged for review.
func (r *Repository) autoFulfillWithTxn(ctx context.Context, qtx *queries.Queries, today time.Time) error {
	rows, err := qtx.GetAutoFulfillDue(ctx, today.AddDate(0, 0, 1))
	if err != nil {
		return err
	}
	for _, row := range rows {
		tx := &models.Transaction{
			ID:          row.ID,
			Account:     models.Account{ID: row.AccountID},
			Kind:        models.TxKind(row.Kind),
			Amount:      row.Amount,
			ToAmount:    row.ToAmount.Float64,
			FulfilledAt: row.ScheduledTo,
		}
		if row.ToAccountID.Valid {
			tx.ToAccount = &models.Account{ID: row.ToAccountID.String}
		}
		if err := qtx.FulfillTransactionOn(ctx, queries.FulfillTransactionOnParams{
			ID: tx.ID, FulfilledAt: sql.NullTime{Time: tx.FulfilledAt, Valid: true}, NeedsReview: true,
		}); err != nil {
			return err
		}
		if err := r.applyTxToAccountWithTxn(qtx, ctx, tx); err != nil {
			return err
		}
	}
	return nil
}

// mergeSMSIntoOccurrenceWithTxn finds the occurrence of a recurrent tx the
// SMS transaction sms likely pays and makes it the paid one (models.MergeSMS),
// returning it, nil (and nothing done) if none matches.
func (r *Repository) mergeSMSIntoOccurrenceWithTxn(ctx context.Context, qtx *queries.Queries, sms *models.Transaction) (*models.Transaction, error) {
	if sms.FulfilledAt.IsZero() || (sms.Kind != models.ExpenseTxKind && sms.Kind != models.IncomeTxKind) {
		return nil, nil
	}
	rows, err := qtx.GetRecurrentOccurrenceCandidates(ctx, queries.GetRecurrentOccurrenceCandidatesParams{
		AccountID: sms.Account.ID,
		ID:        sms.ID,
		Since:     sms.ScheduledTo.Add(-models.RecurrentMatchWindow),
		Until:     sms.ScheduledTo.Add(models.RecurrentMatchWindow),
	})
	if err != nil {
		logger.FromCtx(ctx).Error("querying the occurrences an sms may pay", zap.Error(err))
		return nil, err
	}
	cands := make([]models.Transaction, len(rows))
	for i := range rows {
		cands[i] = *leanTransaction(&rows[i])
	}
	occ := models.PickRecurrentOccurrence(sms, cands)
	if occ == nil {
		return nil, nil
	}
	return occ, r.mergeIntoOccurrenceWithTxn(ctx, qtx, occ, sms)
}

// mergeIntoOccurrenceWithTxn saves occ as paid by the SMS transaction sms
// (not stored, or already deleted).
func (r *Repository) mergeIntoOccurrenceWithTxn(ctx context.Context, qtx *queries.Queries, occ, sms *models.Transaction) error {
	old := *occ
	occ.MergeSMS(sms)
	if err := r.updateTransactionWithTxn(ctx, qtx, &old, occ); err != nil {
		return err
	}
	if err := qtx.FulfillTransactionOn(ctx, queries.FulfillTransactionOnParams{
		ID: occ.ID, FulfilledAt: sql.NullTime{Time: occ.FulfilledAt, Valid: true}, NeedsReview: true,
	}); err != nil {
		return err
	}
	if old.FulfilledAt.IsZero() {
		if err := r.applyTxToAccountWithTxn(qtx, ctx, occ); err != nil {
			return err
		}
	}
	logger.FromCtx(ctx).Info("merged an sms into an occurrence", zap.String("id", occ.ID), zap.String("recurrent_id", occ.RecurrentTxID))
	return nil
}

// GetRecurrentMatchCandidates are the occurrences of recurrent txs the SMS
// transaction tx may pay, within models.RecurrentManualMatchWindow, soonest
// first, to match it by hand.
func (r *Repository) GetRecurrentMatchCandidates(ctx context.Context, tx *models.Transaction) ([]models.Transaction, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetRecurrentOccurrenceCandidates(ctx, queries.GetRecurrentOccurrenceCandidatesParams{
		AccountID: tx.Account.ID,
		ID:        tx.ID,
		Since:     tx.ScheduledTo.Add(-models.RecurrentManualMatchWindow),
		Until:     tx.ScheduledTo.Add(models.RecurrentManualMatchWindow),
	})
	if err != nil {
		return nil, err
	}
	var res []models.Transaction
	for i := range rows {
		occ := leanTransaction(&rows[i])
		if !models.MayPayOccurrence(tx, occ) {
			continue
		}
		full, err := r.GetTransaction(ctx, occ.ID)
		if err != nil {
			return nil, err
		}
		res = append(res, *full)
	}
	return res, nil
}

// MatchSMSToOccurrence makes occ the transaction the SMS transaction sms
// paid: its SMS moves to occ, sms is deleted, occ takes its amount and date.
func (r *Repository) MatchSMSToOccurrence(ctx context.Context, sms, occ *models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := qtx.MoveSMSMessagesTransaction(ctx, queries.MoveSMSMessagesTransactionParams{
			FromID: sql.NullString{String: sms.ID, Valid: true},
			ToID:   sql.NullString{String: occ.ID, Valid: true},
		}); err != nil {
			return err
		}
		if err := r.deleteTransactionWithTxn(ctx, qtx, sms); err != nil {
			return err
		}
		return r.mergeIntoOccurrenceWithTxn(ctx, qtx, occ, sms)
	})
}

// userToday is today in the user's time zone, as a UTC date.
func (r *Repository) userToday(ctx context.Context) time.Time {
	loc, err := r.GetTimezone(ctx)
	if err != nil {
		loc = time.UTC
	}
	y, m, d := time.Now().In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func dbRecurrentTxsDetailsToRecurrentTxs(dbRts []queries.GetRecurrentTxsRow) []models.RecurrentTx {
	var rts []models.RecurrentTx

	for _, dbRt := range dbRts {
		rts = append(rts, *dbRecurrentTxRowToRecurrentTx(dbRt))
	}

	return rts
}

// dbRecurrentTxRowToRecurrentTx adds what the details view lacks: the
// accounts' kinds, the counterparty's icon and the category's color.
func dbRecurrentTxRowToRecurrentTx(row queries.GetRecurrentTxsRow) *models.RecurrentTx {
	rt := dbRecurrentTxDetailsToRecurrentTx(row.RecurrentTxsDetail)
	rt.Account.Kind.Enum = models.AccKind(row.AccountKindEnum.Int64)
	if rt.ToAccount != nil {
		rt.ToAccount.Kind.Enum = models.AccKind(row.ToAccountKindEnum.Int64)
	}
	if rt.Counterparty != nil {
		rt.Counterparty.Icon = row.CounterpartyIcon.String
		rt.Counterparty.Color = row.CounterpartyColor.String
	}
	if rt.Category != nil {
		rt.Category.Color = row.CategoryColor.String
	}
	return rt
}

func dbRecurrentTxDetailsToRecurrentTx(dbRtx queries.RecurrentTxsDetail) *models.RecurrentTx {
	rt := models.RecurrentTx{
		ID:          dbRtx.RecurrentTxID,
		Account:     models.Account{ID: dbRtx.RecurrentTxAccountID},
		Amount:      dbRtx.RecurrentTxAmount,
		Description: dbRtx.RecurrentTxDescription.String,
		CreatedAt:   dbRtx.RecurrentTxCreatedAt,

		Kind: models.TxKind(dbRtx.RecurrentTxKind),

		ToAmount:        dbRtx.RecurrentTxToAmount.Float64,
		Every:           uint(dbRtx.RecurrentTxEvery),
		Unit:            models.RecurrentUnit(dbRtx.RecurrentTxUnit),
		StartsAt:        dbRtx.RecurrentTxStartsAt,
		StopsAt:         dbRtx.RecurrentTxStopsAt.Time,
		Anchor:          dbRtx.RecurrentTxAnchor.Time,
		NextIndex:       int(dbRtx.RecurrentTxNextIndex),
		NextUnset:       !dbRtx.RecurrentTxNextOn.Valid,
		AutoFulfill:     dbRtx.RecurrentTxAutoFulfill,
		PausedAt:        dbRtx.RecurrentTxPausedAt.Time,
		GamyaaAccountID: dbRtx.GamyaaAccountID.String,
	}

	if dbRtx.AccountName.Valid {
		rt.Account.Name = dbRtx.AccountName.String
		rt.Account.Kind = models.AccountKind{ID: dbRtx.AccountKindID.String}
		rt.Account.Amount = dbRtx.AccountAmount.Float64
		rt.Account.CardNumber = dbRtx.AccountCardNumber.String
		rt.Account.CardLimit = dbRtx.AccountCardLimit.Float64
		rt.Account.NameOnCard = dbRtx.AccountName.String
		rt.Account.Iban = dbRtx.AccountIban.String
		rt.Account.Note = dbRtx.AccountNote.String
		rt.Account.IsArchive = dbRtx.AccountIsArchived.Bool
		rt.Account.ExpirationDate = dbRtx.AccountExpirationDate.Time
		rt.Account.CreatedAt = dbRtx.AccountCreatedAt.Time

		if dbRtx.AccountCounterpartyID.Valid {
			rt.Account.Counterparty = &models.Counterparty{
				ID: dbRtx.AccountCounterpartyID.String,
			}
		}
	}

	if dbRtx.AccountCurrencyID.Valid {
		rt.Account.Currency = models.Currency{
			ID:      dbRtx.AccountCurrencyID.String,
			String:  dbRtx.AccountCurrencyString.String,
			Symbol:  dbRtx.AccountCurrencySymbol.String,
			InGrams: dbRtx.AccountCurrencyInGrams.Bool,
		}
	}

	if dbRtx.RecurrentTxToAccountID.Valid {
		rt.ToAccount = &models.Account{
			ID:   dbRtx.RecurrentTxToAccountID.String,
			Name: dbRtx.ToAccountName.String,
			Kind: models.AccountKind{
				ID: dbRtx.ToAccountKindID.String,
			},
			Amount:             dbRtx.ToAccountAmount.Float64,
			CardNumber:         dbRtx.ToAccountCardNumber.String,
			CardLimit:          dbRtx.ToAccountCardLimit.Float64,
			NameOnCard:         dbRtx.ToAccountNameOnCard.String,
			Iban:               dbRtx.ToAccountIban.String,
			Note:               dbRtx.ToAccountNote.String,
			IsArchive:          dbRtx.ToAccountIsArchived.Bool,
			ExcludeFromBalance: dbRtx.ToAccountExcludeFromBalance.Bool,
			ExpirationDate:     dbRtx.ToAccountExpirationDate.Time,
			CreatedAt:          dbRtx.ToAccountCreatedAt.Time,
		}
		if dbRtx.ToAccountCurrencyID.Valid {
			rt.ToAccount.Currency = models.Currency{
				ID:      dbRtx.ToAccountCurrencyID.String,
				String:  dbRtx.ToAccountCurrencyString.String,
				Symbol:  dbRtx.ToAccountCurrencySymbol.String,
				InGrams: dbRtx.ToAccountCurrencyInGrams.Bool,
			}
		}
		if dbRtx.ToAccountCounterpartyID.Valid {
			rt.ToAccount.Counterparty = &models.Counterparty{
				ID: dbRtx.ToAccountCounterpartyID.String,
			}
		}
	}

	if dbRtx.RecurrentTxCounterpartyID.Valid {
		rt.Counterparty = &models.Counterparty{
			ID:   dbRtx.RecurrentTxCounterpartyID.String,
			Name: dbRtx.CounterpartyName.String,
		}
	}

	if dbRtx.RecurrentTxCategoryID.Valid {
		kinds, _ := unmarshalTxKinds(dbRtx.CategoryKinds)
		rt.Category = &models.Category{
			ID:           dbRtx.RecurrentTxCategoryID.String,
			Name:         dbRtx.CategoryName.String,
			BudgetAmount: dbRtx.CategoryBudgetAmount.Float64,
			Kinds:        kinds,
			CreatedAt:    dbRtx.CategoryCreatedAt.Time,
		}
		if dbRtx.CategoryBudgetCurrencyID.Valid {
			rt.Category.BudgetCurrency = &models.Currency{
				ID: dbRtx.CategoryBudgetCurrencyID.String,
			}
		}
	}

	return &rt
}

// GetRecurrentTxTransactions is every transaction generated from the
// recurrent transaction id, newest first.
func (r *Repository) GetRecurrentTxTransactions(ctx context.Context, id string) ([]models.Transaction, error) {
	p := queries.GetTransactionsParams{RecurrentTxID: id, Lim: -1}
	return r.listTransactions(ctx, p)
}
