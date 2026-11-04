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

	// the 50 scheduled in 3 days counts if the statement closes by then
	wantSched := 0.0
	if !time.Now().AddDate(0, 0, 3).In(loc).After(card.Cycle.NextClose(time.Now().In(loc)).AddDate(0, 0, 1)) {
		wantSched = 50
	}
	if s.SoFar != 900 || s.Scheduled != wantSched || s.Projected != 900+wantSched {
		t.Errorf("so far %v scheduled %v projected %v, want 900 / %v / %v", s.SoFar, s.Scheduled, s.Projected, wantSched, 900+wantSched)
	}

	next := card.Cycle.NextClose(time.Now().In(loc))
	for _, tx := range []*models.Transaction{
		// on the next statement's day: in it
		{Amount: 200, Kind: models.ExpenseTxKind, ScheduledTo: next.Add(20 * time.Hour)},
		// after it: not
		{Amount: 70, Kind: models.ExpenseTxKind, ScheduledTo: next.AddDate(0, 0, 1).Add(time.Hour)},
		// a scheduled payment
		{Amount: 100, Kind: models.TransferTxKind, Account: *bank, ToAccount: card, ToAmount: 100, ScheduledTo: next.Add(time.Hour)},
	} {
		tx.ID = idgen.New()
		if tx.Account.ID == "" {
			tx.Account = *card
		}
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
	}

	cards, err := repo.GetCardStatements(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].ID != card.ID {
		t.Fatalf("cards %v, want the card", cards)
	}
	if s := cards[0].Statement; s.Due != 600 || s.Scheduled != wantSched+100 || s.Projected != 1000+wantSched {
		t.Errorf("due %v scheduled %v projected %v, want 600 / %v / %v", s.Due, s.Scheduled, s.Projected, wantSched+100, 1000+wantSched)
	}

	// paying the rest clears it
	pay := &models.Transaction{
		ID: idgen.New(), Amount: 600, Kind: models.TransferTxKind, Account: *bank, ToAccount: card, ToAmount: 600,
		ScheduledTo: since.Add(2 * time.Hour), FulfilledAt: since.Add(2 * time.Hour),
	}
	if err := repo.CreateTransaction(ctx, pay, nil, nil); err != nil {
		t.Fatal(err)
	}
	if cards, err = repo.GetCardStatements(ctx); err != nil || len(cards) != 1 || cards[0].Statement.Due != 0 {
		t.Errorf("cards %v (%v), want the card with nothing due once paid", cards, err)
	}
}

func TestCardCycleForm(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
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
