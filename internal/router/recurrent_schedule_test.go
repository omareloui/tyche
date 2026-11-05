package router_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func utcToday() time.Time {
	y, m, d := time.Now().UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// createTestRecurrentTx creates a recurrent expense of account from its form,
// returning it.
func createTestRecurrentTx(t *testing.T, ctx context.Context, repo *repository.Repository, hd *handler.Handler,
	acc *models.Account, amount string, start time.Time, unit models.RecurrentUnit, extra url.Values,
) *models.RecurrentTx {
	t.Helper()
	form := url.Values{
		"kind":         {models.ExpenseTxKind.String()},
		"account_id":   {acc.ID},
		"amount":       {amount},
		"description":  {"Gym"},
		"scheduled_to": {start.Format(time.DateOnly)},
		"is_recurrent": {"on"},
		"every":        {"1"},
		"unit":         {strconv.Itoa(int(unit))},
	}
	for k, v := range extra {
		form[k] = v
	}
	before, err := repo.GetRecurrentTxs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := callHandler(ctx, hd.CreateRecurrentTx)(httptest.NewRecorder(), "POST", "/recurrent-tx", "", form); err != nil {
		t.Fatal(err)
	}
	rts, err := repo.GetRecurrentTxs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rts) != len(before)+1 {
		t.Fatalf("recurrent txs = %d, want %d", len(rts), len(before)+1)
	}
	for _, rt := range rts {
		if !containsRtx(before, rt.ID) {
			got, err := repo.GetRecurrentTx(ctx, rt.ID)
			if err != nil {
				t.Fatal(err)
			}
			return got
		}
	}
	t.Fatal("new recurrent tx not found")
	return nil
}

func containsRtx(rts []models.RecurrentTx, id string) bool {
	for _, rt := range rts {
		if rt.ID == id {
			return true
		}
	}
	return false
}

func occurrences(t *testing.T, ctx context.Context, repo *repository.Repository, id string) (scheduled, fulfilled []models.Transaction) {
	t.Helper()
	txs, err := repo.GetRecurrentTxTransactions(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, tx := range txs {
		if tx.FulfilledAt.IsZero() {
			scheduled = append(scheduled, tx)
		} else {
			fulfilled = append(fulfilled, tx)
		}
	}
	return scheduled, fulfilled
}

func TestRecurrentEditPauseSkip(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	hd := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	today := utcToday()

	rt := createTestRecurrentTx(t, ctx, repo, hd, bank, "100", today.AddDate(0, 0, 3), models.RecurrentUnitMonth, nil)
	sched, _ := occurrences(t, ctx, repo, rt.ID)
	if len(sched) < 2 {
		t.Fatalf("scheduled = %d, want through next month", len(sched))
	}
	// Writing them again changes nothing.
	if err := repo.TopUpRecurrentTxs(ctx); err != nil {
		t.Fatal(err)
	}
	if again, _ := occurrences(t, ctx, repo, rt.ID); len(again) != len(sched) {
		t.Errorf("top up again wrote %d, had %d", len(again), len(sched))
	}

	// One edited on its own keeps its edit when the recurrent tx's amount
	// changes; the others follow, on the same days.
	overridden := sched[0] // newest first: the latest
	edited := overridden
	edited.Amount = 999
	if err := repo.UpdateTransaction(ctx, &overridden, &edited); err != nil {
		t.Fatal(err)
	}
	editForm := url.Values{
		"kind":         {models.ExpenseTxKind.String()},
		"account_id":   {bank.ID},
		"amount":       {"150"},
		"description":  {"Gym"},
		"is_recurrent": {"on"},
		"every":        {"1"},
		"unit":         {strconv.Itoa(int(models.RecurrentUnitMonth))},
		"next_on":      {sched[len(sched)-1].ScheduledTo.Format(time.DateOnly)},
	}
	if _, err := callHandler(ctx, hd.EditRecurrentTx)(httptest.NewRecorder(), "PUT", "/recurrent-tx/"+rt.ID, rt.ID, editForm); err != nil {
		t.Fatal(err)
	}
	after, _ := occurrences(t, ctx, repo, rt.ID)
	if len(after) != len(sched) {
		t.Fatalf("scheduled after edit = %d, want %d", len(after), len(sched))
	}
	for i, tx := range after {
		want := 150.0
		if tx.ID == overridden.ID {
			want = 999
		}
		if tx.Amount != want || !tx.ScheduledTo.Equal(sched[i].ScheduledTo) {
			t.Errorf("after edit %s: %v on %s, want %v on %s", tx.ID, tx.Amount, tx.ScheduledTo, want, sched[i].ScheduledTo)
		}
	}

	// Moving the next one moves the ones after it, the one edited on its own
	// stays.
	next := sched[len(sched)-1].ScheduledTo.AddDate(0, 0, 2)
	editForm.Set("next_on", next.Format(time.DateOnly))
	if _, err := callHandler(ctx, hd.EditRecurrentTx)(httptest.NewRecorder(), "PUT", "/recurrent-tx/"+rt.ID, rt.ID, editForm); err != nil {
		t.Fatal(err)
	}
	moved, _ := occurrences(t, ctx, repo, rt.ID)
	for _, tx := range moved {
		if tx.ID == overridden.ID {
			continue
		}
		if tx.ScheduledTo.Day() != next.Day() && next.Day() <= 28 {
			t.Errorf("moved one on %s, want on day %d", tx.ScheduledTo, next.Day())
		}
	}
	if !containsTx(moved, overridden.ID) {
		t.Error("the one edited on its own got deleted")
	}

	// Paused: nothing ahead, nothing written.
	if _, err := callHandler(ctx, hd.PauseRecurrentTx)(httptest.NewRecorder(), "PUT", "/recurrent-tx/"+rt.ID+"/pause", rt.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.TopUpRecurrentTxs(ctx); err != nil {
		t.Fatal(err)
	}
	if paused, _ := occurrences(t, ctx, repo, rt.ID); len(paused) != 0 {
		t.Errorf("paused has %d scheduled", len(paused))
	}
	if got, _ := repo.GetRecurrentTx(ctx, rt.ID); !got.IsPaused() {
		t.Error("not paused")
	}

	// Resumed: from today on.
	if _, err := callHandler(ctx, hd.ResumeRecurrentTx)(httptest.NewRecorder(), "PUT", "/recurrent-tx/"+rt.ID+"/resume", rt.ID, nil); err != nil {
		t.Fatal(err)
	}
	resumed, _ := occurrences(t, ctx, repo, rt.ID)
	if len(resumed) == 0 {
		t.Fatal("resumed has nothing scheduled")
	}
	for _, tx := range resumed {
		if tx.ScheduledTo.Before(today) {
			t.Errorf("resumed one on %s, before today", tx.ScheduledTo)
		}
	}

	// A skipped one isn't written again.
	skipped := resumed[len(resumed)-1]
	r := httptest.NewRequest("DELETE", "/recurrent-tx/"+rt.ID+"/occurrences/"+skipped.ID, nil).WithContext(ctx)
	r.SetPathValue("id", rt.ID)
	r.SetPathValue("txID", skipped.ID)
	if _, err := hd.SkipRecurrentOccurrence(httptest.NewRecorder(), r); err != nil {
		t.Fatal(err)
	}
	if err := repo.TopUpRecurrentTxs(ctx); err != nil {
		t.Fatal(err)
	}
	left, _ := occurrences(t, ctx, repo, rt.ID)
	if len(left) != len(resumed)-1 || containsTx(left, skipped.ID) {
		t.Errorf("after skip %d scheduled, had %d", len(left), len(resumed))
	}
	for _, tx := range left {
		if tx.ScheduledTo.Equal(skipped.ScheduledTo) {
			t.Error("skipped one written again")
		}
	}
}

func containsTx(txs []models.Transaction, id string) bool {
	for _, tx := range txs {
		if tx.ID == id {
			return true
		}
	}
	return false
}

func TestRecurrentAutoFulfill(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	hd := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	today := utcToday()

	// Daily, from 2 days ago, paying by itself: the 3 until today are
	// fulfilled on their days, flagged for review.
	rt := createTestRecurrentTx(t, ctx, repo, hd, bank, "10", today.AddDate(0, 0, -2), models.RecurrentUnitDay,
		url.Values{"auto_fulfill": {"on"}})
	if !rt.AutoFulfill {
		t.Fatal("auto fulfill not saved")
	}
	sched, done := occurrences(t, ctx, repo, rt.ID)
	if len(done) != 3 || len(sched) == 0 {
		t.Fatalf("fulfilled %d, scheduled %d, want 3 and the ones ahead", len(done), len(sched))
	}
	for _, tx := range sched {
		if !tx.ScheduledTo.After(today) {
			t.Errorf("one due on %s left scheduled", tx.ScheduledTo)
		}
	}
	for _, tx := range append(sched, done...) {
		if !tx.RecurrentAuto {
			t.Errorf("%s not marked auto in the list", tx.ID)
		}
	}
	if one, err := repo.GetTransaction(ctx, done[0].ID); err != nil || !one.RecurrentAuto {
		t.Errorf("single tx not marked auto: %v", err)
	}
	for _, tx := range done[:2] {
		if !tx.NeedsReview || !tx.FulfilledAt.Equal(tx.ScheduledTo) {
			t.Errorf("auto fulfilled %+v", tx)
		}
	}
	if got, err := repo.GetAccount(ctx, bank.ID); err != nil {
		t.Fatal(err)
	} else if got.Amount != 970 {
		t.Errorf("balance = %v, want 970", got.Amount)
	}
}

func TestRecurrentSMSMerge(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	hd := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	today := utcToday()

	rt := createTestRecurrentTx(t, ctx, repo, hd, bank, "300", today.AddDate(0, 0, 2), models.RecurrentUnitMonth, nil)
	sched, _ := occurrences(t, ctx, repo, rt.ID)
	first := sched[len(sched)-1]

	sms := func(amount float64, on time.Time) (*models.SMSMessage, *models.Transaction) {
		msg := &models.SMSMessage{ID: idgen.New(), Sender: "HSBC", Body: "debit", ReceivedAt: on, Hash: idgen.New(), Status: models.SMSStatusParsed}
		tx := &models.Transaction{
			ID: idgen.New(), Account: *bank, Kind: models.ExpenseTxKind, Amount: amount,
			ScheduledTo: on, FulfilledAt: on, NeedsReview: true,
		}
		if err := repo.CreateSMSMessage(ctx, msg, tx); err != nil {
			t.Fatal(err)
		}
		return msg, tx
	}

	// Two days early with a new price: it's the payment of the first one.
	msg, _ := sms(320, today)
	if msg.TransactionID != first.ID {
		t.Fatalf("sms transaction = %s, want the occurrence %s", msg.TransactionID, first.ID)
	}
	got, err := repo.GetTransaction(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FulfilledAt.IsZero() || got.Amount != 320 || !got.NeedsReview || !got.FromSMS || got.RecurrentTxID != rt.ID {
		t.Errorf("merged occurrence = %+v", got)
	}

	// Far from any: a transaction of its own, matched by hand with the next
	// one.
	far := today.AddDate(0, 0, 20)
	msg2, tx2 := sms(50, far)
	if msg2.TransactionID != tx2.ID {
		t.Fatalf("far sms merged into %s", msg2.TransactionID)
	}
	full, err := repo.GetTransaction(ctx, tx2.ID)
	if err != nil {
		t.Fatal(err)
	}
	cands, err := repo.GetRecurrentMatchCandidates(ctx, full)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) == 0 {
		t.Fatal("no occurrence to match by hand")
	}
	r := httptest.NewRequest("PUT", "/transactions/"+tx2.ID+"/recurrent-match",
		strings.NewReader(url.Values{"occurrence_id": {cands[0].ID}}.Encode())).WithContext(ctx)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", tx2.ID)
	if _, err := hd.MatchRecurrentOccurrence(httptest.NewRecorder(), r); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetTransaction(ctx, tx2.ID); err == nil {
		t.Error("sms transaction kept after matching")
	}
	matched, err := repo.GetTransaction(ctx, cands[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if matched.FulfilledAt.IsZero() || matched.Amount != 50 || !matched.FromSMS {
		t.Errorf("matched occurrence = %+v", matched)
	}

	if got, err := repo.GetAccount(ctx, bank.ID); err != nil {
		t.Fatal(err)
	} else if got.Amount != 1000-320-50 {
		t.Errorf("balance = %v, want %v", got.Amount, 1000-320-50)
	}
}

func TestRecurrentGamyaaLocked(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 10000)
	hd := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	gam := &models.Account{ID: idgen.New(), Name: "Office", Currency: *egp, Gamyaa: &models.Gamyaa{
		Installment: 1000, Members: 3, Shares: 1, Turns: []int{2},
		StartsOn: utcToday().AddDate(0, 1, 0), PayFrom: bank,
	}}
	for _, k := range kinds {
		if k.Enum == models.AccKindGamyaa {
			gam.Kind = k
		}
	}
	if err := repo.CreateGamyaa(ctx, gam); err != nil {
		t.Fatal(err)
	}
	rts, err := repo.GetRecurrentTxs(ctx)
	if err != nil || len(rts) != 1 {
		t.Fatalf("recurrent txs = %d, %v", len(rts), err)
	}
	rt := rts[0]
	if rt.GamyaaAccountID != gam.ID {
		t.Fatalf("gam'yaa = %q", rt.GamyaaAccountID)
	}

	// Writing the occurrences adds none to its installments.
	if err := repo.TopUpRecurrentTxs(ctx); err != nil {
		t.Fatal(err)
	}
	if sched, _ := occurrences(t, ctx, repo, rt.ID); len(sched) != 3 {
		t.Errorf("installments = %d, want 3", len(sched))
	}

	for name, fn := range map[string]handler.HandlerMethod{
		"edit form": hd.GetEditRecurrentTx, "edit": hd.EditRecurrentTx, "delete": hd.DeleteRecurrentTx, "pause": hd.PauseRecurrentTx,
	} {
		_, err := callHandler(ctx, fn)(httptest.NewRecorder(), "PUT", "/recurrent-tx/"+rt.ID, rt.ID, url.Values{})
		var re *resperr.RespError
		if !errors.As(err, &re) || re.Code != http.StatusForbidden {
			t.Errorf("%s: err = %v, want 403", name, err)
		}
	}
	if sched, _ := occurrences(t, ctx, repo, rt.ID); len(sched) != 3 {
		t.Errorf("installments after the refused actions = %d, want 3", len(sched))
	}
}
