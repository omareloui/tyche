package repository

import (
	"context"
	"database/sql"
	"strings"

	"github.com/omareloui/tyche/internal/db"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

func (r *Repository) CreateAccount(ctx context.Context, a *models.Account) error {
	l := logger.FromCtx(ctx)
	if err := r.q.CreateAccount(ctx, db.CreateAccountParams{
		ID:                 a.ID,
		Name:               a.Name,
		KindID:             a.Kind.ID,
		Amount:             a.Amount,
		CurrencyID:         a.Currency.ID,
		CardNumber:         sql.NullString{String: a.CardNumber, Valid: a.CardNumber != ""},
		Iban:               sql.NullString{String: a.IBAN, Valid: a.IBAN != ""},
		Note:               sql.NullString{String: a.Note, Valid: a.Note != ""},
		IsArchived:         a.IsArchive,
		ExcludeFromBalance: a.ExcludeFromBalance,
	}); err != nil {
		l.Error("creating account", zap.Error(err))
		if strings.Contains(err.Error(), "UNIQUE constraint failed: accounts.name") {
			return newDuplicationError("Name")
		}

		if strings.Contains(err.Error(), "UNIQUE constraint failed: accounts.card_number") {
			return newDuplicationError("CardNumber")
		}

		if strings.Contains(err.Error(), "UNIQUE constraint failed: accounts.iban") {
			return newDuplicationError("IBAN")
		}
		return err
	}
	l.Debug("created account")
	return nil
}

func (r *Repository) GetAccounts(ctx context.Context) ([]models.Account, error) {
	l := logger.FromCtx(ctx)
	accs, err := r.q.GetAccounts(ctx)
	if err != nil {
		l.Error("querying accounts", zap.Error(err))
		return nil, err
	}
	l.Debug("queried accounts")
	return dbAccountsRowToAccounts(accs), nil
}

func (r *Repository) GetAccount(ctx context.Context, id string) (*models.Account, error) {
	l := logger.FromCtx(ctx)
	acc, err := r.q.GetAccount(ctx, id)
	if err != nil {
		l.Error("querying account", zap.Error(err))
		return nil, err
	}
	l.Debug("queried account")
	return dbAccountRowToAccount(&acc), nil
}

func (r *Repository) UpdateAccount(ctx context.Context, a *models.Account) error {
	l := logger.FromCtx(ctx)
	if err := r.q.UpdateAccount(ctx, db.UpdateAccountParams{
		ID:                 a.ID,
		Name:               a.Name,
		KindID:             a.Kind.ID,
		CurrencyID:         a.Currency.ID,
		CardNumber:         sql.NullString{String: a.CardNumber, Valid: a.CardNumber != ""},
		Iban:               sql.NullString{String: a.IBAN, Valid: a.IBAN != ""},
		Note:               sql.NullString{String: a.Note, Valid: a.Note != ""},
		IsArchived:         a.IsArchive,
		ExcludeFromBalance: a.ExcludeFromBalance,
	}); err != nil {
		l.Error("updating account", zap.Error(err))

		if strings.Contains(err.Error(), "UNIQUE constraint failed: accounts.name") {
			return newDuplicationError("Name")
		}

		if strings.Contains(err.Error(), "UNIQUE constraint failed: accounts.card_number") {
			return newDuplicationError("CardNumber")
		}

		if strings.Contains(err.Error(), "UNIQUE constraint failed: accounts.iban") {
			return newDuplicationError("IBAN")
		}
		return err
	}
	l.Debug("updated account")
	return nil
}

func (r *Repository) SetDefaultAccountExpect(ctx context.Context, id string) error {
	return r.transact(func(qtx *db.Queries) error {
		l := logger.FromCtx(ctx)
		err := qtx.UnsetDefaultAccountExpect(ctx, id)
		if err != nil {
			l.Error("unsetting default accounts", zap.Error(err))
			return err
		}
		err = qtx.SetDefaultAccount(ctx, id)
		if err != nil {
			l.Error("setting default accounts", zap.Error(err))
			return err
		}
		return nil
	})
}

func (r *Repository) DeleteAccount(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)
	if err := r.q.DeleteAccount(ctx, id); err != nil {
		l.Error("deleting account", zap.Error(err))
		return err
	}
	l.Debug("deleted account")
	return nil
}

func (r *Repository) applyTxToAccountWithTxn(qtx *db.Queries, ctx context.Context, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)

	if tx.Kind.Kind == models.TransferTxKind {
		if err := r.removeFromAccountWithTxn(qtx, ctx, tx.Account.ID, tx.Amount); err != nil {
			l.Error("applying transfer: remove from account", zap.Error(err), zap.Any("transaction", tx))
			return err
		}
		l.Debug("applied transfer: remove from account")
		if err := r.addToAccountWithTxn(qtx, ctx, tx.ToAccount.ID, tx.ToAmount); err != nil {
			l.Error("applying transfer: add to account", zap.Error(err), zap.Any("transaction", tx))
			return err
		}
		l.Debug("applied transfer: add to account")
	} else {
		upBalance := r.removeFromAccountWithTxn
		if tx.Kind.IncreaseBalance {
			upBalance = r.addToAccountWithTxn
		}
		if err := upBalance(qtx, ctx, tx.Account.ID, tx.Amount); err != nil {
			l.Error("applying transaction to account", zap.Error(err), zap.Any("transaction", tx))
			return err
		}
		l.Debug("applied transaction to account", zap.Any("transaction", tx))
	}
	return nil
}

func (r *Repository) undoTxToAccountWithTxn(qtx *db.Queries, ctx context.Context, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)
	if tx.Kind.Kind == models.TransferTxKind {
		if err := r.addToAccountWithTxn(qtx, ctx, tx.Account.ID, tx.Amount); err != nil {
			l.Error("undoing transfer: remove from account", zap.Error(err), zap.Any("transaction", tx))
			return err
		}
		l.Debug("undone transfer: remove from account")
		if err := r.removeFromAccountWithTxn(qtx, ctx, tx.ToAccount.ID, tx.ToAmount); err != nil {
			l.Error("undoing transfer: add to account", zap.Error(err), zap.Any("transaction", tx))
			return err
		}
		l.Debug("undone transfer: add to account")
	} else {
		upBalance := r.addToAccountWithTxn
		if tx.Kind.IncreaseBalance {
			upBalance = r.removeFromAccountWithTxn
		}
		if err := upBalance(qtx, ctx, tx.Account.ID, tx.Amount); err != nil {
			l.Error("undoing transaction to account", zap.Error(err), zap.Any("transaction", tx))
			return err
		}
		l.Debug("undone transaction to account", zap.Any("transaction", tx))
	}
	return nil
}

func (r *Repository) addToAccountWithTxn(qtx *db.Queries, ctx context.Context, id string, amount float64) error {
	err := qtx.AddToAccount(ctx, db.AddToAccountParams{ID: id, Amount: amount})
	if err != nil {
		l := logger.FromCtx(ctx)
		l.Error("adding to account", zap.Error(err), zap.String("id", id), zap.Float64("amount", amount))
		return err
	}
	return nil
}

func (r *Repository) removeFromAccountWithTxn(qtx *db.Queries, ctx context.Context, id string, amount float64) error {
	err := qtx.RemoveFromAccount(ctx, db.RemoveFromAccountParams{ID: id, Amount: amount})
	if err != nil {
		l := logger.FromCtx(ctx)
		l.Error("removing from account", zap.Error(err), zap.String("id", id), zap.Float64("amount", amount))
		return err
	}
	return nil
}

func (r *Repository) GetBalances(ctx context.Context) ([]models.Balance, error) {
	dbBalances, err := r.q.GetBalances(ctx)
	if err != nil {
		l := logger.FromCtx(ctx)
		l.Error("querying balances", zap.Error(err))
		return nil, err
	}
	return dbBalancesToBalances(dbBalances), nil
}

func dbAccountRowToAccount[T db.GetAccountRow | db.GetAccountsRow](dbAcc *T) *models.Account {
	_dbAcc := db.GetAccountsRow(*dbAcc)
	return &models.Account{
		ID:           _dbAcc.ID,
		Name:         _dbAcc.Name,
		Amount:       _dbAcc.Amount,
		ActualAmount: _dbAcc.ActualAmount.(float64),
		IsDefault:    _dbAcc.IsDefault,
		Currency: models.Currency{
			ID:        _dbAcc.CurrencyID,
			Enum:      models.CurrencyEnum(_dbAcc.CurrencyEnum.Int64),
			EgpFxRate: _dbAcc.EgpFxRate.Float64,
		},
		Kind: models.AccountKind{
			ID:   _dbAcc.KindID,
			Enum: models.AccountKindEnum(_dbAcc.KindEnum.Int64),
		},
		IBAN:               _dbAcc.Iban.String,
		CardNumber:         _dbAcc.CardNumber.String,
		Note:               _dbAcc.Note.String,
		IsArchive:          _dbAcc.IsArchived,
		ExcludeFromBalance: _dbAcc.ExcludeFromBalance,
		CreatedAt:          _dbAcc.CreatedAt,
	}
}

func dbAccountsRowToAccounts(dbAccs []db.GetAccountsRow) []models.Account {
	acc := make([]models.Account, len(dbAccs))
	for i, dbAcc := range dbAccs {
		acc[i] = *dbAccountRowToAccount(&dbAcc)
	}
	return acc
}

func dbBalancesToBalances(dbBs []db.GetBalancesRow) []models.Balance {
	b := make([]models.Balance, len(dbBs))
	for i, dbB := range dbBs {
		b[i] = models.Balance{
			Amount: dbB.Balance,
			Currency: models.Currency{
				ID:        dbB.CurrencyID,
				Enum:      models.CurrencyEnum(dbB.CurrencyEnum.Int64),
				EgpFxRate: dbB.EgpFxRate.Float64,
			},
		}
	}
	return b
}
