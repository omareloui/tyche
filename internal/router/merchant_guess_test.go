package router_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/omareloui/tyche/internal/merchantguess"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

// fakeMerchantGuesser answers with answer, with a billed usage.
type fakeMerchantGuesser struct {
	answer func(in merchantguess.In) []merchantguess.Answer
	err    error
	calls  int
	lastIn merchantguess.In
}

func (f *fakeMerchantGuesser) Model() string { return "fake-flash-lite" }

func (f *fakeMerchantGuesser) Guess(ctx context.Context, in merchantguess.In) ([]merchantguess.Answer, gemini.Usage, error) {
	f.calls++
	f.lastIn = in
	usage := gemini.Usage{InputTokens: 2500, OutputTokens: 200}
	if f.err != nil {
		return nil, usage, f.err
	}
	return f.answer(in), usage, nil
}

// merchantRef is the prompt's ref of the merchant named name.
func merchantRef(in merchantguess.In, name string) string {
	return fmt.Sprintf("M%d", slices.IndexFunc(in.Merchants, func(m merchantguess.Merchant) bool { return m.Name == name })+1)
}

func categoryRef(in merchantguess.In, name string) string {
	return fmt.Sprintf("C%d", slices.IndexFunc(in.Categories, func(c merchantguess.Category) bool { return c.Name == name })+1)
}

func TestMerchantGuess(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.DefaultCurrency = "e2e@example.com", *egp
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	acc := &models.Account{ID: idgen.New(), Name: "HSBC", Amount: 10000, Currency: *egp, SmsRefs: []string{"123-456***-789"}}
	for _, k := range kinds {
		if k.Enum == models.AccKindBankAcc {
			acc.Kind = k
		}
	}
	if err := repo.CreateAccount(ctx, acc); err != nil {
		t.Fatal(err)
	}

	shopping := &models.Category{ID: idgen.New(), Name: "Shopping", Color: "blue", Kinds: []models.TxKind{models.ExpenseTxKind}}
	salary := &models.Category{ID: idgen.New(), Name: "Paycheck", Color: "green", Kinds: []models.TxKind{models.IncomeTxKind}}
	for _, c := range []*models.Category{shopping, salary} {
		if err := repo.CreateCategory(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	goods := &models.Counterparty{ID: idgen.New(), Name: "Goods Shop", IsMerchant: true, DefaultCategory: shopping}
	person := &models.Counterparty{ID: idgen.New(), Name: "Ahmed Ali"}
	for _, cp := range []*models.Counterparty{goods, person} {
		if err := repo.CreateCounterparty(ctx, cp); err != nil {
			t.Fatal(err)
		}
	}

	device := &models.Device{ID: idgen.New(), Name: "test", InstallID: "install-1", TokenID: idgen.New()}
	if err := repo.RegisterDevice(ctx, device); err != nil {
		t.Fatal(err)
	}
	token, err := jwt.NewDeviceToken(usr, device.ID, device.TokenID, "")
	if err != nil {
		t.Fatal(err)
	}
	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}

	newServer := func(g *fakeMerchantGuesser, limit int) *httptest.Server {
		h := handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)
		h.SetReceiptOCR(nil, config.ReceiptOCR{MerchantGuessesPerMonth: limit})
		h.SetMerchantGuess(g)
		srv := httptest.NewServer(router.New(h))
		t.Cleanup(srv.Close)
		return srv
	}
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	// ingest posts an sms of a purchase at merchant, answering its
	// transaction.
	ingest := func(srv *httptest.Server, merchant string) *models.Transaction {
		t.Helper()
		at = at.Add(time.Minute)
		body := fmt.Sprintf("From HSBC: 20NOV26 %s Purchase from 123-456***-789 EGP 220.00- Your available balance is EGP 4,554.36", merchant)
		b, _ := json.Marshal(map[string]any{"messages": []map[string]any{{"sender": "HSBC", "body": body, "received_at": at}}})
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/sms", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out ingestResp
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil || res.StatusCode != http.StatusOK {
			t.Fatalf("ingest: status %d, %v", res.StatusCode, err)
		}
		if out.Results[0].Status != "parsed" {
			t.Fatalf("sms %s: %s", out.Results[0].Status, out.Results[0].Error)
		}
		tx, err := repo.GetTransaction(ctx, out.Results[0].TransactionID)
		if err != nil {
			t.Fatal(err)
		}
		return tx
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
	counterparty := func(id string) *models.Counterparty {
		t.Helper()
		cp, err := repo.GetCounterparty(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return cp
	}

	t.Run("off until turned on", func(t *testing.T) {
		g := &fakeMerchantGuesser{answer: func(merchantguess.In) []merchantguess.Answer { return nil }}
		tx := ingest(newServer(g, 100), "OFF STORE 1")
		if g.calls != 0 {
			t.Errorf("calls = %d with the setting off", g.calls)
		}
		if tx.Counterparty == nil || !strings.EqualFold(tx.Counterparty.Name, "OFF STORE 1") {
			t.Errorf("counterparty = %+v, want one named after the sms", tx.Counterparty)
		}

		srv := newServer(g, 100)
		if res, b := do(srv, http.MethodPut, "/settings/ai", url.Values{"ai_merchants": {"on"}}); res.StatusCode != http.StatusOK || !strings.Contains(b, "Saved") {
			t.Fatalf("turning it on: %d %s", res.StatusCode, b)
		}
		if on, _ := repo.GetAIMerchants(ctx); !on {
			t.Fatal("still off")
		}
		if _, page := do(srv, http.MethodGet, "/settings", nil); !strings.Contains(page, " of 100 checks") {
			t.Error("the settings don't show the month's checks")
		}
	})

	t.Run("matches a merchant, then the alias does", func(t *testing.T) {
		const raw = "PAYPRO-*GOODS SHOP 12"
		g := &fakeMerchantGuesser{answer: func(in merchantguess.In) []merchantguess.Answer {
			return []merchantguess.Answer{{Raw: raw, MatchRef: merchantRef(in, "Goods Shop")}}
		}}
		srv := newServer(g, 100)
		tx := ingest(srv, raw)
		if g.calls != 1 || !slices.Equal(g.lastIn.Names, []string{raw}) {
			t.Fatalf("calls = %d, names %q", g.calls, g.lastIn.Names)
		}
		for _, m := range g.lastIn.Merchants {
			if m.Name == person.Name {
				t.Error("a person was sent")
			}
		}
		for _, c := range g.lastIn.Categories {
			if c.Name == salary.Name {
				t.Error("an income category was sent")
			}
		}
		if tx.Counterparty == nil || tx.Counterparty.ID != goods.ID || tx.Category == nil || tx.Category.ID != shopping.ID || !tx.NeedsReview {
			t.Fatalf("tx counterparty %+v, category %+v, review %v", tx.Counterparty, tx.Category, tx.NeedsReview)
		}
		if !slices.Contains(counterparty(goods.ID).Aliases, raw) {
			t.Error("the alias wasn't added")
		}
		if _, page := do(srv, http.MethodGet, "/counterparties/"+goods.ID+"/details", nil); !strings.Contains(page, raw) || !strings.Contains(page, "Matched by AI") {
			t.Error("the merchant's details don't show the AI alias")
		}
		if gs, _ := repo.GetTxMerchantGuess(ctx, tx.ID); gs == nil || gs.Created {
			t.Errorf("tx guess = %+v", gs)
		}
		_, page := do(srv, http.MethodGet, "/transactions/"+tx.ID+"/details", nil)
		if !strings.Contains(page, "AI matched") || !strings.Contains(page, "Wrong match") {
			t.Error("the details don't tell the AI matched it")
		}

		again := ingest(srv, raw)
		if g.calls != 1 {
			t.Errorf("calls = %d, the alias should match it", g.calls)
		}
		if again.Counterparty == nil || again.Counterparty.ID != goods.ID {
			t.Errorf("again: counterparty %+v", again.Counterparty)
		}
		if gs, _ := repo.GetTxMerchantGuess(ctx, again.ID); gs == nil {
			t.Error("the alias' match isn't marked")
		}
	})

	t.Run("creates a merchant with its category", func(t *testing.T) {
		const raw = "AMZN MKTP EG*4821"
		g := &fakeMerchantGuesser{answer: func(in merchantguess.In) []merchantguess.Answer {
			return []merchantguess.Answer{{Raw: raw, NewName: "Amazon", CategoryRef: categoryRef(in, "Shopping"), Labels: []string{"online store"}}}
		}}
		tx := ingest(newServer(g, 100), raw)
		if tx.Counterparty == nil || tx.Counterparty.Name != "Amazon" || tx.Category == nil || tx.Category.ID != shopping.ID {
			t.Fatalf("tx counterparty %+v, category %+v", tx.Counterparty, tx.Category)
		}
		cp := counterparty(tx.Counterparty.ID)
		if !cp.IsMerchant || !slices.Contains(cp.Aliases, raw) || !slices.Equal(cp.Labels, []string{"Online store"}) {
			t.Errorf("merchant %+v", cp)
		}
		if gs, _ := repo.GetTxMerchantGuess(ctx, tx.ID); gs == nil || !gs.Created {
			t.Errorf("tx guess = %+v", gs)
		}
	})

	t.Run("a made up ref falls back", func(t *testing.T) {
		const raw = "MYSTERY 77"
		g := &fakeMerchantGuesser{answer: func(merchantguess.In) []merchantguess.Answer {
			return []merchantguess.Answer{{Raw: raw, MatchRef: "M999"}}
		}}
		tx := ingest(newServer(g, 100), raw)
		if g.calls != 1 || tx.Counterparty == nil || !strings.EqualFold(tx.Counterparty.Name, raw) {
			t.Errorf("calls %d, counterparty %+v", g.calls, tx.Counterparty)
		}
	})

	t.Run("an error doesn't fail the ingest", func(t *testing.T) {
		g := &fakeMerchantGuesser{err: errors.New("gemini answered 503")}
		tx := ingest(newServer(g, 100), "FAILING 1")
		if g.calls != 1 || tx.Counterparty == nil || !strings.EqualFold(tx.Counterparty.Name, "FAILING 1") {
			t.Errorf("calls %d, counterparty %+v", g.calls, tx.Counterparty)
		}
	})

	t.Run("over the limit no call", func(t *testing.T) {
		g := &fakeMerchantGuesser{answer: func(merchantguess.In) []merchantguess.Answer { return nil }}
		ingest(newServer(g, 0), "LIMITED 1")
		if g.calls != 0 {
			t.Errorf("calls = %d over the limit", g.calls)
		}
	})

	t.Run("wrong match", func(t *testing.T) {
		const raw = "GOODS SHOPPING MALL"
		g := &fakeMerchantGuesser{answer: func(in merchantguess.In) []merchantguess.Answer {
			return []merchantguess.Answer{{Raw: raw, MatchRef: merchantRef(in, "Goods Shop")}}
		}}
		srv := newServer(g, 100)
		tx := ingest(srv, raw)
		if tx.Counterparty == nil || tx.Counterparty.ID != goods.ID {
			t.Fatalf("counterparty %+v", tx.Counterparty)
		}
		res, b := do(srv, http.MethodDelete, "/transactions/"+tx.ID+"/merchant-guess", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(b, "/transactions/"+tx.ID+"/pick/counterparty") {
			t.Fatalf("status %d, body %s", res.StatusCode, b)
		}
		if slices.Contains(counterparty(goods.ID).Aliases, raw) {
			t.Error("the alias is still there")
		}
		if gs, _ := repo.GetTxMerchantGuess(ctx, tx.ID); gs != nil {
			t.Errorf("still marked %+v", gs)
		}
		if res, _ := do(srv, http.MethodDelete, "/transactions/"+tx.ID+"/merchant-guess", nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("undoing twice: %d", res.StatusCode)
		}

		// never asked again
		again := ingest(srv, raw)
		if g.calls != 1 || again.Counterparty == nil || again.Counterparty.ID == goods.ID {
			t.Errorf("calls %d, counterparty %+v", g.calls, again.Counterparty)
		}
	})

	t.Run("a batch's strings in one call", func(t *testing.T) {
		g := &fakeMerchantGuesser{answer: func(in merchantguess.In) []merchantguess.Answer {
			var out []merchantguess.Answer
			for _, n := range in.Names {
				out = append(out, merchantguess.Answer{Raw: n, NewName: "Talabat"})
			}
			return out
		}}
		srv := newServer(g, 100)
		var msgs []map[string]any
		for _, m := range []string{"TALABAT EG 1", "TALABAT EG 2", "TALABAT EG 1"} {
			at = at.Add(time.Minute)
			msgs = append(msgs, map[string]any{"sender": "HSBC", "received_at": at,
				"body": fmt.Sprintf("From HSBC: 20NOV26 %s Purchase from 123-456***-789 EGP 50.00- Your available balance is EGP 4,554.36", m)})
		}
		b, _ := json.Marshal(map[string]any{"messages": msgs})
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/sms", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var out ingestResp
		_ = json.NewDecoder(res.Body).Decode(&out)
		res.Body.Close()
		if g.calls != 1 || len(g.lastIn.Names) != 2 {
			t.Fatalf("calls %d, names %q", g.calls, g.lastIn.Names)
		}
		var cpIDs []string
		for _, r := range out.Results {
			tx, err := repo.GetTransaction(ctx, r.TransactionID)
			if err != nil {
				t.Fatal(err)
			}
			cpIDs = append(cpIDs, tx.Counterparty.ID)
		}
		if cpIDs[0] != cpIDs[1] || cpIDs[1] != cpIDs[2] {
			t.Errorf("one merchant for both strings, got %q", cpIDs)
		}
		cp := counterparty(cpIDs[0])
		if cp.Name != "Talabat" || len(cp.Aliases) != 2 {
			t.Errorf("merchant %+v", cp)
		}
	})
}
