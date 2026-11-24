package handler

import (
	"context"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/shareddb"
	"github.com/omareloui/tyche/internal/sharedqueries"
	"github.com/omareloui/tyche/web/views"
)

func TestTransferInputs(t *testing.T) {
	restore := fxrate.SetRates(map[string]map[string]float64{
		"usd": {"egp": 50, "eur": 0.8},
		"egp": {"usd": 0.02, "eur": 0.016},
	})
	t.Cleanup(restore)

	bank := models.AccountKind{Enum: models.AccKindBankAcc}
	accs := []models.Account{
		{ID: "egp1", Currency: models.Currency{String: "EGP"}, Kind: bank},
		{ID: "usd1", Currency: models.Currency{String: "USD"}, Kind: bank},
		{ID: "eur1", Currency: models.Currency{String: "EUR"}, Kind: bank},
	}

	parse := func(trigger string, vals url.Values) (from, to, toAmt string) {
		t.Helper()
		vals.Set("kind", models.TransferTxKind.String())
		r := httptest.NewRequest("POST", "/tx-inputs/f1", strings.NewReader(vals.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Hx-Trigger", trigger)
		r.SetPathValue("id", "f1")
		fd, err := Handler{}.parseTransactionAmountsAndKindFD(r, accs, nil)
		if err != nil {
			t.Fatal(err)
		}
		return fd.AccountIFD.AccountID.Value, fd.ToAccountIFD.AccountID.Value, fd.ToAccountIFD.Amount.Value
	}

	// Picking another to account keeps the from one.
	from, to, toAmt := parse("to_account_id-f1", url.Values{
		"account_id": {"usd1"}, "to_account_id": {"eur1"}, "amount": {"$ 100.00"},
	})
	if from != "usd1" || to != "eur1" {
		t.Errorf("accounts = %s → %s, want usd1 → eur1", from, to)
	}
	// 100 USD at 0.8 EUR a dollar.
	if toAmt != "80.00" {
		t.Errorf("to amount = %s, want 80.00", toAmt)
	}

	// Typing the amount converts it.
	_, _, toAmt = parse("amount-f1", url.Values{
		"account_id": {"usd1"}, "to_account_id": {"egp1"}, "amount": {"$ 12.00"}, "to_amount": {"1"},
	})
	if toAmt != "600.00" {
		t.Errorf("to amount = %s, want 600.00", toAmt)
	}

	// Changing the kind works the amount out again.
	_, _, toAmt = parse("kind-f1", url.Values{
		"account_id": {"egp1"}, "to_account_id": {"usd1"}, "amount": {"500"},
	})
	if toAmt != "10.00" {
		t.Errorf("to amount = %s, want 10.00", toAmt)
	}

	// Picking the from account as the to one moves the from one.
	from, to, _ = parse("to_account_id-f1", url.Values{
		"account_id": {"usd1"}, "to_account_id": {"usd1"}, "amount": {"1"},
	})
	if from == "usd1" || to != "usd1" {
		t.Errorf("accounts = %s → %s, want another → usd1", from, to)
	}

	// Another field changing keeps a received amount typed by hand.
	_, _, toAmt = parse("description-f1", url.Values{
		"account_id": {"usd1"}, "to_account_id": {"egp1"}, "amount": {"10"}, "to_amount": {"499"},
	})
	if toAmt != "499" {
		t.Errorf("to amount = %s, want 499", toAmt)
	}

	// A new date (asked by the received amount) works it out again.
	_, _, toAmt = parse("to_amount-f1", url.Values{
		"account_id": {"usd1"}, "to_account_id": {"egp1"}, "amount": {"10"}, "to_amount": {"499"},
		"scheduled_to": {"2026-11-01"},
	})
	if toAmt != "500.00" {
		t.Errorf("to amount = %s, want 500.00", toAmt)
	}
}

func TestTransferInputsDayRate(t *testing.T) {
	restore := fxrate.SetRates(map[string]map[string]float64{"usd": {"egp": 50}})
	t.Cleanup(restore)

	dir := t.TempDir()
	t.Chdir("../..") // the shared DB's migrations are read from ./sql/shared
	db, q, err := shareddb.Open(filepath.Join(dir, "shared.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if err := q.UpsertDailyFxRate(context.Background(), sharedqueries.UpsertDailyFxRateParams{
		Day: day, Base: "usd", Quote: "egp", Rate: 48,
	}); err != nil {
		t.Fatal(err)
	}
	h := Handler{fxJob: fxrate.NewJob(db, q, []string{"usd", "egp"}, time.Hour)}

	bank := models.AccountKind{Enum: models.AccKindBankAcc}
	accs := []models.Account{
		{ID: "usd1", Currency: models.Currency{String: "USD"}, Kind: bank},
		{ID: "egp1", Currency: models.Currency{String: "EGP"}, Kind: bank},
	}
	toAmount := func(date string) string {
		t.Helper()
		vals := url.Values{
			"kind": {models.TransferTxKind.String()}, "account_id": {"usd1"}, "to_account_id": {"egp1"},
			"amount": {"10"}, "scheduled_to": {date},
		}
		r := httptest.NewRequest("POST", "/tx-inputs/f1", strings.NewReader(vals.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Hx-Trigger", "to_amount-f1")
		r.SetPathValue("id", "f1")
		fd, err := h.parseTransactionAmountsAndKindFD(r, accs, nil)
		if err != nil {
			t.Fatal(err)
		}
		return fd.ToAccountIFD.Amount.Value
	}

	if got := toAmount("2026-11-01"); got != "480.00" {
		t.Errorf("on Nov 1 = %s, want 480.00 (that day's rate)", got)
	}
	if got := toAmount(time.Now().UTC().Format(time.DateOnly)); got != "500.00" {
		t.Errorf("today = %s, want 500.00 (today's rate)", got)
	}
}

func TestForeignInputs(t *testing.T) {
	restore := fxrate.SetRates(map[string]map[string]float64{"usd": {"egp": 50}})
	t.Cleanup(restore)

	h := Handler{}
	bank := models.AccountKind{Enum: models.AccKindBankAcc}
	egp := models.Currency{String: "EGP", Symbol: "EGP"}
	accs := []models.Account{
		{ID: "card", Currency: egp, Kind: bank, FxFeePercent: 3, FxFeeMin: 20},
	}
	parse := func(kind models.TxKind, cur, orig, trigger string) *views.TransactionAmountsAndKindFD {
		t.Helper()
		vals := url.Values{
			"kind": {kind.String()}, "account_id": {"card"}, "amount": {""},
			"original_currency": {cur}, "original_amount": {orig},
		}
		r := httptest.NewRequest("POST", "/tx-inputs/f1", strings.NewReader(vals.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Hx-Trigger", trigger+"-f1")
		r.SetPathValue("id", "f1")
		fd, err := h.parseTransactionAmountsAndKindFD(r, accs, nil)
		if err != nil {
			t.Fatal(err)
		}
		return fd
	}

	fd := parse(models.ExpenseTxKind, "usd", "100", "original_amount")
	if fd.AccountIFD.Amount.Value != "5150.00" || fd.Foreign == nil || fd.Foreign.Fee != "150" {
		t.Errorf("expense = %s, fee %+v, want 5150.00 with a 150 fee", fd.AccountIFD.Amount.Value, fd.Foreign)
	}
	if fd := parse(models.IncomeTxKind, "USD", "100", "kind"); fd.AccountIFD.Amount.Value != "5000.00" || fd.Foreign.Fee != "" {
		t.Errorf("income = %s, fee %q, want 5000.00 without a fee", fd.AccountIFD.Amount.Value, fd.Foreign.Fee)
	}
	if fd := parse(models.ExpenseTxKind, "EGP", "100", "original_amount"); fd.Foreign.Currency.Value != "" || fd.AccountIFD.Amount.Value != "" {
		t.Errorf("paid in the account's currency = %+v, amount %q", fd.Foreign, fd.AccountIFD.Amount.Value)
	}
	if fd := parse(models.TransferTxKind, "USD", "100", "kind"); fd.Foreign != nil {
		t.Errorf("transfers can't be foreign, got %+v", fd.Foreign)
	}
}
