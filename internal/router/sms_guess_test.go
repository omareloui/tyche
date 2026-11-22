package router_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/smsguess"
	"github.com/omareloui/tyche/internal/valid"
)

// fakeGuesser answers its guesses in turn (the last one again after), with a
// billed usage.
type fakeGuesser struct {
	guesses []*models.SMSTemplateGuess
	err     error
	calls   int
	// lastIn is what the last call was asked.
	lastIn smsguess.In
}

func (f *fakeGuesser) Model() string { return "fake-flash" }

func (f *fakeGuesser) Guess(ctx context.Context, in smsguess.In) (*models.SMSTemplateGuess, gemini.Usage, error) {
	f.calls++
	f.lastIn = in
	usage := gemini.Usage{InputTokens: 900, OutputTokens: 100}
	if f.err != nil {
		return nil, usage, f.err
	}
	g := f.guesses[min(f.calls, len(f.guesses))-1]
	return g, usage, nil
}

func TestGuessSMSTemplate(t *testing.T) {
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

	const body = "Dear customer, card 4321 was charged EGP 250.50 at SPINNEYS on 12/11/2026. Call 19000"
	unparsed := func(id, body string) *models.SMSMessage {
		t.Helper()
		msg := &models.SMSMessage{ID: id, Sender: "NewBank", Body: body, ReceivedAt: time.Now(), Hash: id, Status: models.SMSStatusUnparsed, Error: "no template matched"}
		if err := repo.CreateSMSMessage(ctx, msg, nil); err != nil {
			t.Fatal(err)
		}
		return msg
	}
	msg := unparsed("01K8KQZ4N1XQ5WJ5M3Q6G7Y8S1", body)

	good := &models.SMSTemplateGuess{
		Name: "Card purchase", Bank: "New Bank", Action: models.SMSTemplateTransaction, TxKind: models.ExpenseTxKind,
		Body: "Dear customer, card {{account_id}} was charged {{currency}} {{amount}} at {{counterparty:merchant}} on {{date}}.{{*}}",
	}
	bad := &models.SMSTemplateGuess{Action: models.SMSTemplateTransaction, TxKind: models.ExpenseTxKind, Body: "Your card {{account_id}} paid {{amount}}"}

	newServer := func(g *fakeGuesser, limit int) *httptest.Server {
		h := handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)
		h.SetReceiptOCR(nil, config.ReceiptOCR{SMSGuessesPerMonth: limit})
		if g != nil {
			h.SetSMSGuess(g)
		}
		srv := httptest.NewServer(router.New(h))
		t.Cleanup(srv.Close)
		return srv
	}
	do := func(srv *httptest.Server, method, path string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(url.Values{}.Encode()))
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

	t.Run("off", func(t *testing.T) {
		srv := newServer(nil, 5)
		if res, _ := do(srv, http.MethodPost, "/sms/"+msg.ID+"/guess"); res.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d", res.StatusCode)
		}
		if _, page := do(srv, http.MethodGet, "/sms"); strings.Contains(page, "Guess with AI") {
			t.Error("the button shows while guessing is off")
		}
	})

	t.Run("not verified twice", func(t *testing.T) {
		g := &fakeGuesser{guesses: []*models.SMSTemplateGuess{bad}}
		srv := newServer(g, 5)
		res, b := do(srv, http.MethodPost, "/sms/"+msg.ID+"/guess")
		if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(b, "Make one by hand") {
			t.Fatalf("status = %d, body = %s", res.StatusCode, b)
		}
		if res.Header.Get("HX-Retarget") != "#sms-guess-err-"+msg.ID {
			t.Errorf("retarget = %q", res.Header.Get("HX-Retarget"))
		}
		if g.calls != 2 || g.lastIn.Previous == nil || g.lastIn.PreviousErr == nil {
			t.Errorf("calls = %d, the retry was told %+v / %v", g.calls, g.lastIn.Previous, g.lastIn.PreviousErr)
		}
		got, _ := repo.GetSMSMessage(ctx, msg.ID)
		if got.TemplateGuess != nil {
			t.Error("an unverified guess was kept")
		}
	})

	t.Run("api error", func(t *testing.T) {
		g := &fakeGuesser{err: errors.New("gemini answered 503")}
		res, b := do(newServer(g, 5), http.MethodPost, "/sms/"+msg.ID+"/guess")
		if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(b, "try again in a bit") {
			t.Fatalf("status = %d, body = %s", res.StatusCode, b)
		}
		if g.calls != 1 {
			t.Errorf("calls = %d, an api error isn't retried", g.calls)
		}
	})

	t.Run("guessed after a retry", func(t *testing.T) {
		g := &fakeGuesser{guesses: []*models.SMSTemplateGuess{bad, good}}
		srv := newServer(g, 5)
		res, b := do(srv, http.MethodPost, "/sms/"+msg.ID+"/guess")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, body = %s", res.StatusCode, b)
		}
		form, _ := url.Parse(res.Header.Get("HX-Redirect"))
		if form.Path != "/sms/templates" || form.Query().Get("form") != "/sms/templates/new?guess="+msg.ID {
			t.Fatalf("redirect = %q", res.Header.Get("HX-Redirect"))
		}
		if len(g.lastIn.Examples) == 0 || g.lastIn.Body != body || g.lastIn.Sender != "NewBank" {
			t.Errorf("asked %+v", g.lastIn)
		}

		// nothing saved nor retried: the user does it
		got, _ := repo.GetSMSMessage(ctx, msg.ID)
		if got.Status != models.SMSStatusUnparsed || got.TransactionID != "" {
			t.Errorf("sms = %+v", got)
		}
		if got.TemplateGuess == nil || got.TemplateGuess.Body != good.Body {
			t.Fatalf("guess kept = %+v", got.TemplateGuess)
		}
		if mine, _ := repo.GetSMSTemplates(ctx); len(mine) != 0 {
			t.Errorf("templates = %+v", mine)
		}

		// again: the kept guess, no call
		if res, _ := do(srv, http.MethodPost, "/sms/"+msg.ID+"/guess"); res.StatusCode != http.StatusOK || g.calls != 2 {
			t.Errorf("status = %d, calls = %d", res.StatusCode, g.calls)
		}

		// the form is prefilled with it
		_, drawer := do(srv, http.MethodGet, form.Query().Get("form"))
		for _, want := range []string{"Card purchase", "New Bank", "{{counterparty:merchant}}", "SPINNEYS", "NewBank"} {
			if !strings.Contains(drawer, want) {
				t.Errorf("the form lacks %q", want)
			}
		}

		// the card opens it
		if _, page := do(srv, http.MethodGet, "/sms"); !strings.Contains(page, "Open AI guess") {
			t.Error("the card doesn't link the kept guess")
		}
	})

	t.Run("limit", func(t *testing.T) {
		other := unparsed("01K8KQZ4N1XQ5WJ5M3Q6G7Y8S2", "Another unknown sms 100 EGP")
		g := &fakeGuesser{guesses: []*models.SMSTemplateGuess{good}}
		// the earlier runs billed 3 guesses this month (a retry is one guess)
		res, b := do(newServer(g, 3), http.MethodPost, "/sms/"+other.ID+"/guess")
		if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(b, "the most you can") || g.calls != 0 {
			t.Errorf("status = %d, calls = %d, body = %s", res.StatusCode, g.calls, b)
		}
		// receipts count apart
		usrID := models.TenantID(usr.OAuthProvider, usr.OAuthID)
		if left, _, _ := repo.AICallsLeft(ctx, usrID, models.ScanReceipt, 3, time.Now()); left != 3 {
			t.Errorf("receipt scans left = %d", left)
		}
		// an admin's limit for the tenant overrides the default
		ten := 10
		if err := repo.SetAILimit(ctx, usrID, models.ScanSMSGuess, &ten); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = repo.SetAILimit(ctx, usrID, models.ScanSMSGuess, nil) })
		if res, b := do(newServer(g, 3), http.MethodPost, "/sms/"+other.ID+"/guess"); strings.Contains(b, "the most you can") || g.calls == 0 {
			t.Errorf("past the default with a raised limit: status = %d, calls = %d, body = %s", res.StatusCode, g.calls, b)
		}
	})
}
