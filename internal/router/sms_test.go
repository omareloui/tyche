package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	_ "github.com/omareloui/tyche/internal/sqlite"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/shareddb"
	"github.com/omareloui/tyche/internal/sharedqueries"
	"github.com/omareloui/tyche/internal/smstemplates"
	"github.com/omareloui/tyche/internal/valid"
	"github.com/omareloui/tyche/internal/vault"
)

const hsbcSMS = "From HSBC: 26AUG25 PAYPRO-*SHOP FOR GOODS Purchase from 123-456***-789 EGP 220.00- Your available balance is EGP 4,554.36"

type ingestResp struct {
	Results []struct {
		Status        string `json:"status"`
		TransactionID string `json:"transaction_id"`
		Error         string `json:"error"`
	} `json:"results"`
}

// setupTenant runs from a temp dir so the tenant database is thrown away.
func setupTenant(t *testing.T) (*repository.Repository, context.Context, *models.User) {
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

	usr := &models.User{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZA", OAuthID: "e2e", OAuthProvider: models.Google}
	ctx := context.WithValue(context.Background(), middleware.AccessClaimsCtxKey{}, &jwt.AccessClaims{
		ID: usr.ID, OAuthID: usr.OAuthID, OAuthProvider: usr.OAuthProvider,
	})
	return testRepo(t), ctx, usr
}

// testRepo is a repository on the shared database of testVaults.
func testRepo(t *testing.T) *repository.Repository {
	t.Helper()
	vaults, q := testVaults(t)
	return repository.New(vaults, q)
}

// testVaults opens the vaults in the shared database, from the temp dir of
// setupTenant, with a cheap key derivation.
func testVaults(t *testing.T) (*vault.Store, *sharedqueries.Queries) {
	t.Helper()
	db, q, err := shareddb.Open("data/shared.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return vault.New(db, q, vault.KDFParams{Time: 1, Memory: 1024, Threads: 1}), q
}

// sharedTemplates opens a seeded shared database, from the temp dir of
// setupTenant.
func sharedTemplates(t *testing.T) *smstemplates.Store {
	t.Helper()
	db, q, err := shareddb.Open("data/shared.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := smstemplates.New(q)
	if err := s.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIngestSMS(t *testing.T) {
	repo, ctx, usr := setupTenant(t)

	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var bankKind models.AccountKind
	for _, k := range kinds {
		if k.Enum == models.AccKindBankAcc {
			bankKind = k
		}
	}

	acc := &models.Account{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZB", Name: "HSBC e2e", Amount: 1000,
		Currency: *egp, Kind: bankKind, SmsRefs: []string{"123-456***-789"},
	}
	if err := repo.CreateAccount(ctx, acc); err != nil {
		t.Fatal(err)
	}

	device := &models.Device{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZC", Name: "test",
		InstallID: "install-1", TokenID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZD",
	}
	if err := repo.RegisterDevice(ctx, device); err != nil {
		t.Fatal(err)
	}
	token, err := jwt.NewDeviceToken(usr, device.ID, device.TokenID, "")
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)))
	defer srv.Close()

	post := func(token string, body any) (*http.Response, ingestResp) {
		t.Helper()
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/sms", bytes.NewReader(b))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out ingestResp
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res, out
	}

	receivedAt := time.Date(2025, 8, 26, 14, 0, 0, 0, time.UTC)
	batch := map[string]any{"messages": []map[string]any{
		{"sender": "HSBC", "body": hsbcSMS, "received_at": receivedAt},
		{"sender": "HSBC", "body": "Your OTP is 1234", "received_at": receivedAt},
		{"sender": "HSBC", "body": "From HSBC: 26AUG25 SHOP Purchase from 999-999***-111 EGP 5.00- Your available balance is EGP 1.00", "received_at": receivedAt},
	}}

	t.Run("rejects missing and invalid tokens", func(t *testing.T) {
		if res, _ := post("", batch); res.StatusCode != http.StatusUnauthorized {
			t.Errorf("no token: got %d", res.StatusCode)
		}
		pair, _ := jwt.NewPair(usr)
		if res, _ := post(pair.Access.Encoded, batch); res.StatusCode != http.StatusUnauthorized {
			t.Errorf("access token: got %d", res.StatusCode)
		}
	})

	var txID string
	t.Run("creates transactions", func(t *testing.T) {
		res, out := post(token, batch)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("got %d", res.StatusCode)
		}
		want := []string{"parsed", "unparsed", "unmatched"}
		for i, w := range want {
			if out.Results[i].Status != w {
				t.Errorf("message %d: status %q, want %q (%s)", i, out.Results[i].Status, w, out.Results[i].Error)
			}
		}
		txID = out.Results[0].TransactionID

		tx, err := repo.GetTransaction(ctx, txID)
		if err != nil {
			t.Fatal(err)
		}
		if !tx.NeedsReview || tx.Amount != 220 || tx.Kind != models.ExpenseTxKind || tx.Account.ID != acc.ID || tx.FulfilledAt.IsZero() {
			t.Errorf("unexpected transaction %+v", tx)
		}
		gotAcc, err := repo.GetAccount(ctx, acc.ID)
		if err != nil {
			t.Fatal(err)
		}
		if gotAcc.Amount != 780 {
			t.Errorf("account amount = %v, want 780", gotAcc.Amount)
		}

		msgs, err := repo.GetSMSMessagesByStatus(ctx, models.SMSStatusUnparsed, models.SMSStatusUnmatched)
		if err != nil {
			t.Fatal(err)
		}
		if len(msgs) != 2 {
			t.Errorf("stored %d failed messages, want 2", len(msgs))
		}
	})

	t.Run("unparsed sms link to a new template", func(t *testing.T) {
		pair, _ := jwt.NewPair(usr)
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/sms", nil)
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		body := string(b)
		want := "/sms/templates?" + url.Values{"form": {"/sms/templates/new?" + url.Values{"scope": {"mine"}, "sender": {"HSBC"}, "sms": {"Your OTP is 1234"}}.Encode()}}.Encode()
		if !strings.Contains(body, html.EscapeString(want)) {
			t.Errorf("no make template link %q", want)
		}
		if strings.Count(body, "Make template") != 1 {
			t.Errorf("want the link on the unparsed sms only, got %d", strings.Count(body, "Make template"))
		}
		if !strings.Contains(body, `href="/sms/templates"`) {
			t.Error("no link to the templates page")
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		_, out := post(token, batch)
		for i, r := range out.Results {
			if r.Status != "duplicate" {
				t.Errorf("message %d: status %q, want duplicate", i, r.Status)
			}
		}
		if out.Results[0].TransactionID != txID {
			t.Errorf("duplicate returned tx %q, want %q", out.Results[0].TransactionID, txID)
		}
		txs, err := repo.GetTransactions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(txs) != 1 {
			t.Errorf("got %d transactions, want 1", len(txs))
		}
	})

	t.Run("retries unmatched sms after setting the sms ref", func(t *testing.T) {
		other := &models.Account{
			ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZD", Name: "Other e2e", Amount: 10,
			Currency: *egp, Kind: bankKind, SmsRefs: []string{"999-999***-111"},
		}
		if err := repo.CreateAccount(ctx, other); err != nil {
			t.Fatal(err)
		}

		msgs, err := repo.GetSMSMessagesByStatus(ctx, models.SMSStatusUnmatched)
		if err != nil || len(msgs) != 1 {
			t.Fatalf("got %d unmatched messages, err: %v", len(msgs), err)
		}

		pair, _ := jwt.NewPair(usr)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/sms/"+msgs[0].ID+"/retry", nil)
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("got %d", res.StatusCode)
		}

		msg, err := repo.GetSMSMessage(ctx, msgs[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if msg.Status != models.SMSStatusParsed || msg.TransactionID == "" {
			t.Errorf("unexpected message after retry %+v", msg)
		}
		gotAcc, err := repo.GetAccount(ctx, other.ID)
		if err != nil {
			t.Fatal(err)
		}
		if gotAcc.Amount != 5 {
			t.Errorf("account amount = %v, want 5", gotAcc.Amount)
		}
	})

	t.Run("refunds add to the account with the merchant's expense category", func(t *testing.T) {
		card := &models.Account{
			ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZH", Name: "CIB e2e", Amount: 100,
			Currency: *egp, Kind: bankKind, SmsRefs: []string{"5678"},
		}
		if err := repo.CreateAccount(ctx, card); err != nil {
			t.Fatal(err)
		}
		transport := &models.Category{
			ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZJ", Name: "Transport e2e", Color: "#000000",
			Kinds: []models.TxKind{models.ExpenseTxKind},
		}
		if err := repo.CreateCategory(ctx, transport); err != nil {
			t.Fatal(err)
		}
		if err := repo.CreateCounterparty(ctx, &models.Counterparty{
			ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZK", Name: "Ride Service", IsMerchant: true, DefaultCategory: transport,
		}); err != nil {
			t.Fatal(err)
		}

		refund := map[string]any{"messages": []map[string]any{{
			"sender":      "CIB",
			"body":        "The transaction on your credit card#5678  from Ride Service  with EGP 40.00 on 21/10/26  at 18:04 has been refunded. Please try again. Thank you",
			"received_at": receivedAt,
		}}}
		res, out := post(token, refund)
		if res.StatusCode != http.StatusOK || len(out.Results) != 1 || out.Results[0].TransactionID == "" {
			t.Fatalf("got %d %+v", res.StatusCode, out)
		}

		tx, err := repo.GetTransaction(ctx, out.Results[0].TransactionID)
		if err != nil {
			t.Fatal(err)
		}
		if tx.Kind != models.RefundTxKind || tx.Category == nil || tx.Category.ID != transport.ID {
			t.Errorf("unexpected transaction %+v", tx)
		}
		gotAcc, err := repo.GetAccount(ctx, card.ID)
		if err != nil {
			t.Fatal(err)
		}
		if gotAcc.Amount != 140 {
			t.Errorf("account amount = %v, want 140", gotAcc.Amount)
		}
	})

	t.Run("uses the sms date in the user's time zone and their own templates first", func(t *testing.T) {
		card := &models.Account{
			ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZM", Name: "CIB debit e2e", Amount: 100,
			Currency: *egp, Kind: bankKind, SmsRefs: []string{"4321"},
		}
		if err := repo.CreateAccount(ctx, card); err != nil {
			t.Fatal(err)
		}
		withRow := *usr
		withRow.Email, withRow.FirstName, withRow.LastName, withRow.DefaultCurrency = "e2e@example.com", "E2E", "User", *egp
		if err := repo.CreateUser(ctx, &withRow); err != nil {
			t.Fatal(err)
		}
		dubai, _ := time.LoadLocation("Asia/Dubai")
		if err := repo.SetTimezone(ctx, dubai); err != nil {
			t.Fatal(err)
		}
		mine := &models.SMSTemplate{
			ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZN", Name: "mine", Action: models.SMSTemplateTransaction,
			TxKind: models.IncomeTxKind, IsEnabled: true, Body: "تم خصم مبلغ {{*}}",
		}
		if err := repo.CreateSMSTemplate(ctx, mine); err != nil {
			t.Fatal(err)
		}

		sms := "تم خصم مبلغ  EGP 25.00 من بطاقة الخصم المباشر المنتهية بـ **4321 عند Bakery في 15/10/25 18:12 ، الرصيد المتاح 75.00 EGP"
		msgs := map[string]any{"messages": []map[string]any{{
			"sender": "CIB", "body": sms, "received_at": time.Date(2025, 10, 15, 16, 0, 0, 0, time.UTC),
		}}}
		// the user's template has no amount, so it can't build a transaction
		// and the shared one reads it
		res, out := post(token, msgs)
		if res.StatusCode != http.StatusOK || out.Results[0].TransactionID == "" {
			t.Fatalf("got %d %+v", res.StatusCode, out)
		}
		tx, err := repo.GetTransaction(ctx, out.Results[0].TransactionID)
		if err != nil {
			t.Fatal(err)
		}
		if want := time.Date(2025, 10, 15, 14, 12, 0, 0, time.UTC); !tx.FulfilledAt.Equal(want) {
			t.Errorf("fulfilled at %v, want %v", tx.FulfilledAt, want)
		}
		cps, err := repo.GetCounterparties(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if tx.Counterparty == nil || !slices.ContainsFunc(cps, func(cp models.Counterparty) bool {
			return cp.ID == tx.Counterparty.ID && cp.Name == "Bakery" && cp.IsMerchant
		}) {
			t.Errorf("counterparty = %+v, want a new Bakery merchant", tx.Counterparty)
		}

		// an ignore template of the user wins over the shared ones
		mine.Action, mine.TxKind = models.SMSTemplateIgnore, models.NilTxKind
		if err := repo.UpdateSMSTemplate(ctx, mine); err != nil {
			t.Fatal(err)
		}
		msgs["messages"].([]map[string]any)[0]["received_at"] = time.Date(2025, 10, 15, 16, 5, 0, 0, time.UTC)
		if _, out := post(token, msgs); out.Results[0].Status != "ignored" {
			t.Errorf("status %q, want ignored", out.Results[0].Status)
		}
	})

	t.Run("retrying an sms a new template ignores shows it as ignored", func(t *testing.T) {
		if err := repo.CreateSMSTemplate(ctx, &models.SMSTemplate{
			ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZP", Name: "otp", Action: models.SMSTemplateIgnore,
			IsEnabled: true, Body: "Your OTP is {{*}}",
		}); err != nil {
			t.Fatal(err)
		}
		msgs, err := repo.GetSMSMessagesByStatus(ctx, models.SMSStatusUnparsed)
		if err != nil || len(msgs) != 1 {
			t.Fatalf("got %d unparsed messages, err: %v", len(msgs), err)
		}

		pair, _ := jwt.NewPair(usr)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/sms/"+msgs[0].ID+"/retry", nil)
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		body := string(b)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, `data-status="ignored"`) || !strings.Contains(body, "A template ignores") {
			t.Errorf("got %d, want the card as ignored", res.StatusCode)
		}
		if strings.Contains(body, "Failed") || strings.Contains(body, "/retry") {
			t.Error("an ignored card shouldn't look failed nor offer a retry")
		}
	})

	t.Run("rejects revoked devices", func(t *testing.T) {
		if err := repo.RevokeDevice(ctx, device.ID); err != nil {
			t.Fatal(err)
		}
		if res, _ := post(token, batch); res.StatusCode != http.StatusUnauthorized {
			t.Errorf("got %d", res.StatusCode)
		}
	})

	t.Run("signing in again from the same install reuses the device", func(t *testing.T) {
		again := &models.Device{
			ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZE", Name: "renamed",
			InstallID: device.InstallID, TokenID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZF",
		}
		if err := repo.RegisterDevice(ctx, again); err != nil {
			t.Fatal(err)
		}
		if again.ID != device.ID {
			t.Errorf("device id = %s, want %s", again.ID, device.ID)
		}

		devices, err := repo.GetDevices(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(devices) != 1 || devices[0].Name != "renamed" || devices[0].IsRevoked() {
			t.Fatalf("unexpected devices %+v", devices)
		}

		newToken, err := jwt.NewDeviceToken(usr, again.ID, again.TokenID, "")
		if err != nil {
			t.Fatal(err)
		}
		if res, _ := post(newToken, batch); res.StatusCode != http.StatusOK {
			t.Errorf("new token got %d", res.StatusCode)
		}
		if res, _ := post(token, batch); res.StatusCode != http.StatusUnauthorized {
			t.Errorf("old token got %d", res.StatusCode)
		}
	})

	t.Run("accepts tokens of devices registered before token ids", func(t *testing.T) {
		legacy := &models.Device{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZG", Name: "legacy"}
		if err := repo.CreateDevice(ctx, legacy); err != nil {
			t.Fatal(err)
		}
		legacyToken, err := jwt.NewDeviceToken(usr, legacy.ID, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if res, _ := post(legacyToken, batch); res.StatusCode != http.StatusOK {
			t.Errorf("got %d", res.StatusCode)
		}
	})
}
