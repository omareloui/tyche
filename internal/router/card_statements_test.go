package router_test

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestCardStatement(t *testing.T) {
	repo, ctx, _ := setupTenant(t)

	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 5000)
	card := newTestAccount(t, repo, ctx, "Visa", "EGP", models.AccKindCreditCard, 0)
	card.Cycle = models.CardCycle{StatementDay: 15, DueDays: 25}
	if err := repo.UpdateAccount(ctx, card); err != nil {
		t.Fatal(err)
	}

	loc, err := repo.GetTimezone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	closed := card.Cycle.LastClose(time.Now().In(loc))
	since := closed.AddDate(0, 0, 1)

	for _, tx := range []*models.Transaction{
		// on the statement's day: in it
		{Amount: 1000, Kind: models.ExpenseTxKind, FulfilledAt: closed.Add(20 * time.Hour)},
		// after it closed: in the next one
		{Amount: 300, Kind: models.ExpenseTxKind, FulfilledAt: since.Add(time.Hour)},
		// a payment
		{Amount: 400, Kind: models.TransferTxKind, Account: *bank, ToAccount: card, ToAmount: 400, FulfilledAt: since.Add(time.Hour)},
		// scheduled, not paid yet
		{Amount: 50, Kind: models.ExpenseTxKind},
	} {
		tx.ID = idgen.New()
		if tx.Account.ID == "" {
			tx.Account = *card
		}
		tx.ScheduledTo = tx.FulfilledAt
		if tx.ScheduledTo.IsZero() {
			tx.ScheduledTo = time.Now().AddDate(0, 0, 3)
		}
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
	}

	got, err := repo.GetAccount(ctx, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	s := got.Statement
	if s == nil {
		t.Fatal("no statement")
	}
	if !s.ClosedOn.Equal(closed) || !s.DueOn.Equal(closed.AddDate(0, 0, 25)) {
		t.Errorf("closed %v due %v", s.ClosedOn, s.DueOn)
	}
	if s.Balance != 1000 || s.Paid != 400 || s.Due != 600 {
		t.Errorf("balance %v paid %v due %v, want 1000 / 400 / 600", s.Balance, s.Paid, s.Due)
	}

	dues, err := repo.GetCardDues(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dues) != 1 || dues[0].ID != card.ID {
		t.Fatalf("dues %v, want the card", dues)
	}

	// paying the rest clears it
	pay := &models.Transaction{
		ID: idgen.New(), Amount: 600, Kind: models.TransferTxKind, Account: *bank, ToAccount: card, ToAmount: 600,
		ScheduledTo: since.Add(2 * time.Hour), FulfilledAt: since.Add(2 * time.Hour),
	}
	if err := repo.CreateTransaction(ctx, pay, nil, nil); err != nil {
		t.Fatal(err)
	}
	if dues, err = repo.GetCardDues(ctx); err != nil || len(dues) != 0 {
		t.Errorf("dues %v (%v), want none once paid", dues, err)
	}
}

func TestCardCycleForm(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil)
	card := newTestAccount(t, repo, ctx, "Visa", "EGP", models.AccKindCreditCard, 0)
	edit := callHandler(ctx, h.EditAccount)

	form := func(stmt, mode, due string) url.Values {
		return url.Values{
			"name": {card.Name}, "kind_id": {card.Kind.ID}, "currency_id": {card.Currency.ID},
			"statement_day": {stmt}, "due_mode": {mode}, "due_value": {due},
		}
	}

	w := httptest.NewRecorder()
	if _, err := edit(w, "PUT", "/accounts/"+card.ID, card.ID, form("25", "day", "15")); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetAccount(ctx, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := (models.CardCycle{StatementDay: 25, DueDay: 15}); got.Cycle != want || got.Statement == nil {
		t.Errorf("cycle %+v statement %v, want %+v", got.Cycle, got.Statement, want)
	}

	w = httptest.NewRecorder()
	c, err := edit(w, "PUT", "/accounts/"+card.ID, card.ID, form("40", "days", ""))
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := c.Render(ctx, &sb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "at most 31") || !strings.Contains(sb.String(), "This field is required") {
		t.Errorf("no cycle errors in the form")
	}
	if got, _ := repo.GetAccount(ctx, card.ID); got.Cycle.StatementDay != 25 {
		t.Errorf("invalid cycle saved: %+v", got.Cycle)
	}
}
