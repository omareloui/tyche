package repository

import (
	"database/sql"

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
		ID:                         rt.ID,
		AccountID:                  rt.Account.ID,
		Unit:                       int64(rt.Unit),
		Every:                      int64(rt.Every),
		Amount:                     rt.Amount,
		Kind:                       int64(rt.Kind),
		CategoryID:                 catId,
		CounterpartyID:             cpId,
		Description:                sql.NullString{String: rt.Description, Valid: rt.Description != ""},
		ToAccountID:                toAccId,
		ToAmount:                   rt.ToAmount,
		StartsAt:                   rt.StartsAt,
		StopsAt:                    sql.NullTime{Time: rt.StopsAt, Valid: !rt.StopsAt.IsZero()},
		LastTransactionScheduledTo: rt.LastTransactionScheduledTo,
	}); err != nil {
		l.Error("creating recurrent transaction", zap.Error(err), zap.Any("recurrent tx", rt))
		return err
	}
	l.Debug("created recurrent transaction", zap.Any("recurrent tx", rt))
	return nil
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

	return rt, nil
}

func (r *Repository) UpdateRecurrentTx(ctx context.Context, rt *models.RecurrentTx, scheduledTxs []models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		if err := qtx.DeleteScheduledTransactionsOfRecurrentTx(ctx, sql.NullString{String: rt.ID, Valid: rt.ID != ""}); err != nil {
			l.Error("deleting scheduled transaction of transaction", zap.Error(err))
			return err
		}
		l.Debug("deleted scheduled transaction of transaction")

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

		if err := qtx.UpdateRecurrentTx(ctx, queries.UpdateRecurrentTxParams{
			ID:                         rt.ID,
			AccountID:                  rt.Account.ID,
			Amount:                     rt.Amount,
			Kind:                       int64(rt.Kind),
			CategoryID:                 catId,
			CounterpartyID:             cpId,
			Unit:                       int64(rt.Unit),
			Every:                      int64(rt.Every),
			Description:                sql.NullString{String: rt.Description, Valid: rt.Description != ""},
			ToAccountID:                toAccId,
			ToAmount:                   rt.ToAmount,
			StopsAt:                    sql.NullTime{Time: rt.StopsAt, Valid: !rt.StopsAt.IsZero()},
			LastTransactionScheduledTo: rt.LastTransactionScheduledTo,
		}); err != nil {
			l.Error("updating recurrent tx", zap.Any("recurrent tx", rt), zap.Error(err))
			return err
		}
		l.Debug("updated recurrent tx", zap.Any("recurrent tx", rt))

		for _, stx := range scheduledTxs {
			if err := r.createTransactionWithTxn(qtx, ctx, &stx); err != nil {
				return err
			}
		}

		return nil
	})
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

func (r *Repository) GenerateScheduledTransactions(ctx context.Context, rt *models.RecurrentTx, scheduledTxs []models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := qtx.UpdateRecurrentTxLastTransactionCreatedAt(ctx, queries.UpdateRecurrentTxLastTransactionCreatedAtParams{
			ID:                         rt.ID,
			LastTransactionScheduledTo: rt.LastTransactionScheduledTo,
		}); err != nil {
			return err
		}
		for _, stx := range scheduledTxs {
			if err := r.createTransactionWithTxn(qtx, ctx, &stx); err != nil {
				return err
			}
		}
		return nil
	})
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

		ToAmount:                   dbRtx.RecurrentTxToAmount.Float64,
		Every:                      uint(dbRtx.RecurrentTxEvery),
		Unit:                       models.RecurrentUnit(dbRtx.RecurrentTxUnit),
		StartsAt:                   dbRtx.RecurrentTxStartsAt,
		StopsAt:                    dbRtx.RecurrentTxStopsAt.Time,
		LastTransactionScheduledTo: dbRtx.RecurrentTxLastTransactionScheduledTo,
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
