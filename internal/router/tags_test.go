package router_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestTags(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.DefaultCurrency = "e2e@example.com", *egp
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 10000)
	cash := newTestAccount(t, repo, ctx, "Wallet", "EGP", models.AccKindBankAcc, 100)

	today := tenantToday()
	from, to := today.AddDate(0, 0, -5), today.AddDate(0, 0, 5)
	trip := &models.Tag{Name: "Dubai trip", Color: "blue", StartsOn: &from, EndsOn: &to, AutoTag: true}
	if err := repo.SaveTag(ctx, trip); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveTag(ctx, &models.Tag{Name: "dubai TRIP"}); !errors.Is(err, repository.ErrTagTaken) {
		t.Errorf("same name any case: err = %v, want ErrTagTaken", err)
	}

	newTx := func(kind models.TxKind, amount float64, on time.Time, tags []models.Tag, mods ...func(*models.Transaction)) *models.Transaction {
		t.Helper()
		tx := &models.Transaction{
			ID: idgen.New(), Account: *bank, Kind: kind, Amount: amount,
			ScheduledTo: on, FulfilledAt: on, Tags: tags,
		}
		for _, m := range mods {
			m(tx)
		}
		if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
		return tx
	}
	tagIDs := func(id string) []string {
		t.Helper()
		tx, err := repo.GetTransaction(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, tg := range tx.Tags {
			out = append(out, tg.ID)
		}
		return out
	}

	// Auto tagged in the trip's days, not outside, not transfers; a typed
	// name creates its tag.
	inTrip := newTx(models.ExpenseTxKind, 100, today, []models.Tag{{Name: "Food"}})
	outTrip := newTx(models.ExpenseTxKind, 50, today.AddDate(0, 0, -20), nil)
	transfer := newTx(models.TransferTxKind, 200, today, nil, func(tx *models.Transaction) {
		tx.ToAccount, tx.ToAmount = cash, 200
	})
	refund := newTx(models.RefundTxKind, 10, today, nil)
	if got := tagIDs(inTrip.ID); len(got) != 2 || !slices.Contains(got, trip.ID) {
		t.Errorf("in trip tags = %v, want Food + the trip", got)
	}
	if got := tagIDs(outTrip.ID); len(got) != 0 {
		t.Errorf("out of trip tags = %v, want none", got)
	}
	if got := tagIDs(transfer.ID); len(got) != 0 {
		t.Errorf("transfer tags = %v, want none", got)
	}

	// A reimbursement of the expense nets it.
	reimb := newTx(models.IncomeTxKind, 30, today, nil)
	if err := repo.LinkReimbursement(ctx, reimb.ID, inTrip.ID); err != nil {
		t.Fatal(err)
	}

	// Filter: the list, its summary and "select all" agree.
	f := models.TxFilter{TagID: trip.ID}
	ids, total, err := repo.GetFilteredTransactionIDs(ctx, f, 100)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || !slices.Contains(ids, inTrip.ID) || !slices.Contains(ids, refund.ID) {
		t.Errorf("filtered = %v (%d), want the expense and refund (the reimbursement nested)", ids, total)
	}
	sum, err := repo.GetTransactionsSummary(ctx, f, egp)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Count != 2 || sum.Out != 60 {
		t.Errorf("summary = %+v, want 2 (reimbursement nested) and 60 out", sum)
	}

	// Totals follow the analysis rule: 100 − 30 reimbursed − 10 refunded.
	spends, err := repo.GetTagSpending(ctx, trip.ID)
	if err != nil {
		t.Fatal(err)
	}
	tags, err := repo.GetTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sums := models.NewTagSummaries(tags, spends, nil, *egp)
	if s := sums[trip.ID]; s.Spent != 60 || len(s.ByDay) != 1 {
		t.Errorf("trip spent = %v by day %v, want 60 on one day", s.Spent, s.ByDay)
	}

	// The trip's days: the untagged ones in them, transfers aside.
	early := newTx(models.ExpenseTxKind, 20, today.AddDate(0, 0, -4), nil)
	if got := tagIDs(early.ID); !slices.Contains(got, trip.ID) {
		t.Errorf("early not auto tagged: %v", got)
	}
	if err := repo.SetTagArchived(ctx, trip.ID, true); err != nil {
		t.Fatal(err)
	}
	late := newTx(models.ExpenseTxKind, 5, today, nil)
	if got := tagIDs(late.ID); len(got) != 0 {
		t.Errorf("archived trip still auto tags: %v", got)
	}
	if err := repo.SetTagArchived(ctx, trip.ID, false); err != nil {
		t.Fatal(err)
	}
	c, err := repo.GetTagRangeCandidates(ctx, trip)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.IDs, []string{late.ID}) {
		t.Errorf("range candidates = %v, want late (made while archived)", c.IDs)
	}
	if _, err := repo.TagTransactions(ctx, trip.ID, c.IDs); err != nil {
		t.Fatal(err)
	}
	if got := tagIDs(late.ID); !slices.Contains(got, trip.ID) {
		t.Errorf("late not tagged: %v", got)
	}

	// Bulk edit: add one, remove the trip.
	old, err := repo.GetTransaction(ctx, early.ID)
	if err != nil {
		t.Fatal(err)
	}
	edit := models.TxBulkEdit{AddTags: []string{"Gifts"}, RemoveTags: []string{"dubai trip"}}
	upd, skips := edit.Apply(old, today)
	if len(skips) > 0 {
		t.Fatalf("skips = %v", skips)
	}
	if err := repo.BulkUpdateTransactions(ctx, []*models.Transaction{old}, []*models.Transaction{upd}, models.TxBulkReviewKeep); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetTransaction(ctx, early.ID)
	if names := models.TagNames(got.Tags); !slices.Equal(names, []string{"Gifts"}) {
		t.Errorf("bulk edited tags = %v, want Gifts", names)
	}
	// An edit without tags leaves them alone.
	edit = models.TxBulkEdit{Review: models.TxBulkReviewDone}
	upd, _ = edit.Apply(got, today)
	if err := repo.BulkUpdateTransactions(ctx, []*models.Transaction{got}, []*models.Transaction{upd}, edit.Review); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.GetTransaction(ctx, early.ID); len(got.Tags) != 1 {
		t.Errorf("review only edit changed tags: %v", models.TagNames(got.Tags))
	}

	// Over HTTP: the page, the details' editor and the list's filter.
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)
	srv := httptest.NewServer(router.New(h))
	t.Cleanup(srv.Close)
	do := func(method, path string, form url.Values) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	if res, body := do("GET", "/tags", nil); res.StatusCode != 200 || !strings.Contains(body, "Dubai trip") {
		t.Errorf("GET /tags = %d", res.StatusCode)
	}
	if res, body := do("GET", "/tags/"+trip.ID+"/details", nil); res.StatusCode != 200 || !strings.Contains(body, "By category") {
		t.Errorf("GET details = %d", res.StatusCode)
	}
	if res, body := do("POST", "/tags", url.Values{"name": {"Wedding"}, "color": {"pink"}, "amount": {"5000"}, "currency_id": {egp.ID}}); res.StatusCode != 200 || strings.Contains(body, "role=\"alert\"") {
		t.Errorf("POST /tags = %d", res.StatusCode)
	}
	if res, body := do("POST", "/tags", url.Values{"name": {"Bad"}, "starts_on": {"2026-03-10"}, "ends_on": {"2026-03-01"}}); res.StatusCode != 200 || !strings.Contains(body, "the last on or after the first") {
		t.Errorf("POST /tags with days backwards = %d, want the form with an error", res.StatusCode)
	}
	res, _ := do("PUT", "/transactions/"+outTrip.ID+"/tags", url.Values{"tags": {"Wedding"}, "add": {"Moving"}})
	if res.StatusCode != 200 {
		t.Fatalf("PUT tags = %d", res.StatusCode)
	}
	if got, _ := repo.GetTransaction(ctx, outTrip.ID); !slices.Equal(models.TagNames(got.Tags), []string{"Moving", "Wedding"}) {
		t.Errorf("tags = %v, want Moving + Wedding", models.TagNames(got.Tags))
	}
	if res, body := do("GET", "/transactions?tag="+trip.ID, nil); res.StatusCode != 200 || !strings.Contains(body, "Dubai trip") {
		t.Errorf("GET /transactions?tag= = %d", res.StatusCode)
	}

	// Deleting a tag keeps its transactions; resetting the tags empties them.
	if res, _ := do("DELETE", "/tags/"+trip.ID, nil); res.StatusCode != 200 {
		t.Errorf("DELETE tag = %d", res.StatusCode)
	}
	if got, err := repo.GetTransaction(ctx, inTrip.ID); err != nil || len(got.Tags) != 1 {
		t.Errorf("after delete: err %v tags %v, want Food left", err, got.Tags)
	}
	if err := repo.Reset(ctx, models.ResetOptions{models.ResetTags: true}); err != nil {
		t.Fatal(err)
	}
	if tags, _ := repo.GetTags(ctx); len(tags) != 0 {
		t.Errorf("tags after reset = %v", models.TagNames(tags))
	}
	if got, _ := repo.GetTransaction(ctx, inTrip.ID); len(got.Tags) != 0 {
		t.Errorf("tx tags after reset = %v", models.TagNames(got.Tags))
	}
}
