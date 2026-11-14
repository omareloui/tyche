package router_test

import (
	"bytes"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestQuickNoteEdit(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	hd := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)

	render := func(t *testing.T, c templ.Component) string {
		t.Helper()
		var b bytes.Buffer
		if err := c.Render(ctx, &b); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	put := func(t *testing.T, fn handler.HandlerMethod, id, note string) string {
		t.Helper()
		c, err := callHandler(ctx, fn)(httptest.NewRecorder(), "PUT", "/x/"+id+"/note", id, url.Values{"note": {note}})
		if err != nil {
			t.Fatal(err)
		}
		return render(t, c)
	}

	t.Run("transaction", func(t *testing.T) {
		tx := &models.Transaction{
			ID: idgen.New(), Account: *bank, Kind: models.ExpenseTxKind, Amount: 10,
			ScheduledTo: time.Now(), FulfilledAt: time.Now(),
		}
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
		body := put(t, hd.SetTransactionNote, tx.ID, "  **bold** <script>x</script> ")
		if !strings.Contains(body, "<strong>bold</strong>") || strings.Contains(body, "<script>") {
			t.Errorf("answer doesn't render the note safely: %s", body)
		}
		if got, _ := repo.GetTransaction(ctx, tx.ID); got.Note != "**bold** <script>x</script>" {
			t.Errorf("note = %q", got.Note)
		}

		body = put(t, hd.SetTransactionNote, tx.ID, strings.Repeat("a", models.MaxNoteLen+1))
		if !strings.Contains(body, "Keep it under") {
			t.Error("an overlong note isn't refused")
		}
		if got, _ := repo.GetTransaction(ctx, tx.ID); got.Note != "**bold** <script>x</script>" {
			t.Errorf("refused note saved: %q", got.Note[:20])
		}

		put(t, hd.SetTransactionNote, tx.ID, "")
		if got, _ := repo.GetTransaction(ctx, tx.ID); got.Note != "" {
			t.Errorf("note not cleared: %q", got.Note)
		}
	})

	t.Run("recurrent tx and its occurrences", func(t *testing.T) {
		rt := createTestRecurrentTx(t, ctx, repo, hd, bank, "100", utcToday().AddDate(0, 0, 3), models.RecurrentUnitMonth, nil)
		sched, _ := occurrences(t, ctx, repo, rt.ID)
		if len(sched) < 2 {
			t.Fatalf("scheduled = %d", len(sched))
		}
		// One with its own note keeps it.
		put(t, hd.SetTransactionNote, sched[0].ID, "mine")
		put(t, hd.SetRecurrentTxNote, rt.ID, "shared")
		if got, _ := repo.GetRecurrentTx(ctx, rt.ID); got.Note != "shared" {
			t.Errorf("recurrent note = %q", got.Note)
		}
		after, _ := occurrences(t, ctx, repo, rt.ID)
		for _, tx := range after {
			want := "shared"
			if tx.ID == sched[0].ID {
				want = "mine"
			}
			if tx.Note != want {
				t.Errorf("occurrence %s note = %q, want %q", tx.ID, tx.Note, want)
			}
		}
	})

	t.Run("account", func(t *testing.T) {
		put(t, hd.SetAccountNote, bank.ID, "branch: _downtown_")
		if got, _ := repo.GetAccount(ctx, bank.ID); got.Note != "branch: _downtown_" {
			t.Errorf("account note = %q", got.Note)
		}
	})

	t.Run("wish", func(t *testing.T) {
		egp, _ := repo.GetCurrencyByString(ctx, "EGP")
		item := &models.WishlistItem{ID: idgen.New(), Name: "Lamp", Price: 400, Currency: *egp, CoolingDays: 7, CreatedAt: time.Now()}
		if err := repo.CreateWishlistItem(ctx, item); err != nil {
			t.Fatal(err)
		}
		put(t, hd.SetWishlistItemNote, item.ID, "for the desk")
		if got, _ := repo.GetWishlistItem(ctx, item.ID); got.Note != "for the desk" {
			t.Errorf("wish note = %q", got.Note)
		}
	})

	t.Run("counterparty", func(t *testing.T) {
		cp := &models.Counterparty{ID: idgen.New(), Name: "Shop", IsMerchant: true}
		if err := repo.CreateCounterparty(ctx, cp); err != nil {
			t.Fatal(err)
		}
		put(t, hd.SetCounterpartyNote, cp.ID, "closes at 10")
		if got, _ := repo.GetCounterparty(ctx, cp.ID); got.Note != "closes at 10" {
			t.Errorf("counterparty note = %q", got.Note)
		}
		if body := put(t, hd.SetCounterpartyNote, cp.ID, strings.Repeat("a", models.MaxCounterpartyNoteLen+1)); !strings.Contains(body, "Keep it under") {
			t.Error("an overlong note isn't refused")
		}
	})
}
