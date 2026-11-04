package router_test

import (
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
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

func TestTransactionsPage(t *testing.T) {
	repo, ctx, _ := setupTenant(t)

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
	bank := &models.Account{ID: idgen.New(), Name: "Bank", Amount: 1_000_000, Currency: *egp, Kind: bankKind}
	if err := repo.CreateAccount(ctx, bank); err != nil {
		t.Fatal(err)
	}

	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	create := func(tx *models.Transaction) *models.Transaction {
		t.Helper()
		tx.ID, tx.Account = idgen.New(), *bank
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
		return tx
	}

	// 20 fulfilled expenses a month, January to June: 120 in all.
	var firstJan *models.Transaction
	for m := time.January; m <= time.June; m++ {
		for d := 1; d <= 20; d++ {
			tx := create(&models.Transaction{
				Kind: models.ExpenseTxKind, Amount: float64(d), Description: "groceries",
				ScheduledTo: day(m, d), FulfilledAt: day(m, d),
			})
			if firstJan == nil {
				firstJan = tx
			}
		}
	}
	create(&models.Transaction{Kind: models.ExpenseTxKind, Amount: 500, Description: "rent", ScheduledTo: time.Now().AddDate(0, 1, 0)})
	// Pays back a January expense, from the June chunk.
	refund := create(&models.Transaction{
		Kind: models.IncomeTxKind, Amount: 1, Description: "friend",
		ScheduledTo: day(time.June, 25), FulfilledAt: day(time.June, 25),
	})
	if err := repo.LinkReimbursement(ctx, refund.ID, firstJan.ID); err != nil {
		t.Fatal(err)
	}

	months := func(txs []models.Transaction) []time.Month {
		var ms []time.Month
		for _, tx := range txs {
			if m := tx.ScheduledTo.Month(); len(ms) == 0 || ms[len(ms)-1] != m {
				ms = append(ms, m)
			}
		}
		return ms
	}

	t.Run("chunks are whole months until everything is loaded", func(t *testing.T) {
		page, err := repo.GetTransactionsPage(ctx, models.TxFilter{}, time.Time{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Upcoming) != 1 {
			t.Errorf("got %d upcoming, want 1", len(page.Upcoming))
		}
		// The 50th is in April: June, May, April whole.
		if got := months(page.History); !slices.Equal(got, []time.Month{time.June, time.May, time.April}) {
			t.Errorf("first chunk months = %v", got)
		}
		if len(page.History) != 60 || !page.HasMore || !page.Until.Equal(day(time.April, 1)) {
			t.Errorf("first chunk: %d txs, has more %v, until %v", len(page.History), page.HasMore, page.Until)
		}

		total := len(page.History)
		for page.HasMore {
			before := page.Until
			if page, err = repo.GetTransactionsPage(ctx, models.TxFilter{}, before, nil); err != nil {
				t.Fatal(err)
			}
			if len(page.Upcoming) != 0 {
				t.Error("older chunk has upcoming transactions")
			}
			total += len(page.History)
		}
		if total != 120 {
			t.Errorf("loaded %d fulfilled transactions in all, want 120 (the reimbursement nested)", total)
		}

		all := time.Time{}
		page, err = repo.GetTransactionsPage(ctx, models.TxFilter{}, time.Time{}, &all)
		if err != nil {
			t.Fatal(err)
		}
		last := page.History[len(page.History)-1]
		if last.ID != firstJan.ID || len(last.Reimbursements) != 1 || last.Reimbursements[0].ID != refund.ID {
			t.Errorf("oldest expense %s has reimbursements %v, want %s", last.ID, last.Reimbursements, refund.ID)
		}
		if page.HasMore {
			t.Error("everything loaded but has more")
		}
	})

	t.Run("re-rendering keeps what's loaded", func(t *testing.T) {
		until := day(time.February, 1)
		page, err := repo.GetTransactionsPage(ctx, models.TxFilter{}, time.Time{}, &until)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.History) != 100 || !page.HasMore {
			t.Errorf("got %d txs, has more %v, want 100 and more", len(page.History), page.HasMore)
		}
	})

	t.Run("summary follows the analysis rule", func(t *testing.T) {
		sum, err := repo.GetTransactionsSummary(ctx, models.TxFilter{AccountID: bank.ID}, egp)
		if err != nil {
			t.Fatal(err)
		}
		// 6 months of 1..20, net of the 1 paid back; the reimbursement isn't
		// income and the scheduled rent isn't spent yet.
		want := models.TxSummary{Count: 121, Scheduled: 1, In: 0, Out: 6*210 - 1}
		if *sum != want {
			t.Errorf("summary = %+v, want %+v", *sum, want)
		}
	})

	t.Run("filters", func(t *testing.T) {
		min, max := 5.0, 6.0
		tests := []struct {
			name string
			f    models.TxFilter
			want int
		}{
			{"search description", models.TxFilter{Q: "GROCER"}, 120},
			{"search a nested reimbursement", models.TxFilter{Q: "friend"}, 1},
			{"search account", models.TxFilter{Q: "bank"}, 121},
			{"kind", models.TxFilter{Kind: models.IncomeTxKind}, 0},
			{"scheduled", models.TxFilter{Status: models.TxStatusScheduled}, 1},
			{"fulfilled", models.TxFilter{Status: models.TxStatusFulfilled}, 120},
			{"from sms", models.TxFilter{Source: models.TxSourceSMS}, 0},
			{"by hand", models.TxFilter{Source: models.TxSourceManual}, 121},
			{"dates", models.TxFilter{From: day(time.March, 10), To: day(time.March, 12)}, 3},
			{"amounts", models.TxFilter{MinAmount: &min, MaxAmount: &max}, 12},
			{"account", models.TxFilter{AccountID: bank.ID}, 121},
			{"other account", models.TxFilter{AccountID: idgen.New()}, 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				all := time.Time{}
				page, err := repo.GetTransactionsPage(ctx, tt.f, time.Time{}, &all)
				if err != nil {
					t.Fatal(err)
				}
				if got := len(page.Upcoming) + len(page.History); got != tt.want {
					t.Errorf("got %d transactions, want %d", got, tt.want)
				}
				sum, err := repo.GetTransactionsSummary(ctx, tt.f, egp)
				if err != nil {
					t.Fatal(err)
				}
				if sum.Count != tt.want {
					t.Errorf("summary counts %d transactions, want %d", sum.Count, tt.want)
				}
			})
		}
	})
}

func TestTxFilterToIncludesTheWholeDay(t *testing.T) {
	repo, ctx, _ := setupTenant(t)

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
	bank := &models.Account{ID: idgen.New(), Name: "Bank", Amount: 1000, Currency: *egp, Kind: bankKind}
	if err := repo.CreateAccount(ctx, bank); err != nil {
		t.Fatal(err)
	}

	// Late on the last day, as SMS transactions carry their time.
	at := time.Date(2026, time.March, 12, 23, 30, 0, 0, time.UTC)
	tx := &models.Transaction{
		ID: idgen.New(), Account: *bank, Kind: models.ExpenseTxKind, Amount: 10,
		ScheduledTo: at, FulfilledAt: at,
	}
	if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
		t.Fatal(err)
	}

	day := func(d int) time.Time { return time.Date(2026, time.March, d, 0, 0, 0, 0, time.UTC) }
	tests := []struct {
		name string
		f    models.TxFilter
		want int
	}{
		{"ending that day", models.TxFilter{From: day(10), To: day(12)}, 1},
		{"only that day", models.TxFilter{From: day(12), To: day(12)}, 1},
		{"ending the day before", models.TxFilter{From: day(10), To: day(11)}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			all := time.Time{}
			page, err := repo.GetTransactionsPage(ctx, tt.f, time.Time{}, &all)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(page.History); got != tt.want {
				t.Errorf("got %d transactions, want %d", got, tt.want)
			}
			sum, err := repo.GetTransactionsSummary(ctx, tt.f, egp)
			if err != nil {
				t.Fatal(err)
			}
			if sum.Count != tt.want {
				t.Errorf("summary counts %d, want %d", sum.Count, tt.want)
			}
		})
	}
}

func TestDeleteTransactionsBatch(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 100)

	create := func(amount float64) *models.Transaction {
		t.Helper()
		tx := &models.Transaction{
			ID: idgen.New(), Account: *bank, Amount: amount, Kind: models.ExpenseTxKind,
			ScheduledTo: time.Now(), FulfilledAt: time.Now(),
		}
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
		return tx
	}
	a, b, kept := create(10), create(20), create(5)

	// One id twice and one already gone are fine.
	w := httptest.NewRecorder()
	if _, err := callHandler(ctx, h.DeleteTransactions)(w, "POST", "/transactions/delete", "", url.Values{
		"id": {a.ID, b.ID, a.ID, idgen.New()},
	}); err != nil {
		t.Fatal(err)
	}
	if got := w.Header().Get("HX-Retarget"); got != "#tx-list" {
		t.Errorf("HX-Retarget = %q, want #tx-list", got)
	}
	for _, tx := range []*models.Transaction{a, b} {
		if _, err := repo.GetTransaction(ctx, tx.ID); err == nil {
			t.Errorf("transaction %v not deleted", tx.Amount)
		}
	}
	if _, err := repo.GetTransaction(ctx, kept.ID); err != nil {
		t.Errorf("kept transaction: %v", err)
	}
	acc, err := repo.GetAccount(ctx, bank.ID)
	if err != nil {
		t.Fatal(err)
	}
	if acc.Amount != 95 {
		t.Errorf("bank = %v, want 95", acc.Amount)
	}

	for name, ids := range map[string][]string{"none": nil, "invalid": {"nope"}} {
		if _, err := callHandler(ctx, h.DeleteTransactions)(httptest.NewRecorder(), "POST", "/transactions/delete", "", url.Values{"id": ids}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
