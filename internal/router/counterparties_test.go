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

func TestArchiveCounterparty(t *testing.T) {
	repo, ctx, _ := setupTenant(t)

	cp := &models.Counterparty{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZH", Name: "Old Gym", IsMerchant: true}
	if err := repo.CreateCounterparty(ctx, cp); err != nil {
		t.Fatal(err)
	}

	if err := repo.SetCounterpartyArchived(ctx, cp.ID, true); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetCounterparty(ctx, cp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsArchived {
		t.Error("counterparty not archived")
	}

	// Editing it keeps it archived.
	got.Name = "Old Gym Club"
	if err := repo.UpdateCounterparty(ctx, got); err != nil {
		t.Fatal(err)
	}
	cps, err := repo.GetCounterparties(ctx)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(cps, func(c models.Counterparty) bool { return c.ID == cp.ID })
	if i < 0 || !cps[i].IsArchived || cps[i].Name != "Old Gym Club" {
		t.Errorf("listed counterparty = %+v, want archived Old Gym Club", cps)
	}

	if err := repo.SetCounterpartyArchived(ctx, cp.ID, false); err != nil {
		t.Fatal(err)
	}
	if got, err = repo.GetCounterparty(ctx, cp.ID); err != nil || got.IsArchived {
		t.Errorf("unarchived counterparty = %+v (%v), want not archived", got, err)
	}
}

func TestCounterpartyRatingNoteAndLocations(t *testing.T) {
	repo, ctx, _ := setupTenant(t)

	cp := &models.Counterparty{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZJ", Name: "Cilantro", IsMerchant: true, Rating: 4, Note: "Good wifi",
		Locations: []models.CounterpartyLocation{
			{Label: "Zamalek", Address: "26 July St"},
			{Label: "Maadi", MapsURL: "https://maps.app.goo.gl/x"},
		},
	}
	if err := repo.CreateCounterparty(ctx, cp); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetCounterparty(ctx, cp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Rating != 4 || got.Note != "Good wifi" {
		t.Errorf("rating, note = %d, %q, want 4, Good wifi", got.Rating, got.Note)
	}
	if labels := locationLabels(got.Locations); !slices.Equal(labels, []string{"Zamalek", "Maadi"}) {
		t.Errorf("locations = %q, want Zamalek, Maadi", labels)
	}

	// Editing replaces the locations, in the new order.
	got.Locations = []models.CounterpartyLocation{got.Locations[1], {Label: "Heliopolis"}}
	if err := repo.UpdateCounterparty(ctx, got); err != nil {
		t.Fatal(err)
	}
	cps, err := repo.GetCounterparties(ctx)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(cps, func(c models.Counterparty) bool { return c.ID == cp.ID })
	if i < 0 {
		t.Fatal("counterparty not listed")
	}
	if labels := locationLabels(cps[i].Locations); !slices.Equal(labels, []string{"Maadi", "Heliopolis"}) {
		t.Errorf("listed locations = %q, want Maadi, Heliopolis", labels)
	}

	if err := repo.SetCounterpartyRating(ctx, cp.ID, 0); err != nil {
		t.Fatal(err)
	}
	if got, err = repo.GetCounterparty(ctx, cp.ID); err != nil || got.Rating != 0 {
		t.Errorf("cleared rating = %+v (%v), want 0", got, err)
	}

	// Deleting it drops its locations.
	if err := repo.DeleteCounterparty(ctx, cp.ID); err != nil {
		t.Fatal(err)
	}
	cps, err = repo.GetCounterparties(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cps {
		if len(c.Locations) > 0 {
			t.Errorf("locations left after delete: %+v", c)
		}
	}
}

func TestMergeCounterpartiesLocations(t *testing.T) {
	repo, ctx, _ := setupTenant(t)

	from := &models.Counterparty{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZK", Name: "Cilantro Cafe", IsMerchant: true, Rating: 5, Note: "Quiet",
		Locations: []models.CounterpartyLocation{{Label: "Maadi"}},
	}
	into := &models.Counterparty{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZM", Name: "Cilantro", IsMerchant: true, Note: "Good wifi",
		Locations: []models.CounterpartyLocation{{Label: "Zamalek"}},
	}
	for _, cp := range []*models.Counterparty{from, into} {
		if err := repo.CreateCounterparty(ctx, cp); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := repo.MergeCounterparties(ctx, from.ID, into.ID); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetCounterparty(ctx, into.ID)
	if err != nil {
		t.Fatal(err)
	}
	if labels := locationLabels(got.Locations); !slices.Equal(labels, []string{"Zamalek", "Maadi"}) {
		t.Errorf("locations = %q, want Zamalek, Maadi", labels)
	}
	if got.Rating != 5 || got.Note != "Good wifi\n\nQuiet" {
		t.Errorf("rating, note = %d, %q, want 5, joined notes", got.Rating, got.Note)
	}
}

func locationLabels(locs []models.CounterpartyLocation) []string {
	labels := make([]string, len(locs))
	for i, l := range locs {
		labels[i] = l.Label
	}
	return labels
}
