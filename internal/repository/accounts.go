package repository

import (
	"context"
	"database/sql"
	"strings"

	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) CreateAccount(ctx context.Context, a *models.Account) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	cpId := ""
	if a.Counterparty != nil {
		cpId = a.Counterparty.ID
	}

	if err := q.CreateAccount(ctx, queries.CreateAccountParams{
		ID:             a.ID,
		Name:           a.Name,
		KindID:         a.Kind.ID,
		Amount:         a.Amount,
		CurrencyID:     a.Currency.ID,
		CardLimit:      a.CardLimit,
		NameOnCard:     sql.NullString{String: a.NameOnCard, Valid: a.NameOnCard != ""},
		CounterpartyID: sql.NullString{String: cpId, Valid: cpId != ""},
		Iban:           sql.NullString{String: a.Iban, Valid: a.Iban != ""},
		SmsRef:         sql.NullString{String: a.SmsRef, Valid: a.SmsRef != ""},
		CardNumber:     sql.NullString{String: a.CardNumber, Valid: a.CardNumber != ""},

		Note:               sql.NullString{String: a.Note, Valid: a.Note != ""},
		IsArchived:         a.IsArchive,
		ExcludeFromBalance: a.ExcludeFromBalance,
		ExpirationDate:     sql.NullTime{Time: a.ExpirationDate, Valid: !a.ExpirationDate.IsZero()},
	}); err != nil {
		l.Error("creating account", zap.Error(err))
		if strings.Contains(err.Error(), "UNIQUE constraint failed: accounts.name") {
			return newDuplicationError("Name", "CounterpartyID")
		}

		if strings.Contains(err.Error(), "UNIQUE constraint failed: accounts.card_number") {
			return newDuplicationError("CardNumber")
		}

		if strings.Contains(err.Error(), "UNIQUE constraint failed: accounts.iban") {
			return newDuplicationError("Iban")
		}
		return err
	}
	l.Debug("created account")
	return nil
}

func (r *Repository) GetAccounts(ctx context.Context) ([]models.Account, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	accs, err := q.GetAccounts(ctx)
	if err != nil {
		l.Error("querying accounts", zap.Error(err))
		return nil, err
	}
	l.Debug("queried accounts")

	return dbAccountsRowToAccounts(accs), nil
}

func (r *Repository) GetAccount(ctx context.Context, id string) (*models.Account, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	acc, err := q.GetAccount(ctx, id)
	if err != nil {
		l.Error("querying account", zap.Error(err))
		return nil, err
	}
	l.Debug("queried account")
	return dbAccountRowToAccount(&acc), nil
}

func (r *Repository) UpdateAccount(ctx context.Context, a *models.Account) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	cpId := ""
	if a.Counterparty != nil {
		cpId = a.Counterparty.ID
	}

	if err := q.UpdateAccount(ctx, queries.UpdateAccountParams{
		ID:                 a.ID,
		Name:               a.Name,
		KindID:             a.Kind.ID,
		CurrencyID:         a.Currency.ID,
		CounterpartyID:     sql.NullString{String: cpId, Valid: cpId != ""},
		Iban:               sql.NullString{String: a.Iban, Valid: a.Iban != ""},
		SmsRef:             sql.NullString{String: a.SmsRef, Valid: a.SmsRef != ""},
		CardNumber:         sql.NullString{String: a.CardNumber, Valid: a.CardNumber != ""},
		NameOnCard:         sql.NullString{String: a.NameOnCard, Valid: a.NameOnCard != ""},
		CardLimit:          a.CardLimit,
		Note:               sql.NullString{String: a.Note, Valid: a.Note != ""},
		IsArchived:         a.IsArchive,
		ExcludeFromBalance: a.ExcludeFromBalance,
		ExpirationDate:     sql.NullTime{Time: a.ExpirationDate, Valid: !a.ExpirationDate.IsZero()},
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

func (r *Repository) DeleteAccount(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	if err := q.DeleteAccount(ctx, id); err != nil {
		l.Error("deleting account", zap.Error(err))
		return err
	}
	l.Debug("deleted account")
	return nil
}

func (r *Repository) applyTxToAccountWithTxn(qtx *queries.Queries, ctx context.Context, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)

	if tx.Kind == models.TransferTxKind || tx.Kind == models.DebtTxKind {
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
		if tx.Kind == models.IncomeTxKind {
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

func (r *Repository) undoTxToAccountWithTxn(qtx *queries.Queries, ctx context.Context, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)
	if tx.Kind == models.TransferTxKind {
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
		if tx.Kind == models.IncomeTxKind {
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

func (r *Repository) addToAccountWithTxn(qtx *queries.Queries, ctx context.Context, id string, amount float64) error {
	err := qtx.AddToAccount(ctx, queries.AddToAccountParams{ID: id, Amount: amount})
	if err != nil {
		l := logger.FromCtx(ctx)
		l.Error("adding to account", zap.Error(err), zap.String("id", id), zap.Float64("amount", amount))
		return err
	}
	return nil
}

func (r *Repository) removeFromAccountWithTxn(qtx *queries.Queries, ctx context.Context, id string, amount float64) error {
	err := qtx.RemoveFromAccount(ctx, queries.RemoveFromAccountParams{ID: id, Amount: amount})
	if err != nil {
		l := logger.FromCtx(ctx)
		l.Error("removing from account", zap.Error(err), zap.String("id", id), zap.Float64("amount", amount))
		return err
	}
	return nil
}

func (r *Repository) GetBalances(ctx context.Context) ([]models.Balance, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbBalances, err := q.GetBalances(ctx)
	if err != nil {
		l.Error("querying balances", zap.Error(err))
		return nil, err
	}

	balances := dbBalancesToBalances(dbBalances)

	var claims *jwt.AccessClaims
	if v := ctx.Value(middleware.AccessClaimsCtxKey{}); v != nil {
		claims = v.(*jwt.AccessClaims)
	}

	if claims == nil {
		return balances, nil
	}

	var sum float64
	for _, b := range balances {
		res := b.AmountWithCurrency(&claims.DefaultCurrency)
		sum += res
	}

	return []models.Balance{{Amount: sum, Currency: claims.DefaultCurrency}}, nil
}

func dbAccountToAccount(
	dbAcc *queries.Account, actualAmount float64,
	cur *queries.Currency, k *queries.AccountKind,
	cpName *sql.NullString, isDefault bool,
) *models.Account {
	acc := &models.Account{
		ID:                 dbAcc.ID,
		Name:               dbAcc.Name,
		Amount:             dbAcc.Amount,
		ActualAmount:       actualAmount,
		IsDefault:          isDefault,
		NameOnCard:         dbAcc.NameOnCard.String,
		CardLimit:          dbAcc.CardLimit,
		Iban:               dbAcc.Iban.String,
		SmsRef:             dbAcc.SmsRef.String,
		CardNumber:         dbAcc.CardNumber.String,
		Note:               dbAcc.Note.String,
		IsArchive:          dbAcc.IsArchived,
		ExcludeFromBalance: dbAcc.ExcludeFromBalance,
		ExpirationDate:     dbAcc.ExpirationDate.Time,
		CreatedAt:          dbAcc.CreatedAt,

		Currency: models.Currency{ID: dbAcc.CurrencyID},
		Kind:     models.AccountKind{ID: dbAcc.KindID},
	}
	if cur != nil {
		acc.Currency = *dbCurrencyToCurrency(cur, false)
	}

	if dbAcc.CounterpartyID.Valid {
		acc.Counterparty = &models.Counterparty{ID: dbAcc.CounterpartyID.String}
		if cpName != nil && cpName.Valid {
			acc.Counterparty.Name = cpName.String
		}
	}

	if k != nil {
		acc.Kind = *dbAccountKindToAccountKind(k)
	}
	return acc
}

func dbAccountRowToAccount[T queries.GetAccountRow | queries.GetAccountsRow](dbAcc *T) *models.Account {
	_dbAcc := queries.GetAccountRow(*dbAcc)
	return dbAccountToAccount(
		&_dbAcc.Account,
		_dbAcc.ActualAmount.(float64),
		&_dbAcc.Currency,
		&_dbAcc.AccountKind,
		&_dbAcc.CounterpartyName,
		_dbAcc.IsDefault.(int64) == 1)
}

func dbAccountsRowToAccounts(dbAccs []queries.GetAccountsRow) []models.Account {
	acc := make([]models.Account, len(dbAccs))
	for i, dbAcc := range dbAccs {
		acc[i] = *dbAccountRowToAccount(&dbAcc)
	}
	return acc
}

func dbBalancesToBalances(dbBs []queries.GetBalancesRow) []models.Balance {
	b := make([]models.Balance, len(dbBs))
	for i, dbB := range dbBs {
		b[i] = models.Balance{
			Amount: dbB.Balance,
			Currency: models.Currency{
				ID:     dbB.CurrencyID,
				String: dbB.Currency.String,
				Symbol: dbB.Currency.Symbol,
			},
		}
	}
	return b
}
