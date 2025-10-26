package seeder

import (
	"context"
	"database/sql"
	"slices"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"github.com/omareloui/tyche/internal/slicesutils"
)

type Seeder struct{}

func NewSeeder() *Seeder {
	return &Seeder{}
}

func (s *Seeder) Seed(ctx context.Context, q *queries.Queries, oauthID string) {
	wg := new(sync.WaitGroup)

	l := logger.FromCtx(ctx)

	var _user *queries.GetUserByOAuthIDRow

	user, err := q.GetUserByOAuthID(ctx, oauthID)
	if err != nil {
		l.Warn("couldn't get the user to check seed time", zap.String("oauth", oauthID), zap.Error(err))
	} else {
		_user = &user
	}

	defer s.updateSeededAt(ctx, q, oauthID, _user)

	go s.currencies(ctx, wg, q)
	go s.accountKinds(ctx, wg, q)
	go s.txKinds(ctx, wg, q)
	wg.Add(3)

	wg.Wait()

	l.Debug("ensured seeding currencies, account kinds, and transaction kinds", zap.String("oauthID", oauthID))

	if _user != nil && !user.User.SeededAt.Time.IsZero() {
		return
	}

	go s.categories(ctx, wg, q)
	go s.accounts(ctx, wg, q)
	wg.Add(2)

	l.Debug("seeded categories, and accounts", zap.String("oauthID", oauthID))

	wg.Wait()
}

func (s *Seeder) updateSeededAt(ctx context.Context, q *queries.Queries, oauthID string, user *queries.GetUserByOAuthIDRow) {
	if user == nil {
		return
	}

	if err := q.UpdateSeededAt(ctx, oauthID); err != nil {
		l := logger.Get()
		l.Error("error updating seed time for user", zap.String("oauthID", oauthID), zap.Error(err))
	}
}

func (s *Seeder) accounts(ctx context.Context, wg *sync.WaitGroup, q *queries.Queries) {
	defer wg.Done()

	l := logger.FromCtx(ctx)

	kinds, err := q.GetAccountKinds(ctx)
	if err != nil {
		l.Error("error seeding accounts, could't get kinds", zap.Error(err))
		return
	}

	cashKnd := slicesutils.Find(kinds, func(k queries.GetAccountKindsRow) bool {
		return k.AccountKind.Enum == int64(models.AccKindCash)
	})
	if cashKnd == nil {
		l := logger.FromCtx(ctx)
		l.Error("Couldn't find kind", zap.String("kind", models.IncomeTxKind.String()), zap.Error(err))
		return
	}

	currencies, err := q.GetAllCurrencies(ctx)
	if err != nil {
		l.Error("error seeding accounts, could't get currencies", zap.Error(err))
		return
	}

	cur, err := q.GetDefaultCurrency(ctx)
	if err != nil {
		if _cur := slicesutils.Find(currencies,
			func(c queries.GetAllCurrenciesRow) bool {
				return c.Currency.String == models.GetDefaultCurrency().String
			}); _cur != nil {
			cur = queries.GetDefaultCurrencyRow(*_cur)
		}
	}

	seed := []queries.CreateAccountParams{
		{ID: idgen.New(), Name: "Cash", KindID: cashKnd.AccountKind.ID, CurrencyID: cur.Currency.ID},
	}

	seedFn(ctx, q.GetAccounts, seed, q.CreateAccount,
		func(s queries.CreateAccountParams) string { return s.Name + s.KindID + s.CurrencyID },
		func(e queries.GetAccountsRow) string { return e.Account.Name + e.AccountKind.ID + e.Currency.ID })
}

func (s *Seeder) categories(ctx context.Context, wg *sync.WaitGroup, q *queries.Queries) {
	defer wg.Done()

	l := logger.FromCtx(ctx)

	kinds, err := q.GetTransactionKinds(ctx)
	if err != nil {
		l.Error("error seeding categories, could't get kinds", zap.Error(err))
		return
	}

	incomeKnd := slicesutils.Find(kinds, func(k queries.GetTransactionKindsRow) bool {
		return k.TransactionKind.Enum == int64(models.IncomeTxKind)
	})
	if incomeKnd == nil {
		l := logger.FromCtx(ctx)
		l.Error("Couldn't find kind", zap.String("kind", models.IncomeTxKind.String()), zap.Error(err))
		return
	}
	incomeId := incomeKnd.TransactionKind.ID

	expensiveKnd := slicesutils.Find(kinds, func(k queries.GetTransactionKindsRow) bool {
		return k.TransactionKind.Enum == int64(models.ExpenseTxKind)
	})
	if expensiveKnd == nil {
		l := logger.FromCtx(ctx)
		l.Error("Couldn't find kind", zap.String("kind", models.ExpenseTxKind.String()), zap.Error(err))
		return
	}
	expensiveId := expensiveKnd.TransactionKind.ID

	seed := []queries.CreateCategoryParams{
		{ID: idgen.New(), Name: "Salary", Color: models.GetRandomCategoryColor(), KindID: incomeId},
		{ID: idgen.New(), Name: "Groceries", Color: models.GetRandomCategoryColor(), KindID: expensiveId},
		{ID: idgen.New(), Name: "Transportation", Color: models.GetRandomCategoryColor(), KindID: expensiveId},
	}

	seedFn(ctx, q.GetCategories, seed, q.CreateCategory,
		func(s queries.CreateCategoryParams) string { return s.Name + s.KindID },
		func(e queries.GetCategoriesRow) string { return e.Category.Name + e.TransactionKind.ID })
}

func (s *Seeder) currencies(ctx context.Context, wg *sync.WaitGroup, q *queries.Queries) {
	defer wg.Done()

	seedFn(ctx, q.GetAllCurrencies, currSeed, q.CreateCurrency,
		func(s queries.CreateCurrencyParams) string { return s.String },
		func(e queries.GetAllCurrenciesRow) string { return e.Currency.String })
}

func (s *Seeder) txKinds(ctx context.Context, wg *sync.WaitGroup, q *queries.Queries) {
	defer wg.Done()

	seed := []queries.CreateTransactionKindParams{
		{ID: idgen.New(), Enum: int64(models.ExpenseTxKind), IncreaseBalance: sql.NullBool{Bool: false, Valid: true}},
		{ID: idgen.New(), Enum: int64(models.IncomeTxKind), IncreaseBalance: sql.NullBool{Bool: true, Valid: true}},
		{ID: idgen.New(), Enum: int64(models.TransferTxKind), IncreaseBalance: sql.NullBool{Valid: false}},
		{ID: idgen.New(), Enum: int64(models.DebtTxKind), IncreaseBalance: sql.NullBool{Valid: false}},
	}

	seedFn(ctx, q.GetTransactionKinds, seed, q.CreateTransactionKind,
		func(s queries.CreateTransactionKindParams) int64 { return s.Enum },
		func(e queries.GetTransactionKindsRow) int64 { return e.TransactionKind.Enum })
}

func (s *Seeder) accountKinds(ctx context.Context, wg *sync.WaitGroup, q *queries.Queries) {
	defer wg.Done()

	seed := []queries.CreateAccountKindParams{
		{ID: idgen.New(), Enum: int64(models.AccKindCreditCard)},
		{ID: idgen.New(), Enum: int64(models.AccKindBankAcc)},
		{ID: idgen.New(), Enum: int64(models.AccKindCash)},
		{ID: idgen.New(), Enum: int64(models.AccKindDebt)},
	}

	seedFn(ctx, q.GetAccountKinds, seed, q.CreateAccountKind,
		func(s queries.CreateAccountKindParams) int64 { return s.Enum },
		func(e queries.GetAccountKindsRow) int64 { return e.AccountKind.Enum })
}

func seedFn[E any, S any, ID comparable](ctx context.Context,
	existsFn func(context.Context) ([]E, error), seed []S,
	createFn func(context.Context, S) error,
	seedIDFn func(S) ID, existsIDFn func(E) ID,
) {
	l := logger.FromCtx(ctx)

	exists, err := existsFn(ctx)
	if err != nil {
		l.Error("error while getting exiting data to seed", zap.Error(err))
		return
	}

	for _, s := range seed {
		seededIdx := slices.IndexFunc(exists, func(e E) bool { return existsIDFn(e) == seedIDFn(s) })
		if seededIdx > -1 {
			continue
		}

		if err := createFn(ctx, s); err != nil {
			l.Error("error while seeding", zap.Any("seed", s), zap.Error(err))
		}
	}
}
