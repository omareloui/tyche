package router_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
)

func TestReconvertForeignTxs(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.DefaultCurrency = "e2e@example.com", *egp
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	card := newTestAccount(t, repo, ctx, "Card", "EGP", models.AccKindCreditCard, 0)
	card.FxFeePercent, card.FxFeeMin = 3, 20
	if err := repo.UpdateAccount(ctx, card); err != nil {
		t.Fatal(err)
	}

	nov := func(d int) time.Time { return time.Date(2026, time.November, d, 12, 0, 0, 0, time.UTC) }
	create := func(kind models.TxKind, amount, orig float64, day time.Time) string {
		t.Helper()
		tx := &models.Transaction{
			ID: idgen.New(), Account: *card, Kind: kind, Amount: amount,
			ScheduledTo: day, FulfilledAt: day,
			Original: &models.ForeignAmount{Amount: orig, Currency: "USD"},
		}
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
		return tx.ID
	}
	// Converted at 50 when read, without a fee.
	expense := create(models.ExpenseTxKind, 5000, 100, nov(1))
	refund := create(models.RefundTxKind, 500, 10, nov(1))
	later := create(models.ExpenseTxKind, 2500, 50, nov(2))

	balance := func() float64 {
		t.Helper()
		a, err := repo.GetAccount(ctx, card.ID)
		if err != nil {
			t.Fatal(err)
		}
		return a.Amount
	}
	check := func(id string, amount, fee float64) {
		t.Helper()
		tx, err := repo.GetTransaction(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(tx.Amount-amount) > 1e-6 || math.Abs(tx.FxFee-fee) > 1e-6 {
			t.Errorf("amount, fee = %v, %v, want %v, %v", tx.Amount, tx.FxFee, amount, fee)
		}
	}
	start := balance()

	// Nov 1's rate is had, Nov 2's only as today's fallback: left for later.
	rates := map[int]float64{1: 48}
	rateOn := func(_ context.Context, day time.Time, from, to string) (float64, bool, error) {
		if from != "USD" || to != "EGP" {
			t.Errorf("rate %s → %s", from, to)
		}
		if v, ok := rates[day.Day()]; ok {
			return v, true, nil
		}
		return 51, false, nil
	}
	if err := repo.ReconvertForeignTxs(ctx, rateOn); err != nil {
		t.Fatal(err)
	}
	check(expense, 4944, 144) // 4800 + 3%
	check(refund, 480, 0)     // no fee on money coming back
	check(later, 2500, 0)
	// The expense took 56 less, the refund brought 20 less.
	if got := balance() - start; math.Abs(got-36) > 1e-6 {
		t.Errorf("balance moved by %v, want 36", got)
	}

	// The next run gets Nov 2's rate, then it's done.
	rates[2] = 52
	if err := repo.ReconvertForeignTxs(ctx, rateOn); err != nil {
		t.Fatal(err)
	}
	check(later, 2678, 78) // 2600 + 3%
	if got := balance() - start; math.Abs(got-(36-178)) > 1e-6 {
		t.Errorf("balance moved by %v, want %v", got, 36-178)
	}
	if err := repo.ReconvertForeignTxs(ctx, func(context.Context, time.Time, string, string) (float64, bool, error) {
		t.Error("ran again once done")
		return 0, false, nil
	}); err != nil {
		t.Fatal(err)
	}
}
