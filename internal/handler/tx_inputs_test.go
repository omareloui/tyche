package handler

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/models"
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

	// Editing the received amount by hand keeps it.
	_, _, toAmt = parse("to_amount-f1", url.Values{
		"account_id": {"usd1"}, "to_account_id": {"egp1"}, "amount": {"10"}, "to_amount": {"499"},
	})
	if toAmt != "499" {
		t.Errorf("to amount = %s, want 499", toAmt)
	}
}
