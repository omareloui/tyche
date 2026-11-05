package views

import (
	"context"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

func TestBuildTxListUpcomingGroups(t *testing.T) {
	cur := models.Currency{String: "EGP"}
	today := dateOf(time.Now())
	ms := models.MonthStart(25)
	period := ms.Of(today)
	tx := func(day time.Time, kind models.TxKind, amount float64) models.Transaction {
		return models.Transaction{Kind: kind, Amount: amount, ScheduledTo: day, Account: models.Account{Currency: cur}}
	}
	page := &models.TxPage{
		MonthStart: ms,
		Upcoming: []models.Transaction{
			tx(period.From.AddDate(0, -1, 0), models.ExpenseTxKind, 10), // overdue from last month
			tx(period.Last(), models.ExpenseTxKind, 20),
			tx(period.Last(), models.IncomeTxKind, 100),
			tx(period.To, models.ExpenseTxKind, 40),
		},
	}

	l := buildTxList(context.Background(), page, &cur)
	if got := len(l.ThisMonth.Txs); got != 3 {
		t.Fatalf("this month has %d, want 3", got)
	}
	if l.ThisMonth.Overdue != 1 {
		t.Errorf("overdue %d, want 1", l.ThisMonth.Overdue)
	}
	if l.ThisMonth.In != 100 || l.ThisMonth.Out != 30 {
		t.Errorf("this month in %v out %v, want 100 and 30", l.ThisMonth.In, l.ThisMonth.Out)
	}
	if len(l.Later.Txs) != 1 || l.Later.Out != 40 {
		t.Errorf("later has %d (out %v), want 1 (out 40)", len(l.Later.Txs), l.Later.Out)
	}
	if !l.Later.Period.From.Equal(period.To) {
		t.Errorf("later from %v, want %v", l.Later.Period.From, period.To)
	}
}
