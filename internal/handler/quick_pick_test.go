package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/shareddb"
	"github.com/omareloui/tyche/internal/valid"
	"github.com/omareloui/tyche/internal/vault"
)

// pickEnv is a signed up tenant (seeded: accounts, categories) with its
// handler and a context carrying its claims.
type pickEnv struct {
	h    *Handler
	repo *repository.Repository
	ctx  context.Context
	acc  models.Account
}

func newPickEnv(t *testing.T) *pickEnv {
	t.Helper()
	schema, err := filepath.Abs("../../sql")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	t.Chdir(tmp)
	if err := os.Symlink(schema, filepath.Join(tmp, "sql")); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"data", "logs"} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("JWT_SECRET", "test-secret")

	db, q, err := shareddb.Open("data/shared.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	vaults := vault.New(db, q, vault.KDFParams{Time: 1, Memory: 1024, Threads: 1})
	repo := repository.New(vaults, q)
	h := New(repo, valid.New(), sanitizer.New(), nil, nil, nil)

	claims := &jwt.AccessClaims{OAuthID: "picker", OAuthProvider: models.Google, MonthStart: 1}
	ctx := context.WithValue(context.Background(), middleware.AccessClaimsCtxKey{}, claims)
	if err := h.initUser(ctx, &models.User{
		ID: idgen.New(), OAuthID: "picker", OAuthProvider: models.Google,
		Email: "picker@example.com", FirstName: "Pick", LastName: "Er",
	}); err != nil {
		t.Fatal(err)
	}
	accs, err := repo.GetAccounts(ctx)
	if err != nil || len(accs) == 0 {
		t.Fatalf("accounts: %v (%d)", err, len(accs))
	}
	claims.DefaultCurrency = accs[0].Currency
	return &pickEnv{h: h, repo: repo, ctx: ctx, acc: accs[0]}
}

// category is a seeded category taking kind.
func (e *pickEnv) category(t *testing.T, kind models.TxKind, not string) *models.Category {
	t.Helper()
	cats, err := e.repo.GetCategories(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := range cats {
		if cats[i].ID != not && cats[i].HasKind(kind.CategoryKind()) {
			return &cats[i]
		}
	}
	t.Fatalf("no %v category", kind)
	return nil
}

func (e *pickEnv) put(path string, form url.Values, pathValues map[string]string) (*httptest.ResponseRecorder, *http.Request) {
	r := httptest.NewRequest("PUT", path, strings.NewReader(form.Encode())).WithContext(e.ctx)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("HX-Request", "true")
	for k, v := range pathValues {
		r.SetPathValue(k, v)
	}
	return httptest.NewRecorder(), r
}

func (e *pickEnv) expense(t *testing.T, cat *models.Category) *models.Transaction {
	t.Helper()
	now := time.Now().UTC()
	tx := &models.Transaction{
		ID: idgen.New(), Account: e.acc, Amount: 50, Kind: models.ExpenseTxKind,
		Category: cat, ScheduledTo: now, FulfilledAt: now, NeedsReview: true,
	}
	if err := e.repo.CreateTransaction(e.ctx, tx, nil, nil); err != nil {
		t.Fatal(err)
	}
	return tx
}

func statusOf(err error) int {
	var re *resperr.RespError
	if errors.As(err, &re) {
		return re.Code
	}
	return 0
}

func TestSetTxPick(t *testing.T) {
	e := newPickEnv(t)
	food := e.category(t, models.ExpenseTxKind, "")
	tx := e.expense(t, food)
	other := e.category(t, models.ExpenseTxKind, food.ID)

	// A new category, the transaction reviewed.
	w, r := e.put("/", url.Values{"category_id": {other.ID}}, map[string]string{"id": tx.ID, "field": "category"})
	if _, err := e.h.SetTxPick(w, r); err != nil {
		t.Fatalf("pick category: %v", err)
	}
	got, err := e.repo.GetTransaction(e.ctx, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Category == nil || got.Category.ID != other.ID {
		t.Errorf("category = %+v, want %s", got.Category, other.ID)
	}
	if got.NeedsReview {
		t.Error("still needs review after picking")
	}

	// An income category doesn't fit an expense.
	income := e.category(t, models.IncomeTxKind, "")
	if !income.HasKind(models.ExpenseTxKind.CategoryKind()) {
		w, r = e.put("/", url.Values{"category_id": {income.ID}}, map[string]string{"id": tx.ID, "field": "category"})
		if _, err := e.h.SetTxPick(w, r); statusOf(err) != http.StatusUnprocessableEntity {
			t.Errorf("income category on an expense: err = %v, want 422", err)
		}
		if w.Header().Get("HX-Retarget") == "" {
			t.Error("refusal not retargeted to the pick")
		}
	}

	// A counterparty with a default category, on a transaction without one,
	// fills it.
	cp := &models.Counterparty{ID: idgen.New(), Name: "Shop", IsMerchant: true, DefaultCategory: food}
	if err := e.repo.CreateCounterparty(e.ctx, cp); err != nil {
		t.Fatal(err)
	}
	bare := e.expense(t, nil)
	w, r = e.put("/", url.Values{"counterparty_id": {cp.ID}}, map[string]string{"id": bare.ID, "field": "counterparty"})
	if _, err := e.h.SetTxPick(w, r); err != nil {
		t.Fatalf("pick counterparty: %v", err)
	}
	got, err = e.repo.GetTransaction(e.ctx, bare.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Counterparty == nil || got.Counterparty.ID != cp.ID {
		t.Errorf("counterparty = %+v, want %s", got.Counterparty, cp.ID)
	}
	if got.Category == nil || got.Category.ID != food.ID {
		t.Errorf("category = %+v, want the counterparty's default %s", got.Category, food.ID)
	}

	// Clearing it.
	w, r = e.put("/", url.Values{"counterparty_id": {""}}, map[string]string{"id": bare.ID, "field": "counterparty"})
	if _, err := e.h.SetTxPick(w, r); err != nil {
		t.Fatalf("clear counterparty: %v", err)
	}
	if got, _ = e.repo.GetTransaction(e.ctx, bare.ID); got.Counterparty != nil && got.Counterparty.ID != "" {
		t.Errorf("counterparty = %+v, want none", got.Counterparty)
	}

	// Unknown fields aren't picked.
	w, r = e.put("/", url.Values{}, map[string]string{"id": tx.ID, "field": "account"})
	if _, err := e.h.SetTxPick(w, r); statusOf(err) != http.StatusNotFound {
		t.Errorf("unknown field: err = %v, want 404", err)
	}
}

func TestSetCounterpartyCategoryPick(t *testing.T) {
	e := newPickEnv(t)
	food := e.category(t, models.ExpenseTxKind, "")

	shop := &models.Counterparty{ID: idgen.New(), Name: "Shop", IsMerchant: true}
	friend := &models.Counterparty{ID: idgen.New(), Name: "Friend"}
	for _, cp := range []*models.Counterparty{shop, friend} {
		if err := e.repo.CreateCounterparty(e.ctx, cp); err != nil {
			t.Fatal(err)
		}
	}

	w, r := e.put("/", url.Values{"default_category_id": {food.ID}}, map[string]string{"id": shop.ID})
	if _, err := e.h.SetCounterpartyCategoryPick(w, r); err != nil {
		t.Fatalf("pick: %v", err)
	}
	got, err := e.repo.GetCounterparty(e.ctx, shop.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultCategory == nil || got.DefaultCategory.ID != food.ID {
		t.Errorf("default category = %+v, want %s", got.DefaultCategory, food.ID)
	}

	w, r = e.put("/", url.Values{"default_category_id": {food.ID}}, map[string]string{"id": friend.ID})
	if _, err := e.h.SetCounterpartyCategoryPick(w, r); statusOf(err) != http.StatusUnprocessableEntity {
		t.Errorf("non-merchant: err = %v, want 422", err)
	}
}

func TestSetRecurrentTxPick(t *testing.T) {
	e := newPickEnv(t)
	food := e.category(t, models.ExpenseTxKind, "")
	other := e.category(t, models.ExpenseTxKind, food.ID)

	today := e.h.userToday(e.ctx)
	tx := &models.Transaction{
		ID: idgen.New(), Account: e.acc, Amount: 20, Kind: models.ExpenseTxKind,
		Category: food, ScheduledTo: today, FulfilledAt: today,
	}
	rt := &models.RecurrentTx{
		ID: idgen.New(), Account: e.acc, Amount: 20, Kind: models.ExpenseTxKind, Category: food,
		Unit: models.RecurrentUnitMonth, Every: 1, StartsAt: today, Anchor: today, NextIndex: 1,
	}
	tx.RecurrentTx, tx.RecurrentTxID = rt, rt.ID
	if err := e.repo.CreateTransaction(e.ctx, tx, rt, nil); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.TopUpRecurrentTxs(e.ctx); err != nil {
		t.Fatal(err)
	}

	w, r := e.put("/", url.Values{"category_id": {other.ID}}, map[string]string{"id": rt.ID, "field": "category"})
	if _, err := e.h.SetRecurrentTxPick(w, r); err != nil {
		t.Fatalf("pick: %v", err)
	}
	got, err := e.repo.GetRecurrentTx(e.ctx, rt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Category == nil || got.Category.ID != other.ID {
		t.Errorf("category = %+v, want %s", got.Category, other.ID)
	}
	if !got.Anchor.Equal(rt.Anchor) {
		t.Errorf("rescheduled: anchor %v, want %v", got.Anchor, rt.Anchor)
	}
	txs, err := e.repo.GetRecurrentTxTransactions(e.ctx, rt.ID)
	if err != nil {
		t.Fatal(err)
	}
	scheduled := 0
	for _, o := range txs {
		if !o.FulfilledAt.IsZero() {
			continue
		}
		scheduled++
		if o.Category == nil || o.Category.ID != other.ID {
			t.Errorf("scheduled occurrence %s category = %+v, want %s", o.ScheduledTo.Format(time.DateOnly), o.Category, other.ID)
		}
	}
	if scheduled == 0 {
		t.Error("no scheduled occurrence to check")
	}
}
