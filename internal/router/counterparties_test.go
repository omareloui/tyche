package router_test

import (
	"slices"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

func TestMergeCounterparties(t *testing.T) {
	repo, ctx, _ := setupTenant(t)

	from := &models.Counterparty{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZD", Name: "Uber Trip", Aliases: []string{"UBER *TRIP"}, IsMerchant: true}
	into := &models.Counterparty{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZE", Name: "Uber", Aliases: []string{"UBER BV"}, IsMerchant: true}
	for _, cp := range []*models.Counterparty{from, into} {
		if err := repo.CreateCounterparty(ctx, cp); err != nil {
			t.Fatal(err)
		}
	}

	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	acc := &models.Account{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZF", Name: "Uber wallet", Amount: 100,
		Currency: *egp, Kind: kinds[0], Counterparty: from,
	}
	if err := repo.CreateAccount(ctx, acc); err != nil {
		t.Fatal(err)
	}
	tx := &models.Transaction{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZG", Account: *acc, Amount: 10, Kind: models.ExpenseTxKind,
		Counterparty: from, ScheduledTo: time.Now(),
	}
	if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
		t.Fatal(err)
	}

	got, err := repo.MergeCounterparties(ctx, from.ID, into.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"UBER BV", "Uber Trip", "UBER *TRIP"}; !slices.Equal(got.Aliases, want) {
		t.Errorf("returned aliases = %q, want %q", got.Aliases, want)
	}

	stored, err := repo.GetCounterparty(ctx, into.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(stored.Aliases, got.Aliases) {
		t.Errorf("stored aliases = %q, want %q", stored.Aliases, got.Aliases)
	}

	if _, err := repo.GetCounterparty(ctx, from.ID); err == nil {
		t.Error("merged counterparty still exists")
	}

	gotTx, err := repo.GetTransaction(ctx, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotTx.Counterparty == nil || gotTx.Counterparty.ID != into.ID {
		t.Errorf("transaction counterparty = %+v, want %s", gotTx.Counterparty, into.ID)
	}

	gotAcc, err := repo.GetAccount(ctx, acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotAcc.Counterparty == nil || gotAcc.Counterparty.ID != into.ID {
		t.Errorf("account counterparty = %+v, want %s", gotAcc.Counterparty, into.ID)
	}
}
