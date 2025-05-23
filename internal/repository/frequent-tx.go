package repository

import (
	"database/sql"
	"slices"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

func (r *Repository) createFrequentTxWithTxn(qtx *queries.Queries, ctx context.Context, ftx *models.FrequentTx) error {
	l := logger.FromCtx(ctx)

	var catId, cpId, toAccId, goalId sql.NullString

	if ftx.Category != nil {
		catId.Valid = ftx.Category.ID != ""
		catId.String = ftx.Category.ID
	}
	if ftx.Counterparty != nil {
		cpId.Valid = ftx.Counterparty.ID != ""
		cpId.String = ftx.Counterparty.ID
	}
	if ftx.ToAccount != nil {
		toAccId.Valid = ftx.ToAccount.ID != ""
		toAccId.String = ftx.ToAccount.ID
	}
	if ftx.Goal != nil {
		goalId.Valid = ftx.Goal.ID != ""
		goalId.String = ftx.Goal.ID
	}

	if err := qtx.CreateFrequentTx(ctx, queries.CreateFrequentTxParams{
		ID:                         ftx.ID,
		AccountID:                  ftx.Account.ID,
		FrequencyBy:                int64(ftx.FrequencyBy),
		Frequency:                  int64(ftx.Frequency),
		Amount:                     ftx.Amount,
		KindID:                     ftx.Kind.ID,
		CategoryID:                 catId,
		CounterpartyID:             cpId,
		Description:                sql.NullString{String: ftx.Description, Valid: ftx.Description != ""},
		ToAccountID:                toAccId,
		ToAmount:                   ftx.ToAmount,
		GoalID:                     goalId,
		StartsAt:                   ftx.StartsAt,
		StopsAt:                    sql.NullTime{Time: ftx.StopsAt, Valid: !ftx.StopsAt.IsZero()},
		LastTransactionScheduledTo: ftx.LastTransactionScheduledTo,
	}); err != nil {
		l.Error("creating frequent transaction", zap.Error(err), zap.Any("frequent tx", ftx))
		return err
	}
	l.Debug("created frequent transaction", zap.Any("frequent tx", ftx))
	return nil
}

func (r *Repository) createFrequentTxTransactionWithTxn(ctx context.Context, qtx *queries.Queries, ftxId, txId string) error {
	l := logger.FromCtx(ctx)
	if err := qtx.CreateFrequentTxTransaction(ctx, queries.CreateFrequentTxTransactionParams{
		TransactionID: txId,
		FrequencyID:   ftxId,
	}); err != nil {
		l.Error("creating frequent transaction transaction", zap.Error(err))
		return err
	}
	l.Debug("created frequent transaction transaction")
	return nil
}

func (r *Repository) GetFrequentTxs(ctx context.Context) ([]models.FrequentTx, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbFtx, err := q.GetFrequentTxs(ctx)
	if err != nil {
		l.Error("querying frequent transactions", zap.Error(err))
		return nil, err
	}
	l.Debug("queried frequent transactions")

	return dbFrequentTxsRowFrequentTxs(dbFtx), nil
}

func (r *Repository) GetFrequentTx(ctx context.Context, id string) (*models.FrequentTx, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbFtx, err := q.GetFrequentTx(ctx, id)
	if err != nil {
		l.Error("querying frequent transactions", zap.Error(err))
		return nil, err
	}
	l.Debug("queried frequent transactions")

	return dbFrequentTxRowFrequentTx(dbFtx), nil
}

func (r *Repository) UpdateFrequentTx(ctx context.Context, ftx *models.FrequentTx, scheduledTxs []models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		if err := qtx.DeleteScheduledTransactionsOfFrequentTx(ctx, ftx.ID); err != nil {
			l.Error("deleting scheduled transaction of transaction", zap.Error(err))
			return err
		}
		l.Debug("deleted scheduled transaction of transaction")

		var catId, cpId, toAccId, goalId sql.NullString

		if ftx.Category != nil {
			catId.Valid = ftx.Category.ID != ""
			catId.String = ftx.Category.ID
		}
		if ftx.Counterparty != nil {
			cpId.Valid = ftx.Counterparty.ID != ""
			cpId.String = ftx.Counterparty.ID
		}
		if ftx.ToAccount != nil {
			toAccId.Valid = ftx.ToAccount.ID != ""
			toAccId.String = ftx.ToAccount.ID
		}
		if ftx.Goal != nil {
			goalId.Valid = ftx.Goal.ID != ""
			goalId.String = ftx.Goal.ID
		}

		if err := qtx.UpdateFrequentTx(ctx, queries.UpdateFrequentTxParams{
			ID:                         ftx.ID,
			AccountID:                  ftx.Account.ID,
			Amount:                     ftx.Amount,
			KindID:                     ftx.Kind.ID,
			CategoryID:                 catId,
			CounterpartyID:             cpId,
			FrequencyBy:                int64(ftx.FrequencyBy),
			Frequency:                  int64(ftx.Frequency),
			Description:                sql.NullString{String: ftx.Description, Valid: ftx.Description != ""},
			ToAccountID:                toAccId,
			ToAmount:                   ftx.ToAmount,
			GoalID:                     goalId,
			StopsAt:                    sql.NullTime{Time: ftx.StopsAt, Valid: !ftx.StopsAt.IsZero()},
			LastTransactionScheduledTo: ftx.LastTransactionScheduledTo,
		}); err != nil {
			l.Error("updating frequent tx", zap.Any("frequent tx", ftx), zap.Error(err))
			return err
		}
		l.Debug("updated frequent tx", zap.Any("frequent tx", ftx))

		for _, stx := range scheduledTxs {
			if err := r.createTransactionWithTxn(ctx, qtx, &stx); err != nil {
				return err
			}
			if err := r.createFrequentTxTransactionWithTxn(ctx, qtx, ftx.ID, stx.ID); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *Repository) DeleteFrequentTx(ctx context.Context, id string) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		if err := qtx.DeleteScheduledTransactionsOfFrequentTx(ctx, id); err != nil {
			l.Error("deleting scheduled transaction of transaction", zap.Error(err))
			return err
		}
		l.Debug("deleted scheduled transaction of transaction")

		if err := qtx.DeleteFrequentTx(ctx, id); err != nil {
			l.Error("deleting frequent transaction", zap.Error(err))
			return err
		}
		l.Debug("deleted frequent transaction")

		return nil
	})
}

func (r *Repository) GenerateScheduledTransactions(ctx context.Context, ftx *models.FrequentTx, scheduledTxs []models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := qtx.UpdateFrequentTxLastTransactionCreatedAt(ctx, queries.UpdateFrequentTxLastTransactionCreatedAtParams{
			ID:                         ftx.ID,
			LastTransactionScheduledTo: ftx.LastTransactionScheduledTo,
		}); err != nil {
			return err
		}
		for _, stx := range scheduledTxs {
			if err := r.createTransactionWithTxn(ctx, qtx, &stx); err != nil {
				return err
			}
			if err := r.createFrequentTxTransactionWithTxn(ctx, qtx, ftx.ID, stx.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

func dbFrequentTxsRowFrequentTxs[T queries.GetFrequentTxRow | queries.GetFrequentTxsRow](dbFtxs []T) []models.FrequentTx {
	var ftxs []models.FrequentTx

	for _, dbFtx := range dbFtxs {
		ftxs = append(ftxs, *dbFrequentTxRowFrequentTx(queries.GetFrequentTxRow(dbFtx)))
	}

	return ftxs
}

func dbFrequentTxRowFrequentTx(dbFtx queries.GetFrequentTxRow) *models.FrequentTx {
	var toAcc *queries.Account
	var toAccCurr *queries.Currency
	if dbFtx.FrequentToAccountID.Valid {
		toAcc = &queries.Account{
			ID:                 dbFtx.FrequentToAccountID.String,
			Name:               dbFtx.FrequentToAccountName.String,
			KindID:             dbFtx.FrequentToAccountKindID.String,
			Amount:             dbFtx.FrequentToAccountAmount.Float64,
			CurrencyID:         dbFtx.FrequentToAccountCurrencyID.String,
			CounterpartyID:     dbFtx.FrequentToAccountCurrencyID,
			CardNumber:         dbFtx.FrequentToAccountCardNumber,
			CardLimit:          dbFtx.FrequentToAccountCardLimit.Float64,
			NameOnCard:         dbFtx.FrequentToAccountNameOnCard,
			Iban:               dbFtx.FrequentToAccountIban,
			Note:               dbFtx.FrequentToAccountNote,
			IsArchived:         dbFtx.FrequentToAccountIsArchived.Bool,
			ExcludeFromBalance: dbFtx.FrequentToAccountExcludeFromBalance.Bool,
			ExpirationDate:     dbFtx.FrequentToAccountExpirationDate,
			CreatedAt:          dbFtx.FrequentToAccountCreatedAt.Time,
		}
		if dbFtx.FrequentToAccountCurrencyID.Valid {
			toAccCurr = &queries.Currency{
				ID:     toAcc.CurrencyID,
				String: dbFtx.FrequentToAccountCurrencyString.String,
				Symbol: dbFtx.FrequentToAccountCurrencySymbol.String,
			}
		}
	}

	var cp *queries.Counterparty
	if dbFtx.FrequentTx.CounterpartyID.Valid {
		cp = &queries.Counterparty{ID: dbFtx.FrequentTx.CounterpartyID.String, Name: dbFtx.CounterpartyName.String}
	}

	var cat *queries.Category
	if dbFtx.CategoryID.Valid {
		cat = &queries.Category{
			ID:          dbFtx.CategoryID.String,
			Name:        dbFtx.CategoryName.String,
			BudgetInEgp: dbFtx.CategoryBudgetInEgp.Float64,
			KindID:      dbFtx.CategoryKindID.String,
			CreatedAt:   dbFtx.CategoryCreatedAt.Time,
		}
	}

	var gl *queries.Goal
	var glCur *queries.Currency
	if dbFtx.FrequentTx.GoalID.Valid {
		gl = &queries.Goal{
			ID:          dbFtx.FrequentTx.GoalID.String,
			Name:        dbFtx.GoalName.String,
			CurrencyID:  dbFtx.GoalCurrencyID.String,
			Price:       dbFtx.GoalPrice.Float64,
			FulfilledAt: dbFtx.GoalFulfilledAt,
		}
		if dbFtx.GoalCurrencyID.Valid {
			glCur = &queries.Currency{
				ID:     dbFtx.GoalCurrencyID.String,
				String: dbFtx.GoalCurrencyString.String,
				Symbol: dbFtx.GoalCurrencySymbol.String,
			}
		}
	}

	return dbFrequentTxToFrequentTx(
		&dbFtx.FrequentTx, &dbFtx.TransactionKind, &dbFtx.Account,
		&dbFtx.Currency, toAcc, toAccCurr, cat, cp,
		gl, glCur, nil)
}

func dbFrequentTxsWTxsRowToFrequentTxs[T queries.GetFrequentTxWTxsRow | queries.GetFrequentTxsWTxsRow](dbFtxs []T) []models.FrequentTx {
	ftxs := []models.FrequentTx{}

	for _, _dbFtx := range dbFtxs {
		dbFt := queries.GetFrequentTxWTxsRow(_dbFtx)
		ftxIdx := slices.IndexFunc(ftxs, func(ftx models.FrequentTx) bool { return ftx.ID == dbFt.FrequentTx.ID })
		if ftxIdx == -1 {
			var toAcc *queries.Account
			var toAccCurr *queries.Currency
			if dbFt.FrequentToAccountID.Valid {
				toAcc = &queries.Account{
					ID:                 dbFt.FrequentToAccountID.String,
					Name:               dbFt.FrequentToAccountName.String,
					KindID:             dbFt.FrequentToAccountKindID.String,
					Amount:             dbFt.FrequentToAccountAmount.Float64,
					CurrencyID:         dbFt.FrequentToAccountCurrencyID.String,
					CounterpartyID:     dbFt.FrequentToAccountCurrencyID,
					CardNumber:         dbFt.FrequentToAccountCardNumber,
					CardLimit:          dbFt.FrequentToAccountCardLimit.Float64,
					NameOnCard:         dbFt.FrequentToAccountNameOnCard,
					Iban:               dbFt.FrequentToAccountIban,
					Note:               dbFt.FrequentToAccountNote,
					IsArchived:         dbFt.FrequentToAccountIsArchived.Bool,
					ExcludeFromBalance: dbFt.FrequentToAccountExcludeFromBalance.Bool,
					ExpirationDate:     dbFt.FrequentToAccountExpirationDate,
					CreatedAt:          dbFt.FrequentToAccountCreatedAt.Time,
				}
				if dbFt.FrequentToAccountCurrencyID.Valid {
					toAccCurr = &queries.Currency{
						ID:     toAcc.CurrencyID,
						String: dbFt.FrequentToAccountCurrencyString.String,
						Symbol: dbFt.FrequentToAccountCurrencySymbol.String,
					}
				}
			}

			var cp *queries.Counterparty
			if dbFt.FrequentTx.CounterpartyID.Valid {
				cp = &queries.Counterparty{ID: dbFt.FrequentTx.CounterpartyID.String, Name: dbFt.CounterpartyName.String}
			}

			var cat *queries.Category
			if dbFt.CategoryID.Valid {
				cat = &queries.Category{
					ID:          dbFt.CategoryID.String,
					Name:        dbFt.CategoryName.String,
					BudgetInEgp: dbFt.CategoryBudgetInEgp.Float64,
					KindID:      dbFt.CategoryKindID.String,
					CreatedAt:   dbFt.CategoryCreatedAt.Time,
				}
			}

			var gl *queries.Goal
			var glCur *queries.Currency
			if dbFt.FrequentTx.GoalID.Valid {
				gl = &queries.Goal{
					ID:          dbFt.FrequentTx.GoalID.String,
					Name:        dbFt.GoalName.String,
					CurrencyID:  dbFt.GoalCurrencyID.String,
					Price:       dbFt.GoalPrice.Float64,
					FulfilledAt: dbFt.GoalFulfilledAt,
				}
				if dbFt.GoalCurrencyID.Valid {
					glCur = &queries.Currency{
						ID:     dbFt.GoalCurrencyID.String,
						String: dbFt.GoalCurrencyString.String,
						Symbol: dbFt.GoalCurrencySymbol.String,
					}
				}
			}

			ftxs = append(ftxs, *dbFrequentTxToFrequentTx(
				&dbFt.FrequentTx, &dbFt.TransactionKind, &dbFt.Account,
				&dbFt.Currency, toAcc, toAccCurr, cat, cp,
				gl, glCur, nil))
			ftxIdx = len(ftxs) - 1
		}

		if !dbFt.TransactionID.Valid {
			continue
		}
		existingTransactionIdx := slices.IndexFunc(ftxs[ftxIdx].Transactions, func(t models.Transaction) bool {
			return t.ID == dbFt.TransactionID.String
		})
		if existingTransactionIdx > -1 {
			continue
		}

		tx := &queries.Transaction{
			ID:             dbFt.TransactionID.String,
			AccountID:      dbFt.TransactionAccountID.String,
			Amount:         dbFt.TransactionAmount.Float64,
			KindID:         dbFt.TransactionKindID.String,
			CategoryID:     dbFt.TransactionCategoryID,
			CounterpartyID: dbFt.TransactionCounterpartyID,
			Description:    dbFt.TransactionDescription,
			ToAccountID:    dbFt.TransactionToAccountID,
			ToAmount:       dbFt.TransactionToAmount,
			GoalID:         dbFt.TransactionGoalID,
			FulfilledAt:    dbFt.TransactionFulfilledAt,
			ScheduledTo:    dbFt.TransactionScheduledTo.Time,
			CreatedAt:      dbFt.TransactionCreatedAt.Time,
		}

		var kind *queries.TransactionKind
		if dbFt.TransactionKindID.Valid {
			kind = &queries.TransactionKind{
				ID:              dbFt.TransactionKindID.String,
				Enum:            dbFt.TransactionKind.Enum,
				IncreaseBalance: dbFt.TransactionKindIncreaseBalance,
			}
		}

		var cp *queries.Counterparty
		if dbFt.TransactionCounterpartyID.Valid {
			cp = &queries.Counterparty{ID: dbFt.TransactionCounterpartyID.String, Name: dbFt.TransactionCounterpartyName.String}
		}

		var acc *queries.Account
		var accCurr *queries.Currency
		if dbFt.TransactionAccountID.Valid {
			acc = &queries.Account{
				ID:                 dbFt.TransactionAccountID.String,
				Name:               dbFt.TransactionAccountName.String,
				KindID:             dbFt.TransactionAccountKindID.String,
				Amount:             dbFt.TransactionAccountAmount.Float64,
				CurrencyID:         dbFt.TransactionAccountCurrencyID.String,
				CounterpartyID:     dbFt.TransactionAccountCurrencyID,
				CardNumber:         dbFt.TransactionAccountCardNumber,
				CardLimit:          dbFt.TransactionAccountCardLimit.Float64,
				NameOnCard:         dbFt.TransactionAccountNameOnCard,
				Iban:               dbFt.TransactionAccountIban,
				Note:               dbFt.TransactionAccountNote,
				IsArchived:         dbFt.TransactionAccountIsArchived.Bool,
				ExcludeFromBalance: dbFt.TransactionAccountExcludeFromBalance.Bool,
				ExpirationDate:     dbFt.TransactionAccountExpirationDate,
				CreatedAt:          dbFt.TransactionAccountCreatedAt.Time,
			}
			if dbFt.TransactionAccountCurrencyID.Valid {
				accCurr = &queries.Currency{
					ID:     acc.CurrencyID,
					String: dbFt.TransactionCurrencyString.String,
					Symbol: dbFt.TransactionCurrencySymbol.String,
				}
			}
		}

		var toAcc *queries.Account
		var toAccCurr *queries.Currency
		if dbFt.TransactionToAccountID.Valid {
			toAcc = &queries.Account{
				ID:                 dbFt.TransactionToAccountID.String,
				Name:               dbFt.TransactionToAccountName.String,
				KindID:             dbFt.TransactionToAccountKindID.String,
				Amount:             dbFt.TransactionToAccountAmount.Float64,
				CurrencyID:         dbFt.TransactionToAccountCurrencyID.String,
				CounterpartyID:     dbFt.TransactionToAccountCurrencyID,
				CardNumber:         dbFt.TransactionToAccountCardNumber,
				CardLimit:          dbFt.TransactionToAccountCardLimit.Float64,
				NameOnCard:         dbFt.TransactionToAccountNameOnCard,
				Iban:               dbFt.TransactionToAccountIban,
				Note:               dbFt.TransactionToAccountNote,
				IsArchived:         dbFt.TransactionToAccountIsArchived.Bool,
				ExcludeFromBalance: dbFt.TransactionToAccountExcludeFromBalance.Bool,
				ExpirationDate:     dbFt.TransactionToAccountExpirationDate,
				CreatedAt:          dbFt.TransactionToAccountCreatedAt.Time,
			}
			if dbFt.TransactionToAccountCurrencyID.Valid {
				toAccCurr = &queries.Currency{
					ID:     toAcc.CurrencyID,
					String: dbFt.TransactionToAccountCurrencyString.String,
					Symbol: dbFt.TransactionToAccountCurrencySymbol.String,
				}
			}
		}

		var cat *queries.Category
		if dbFt.TransactionCategoryID.Valid {
			cat = &queries.Category{
				ID:          dbFt.TransactionCategoryID.String,
				Name:        dbFt.TransactionCategoryName.String,
				BudgetInEgp: dbFt.TransactionCategoryBudgetInEgp.Float64,
				KindID:      dbFt.TransactionCategoryKindID.String,
				CreatedAt:   dbFt.TransactionCategoryCreatedAt.Time,
			}
		}

		ftxs[ftxIdx].Transactions = append(ftxs[ftxIdx].Transactions, *dbTransactionToTransaction(
			tx, kind, acc, accCurr, toAcc, toAccCurr, cat, cp, nil, nil))
	}

	return ftxs
}

func dbFrequentTxToFrequentTx(
	dbFtx *queries.FrequentTx, knd *queries.TransactionKind, acc *queries.Account,
	cur *queries.Currency, toAcc *queries.Account, toCur *queries.Currency,
	cat *queries.Category, cp *queries.Counterparty, gl *queries.Goal,
	glCur *queries.Currency, txs []queries.Transaction,
) *models.FrequentTx {
	ft := models.FrequentTx{
		ID:          dbFtx.ID,
		Account:     models.Account{ID: dbFtx.AccountID},
		Amount:      dbFtx.Amount,
		Description: dbFtx.Description.String,
		CreatedAt:   dbFtx.CreatedAt,

		ToAmount:                   dbFtx.ToAmount.Float64,
		Frequency:                  uint(dbFtx.Frequency),
		FrequencyBy:                models.FrequencyBy(dbFtx.FrequencyBy),
		StartsAt:                   dbFtx.StartsAt,
		StopsAt:                    dbFtx.StopsAt.Time,
		LastTransactionScheduledTo: dbFtx.LastTransactionScheduledTo,
	}

	if knd != nil {
		ft.Kind = *dbTransactionKindToTransactionKind(knd)
	} else if dbFtx.KindID != "" {
		ft.Kind = models.TransactionKind{ID: dbFtx.KindID}
	}

	if acc != nil {
		ft.Account = *dbAccountToAccount(acc, 0, cur, nil, sql.NullString{}, false)
	} else if dbFtx.ToAccountID.Valid {
		ft.ToAccount = &models.Account{ID: dbFtx.ToAccountID.String}
	}

	if toAcc != nil {
		ft.ToAccount = dbAccountToAccount(toAcc, 0, toCur, nil, sql.NullString{}, false)
	} else if dbFtx.ToAccountID.Valid {
		ft.ToAccount = &models.Account{ID: dbFtx.ToAccountID.String}
	}

	if cat != nil {
		ft.Category = dbCategoryToCategory(cat, knd)
	} else if dbFtx.CategoryID.Valid {
		ft.Category = &models.Category{ID: dbFtx.CategoryID.String}
	}

	if cp != nil {
		ft.Counterparty = dbCounterpartyToCounterparty(cp)
	} else if dbFtx.CounterpartyID.Valid {
		ft.Counterparty = &models.Counterparty{ID: dbFtx.CounterpartyID.String}
	}

	if gl != nil {
		ft.Goal = dbGoalToGoal(gl, glCur)
	} else if dbFtx.GoalID.Valid {
		ft.Goal = &models.Goal{ID: dbFtx.GoalID.String}
	}

	for _, tx := range txs {
		ft.Transactions = append(ft.Transactions, *dbTransactionToTransaction(&tx, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	}

	return &ft
}
