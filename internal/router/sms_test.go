package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
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
	return repository.New(), ctx, usr
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
		Currency: *egp, Kind: bankKind, SmsRef: "123-456***-789",
	}
	if err := repo.CreateAccount(ctx, acc); err != nil {
		t.Fatal(err)
	}

	device := &models.Device{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZC", Name: "test"}
	if err := repo.CreateDevice(ctx, device); err != nil {
		t.Fatal(err)
	}
	token, err := jwt.NewDeviceToken(usr, device.ID)
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New())))
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
			Currency: *egp, Kind: bankKind, SmsRef: "999-999***-111",
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

	t.Run("rejects revoked devices", func(t *testing.T) {
		if err := repo.RevokeDevice(ctx, device.ID); err != nil {
			t.Fatal(err)
		}
		if res, _ := post(token, batch); res.StatusCode != http.StatusUnauthorized {
			t.Errorf("got %d", res.StatusCode)
		}
	})
}
