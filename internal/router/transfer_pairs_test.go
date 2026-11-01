package router_test

import (
	"bytes"
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

// TestTransferPairs ingests the two CIB SMS of a transfer from the current
// account to the credit card, and checks they become one transfer pair,
// out of the income and spending.
func TestTransferPairs(t *testing.T) {
	repo, ctx, usr := setupTenant(t)

	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kindOf := func(e models.AccKind) models.AccountKind {
		for _, k := range kinds {
			if k.Enum == e {
				return k
			}
		}
		t.Fatalf("no account kind %v", e)
		return models.AccountKind{}
	}

	current := &models.Account{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZB", Name: "CIB Current", Amount: 100000,
		Currency: *egp, Kind: kindOf(models.AccKindBankAcc), SmsRefs: []string{"1234"},
	}
	card := &models.Account{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZE", Name: "CIB Credit Card", Amount: -30000,
		Currency: *egp, Kind: kindOf(models.AccKindCreditCard), CardNumber: "4111111111115678",
	}
	for _, a := range []*models.Account{current, card} {
		if err := repo.CreateAccount(ctx, a); err != nil {
			t.Fatal(err)
		}
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

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t))))
	defer srv.Close()

	ingest := func(body string, at time.Time) string {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"messages": []map[string]any{
			{"sender": "CIB", "body": body, "received_at": at},
		}})
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/sms", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out ingestResp
		_ = json.NewDecoder(res.Body).Decode(&out)
		if len(out.Results) != 1 || out.Results[0].Status != "parsed" {
			t.Fatalf("ingesting %q: %+v", body, out)
		}
		return out.Results[0].TransactionID
	}

	now := time.Now().Truncate(time.Second)
	day := now.Add(-2 * time.Hour)
	debit := func(amount string) string {
		return "Your account ending with ********1234 is debited with amount EGP " + amount + "DR On " +
			strings.ToUpper(day.Format("02 Jan 2006")) + " with transfer to  another account.  For more information, please contact 19666."
	}
	credit := func(amount string) string {
		return "تم سداد مبلغ " + amount + " جم فى بطاقتكم الائتمانية المنتهية بـ 5678 بتاريخ " + day.Format("02-01-06")
	}

	flowTotals := func() (in, out float64) {
		t.Helper()
		f, err := repo.GetFlow(ctx, egp)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Days {
			in += d.In
			out += d.Out
		}
		return in, out
	}

	topLevel := func() []models.Transaction {
		t.Helper()
		page, err := repo.GetTransactionsPage(ctx, models.TxFilter{}, time.Time{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return append(page.Upcoming, page.History...)
	}

	outID := ingest(debit("24,715.73"), day)
	inID := ingest(credit("24715.73"), day.Add(10*time.Minute))

	t.Run("pairs the two legs", func(t *testing.T) {
		out, err := repo.GetTransaction(ctx, outID)
		if err != nil {
			t.Fatal(err)
		}
		if out.Kind != models.ExpenseTxKind || out.TransferPairID != inID || out.TransferPair == nil || out.TransferPair.ID != inID {
			t.Fatalf("outgoing leg not paired: %+v", out)
		}
		txs := topLevel()
		if len(txs) != 1 || txs[0].ID != outID || txs[0].TransferPair == nil {
			t.Fatalf("want only the outgoing leg at the top with its pair nested, got %+v", txs)
		}
		if in, out := flowTotals(); in != 0 || out != 0 {
			t.Errorf("flow = +%v -%v, want nothing", in, out)
		}
	})

	t.Run("balances move", func(t *testing.T) {
		c, _ := repo.GetAccount(ctx, current.ID)
		k, _ := repo.GetAccount(ctx, card.ID)
		if c.Amount != 100000-24715.73 || k.Amount != -30000+24715.73 {
			t.Errorf("balances current %v card %v", c.Amount, k.Amount)
		}
	})

	t.Run("shows as one transfer", func(t *testing.T) {
		pair, _ := jwt.NewPair(usr)
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/transactions", nil)
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(res.Body)
		body := buf.String()
		for _, want := range []string{"Both sides", "Sent from CIB Current", "Received in CIB Credit Card", "/transactions/" + outID + "/transfer-pair"} {
			if !strings.Contains(body, want) {
				t.Errorf("page misses %q", want)
			}
		}
		// each SMS shows once, on its own side, none on the transfer row
		for _, sms := range []string{"is debited with amount", "فى بطاقتكم الائتمانية"} {
			if n := strings.Count(body, sms); n != 1 {
				t.Errorf("sms %q shown %d times, want 1", sms, n)
			}
		}
		if !strings.Contains(body, `data-id="`+outID+`"`) || !strings.Contains(body, `data-id="`+inID+`"`) {
			t.Error("want both sides as their own rows")
		}
	})

	t.Run("unlinks", func(t *testing.T) {
		if err := repo.UnlinkTransferPair(ctx, inID); err != nil {
			t.Fatal(err)
		}
		if txs := topLevel(); len(txs) != 2 {
			t.Fatalf("got %d top-level transactions, want 2", len(txs))
		}
		if in, out := flowTotals(); in == 0 || out == 0 {
			t.Errorf("flow = +%v -%v, want both counted again", in, out)
		}
	})

	t.Run("pairs past transfers", func(t *testing.T) {
		n, err := repo.PairTransfers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("paired %d, want 1", n)
		}
		if txs := topLevel(); len(txs) != 1 {
			t.Errorf("got %d top-level transactions, want 1", len(txs))
		}
	})

	t.Run("deleting a leg frees the other", func(t *testing.T) {
		in, err := repo.GetTransaction(ctx, inID)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.DeleteTransaction(ctx, in); err != nil {
			t.Fatal(err)
		}
		out, err := repo.GetTransaction(ctx, outID)
		if err != nil {
			t.Fatal(err)
		}
		if out.IsTransferLeg() {
			t.Errorf("outgoing leg still paired with %s", out.TransferPairID)
		}
	})

	t.Run("credit first, with a fee", func(t *testing.T) {
		later := day.Add(time.Hour)
		in := ingest(credit("995.00"), later)
		out := ingest(debit("1,000.00"), later.Add(3*time.Hour))
		tx, err := repo.GetTransaction(ctx, out)
		if err != nil {
			t.Fatal(err)
		}
		if tx.TransferPair == nil || tx.TransferPair.ID != in || tx.TransferFee() != 5 {
			t.Errorf("want paired with %s and a fee of 5, got %+v (fee %v)", in, tx.TransferPair, tx.TransferFee())
		}
	})

	t.Run("pairs by hand", func(t *testing.T) {
		later := day.Add(30 * time.Minute)
		out := ingest(debit("4,000.00"), later)
		in := ingest(credit("3,500.00"), later.Add(time.Minute))
		if tx, _ := repo.GetTransaction(ctx, out); tx.IsTransferLeg() {
			t.Fatalf("a 500 fee got auto paired with %s", tx.TransferPairID)
		}

		pair, _ := jwt.NewPair(usr)
		do := func(method, path string, form url.Values) string {
			t.Helper()
			req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("HX-Request", "true")
			req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(res.Body)
			if res.StatusCode != http.StatusOK {
				t.Fatalf("%s %s: %d %s", method, path, res.StatusCode, buf.String())
			}
			return buf.String()
		}

		picker := do(http.MethodGet, "/transactions/"+in+"/transfer-pair", nil)
		if !strings.Contains(picker, out) {
			t.Errorf("picker doesn't offer %s", out)
		}

		// an income can't be paired with another income
		otherIn := ingest(credit("100.00"), later.Add(5*time.Hour))
		if body := do(http.MethodPut, "/transactions/"+in+"/transfer-pair", url.Values{"other_id": {otherIn}}); !strings.Contains(body, "only an expense and an income") &&
			!strings.Contains(body, html.EscapeString("only an expense and an income")) {
			t.Errorf("no kinds error in %q", body)
		}

		do(http.MethodPut, "/transactions/"+in+"/transfer-pair", url.Values{"other_id": {out}})
		tx, err := repo.GetTransaction(ctx, out)
		if err != nil {
			t.Fatal(err)
		}
		if tx.TransferPair == nil || tx.TransferPair.ID != in || tx.TransferFee() != 500 {
			t.Errorf("want paired with %s and a fee of 500, got %+v", in, tx.TransferPair)
		}
	})
}
