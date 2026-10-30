package seeder

import (
	"context"
	"encoding/json"
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
	wg.Add(2)

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

	expensesKind := []models.TxKind{models.ExpenseTxKind}
	incomeKind := []models.TxKind{models.IncomeTxKind}

	expensesRaw, err := json.Marshal(expensesKind)
	if err != nil {
		logger.Get().Error("error while marshaling tx kind while seeding", zap.Error(err))
		return
	}

	incomeRaw, err := json.Marshal(incomeKind)
	if err != nil {
		logger.Get().Error("error while marshaling tx kind while seeding", zap.Error(err))
		return
	}

	seed := []queries.CreateCategoryParams{
		{ID: idgen.New(), Name: "Salary", Color: models.GetRandomCategoryColor(), Kinds: incomeRaw},
		{ID: idgen.New(), Name: "Groceries", Color: models.GetRandomCategoryColor(), Kinds: expensesRaw},
		{ID: idgen.New(), Name: "Transportation", Color: models.GetRandomCategoryColor(), Kinds: expensesRaw},
	}

	seedFn(ctx, q.GetCategories, seed, q.CreateCategory,
		func(s queries.CreateCategoryParams) string { return s.Name },
		func(e queries.GetCategoriesRow) string { return e.Category.Name })
}

func (s *Seeder) currencies(ctx context.Context, wg *sync.WaitGroup, q *queries.Queries) {
	defer wg.Done()

	seedFn(ctx, q.GetAllCurrencies, currSeed, q.CreateCurrency,
		func(s queries.CreateCurrencyParams) string { return s.String },
		func(e queries.GetAllCurrenciesRow) string { return e.Currency.String })
}

func (s *Seeder) accountKinds(ctx context.Context, wg *sync.WaitGroup, q *queries.Queries) {
	defer wg.Done()

	seed := []queries.CreateAccountKindParams{
		{ID: idgen.New(), Enum: int64(models.AccKindCreditCard)},
		{ID: idgen.New(), Enum: int64(models.AccKindBankAcc)},
		{ID: idgen.New(), Enum: int64(models.AccKindCash)},
		{ID: idgen.New(), Enum: int64(models.AccKindDebt)},
		{ID: idgen.New(), Enum: int64(models.AccKindGamyaa)},
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
