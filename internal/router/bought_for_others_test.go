package router_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

// TestBoughtForOthers saves expenses the user bought for someone else, with
// and without the income paying them back, and checks they stay out of
// every analysis while still moving the balance.
func TestBoughtForOthers(t *testing.T) {
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
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	sara := &models.Counterparty{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8C1", Name: "Sara"}
	if err := repo.CreateCounterparty(ctx, sara); err != nil {
		t.Fatal(err)
	}
	food := &models.Category{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZB", Name: "Food e2e", Color: "#000000",
		Kinds: []models.TxKind{models.ExpenseTxKind}, BudgetAmount: 5000, BudgetCurrency: egp,
	}
	if err := repo.CreateCategory(ctx, food); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)))
	t.Cleanup(srv.Close)
	do := func(method, path string, form url.Values, hdr ...string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		for i := 0; i+1 < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	balance := func() float64 {
		t.Helper()
		a, err := repo.GetAccount(ctx, bank.ID)
		if err != nil {
			t.Fatal(err)
		}
		return a.Amount
	}
	today := time.Now().Format(time.DateOnly)
	create := func(desc, amount string, extra url.Values) (*http.Response, string) {
		t.Helper()
		form := url.Values{
			"kind": {models.ExpenseTxKind.String()}, "account_id": {bank.ID}, "amount": {amount},
			"scheduled_to": {today}, "description": {desc}, "is_recurrent": {""},
			"category_id": {food.ID},
		}
		for k, v := range extra {
			form[k] = v
		}
		return do("POST", "/transactions", form)
	}
	find := func(desc string, kind models.TxKind) *models.Transaction {
		t.Helper()
		txs, err := repo.GetTransactionsPage(ctx, models.TxFilter{}, time.Time{}, &time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		for _, list := range [][]models.Transaction{txs.Upcoming, txs.History} {
			for i := range list {
				if list[i].Description == desc && list[i].Kind == kind {
					return &list[i]
				}
			}
		}
		return nil
	}
	spent := func() (cat, budget, summary, flow float64) {
		t.Helper()
		cs, err := repo.GetCategorySpending(ctx, time.Time{}, egp)
		if err != nil {
			t.Fatal(err)
		}
		bs, err := repo.GetBudgetSummary(ctx, time.Time{}, egp, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range bs.Items {
			if it.Category != nil && it.Category.ID == food.ID {
				budget = it.Spent
			}
		}
		sum, err := repo.GetTransactionsSummary(ctx, models.TxFilter{}, egp)
		if err != nil {
			t.Fatal(err)
		}
		fl, err := repo.GetFlow(ctx, egp)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(fl.Months); n > 0 {
			flow = fl.Months[n-1].Out
		}
		return cs.Total, budget, sum.Out, flow
	}

	if res, body := create("Lunch", "100", nil); res.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", res.StatusCode, body)
	}

	t.Run("a gift is out of every analysis, the balance moves", func(t *testing.T) {
		res, body := create("Gift", "300", url.Values{"for_others_on": {"on"}, "for_person": {sara.ID}})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		gift := find("Gift", models.ExpenseTxKind)
		if gift == nil || !gift.ForOthers || gift.ForPerson == nil || gift.ForPerson.Name != "Sara" {
			t.Fatalf("gift = %+v", gift)
		}
		if gift.AnalysisAmount() != 0 {
			t.Errorf("analysis amount = %v", gift.AnalysisAmount())
		}
		if b := balance(); b != 600 {
			t.Errorf("bank = %v, want 600", b)
		}
		cat, budget, sum, flow := spent()
		if cat != 100 || budget != 100 || sum != 100 || flow != 100 {
			t.Errorf("spent = %v / %v / %v / %v, want 100 everywhere", cat, budget, sum, flow)
		}
	})

	t.Run("paid back schedules an income reimbursing it", func(t *testing.T) {
		res, body := create("Concert", "250", url.Values{"for_others_on": {"on"}, "for_others_payback": {"on"}})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		c := find("Concert", models.ExpenseTxKind)
		if c == nil || !c.ForOthers || c.ForPerson != nil {
			t.Fatalf("concert = %+v", c)
		}
		if len(c.Reimbursements) != 1 || c.Reimbursements[0].Amount != 250 || !c.Reimbursements[0].FulfilledAt.IsZero() {
			t.Fatalf("reimbursements = %+v", c.Reimbursements)
		}
		if c.ForOthersOwed() != 0 {
			t.Errorf("owed = %v", c.ForOthersOwed())
		}
		cat, budget, sum, flow := spent()
		if cat != 100 || budget != 100 || sum != 100 || flow != 100 {
			t.Errorf("spent = %v / %v / %v / %v, want 100 everywhere", cat, budget, sum, flow)
		}
		// Asking again schedules nothing more.
		res, body = do("PUT", "/transactions/"+c.ID+"/for-others", url.Values{"keep_person": {"1"}, "payback": {"1"}, "details_id": {c.ID}}, "HX-Target", "txd-panel")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if c = find("Concert", models.ExpenseTxKind); len(c.Reimbursements) != 1 {
			t.Errorf("reimbursements = %d", len(c.Reimbursements))
		}
	})

	t.Run("the filter and the row chip", func(t *testing.T) {
		page, err := repo.GetTransactionsPage(ctx, models.TxFilter{For: models.TxForOthers}, time.Time{}, &time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if n := len(page.History) + len(page.Upcoming); n != 2 {
			t.Errorf("for others = %d rows, want 2", n)
		}
		page, err = repo.GetTransactionsPage(ctx, models.TxFilter{For: models.TxForMine}, time.Time{}, &time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		for _, tx := range append(page.History, page.Upcoming...) {
			if tx.ForOthers {
				t.Errorf("%s in mine", tx.Description)
			}
		}
		_, body := do("GET", "/transactions?for=others", nil)
		if !strings.Contains(body, "For Sara") || !strings.Contains(body, "For someone") {
			t.Error("no chips")
		}
	})

	t.Run("marked from the details, then the user's again", func(t *testing.T) {
		lunch := find("Lunch", models.ExpenseTxKind)
		res, body := do("GET", "/transactions/"+lunch.ID+"/for-others", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "for_person") {
			t.Fatalf("form %d: %s", res.StatusCode, body)
		}
		res, body = do("PUT", "/transactions/"+lunch.ID+"/for-others", url.Values{"for_person": {sara.ID}, "details_id": {lunch.ID}}, "HX-Target", "txd-panel")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if !strings.Contains(body, "Bought for someone") {
			t.Error("details without the section")
		}
		if cat, _, _, _ := spent(); cat != 0 {
			t.Errorf("category = %v, want 0", cat)
		}
		// htmx sends a DELETE's parameters in the url (layouts' htmx-config).
		res, body = do("DELETE", "/transactions/"+lunch.ID+"/for-others?details_id="+lunch.ID, nil, "HX-Target", "txd-panel")
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "I bought it for someone") {
			t.Fatalf("status %d: details not answered", res.StatusCode)
		}
		if l := find("Lunch", models.ExpenseTxKind); l.ForOthers || l.ForPerson != nil {
			t.Errorf("lunch = %+v", l)
		}
		if cat, _, _, _ := spent(); cat != 100 {
			t.Errorf("category = %v, want 100", cat)
		}
	})

	t.Run("refused with someone bought it for me", func(t *testing.T) {
		saraAcc := newTestAccount(t, repo, ctx, "Sara debt", "EGP", models.AccKindDebt, 0)
		_, body := create("Both", "50", url.Values{"for_others_on": {"on"}, "bought_by_on": {"on"}, "bought_by": {saraAcc.ID}})
		if !strings.Contains(body, "also be for someone else") {
			t.Error("not refused")
		}
		if find("Both", models.ExpenseTxKind) != nil {
			t.Error("saved")
		}
	})

	t.Run("no longer an expense clears it", func(t *testing.T) {
		gift := find("Gift", models.ExpenseTxKind)
		updated := *gift
		updated.Kind = models.IncomeTxKind
		updated.Category = nil
		if err := repo.UpdateTransaction(ctx, gift, &updated); err != nil {
			t.Fatal(err)
		}
		if g := find("Gift", models.IncomeTxKind); g == nil || g.ForOthers || g.ForPerson != nil {
			t.Errorf("gift = %+v", g)
		}
	})

	t.Run("a scheduled one counts in safe to spend despite its budget", func(t *testing.T) {
		ahead := time.Now().AddDate(0, 0, 3).Format(time.DateOnly)
		res, body := create("Later gift", "70", url.Values{"for_others_on": {"on"}, "scheduled_to": {ahead}})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		out, err := repo.GetUnbudgetedScheduledOutflows(ctx, time.Now().AddDate(0, 1, 0))
		if err != nil {
			t.Fatal(err)
		}
		var total float64
		for _, b := range out {
			total += b.Amount
		}
		if total != 70 {
			t.Errorf("unbudgeted outflows = %v, want 70", total)
		}
	})
}
