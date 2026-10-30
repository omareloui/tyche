package router_test

import (
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

func TestDeleteTransactionUndoesBalances(t *testing.T) {
	repo, ctx, _ := setupTenant(t)

	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var bankKind, debtKind models.AccountKind
	for _, k := range kinds {
		switch k.Enum {
		case models.AccKindBankAcc:
			bankKind = k
		case models.AccKindDebt:
			debtKind = k
		}
	}

	bank := &models.Account{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZA", Name: "Bank", Amount: 100, Currency: *egp, Kind: bankKind}
	debt := &models.Account{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZB", Name: "Friend", Currency: *egp, Kind: debtKind}
	for _, acc := range []*models.Account{bank, debt} {
		if err := repo.CreateAccount(ctx, acc); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name     string
		tx       *models.Transaction
		wantBank float64
		wantDebt float64
	}{
		{
			name: "debt",
			tx: &models.Transaction{
				ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZC", Account: *bank, Amount: 30, Kind: models.DebtTxKind,
				ToAccount: debt, ToAmount: 30,
			},
			wantBank: 70, wantDebt: 30,
		},
		{
			name: "refund",
			tx: &models.Transaction{
				ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZD", Account: *bank, Amount: 20, Kind: models.RefundTxKind,
			},
			wantBank: 120,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			balances := func() (float64, float64) {
				t.Helper()
				b, err := repo.GetAccount(ctx, bank.ID)
				if err != nil {
					t.Fatal(err)
				}
				d, err := repo.GetAccount(ctx, debt.ID)
				if err != nil {
					t.Fatal(err)
				}
				return b.Amount, d.Amount
			}

			tt.tx.ScheduledTo, tt.tx.FulfilledAt = time.Now(), time.Now()
			if err := repo.CreateTransaction(ctx, tt.tx, nil, nil); err != nil {
				t.Fatal(err)
			}
			if b, d := balances(); b != tt.wantBank || d != tt.wantDebt {
				t.Errorf("after create: bank = %v, debt = %v, want %v, %v", b, d, tt.wantBank, tt.wantDebt)
			}

			if err := repo.DeleteTransaction(ctx, tt.tx); err != nil {
				t.Fatal(err)
			}
			if b, d := balances(); b != 100 || d != 0 {
				t.Errorf("after delete: bank = %v, debt = %v, want 100, 0", b, d)
			}
		})
	}
}
