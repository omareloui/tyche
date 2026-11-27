package router_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/splitguess"
	"github.com/omareloui/tyche/internal/valid"
)

// fakeSplitGuesser answers with answer, with a billed usage.
type fakeSplitGuesser struct {
	answer splitguess.Answer
	err    error
	calls  int
	lastIn splitguess.In
}

func (f *fakeSplitGuesser) Model() string { return "fake-flash" }

func (f *fakeSplitGuesser) Guess(ctx context.Context, in splitguess.In) (splitguess.Answer, gemini.Usage, error) {
	f.calls++
	f.lastIn = in
	return f.answer, gemini.Usage{InputTokens: 800, OutputTokens: 300}, f.err
}

// TestDescribeSplit sets a split bill's shares from a sentence: people found
// or created, the items said shared, the others left alone, within the
// month's limit.
func TestDescribeSplit(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.DefaultCurrency = "e2e@example.com", *egp
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}
	ahmed := &models.Counterparty{ID: idgen.New(), Name: "Ahmed"}
	sara := &models.Counterparty{ID: idgen.New(), Name: "Sara Hassan"}
	shop := &models.Counterparty{ID: idgen.New(), Name: "Pizza Place", IsMerchant: true}
	for _, cp := range []*models.Counterparty{ahmed, sara, shop} {
		if err := repo.CreateCounterparty(ctx, cp); err != nil {
			t.Fatal(err)
		}
	}
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	on := tenantToday().AddDate(0, 0, -1)
	tx := &models.Transaction{
		ID: idgen.New(), Account: *bank, Kind: models.ExpenseTxKind, Amount: 470, ScheduledTo: on, FulfilledAt: on, Counterparty: shop,
		Items: []models.TxItem{
			{Name: "Burger", Quantity: 1, Total: 100},
			{Name: "Pizza", Quantity: 2, UnitPrice: 100, Total: 200},
			{Name: "Soda", Quantity: 3, UnitPrice: 20, Total: 60},
			{Name: "Fries", Quantity: 1, Total: 40},
			{Kind: models.TxItemService, Total: 70},
		},
	}
	if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
		t.Fatal(err)
	}
	lines, err := repo.GetTxItems(ctx, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	burger, pizza, soda, fries := lines[0].ID, lines[1].ID, lines[2].ID, lines[3].ID

	newServer := func(g splitguess.Guesser, limit int) *httptest.Server {
		h := handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)
		h.SetReceiptOCR(nil, config.ReceiptOCR{SplitGuessesPerMonth: limit})
		if g != nil {
			h.SetSplitGuess(g)
		}
		srv := httptest.NewServer(router.New(h))
		t.Cleanup(srv.Close)
		return srv
	}
	do := func(srv *httptest.Server, method string, form url.Values) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+"/transactions/"+tx.ID+"/split", strings.NewReader(form.Encode()))
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Target", "txd-panel")
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
	split := func() *models.TxSplit {
		t.Helper()
		s, err := repo.GetTxSplit(ctx, tx.ID)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	item := func(s *models.TxSplit, id string) models.TxItem {
		t.Helper()
		i := slices.IndexFunc(s.Tx.Items, func(it models.TxItem) bool { return it.ID == id })
		if i < 0 {
			t.Fatalf("no line %s", id)
		}
		return s.Tx.Items[i]
	}

	t.Run("off without AI", func(t *testing.T) {
		srv := newServer(nil, 30)
		if _, body := do(srv, http.MethodGet, nil); strings.Contains(body, "Describe the split") {
			t.Error("the panel offers describing while it's off")
		}
		if res, _ := do(srv, http.MethodPut, url.Values{"describe": {"Ahmed had the burger"}}); res.StatusCode != http.StatusNotFound {
			t.Errorf("describing while off: %d", res.StatusCode)
		}
	})

	// Ahmed's in the split, with the fries.
	srv := newServer(nil, 30)
	if res, _ := do(srv, http.MethodPut, url.Values{"person": {ahmed.ID}, "share_" + fries: {ahmed.ID}}); res.StatusCode != http.StatusOK {
		t.Fatalf("adding Ahmed: %d", res.StatusCode)
	}

	t.Run("a sentence sets the shares", func(t *testing.T) {
		g := &fakeSplitGuesser{answer: splitguess.Answer{
			Shares: []splitguess.AnswerShare{
				{Item: "I1", Takers: []splitguess.Taker{{Who: "P1"}}},
				{Item: "I2", Takers: []splitguess.Taker{{Who: "Sara"}, {Who: "me"}}},
				{Item: "I3", Everyone: true},
				{Item: "I9", Takers: []splitguess.Taker{{Who: "P1"}}},
			},
			NewPeople: []string{"Sara", "Mona"},
		}}
		srv := newServer(g, 30)
		if _, body := do(srv, http.MethodGet, nil); !strings.Contains(body, "Describe the split") || !strings.Contains(body, " of 30") {
			t.Error("the panel doesn't offer describing with its quota")
		}
		res, body := do(srv, http.MethodPut, url.Values{"describe": {"Ahmed had the burger, Sara and me shared the pizzas, drinks for everyone, Mona came"}})
		if res.StatusCode != http.StatusOK || g.calls != 1 {
			t.Fatalf("status %d, calls %d", res.StatusCode, g.calls)
		}
		if len(g.lastIn.Lines) != 4 || len(g.lastIn.People) != 1 || g.lastIn.People[0].Name != "Ahmed" {
			t.Errorf("asked %+v", g.lastIn)
		}
		for _, s := range []string{"Set 3 of the 4 items.", "Added Sara Hassan to the split.", "Added Mona to your people."} {
			if !strings.Contains(body, s) {
				t.Errorf("answer lacks %q", s)
			}
		}

		s := split()
		var names []string
		for _, p := range s.People {
			names = append(names, p.Name)
		}
		if !slices.Equal(names, []string{"Ahmed", "Sara Hassan", "Mona"}) {
			t.Fatalf("people = %v", names)
		}
		mona := s.People[2].ID
		if cp, err := repo.GetCounterparty(ctx, mona); err != nil || cp.IsMerchant {
			t.Errorf("Mona = %+v, %v; want a person", cp, err)
		}
		if got := item(s, burger).SharedWith; len(got) != 1 || got[0].Person != ahmed.ID {
			t.Errorf("burger shared with %v", got)
		}
		if got := s.ItemUnits(ptr(item(s, pizza))); got[sara.ID] != 1 || got[models.SplitMe] != 1 {
			t.Errorf("pizza units = %v", got)
		}
		if !item(s, soda).SharedByAll {
			t.Error("soda isn't everyone's")
		}
		if got := item(s, fries).SharedWith; len(got) != 1 || got[0].Person != ahmed.ID {
			t.Errorf("fries (not said) shared with %v, want Ahmed's still", got)
		}
	})

	t.Run("nothing read", func(t *testing.T) {
		g := &fakeSplitGuesser{err: errors.New("boom")}
		srv := newServer(g, 30)
		_, body := do(srv, http.MethodPut, url.Values{"describe": {"whatever"}})
		if !strings.Contains(body, "give it another try") || !strings.Contains(body, `splitDescribe(&#34;whatever&#34;)`) {
			t.Errorf("failed call: %s", body)
		}
		g = &fakeSplitGuesser{answer: splitguess.Answer{Shares: []splitguess.AnswerShare{{Item: "I7", Everyone: true}}}}
		if _, body := do(newServer(g, 30), http.MethodPut, url.Values{"describe": {"cake for all"}}); !strings.Contains(body, "Couldn&#39;t tell who had which item") {
			t.Errorf("nothing matched: %s", body)
		}
	})

	t.Run("limit", func(t *testing.T) {
		g := &fakeSplitGuesser{}
		// 3 billed calls so far (the failed one had tokens)
		_, body := do(newServer(g, 3), http.MethodPut, url.Values{"describe": {"Ahmed had the burger"}})
		if g.calls != 0 || !strings.Contains(body, "this month&#39;s 3 split descriptions") {
			t.Errorf("calls %d: %s", g.calls, body)
		}
	})
}

func ptr[T any](v T) *T { return &v }
