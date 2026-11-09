package seeder

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	_ "github.com/omareloui/tyche/internal/sqlite"
	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/dbtypes"
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

// Seed seeds the tenant's database in one transaction: committing each of the
// hundreds of rows on its own takes tens of seconds on the first sign in.
func (s *Seeder) Seed(ctx context.Context, db *sql.DB, oauthID string) {
	l := logger.FromCtx(ctx)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		l.Error("starting the seed transaction", zap.Error(err))
		return
	}
	defer tx.Rollback()
	q := queries.New(tx)

	var _user *queries.GetUserByOAuthIDRow

	user, err := q.GetUserByOAuthID(ctx, oauthID)
	if err != nil {
		l.Warn("couldn't get the user to check seed time", zap.Error(err))
	} else {
		_user = &user
	}

	s.currencies(ctx, q)
	s.accountKinds(ctx, q)
	s.goalAccounts(ctx, q)
	s.goalHoldings(ctx, tx, q)

	l.Debug("ensured seeding currencies, account kinds, and transaction kinds")

	if _user == nil || user.User.SeededAt.Time.IsZero() {
		s.categories(ctx, q)
		s.accounts(ctx, q)
		l.Debug("seeded categories, and accounts")
	}

	s.updateSeededAt(ctx, q, oauthID, _user)

	if err := tx.Commit(); err != nil {
		l.Error("committing the seed transaction", zap.Error(err))
	}
}

func (s *Seeder) updateSeededAt(ctx context.Context, q *queries.Queries, oauthID string, user *queries.GetUserByOAuthIDRow) {
	if user == nil {
		return
	}

	if err := q.UpdateSeededAt(ctx, oauthID); err != nil {
		l := logger.Get()
		l.Error("error updating seed time for user", zap.Error(err))
	}
}

func (s *Seeder) accounts(ctx context.Context, q *queries.Queries) {
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
		{ID: idgen.New(), Name: "Cash", KindID: cashKnd.AccountKind.ID, CurrencyID: cur.Currency.ID, SmsRefs: dbtypes.JSON("[]")},
	}

	seedFn(ctx, q.GetAccounts, seed, q.CreateAccount,
		func(s queries.CreateAccountParams) string { return s.Name + s.KindID + s.CurrencyID },
		func(e queries.GetAccountsRow) string { return e.Account.Name + e.AccountKind.ID + e.Currency.ID })
}

func (s *Seeder) categories(ctx context.Context, q *queries.Queries) {
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

func (s *Seeder) currencies(ctx context.Context, q *queries.Queries) {
	seedFn(ctx, q.GetAllCurrencies, currSeed, q.CreateCurrency,
		func(s queries.CreateCurrencyParams) string { return s.String },
		func(e queries.GetAllCurrenciesRow) string { return e.Currency.String })
}

func (s *Seeder) accountKinds(ctx context.Context, q *queries.Queries) {
	seed := []queries.CreateAccountKindParams{
		{ID: idgen.New(), Enum: int64(models.AccKindCreditCard)},
		{ID: idgen.New(), Enum: int64(models.AccKindBankAcc)},
		{ID: idgen.New(), Enum: int64(models.AccKindCash)},
		{ID: idgen.New(), Enum: int64(models.AccKindDebt)},
		{ID: idgen.New(), Enum: int64(models.AccKindGamyaa)},
		{ID: idgen.New(), Enum: int64(models.AccKindGoal)},
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

// ShownCurrencies are the codes of the currencies shown by default.
func ShownCurrencies() []string {
	var codes []string
	for _, c := range currSeed {
		if c.IsShown {
			codes = append(codes, c.String)
		}
	}
	return codes
}

// goalAccounts gives the goals without an account theirs (goals made before
// they had one, or imported from such a backup).
func (s *Seeder) goalAccounts(ctx context.Context, q *queries.Queries) {
	l := logger.FromCtx(ctx)

	gls, err := q.GetGoalsWithoutAccount(ctx)
	if err != nil || len(gls) == 0 {
		if err != nil {
			l.Error("getting goals without an account", zap.Error(err))
		}
		return
	}
	kindID, err := q.GetAccountKindID(ctx, int64(models.AccKindGoal))
	if err != nil {
		l.Error("getting the goal account kind", zap.Error(err))
		return
	}

	for _, gl := range gls {
		id := idgen.New()
		// Goals' names aren't unique, their accounts' are.
		name := models.GoalAccountName(gl.Name, gl.Currency)
		for n := 2; ; n++ {
			err = q.CreateAccount(ctx, queries.CreateAccountParams{
				ID: id, Name: name, KindID: kindID, CurrencyID: gl.CurrencyID,
				ExcludeFromBalance: true, SmsRefs: dbtypes.JSON("[]"),
			})
			if err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed: accounts.name") || n > 50 {
				break
			}
			name = fmt.Sprintf("%s (%d)", models.GoalAccountName(gl.Name, gl.Currency), n)
		}
		if err != nil {
			l.Error("creating goal account", zap.String("goal", gl.ID), zap.Error(err))
			continue
		}
		if err := q.SetGoalAccount(ctx, queries.SetGoalAccountParams{ID: gl.ID, AccountID: sql.NullString{String: id, Valid: true}}); err != nil {
			l.Error("setting goal account", zap.String("goal", gl.ID), zap.Error(err))
		}
	}
}
