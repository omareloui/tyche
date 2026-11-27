package router_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/txguess"
	"github.com/omareloui/tyche/internal/valid"
)

// fakeTxGuesser answers with answer, with a billed usage.
type fakeTxGuesser struct {
	answer func(in txguess.In) []txguess.Answer
	err    error
	calls  int
	lastIn txguess.In
}

func (f *fakeTxGuesser) Model() string { return "fake-flash" }

func (f *fakeTxGuesser) Guess(ctx context.Context, in txguess.In) ([]txguess.Answer, gemini.Usage, error) {
	f.calls++
	f.lastIn = in
	usage := gemini.Usage{InputTokens: 2000, OutputTokens: 300}
	if f.err != nil {
		return nil, usage, f.err
	}
	return f.answer(in), usage, nil
}

// txRef is the prompt's ref of the account / category / counterparty named
// name.
func accRef(in txguess.In, name string) string {
	return fmt.Sprintf("A%d", slices.IndexFunc(in.Accounts, func(a txguess.Account) bool { return a.Name == name })+1)
}

func catRef(in txguess.In, name string) string {
	return fmt.Sprintf("C%d", slices.IndexFunc(in.Categories, func(c txguess.Category) bool { return c.Name == name })+1)
}

func cpRef(in txguess.In, name string) string {
	return fmt.Sprintf("P%d", slices.IndexFunc(in.Counterparties, func(c txguess.Counterparty) bool { return c.Name == name })+1)
}

func TestQuickAdd(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.DefaultCurrency = "e2e@example.com", *egp
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	cib := newTestAccount(t, repo, ctx, "CIB", "EGP", models.AccKindBankAcc, 10000)
	cash := newTestAccount(t, repo, ctx, "Cash Wallet", "EGP", models.AccKindCash, 1000)
	if err := repo.SetDefaultAccount(ctx, cash.ID); err != nil {
		t.Fatal(err)
	}
	usr.DefaultAccount = cash

	food := &models.Category{ID: idgen.New(), Name: "Food", Color: "blue", Kinds: []models.TxKind{models.ExpenseTxKind}}
	shopping := &models.Category{ID: idgen.New(), Name: "Shopping", Color: "blue", Kinds: []models.TxKind{models.ExpenseTxKind}}
	for _, c := range []*models.Category{food, shopping} {
		if err := repo.CreateCategory(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	goods := &models.Counterparty{ID: idgen.New(), Name: "Goods Shop", IsMerchant: true, DefaultCategory: shopping}
	ahmed := &models.Counterparty{ID: idgen.New(), Name: "Ahmed Ali"}
	for _, cp := range []*models.Counterparty{goods, ahmed} {
		if err := repo.CreateCounterparty(ctx, cp); err != nil {
			t.Fatal(err)
		}
	}
	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}

	newServer := func(g txguess.Guesser, limit int) *httptest.Server {
		h := handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)
		h.SetReceiptOCR(nil, config.ReceiptOCR{QuickAddsPerMonth: limit})
		h.SetTxGuess(g)
		srv := httptest.NewServer(router.New(h))
		t.Cleanup(srv.Close)
		return srv
	}
	do := func(srv *httptest.Server, method, path string, form url.Values) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("HX-Request", "true")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	// quick posts text, answering the transactions it created.
	quick := func(srv *httptest.Server, text string) (*http.Response, string, []*models.Transaction) {
		t.Helper()
		before, err := repo.GetTransactions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		res, body := do(srv, http.MethodPost, "/transactions/quick", url.Values{"text": {text}})
		after, err := repo.GetTransactions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var news []*models.Transaction
		for i := range after {
			if !slices.ContainsFunc(before, func(b models.Transaction) bool { return b.ID == after[i].ID }) {
				tx, err := repo.GetTransaction(ctx, after[i].ID)
				if err != nil {
					t.Fatal(err)
				}
				news = append(news, tx)
			}
		}
		return res, body, news
	}
	today := tenantToday()

	t.Run("off until turned on", func(t *testing.T) {
		g := &fakeTxGuesser{answer: func(txguess.In) []txguess.Answer { return nil }}
		srv := newServer(g, 200)
		if res, _, txs := quick(srv, "goods shop 120"); res.StatusCode != http.StatusNotFound || len(txs) != 0 || g.calls != 0 {
			t.Fatalf("status %d, txs %d, calls %d", res.StatusCode, len(txs), g.calls)
		}
		if _, page := do(srv, http.MethodGet, "/transactions", nil); strings.Contains(page, "data-quick-add") {
			t.Error("the page offers quick add while it's off")
		}
		if res, b := do(srv, http.MethodPut, "/settings/ai", url.Values{"ai_quick_add": {"on"}}); res.StatusCode != http.StatusOK || !strings.Contains(b, "Saved") {
			t.Fatalf("turning it on: %d %s", res.StatusCode, b)
		}
		if on, _ := repo.GetAIQuickAdd(ctx); !on {
			t.Fatal("still off")
		}
		if _, page := do(srv, http.MethodGet, "/transactions", nil); !strings.Contains(page, "data-quick-add") || !strings.Contains(page, " of 200 with AI") {
			t.Error("the page doesn't offer quick add with its quota")
		}
	})

	t.Run("an amount and a name without AI", func(t *testing.T) {
		g := &fakeTxGuesser{answer: func(txguess.In) []txguess.Answer { return nil }}
		srv := newServer(g, 200)
		res, body, txs := quick(srv, "goods shop 120")
		if res.StatusCode != http.StatusOK || g.calls != 0 || len(txs) != 1 {
			t.Fatalf("status %d, calls %d, txs %d", res.StatusCode, g.calls, len(txs))
		}
		tx := txs[0]
		if tx.Kind != models.ExpenseTxKind || tx.Amount != 120 || tx.Account.ID != cash.ID || !tx.NeedsReview || tx.QuickAddID == "" ||
			tx.Counterparty == nil || tx.Counterparty.ID != goods.ID || tx.Category == nil || tx.Category.ID != shopping.ID {
			t.Errorf("tx = %+v", tx)
		}
		if !strings.Contains(res.Header.Get("HX-Trigger"), "tx-quick-done") {
			t.Errorf("HX-Trigger = %q", res.Header.Get("HX-Trigger"))
		}
		if !strings.Contains(body, `id="quick-add-quota"`) {
			t.Error("no quota out of band")
		}
		if !strings.Contains(body, "goods shop 120") {
			t.Error("the row's Quick chip doesn't show the text")
		}
		if text, _ := repo.GetTxQuickAddText(ctx, tx.ID); text != "goods shop 120" {
			t.Errorf("quick add text = %q", text)
		}
	})

	t.Run("reads a sentence", func(t *testing.T) {
		yesterday := today.AddDate(0, 0, -1)
		g := &fakeTxGuesser{answer: func(in txguess.In) []txguess.Answer {
			return []txguess.Answer{
				{Kind: "expense", Amount: 85, Account: accRef(in, "CIB"), Category: catRef(in, "Food"), Date: yesterday.Format(time.DateOnly), Description: "Coffee", Text: "coffee 85 cib yesterday"},
				// a new merchant, an invalid category ref dropped
				{Kind: "expense", Amount: 120, Category: "C99", NewCounterparty: "Uber", NewIsMerchant: true, Text: "uber 120"},
				// a category on a transfer dropped
				{Kind: "transfer", Amount: 500, Account: accRef(in, "CIB"), ToAccount: accRef(in, "Cash Wallet"), Category: catRef(in, "Food"), Text: "500 cib to cash"},
				{Kind: "expense", Amount: 0, Text: "lunch"},
			}
		}}
		srv := newServer(g, 200)
		res, body, txs := quick(srv, "coffee 85 cib yesterday, uber 120, 500 cib to cash, lunch")
		if res.StatusCode != http.StatusOK || g.calls != 1 || len(txs) != 3 {
			t.Fatalf("status %d, calls %d, txs %d: %s", res.StatusCode, g.calls, len(txs), body)
		}
		if g.lastIn.DefaultAccount != "Cash Wallet" || !strings.Contains(txguess.Prompt(g.lastIn), "Ahmed Ali") {
			t.Errorf("in = %+v", g.lastIn)
		}
		for _, id := range []string{cib.ID, food.ID, goods.ID} {
			if strings.Contains(txguess.Prompt(g.lastIn), id) {
				t.Errorf("the prompt has the id %s", id)
			}
		}
		trigger := res.Header.Get("HX-Trigger")
		if !strings.Contains(trigger, `"created":3`) || !strings.Contains(trigger, `"reason":"No amount"`) {
			t.Errorf("HX-Trigger = %s", trigger)
		}
		byAmount := map[float64]*models.Transaction{}
		for _, tx := range txs {
			byAmount[tx.Amount] = tx
			if !tx.NeedsReview || tx.QuickAddID == "" || tx.FulfilledAt.IsZero() {
				t.Errorf("tx %v: review %v, quick %q, fulfilled %v", tx.Amount, tx.NeedsReview, tx.QuickAddID, tx.FulfilledAt)
			}
		}
		if c := byAmount[85]; c == nil || c.Account.ID != cib.ID || c.Category == nil || c.Category.ID != food.ID || c.Description != "Coffee" ||
			!c.ScheduledTo.Equal(yesterday) {
			t.Errorf("coffee = %+v", c)
		}
		if u := byAmount[120]; u == nil || u.Account.ID != cash.ID || u.Category != nil || u.Counterparty == nil || u.Counterparty.Name != "Uber" || !u.Counterparty.IsMerchant {
			t.Errorf("uber = %+v", u)
		}
		if tr := byAmount[500]; tr == nil || tr.Kind != models.TransferTxKind || tr.ToAccount == nil || tr.ToAccount.ID != cash.ID || tr.ToAmount != 500 || tr.Category != nil {
			t.Errorf("transfer = %+v", tr)
		}
	})

	t.Run("a day ahead is scheduled, a debt goes to the person", func(t *testing.T) {
		g := &fakeTxGuesser{answer: func(in txguess.In) []txguess.Answer {
			return []txguess.Answer{
				{Kind: "expense", Amount: 300, Category: catRef(in, "Food"), Date: today.AddDate(0, 0, 3).Format(time.DateOnly)},
				{Kind: "lend", Amount: 500, Counterparty: cpRef(in, "Ahmed Ali")},
			}
		}}
		res, body, txs := quick(newServer(g, 200), "dinner 300 in 3 days, lent ahmed 500")
		if res.StatusCode != http.StatusOK || len(txs) != 2 {
			t.Fatalf("status %d, txs %d: %s", res.StatusCode, len(txs), body)
		}
		for _, tx := range txs {
			switch tx.Amount {
			case 300:
				if !tx.FulfilledAt.IsZero() {
					t.Errorf("the one ahead is fulfilled: %+v", tx)
				}
			case 500:
				if tx.Kind != models.DebtTxKind || tx.Account.ID != cash.ID || tx.ToAccount == nil || tx.ToAccount.Kind.Enum != models.AccKindDebt ||
					tx.ToAccount.Counterparty == nil || tx.ToAccount.Counterparty.ID != ahmed.ID {
					t.Errorf("debt = %+v (to %+v)", tx, tx.ToAccount)
				}
			}
		}
	})

	t.Run("#words are tags", func(t *testing.T) {
		g := &fakeTxGuesser{answer: func(in txguess.In) []txguess.Answer {
			return []txguess.Answer{{Kind: "expense", Amount: 15, Description: "Coffee", Note: "#work", Tags: []string{"work"}}}
		}}
		res, body, txs := quick(newServer(g, 200), "15 cash coffee from beano's #work")
		if res.StatusCode != http.StatusOK || len(txs) != 1 {
			t.Fatalf("status %d, txs %d: %s", res.StatusCode, len(txs), body)
		}
		if tx := txs[0]; tx.Note != "" || len(tx.Tags) != 1 || tx.Tags[0].Name != "work" {
			t.Errorf("note %q, tags %+v", tx.Note, tx.Tags)
		}
	})

	t.Run("listed by the quick source filter", func(t *testing.T) {
		all := time.Time{}
		page, err := repo.GetTransactionsPage(ctx, models.TxFilter{Source: models.TxSourceQuick}, time.Time{}, &all)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(page.Upcoming) + len(page.History); n != 7 {
			t.Errorf("quick ones = %d, want 7", n)
		}
		page, err = repo.GetTransactionsPage(ctx, models.TxFilter{Source: models.TxSourceManual}, time.Time{}, &all)
		if err != nil {
			t.Fatal(err)
		}
		for _, tx := range append(page.Upcoming, page.History...) {
			if tx.QuickAddID != "" {
				t.Errorf("manual has a quick one: %+v", tx)
			}
		}
	})

	t.Run("nothing valid is a 422", func(t *testing.T) {
		g := &fakeTxGuesser{answer: func(txguess.In) []txguess.Answer {
			return []txguess.Answer{{Kind: "expense", Amount: 0, Text: "lunch"}}
		}}
		res, body, txs := quick(newServer(g, 200), "lunch")
		if res.StatusCode != http.StatusUnprocessableEntity || len(txs) != 0 || !strings.Contains(body, "no amount") ||
			res.Header.Get("HX-Retarget") != "#quick-add-error" {
			t.Errorf("status %d, txs %d, retarget %q: %s", res.StatusCode, len(txs), res.Header.Get("HX-Retarget"), body)
		}
		g.err = errors.New("boom")
		if res, body, _ := quick(newServer(g, 200), "lunch 50 at the club"); res.StatusCode != http.StatusUnprocessableEntity || strings.Contains(body, "boom") {
			t.Errorf("model error: %d %s", res.StatusCode, body)
		}
	})

	t.Run("over the limit only the cheap path works", func(t *testing.T) {
		g := &fakeTxGuesser{answer: func(txguess.In) []txguess.Answer { return nil }}
		srv := newServer(g, 1) // used up by the calls above
		res, body, _ := quick(srv, "coffee 85 cib yesterday")
		if res.StatusCode != http.StatusUnprocessableEntity || g.calls != 0 || !strings.Contains(body, "this month") {
			t.Errorf("status %d, calls %d: %s", res.StatusCode, g.calls, body)
		}
		if res, _, txs := quick(srv, "food 40"); res.StatusCode != http.StatusOK || len(txs) != 1 || g.calls != 0 {
			t.Errorf("cheap path: %d, %d txs", res.StatusCode, len(txs))
		}
	})
}

// TestQuickAddPaysOccurrence checks a quick add paying a scheduled
// recurrent occurrence becomes it.
func TestQuickAddPaysOccurrence(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.DefaultCurrency = "e2e@example.com", *egp
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	usr.DefaultAccount = bank
	if err := repo.SetAIQuickAdd(ctx, true); err != nil {
		t.Fatal(err)
	}
	hd := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	today := tenantToday()
	rt := createTestRecurrentTx(t, ctx, repo, hd, bank, "300", today.AddDate(0, 0, 1), models.RecurrentUnitMonth, nil)
	sched, _ := occurrences(t, ctx, repo, rt.ID)
	first := sched[len(sched)-1]

	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}
	hd.SetTxGuess(&fakeTxGuesser{answer: func(in txguess.In) []txguess.Answer { return nil }})
	srv := httptest.NewServer(router.New(hd))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/transactions/quick", strings.NewReader(url.Values{"text": {"310"}}.Encode()))
	req.Header.Set("HX-Request", "true")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if trigger := res.Header.Get("HX-Trigger"); !strings.Contains(trigger, `"merged":1`) || !strings.Contains(trigger, `"ids":null`) {
		t.Errorf("HX-Trigger = %s", trigger)
	}
	got, err := repo.GetTransaction(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FulfilledAt.IsZero() || got.Amount != 310 || got.QuickAddID == "" || !got.NeedsReview {
		t.Errorf("occurrence = %+v", got)
	}
}
