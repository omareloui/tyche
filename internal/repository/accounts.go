package repository

import (
	"context"
	"database/sql"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) CreateAccount(ctx context.Context, a *models.Account) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return r.createAccountWithTxn(q, ctx, a)
}

func (r *Repository) createAccountWithTxn(q *queries.Queries, ctx context.Context, a *models.Account) error {
	l := logger.FromCtx(ctx)

	cpId := ""
	if a.Counterparty != nil {
		cpId = a.Counterparty.ID
	}

	smsRefs, err := marshalStrings(a.SmsRefs)
	if err != nil {
		return err
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
		SmsRefs:        smsRefs,
		CardNumber:     sql.NullString{String: a.CardNumber, Valid: a.CardNumber != ""},

		Note:               sql.NullString{String: a.Note, Valid: a.Note != ""},
		IsArchived:         a.IsArchive,
		ExcludeFromBalance: a.ExcludeFromBalance,
		ExpirationDate:     sql.NullTime{Time: a.ExpirationDate, Valid: !a.ExpirationDate.IsZero()},
		StatementDay:       nullInt(a.Cycle.StatementDay),
		DueDays:            nullInt(a.Cycle.DueDays),
		DueDay:             nullInt(a.Cycle.DueDay),
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

	res := dbAccountsRowToAccounts(accs)
	if err := r.attachGamyaas(ctx, q, res); err != nil {
		return nil, err
	}
	return res, nil
}

// AttachCardStatements sets the last statement of accs' credit cards with a
// statement cycle, as of today in the user's time zone.
func (r *Repository) AttachCardStatements(ctx context.Context, accs []models.Account) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	var today time.Time
	for i := range accs {
		a := &accs[i]
		if a.Kind.Enum != models.AccKindCreditCard || !a.Cycle.IsSet() || a.IsDeleted() {
			continue
		}
		if today.IsZero() {
			loc, err := r.GetTimezone(ctx)
			if err != nil {
				return err
			}
			today = time.Now().In(loc)
		}
		act, err := q.GetCardActivitySince(ctx, queries.GetCardActivitySinceParams{
			AccountID: sql.NullString{String: a.ID, Valid: true},
			Since:     a.Cycle.LastClose(today).AddDate(0, 0, 1),
			Until:     a.Cycle.NextClose(today).AddDate(0, 0, 1),
		})
		if err != nil {
			l.Error("querying card activity", zap.Error(err), zap.String("id", a.ID))
			return err
		}
		a.Statement = models.NewCardStatement(a, today, act.Net, act.PaidIn, act.Scheduled)
	}
	return nil
}

// GetCardStatements is the active credit cards with a statement cycle, with
// their statements, the next to close first.
func (r *Repository) GetCardStatements(ctx context.Context) ([]models.Account, error) {
	accs, err := r.GetAccounts(ctx)
	if err != nil {
		return nil, err
	}
	accs = slices.DeleteFunc(accs, func(a models.Account) bool {
		return a.IsArchive || a.Kind.Enum != models.AccKindCreditCard || !a.Cycle.IsSet()
	})
	if err := r.AttachCardStatements(ctx, accs); err != nil {
		return nil, err
	}
	slices.SortFunc(accs, func(a, b models.Account) int {
		return a.Statement.NextClose().Compare(b.Statement.NextClose())
	})
	return accs, nil
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

	res := dbAccountRowToAccount(&acc)
	if res.Kind.Enum == models.AccKindGamyaa {
		accs := []models.Account{*res}
		if err := r.attachGamyaas(ctx, q, accs); err != nil {
			return nil, err
		}
		res = &accs[0]
	}
	if res.Cycle.IsSet() {
		accs := []models.Account{*res}
		if err := r.AttachCardStatements(ctx, accs); err != nil {
			return nil, err
		}
		res = &accs[0]
	}
	return res, nil
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

	smsRefs, err := marshalStrings(a.SmsRefs)
	if err != nil {
		return err
	}

	if err := q.UpdateAccount(ctx, queries.UpdateAccountParams{
		ID:                 a.ID,
		Name:               a.Name,
		KindID:             a.Kind.ID,
		CurrencyID:         a.Currency.ID,
		CounterpartyID:     sql.NullString{String: cpId, Valid: cpId != ""},
		Iban:               sql.NullString{String: a.Iban, Valid: a.Iban != ""},
		SmsRefs:            smsRefs,
		CardNumber:         sql.NullString{String: a.CardNumber, Valid: a.CardNumber != ""},
		NameOnCard:         sql.NullString{String: a.NameOnCard, Valid: a.NameOnCard != ""},
		CardLimit:          a.CardLimit,
		Note:               sql.NullString{String: a.Note, Valid: a.Note != ""},
		IsArchived:         a.IsArchive,
		ExcludeFromBalance: a.ExcludeFromBalance,
		ExpirationDate:     sql.NullTime{Time: a.ExpirationDate, Valid: !a.ExpirationDate.IsZero()},
		StatementDay:       nullInt(a.Cycle.StatementDay),
		DueDays:            nullInt(a.Cycle.DueDays),
		DueDay:             nullInt(a.Cycle.DueDay),
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

// DeleteAccount soft deletes the account: it's hidden everywhere but in the
// history of its transactions. Its balance is moved first with moveBalance
// (if any); its recurrent and scheduled transactions go, and it stops being
// the default account.
func (r *Repository) DeleteAccount(ctx context.Context, id string, moveBalance *models.Transaction) error {
	l := logger.FromCtx(ctx)
	return r.transact(ctx, func(qtx *queries.Queries) error {
		steps := []func(context.Context, string) error{
			qtx.DeleteRecurrentTxsOfAccount,
			qtx.DeleteScheduledTxsOfAccount,
			func(ctx context.Context, id string) error {
				return qtx.ClearDefaultAccount(ctx, sql.NullString{String: id, Valid: true})
			},
		}
		for _, step := range steps {
			if err := step(ctx, id); err != nil {
				l.Error("deleting account", zap.Error(err), zap.String("id", id))
				return err
			}
		}
		if moveBalance != nil {
			if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, moveBalance); err != nil {
				return err
			}
		}
		if err := qtx.DeleteAccount(ctx, id); err != nil {
			l.Error("deleting account", zap.Error(err), zap.String("id", id))
			return err
		}
		l.Debug("deleted account", zap.String("id", id))
		return nil
	})
}

// FixBalances sets each account's balance to its fix, recording the
// difference as an adjustment transaction fulfilled at `at`. It returns how
// many accounts changed.
func (r *Repository) FixBalances(ctx context.Context, fixes []models.BalanceFix, at time.Time) (int, error) {
	l := logger.FromCtx(ctx)
	changed := 0
	err := r.transact(ctx, func(qtx *queries.Queries) error {
		for _, f := range fixes {
			diff, err := balanceFixDiff(ctx, qtx, f)
			if err != nil {
				return err
			}
			if diff == 0 {
				continue
			}
			changed++

			tx := &models.Transaction{
				ID:          idgen.New(),
				Account:     models.Account{ID: f.AccountID},
				Amount:      diff,
				Kind:        models.AdjustmentTxKind,
				Description: models.BalanceFixDescription,
				ScheduledTo: at,
				FulfilledAt: at,
			}
			if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, tx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	l.Debug("fixed balances", zap.Int("changed", changed))
	return changed, nil
}

// SetBalances overwrites each account's balance with its fix, leaving no
// transaction (the onboarding's starting balances). It returns how many
// accounts changed.
func (r *Repository) SetBalances(ctx context.Context, fixes []models.BalanceFix) (int, error) {
	l := logger.FromCtx(ctx)
	changed := 0
	err := r.transact(ctx, func(qtx *queries.Queries) error {
		for _, f := range fixes {
			diff, err := balanceFixDiff(ctx, qtx, f)
			if err != nil {
				return err
			}
			if diff == 0 {
				continue
			}
			changed++
			if err := qtx.SetAccountAmount(ctx, queries.SetAccountAmountParams{ID: f.AccountID, Amount: f.Amount}); err != nil {
				l.Error("setting account amount", zap.Error(err), zap.String("id", f.AccountID))
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	l.Debug("set balances", zap.Int("changed", changed))
	return changed, nil
}

// balanceFixDiff is what f adds to its account's balance.
func balanceFixDiff(ctx context.Context, qtx *queries.Queries, f models.BalanceFix) (float64, error) {
	cur, err := qtx.GetAccountAmount(ctx, f.AccountID)
	if err != nil {
		logger.FromCtx(ctx).Error("getting account amount", zap.Error(err), zap.String("id", f.AccountID))
		return 0, err
	}
	// 6 digits: a metal's troy ounces need them (see Currency.Round).
	return math.Round((f.Amount-cur)*1e6) / 1e6, nil
}

func (r *Repository) applyTxToAccountWithTxn(qtx *queries.Queries, ctx context.Context, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)

	if tx.Kind == models.TransferTxKind || tx.Kind == models.DebtTxKind {
		if err := r.removeFromAccountWithTxn(qtx, ctx, tx.Account.ID, tx.Amount); err != nil {
			l.Error("applying transfer: remove from account", zap.Error(err), zap.String("id", tx.ID))
			return err
		}
		l.Debug("applied transfer: remove from account")
		if err := r.addToAccountWithTxn(qtx, ctx, tx.ToAccount.ID, tx.ToAmount); err != nil {
			l.Error("applying transfer: add to account", zap.Error(err), zap.String("id", tx.ID))
			return err
		}
		l.Debug("applied transfer: add to account")
	} else if tx.Kind == models.AdjustmentTxKind {
		// Signed, added as is.
		if err := r.addToAccountWithTxn(qtx, ctx, tx.Account.ID, tx.Amount); err != nil {
			l.Error("applying adjustment to account", zap.Error(err), zap.String("id", tx.ID))
			return err
		}
	} else {
		upBalance := r.removeFromAccountWithTxn
		if tx.Kind.IsIncoming() {
			upBalance = r.addToAccountWithTxn
		}
		if err := upBalance(qtx, ctx, tx.Account.ID, tx.Amount); err != nil {
			l.Error("applying transaction to account", zap.Error(err), zap.String("id", tx.ID))
			return err
		}
		l.Debug("applied transaction to account", zap.String("id", tx.ID))
	}
	return nil
}

func (r *Repository) undoTxToAccountWithTxn(qtx *queries.Queries, ctx context.Context, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)
	if tx.Kind == models.TransferTxKind || tx.Kind == models.DebtTxKind {
		if err := r.addToAccountWithTxn(qtx, ctx, tx.Account.ID, tx.Amount); err != nil {
			l.Error("undoing transfer: remove from account", zap.Error(err), zap.String("id", tx.ID))
			return err
		}
		l.Debug("undone transfer: remove from account")
		if err := r.removeFromAccountWithTxn(qtx, ctx, tx.ToAccount.ID, tx.ToAmount); err != nil {
			l.Error("undoing transfer: add to account", zap.Error(err), zap.String("id", tx.ID))
			return err
		}
		l.Debug("undone transfer: add to account")
	} else if tx.Kind == models.AdjustmentTxKind {
		if err := r.removeFromAccountWithTxn(qtx, ctx, tx.Account.ID, tx.Amount); err != nil {
			l.Error("undoing adjustment to account", zap.Error(err), zap.String("id", tx.ID))
			return err
		}
	} else {
		upBalance := r.addToAccountWithTxn
		if tx.Kind.IsIncoming() {
			upBalance = r.removeFromAccountWithTxn
		}
		if err := upBalance(qtx, ctx, tx.Account.ID, tx.Amount); err != nil {
			l.Error("undoing transaction to account", zap.Error(err), zap.String("id", tx.ID))
			return err
		}
		l.Debug("undone transaction to account", zap.String("id", tx.ID))
	}
	return nil
}

func (r *Repository) addToAccountWithTxn(qtx *queries.Queries, ctx context.Context, id string, amount float64) error {
	err := qtx.AddToAccount(ctx, queries.AddToAccountParams{ID: id, Amount: amount})
	if err != nil {
		l := logger.FromCtx(ctx)
		l.Error("adding to account", zap.Error(err), zap.String("id", id))
		return err
	}
	return nil
}

func (r *Repository) removeFromAccountWithTxn(qtx *queries.Queries, ctx context.Context, id string, amount float64) error {
	err := qtx.RemoveFromAccount(ctx, queries.RemoveFromAccountParams{ID: id, Amount: amount})
	if err != nil {
		l := logger.FromCtx(ctx)
		l.Error("removing from account", zap.Error(err), zap.String("id", id))
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
	dbAcc *queries.Account,
	cur *queries.Currency, k *queries.AccountKind,
	cpName *sql.NullString, isDefault bool,
) *models.Account {
	smsRefs, _ := unmarshalStrings(dbAcc.SmsRefs)
	acc := &models.Account{
		ID:                 dbAcc.ID,
		Name:               dbAcc.Name,
		Amount:             dbAcc.Amount,
		IsDefault:          isDefault,
		NameOnCard:         dbAcc.NameOnCard.String,
		CardLimit:          dbAcc.CardLimit,
		Iban:               dbAcc.Iban.String,
		SmsRefs:            smsRefs,
		CardNumber:         dbAcc.CardNumber.String,
		Note:               dbAcc.Note.String,
		IsArchive:          dbAcc.IsArchived,
		ExcludeFromBalance: dbAcc.ExcludeFromBalance,
		ExpirationDate:     dbAcc.ExpirationDate.Time,
		CreatedAt:          dbAcc.CreatedAt,
		DeletedAt:          dbAcc.DeletedAt.Time,
		Cycle: models.CardCycle{
			StatementDay: int(dbAcc.StatementDay.Int64),
			DueDays:      int(dbAcc.DueDays.Int64),
			DueDay:       int(dbAcc.DueDay.Int64),
		},

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
	acc := dbAccountToAccount(
		&_dbAcc.Account,
		&_dbAcc.Currency,
		&_dbAcc.AccountKind,
		&_dbAcc.CounterpartyName,
		_dbAcc.IsDefault.(int64) == 1)
	acc.SetAside = _dbAcc.SetAside
	if acc.Counterparty != nil {
		acc.Counterparty.Icon = _dbAcc.CounterpartyIcon.String
		acc.Counterparty.Color = _dbAcc.CounterpartyColor.String
	}
	return acc
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
				ID:      dbB.CurrencyID,
				String:  dbB.Currency.String,
				Symbol:  dbB.Currency.Symbol,
				InGrams: dbB.Currency.InGrams,
			},
		}
	}
	return b
}

func nullInt(n int) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(n), Valid: n != 0}
}

// GetAccountHistory is the account's balance day by day over the last
// models.AccountHistoryMonths months, "today" in the user's time zone. A
// gam'yaa's spans its whole life instead, projected through its scheduled
// installments and payouts.
func (r *Repository) GetAccountHistory(ctx context.Context, a *models.Account) (*models.AccountHistory, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	loc, err := r.GetTimezone(ctx)
	if err != nil {
		return nil, err
	}
	today := time.Now().In(loc)
	start := monthStartFromCtx(ctx)
	from, to := models.AccountHistoryFrom(start, today), today
	if g := a.Gamyaa; g != nil {
		from, to = models.GamyaaHistoryRange(g)
	}

	rows, err := q.GetAccountDailyNet(ctx, queries.GetAccountDailyNetParams{
		AccountID: sql.NullString{String: a.ID, Valid: true},
		Since:     from,
	})
	if err != nil {
		logger.FromCtx(ctx).Error("querying account daily net", zap.Error(err), zap.String("id", a.ID))
		return nil, err
	}
	days := make([]models.DayFlow, 0, len(rows))
	for _, row := range rows {
		if d, ok := dayFlow(row.Day, row.MoneyIn, row.MoneyOut); ok {
			days = append(days, d)
		}
	}

	var scheduled []models.DayFlow
	if to.After(today) {
		rows, err := q.GetAccountScheduledDailyNet(ctx, queries.GetAccountScheduledDailyNetParams{
			AccountID: sql.NullString{String: a.ID, Valid: true},
			Until:     to.AddDate(0, 0, 1),
		})
		if err != nil {
			logger.FromCtx(ctx).Error("querying account scheduled daily net", zap.Error(err), zap.String("id", a.ID))
			return nil, err
		}
		for _, row := range rows {
			if d, ok := dayFlow(row.Day, row.MoneyIn, row.MoneyOut); ok {
				scheduled = append(scheduled, d)
			}
		}
	}
	return models.NewAccountHistory(start, a.Amount, days, scheduled, from, today, to), nil
}

func dayFlow(day string, in, out float64) (models.DayFlow, bool) {
	d, err := time.Parse(dayLayout, day)
	return models.DayFlow{Day: d, In: in, Out: out}, err == nil
}

// GetAccountTxStats is how many fulfilled transactions touched the account,
// and when the first and last were.
func (r *Repository) GetAccountTxStats(ctx context.Context, id string) (*models.AccountTxStats, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := q.GetAccountTxStats(ctx, id)
	if err != nil {
		logger.FromCtx(ctx).Error("querying account tx stats", zap.Error(err), zap.String("id", id))
		return nil, err
	}
	s := &models.AccountTxStats{Count: int(row.Count)}
	if len(row.FirstAt) >= len(dayLayout) {
		s.First, _ = time.Parse(dayLayout, row.FirstAt[:len(dayLayout)])
	}
	if len(row.LastAt) >= len(dayLayout) {
		s.Last, _ = time.Parse(dayLayout, row.LastAt[:len(dayLayout)])
	}
	return s, nil
}

// GetAccountRecentTransactions is the account's last n fulfilled
// transactions, in or out of it as /transactions?account= lists them, newest
// first.
func (r *Repository) GetAccountRecentTransactions(ctx context.Context, id string, n int) ([]models.Transaction, error) {
	p := txFilterParams(&models.TxFilter{AccountID: id})
	p.Fulfilled = true
	p.Lim = int64(n)
	return r.listAccountTransactions(ctx, p)
}

// GetAccountUpcomingTransactions is the account's scheduled transactions due
// before until, overdue ones included, soonest first.
func (r *Repository) GetAccountUpcomingTransactions(ctx context.Context, id string, until time.Time) ([]models.Transaction, error) {
	p := txFilterParams(&models.TxFilter{AccountID: id})
	p.Fulfilled = false
	p.Before = until.UTC()
	p.Lim = -1
	txs, err := r.listAccountTransactions(ctx, p)
	if err != nil {
		return nil, err
	}
	slices.Reverse(txs)
	return txs, nil
}

// listAccountTransactions lists an account's transactions with their
// transfer pairs, as a pair is listed once, by its outgoing leg, even when
// the account got the incoming one.
func (r *Repository) listAccountTransactions(ctx context.Context, p queries.GetTransactionsParams) ([]models.Transaction, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	txs, err := r.listTransactions(ctx, p)
	if err != nil {
		return nil, err
	}
	if err := r.loadTransferPairs(ctx, q, txs); err != nil {
		return nil, err
	}
	return txs, nil
}
