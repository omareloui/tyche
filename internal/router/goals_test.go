package router_test

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

type handlerFn func(w *httptest.ResponseRecorder, method, target, id string, form url.Values) (templ.Component, error)

// callHandler calls fn as the router would for {id}, with form as its body.
func callHandler(ctx context.Context, fn handler.HandlerMethod) handlerFn {
	return func(w *httptest.ResponseRecorder, method, target, id string, form url.Values) (templ.Component, error) {
		r := httptest.NewRequest(method, target, strings.NewReader(form.Encode())).WithContext(ctx)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.SetPathValue("id", id)
		return fn(w, r)
	}
}

// newTestAccount creates an account of kind enum.
func newTestAccount(t *testing.T, repo *repository.Repository, ctx context.Context, name, cur string, enum models.AccKind, amount float64) *models.Account {
	t.Helper()
	c, err := repo.GetCurrencyByString(ctx, cur)
	if err != nil {
		t.Fatal(err)
	}
	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a := &models.Account{ID: idgen.New(), Name: name, Amount: amount, Currency: *c}
	for _, k := range kinds {
		if k.Enum == enum {
			a.Kind = k
		}
	}
	if err := repo.CreateAccount(ctx, a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestGoalLifecycle(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)

	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	usd := newTestAccount(t, repo, ctx, "Dollars", "USD", models.AccKindBankAcc, 100)
	t.Cleanup(fxrate.SetRates(map[string]map[string]float64{"usd": {"egp": 48}}))
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}

	amountOf := func(id string) *models.Account {
		t.Helper()
		a, err := repo.GetAccount(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	goal := func() *models.Goal {
		t.Helper()
		gls, err := repo.GetGoals(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(gls) != 1 {
			t.Fatalf("got %d goals, want 1", len(gls))
		}
		return &gls[0]
	}
	today := userToday(t)

	create := callHandler(ctx, h.CreateGoal)
	if _, err := create(httptest.NewRecorder(), "POST", "/goals", "", url.Values{
		"name": {"Laptop"}, "amount": {"800"}, "currency_id": {egp.ID},
	}); err != nil {
		t.Fatal(err)
	}
	gl := goal()
	if gl.Account == nil || gl.Account.ID == "" {
		t.Fatal("the goal has no account")
	}
	if a := amountOf(gl.Account.ID); !a.IsGoal() || !a.ExcludeFromBalance || a.Name != "Goal: Laptop ("+strings.ToUpper(gl.Currency.String)+")" {
		t.Errorf("bad goal account %+v", a)
	}
	accs, err := repo.GetAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range accs {
		if a.ID == gl.Account.ID {
			t.Error("the goal's account is listed with the accounts")
		}
	}
	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range kinds {
		if k.Enum == models.AccKindGoal {
			t.Error("the goal account kind is offered")
		}
	}

	render := func(c templ.Component, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		if err := c.Render(ctx, &b); err != nil {
			t.Fatalf("rendering: %v", err)
		}
		return b.String()
	}
	if body := render(callHandler(ctx, h.GetGoalMoneyForm)(httptest.NewRecorder(), "GET", "/goals/x/money-form?dir=deposit", gl.ID, nil)); !strings.Contains(body, "Add money") {
		t.Error("no deposit form")
	}

	money := callHandler(ctx, h.MoveGoalMoney)
	if _, err := money(httptest.NewRecorder(), "POST", "/goals/x/money?dir=deposit", gl.ID, url.Values{
		"amount": {"500"}, "account_id": {bank.ID}, "scheduled_to": {today},
	}); err != nil {
		t.Fatal(err)
	}
	if b := amountOf(bank.ID); b.Amount != 500 || b.SetAside != 500 {
		t.Errorf("bank after deposit = %v (set aside %v), want 500 (500)", b.Amount, b.SetAside)
	}
	if p := goal().Progress(); p != 500 {
		t.Errorf("progress = %v, want 500", p)
	}

	// Another currency: kept in a holding of its own, worth it at today's
	// rate.
	if _, err := money(httptest.NewRecorder(), "POST", "/goals/x/money?dir=deposit", gl.ID, url.Values{
		"amount": {"10"}, "account_id": {usd.ID}, "scheduled_to": {today},
	}); err != nil {
		t.Fatal(err)
	}
	if u := amountOf(usd.ID); u.Amount != 90 || u.SetAside != 10 {
		t.Errorf("usd after deposit = %v (set aside %v), want 90 (10)", u.Amount, u.SetAside)
	}
	gl = goal()
	if len(gl.Holdings) != 1 || gl.Holdings[0].Amount != 10 || gl.Holdings[0].Currency.String != usd.Currency.String || gl.Account.Amount != 500 {
		t.Fatalf("bad holdings %+v (own %v)", gl.Holdings, gl.Account.Amount)
	}
	if a := amountOf(gl.Holdings[0].ID); !a.IsGoal() || a.Name != "Goal: Laptop (USD)" {
		t.Errorf("bad holding account %+v", a)
	}
	if p := gl.Progress(); p != 980 {
		t.Errorf("progress = %v, want 980", p)
	}
	// It follows the rate.
	fxrate.SetRates(map[string]map[string]float64{"usd": {"egp": 50}})
	if p := goal().Progress(); p != 1000 {
		t.Errorf("progress at a new rate = %v, want 1000", p)
	}
	fxrate.SetRates(map[string]map[string]float64{"usd": {"egp": 48}})
	// A second deposit of the currency goes to the same holding.
	if _, err := money(httptest.NewRecorder(), "POST", "/goals/x/money?dir=deposit", gl.ID, url.Values{
		"amount": {"5"}, "account_id": {usd.ID}, "scheduled_to": {today},
	}); err != nil {
		t.Fatal(err)
	}
	if gl = goal(); len(gl.Holdings) != 1 || gl.Holdings[0].Amount != 15 {
		t.Fatalf("holdings after a second deposit %+v", gl.Holdings)
	}
	// Withdrawing from the holding: capped at it, in its currency.
	usdHolding := gl.Holdings[0].ID
	if c, err := money(httptest.NewRecorder(), "POST", "/goals/x/money?dir=withdraw", gl.ID, url.Values{
		"amount": {"20"}, "account_id": {usd.ID}, "holding_id": {usdHolding},
	}); err != nil || c == nil {
		t.Fatalf("withdrawing too much from the holding: %v", err)
	}
	if _, err := money(httptest.NewRecorder(), "POST", "/goals/x/money?dir=withdraw", gl.ID, url.Values{
		"amount": {"5"}, "account_id": {usd.ID}, "holding_id": {usdHolding},
	}); err != nil {
		t.Fatal(err)
	}
	if u := amountOf(usd.ID); u.Amount != 90 || u.SetAside != 10 {
		t.Errorf("usd after withdraw = %v (set aside %v), want 90 (10)", u.Amount, u.SetAside)
	}
	if p := goal().Progress(); p != 980 {
		t.Errorf("progress after the holding's withdraw = %v, want 980", p)
	}
	// The forms and details show the money per currency.
	if body := render(callHandler(ctx, h.GetGoalMoneyForm)(httptest.NewRecorder(), "GET", "/goals/x/money-form?dir=withdraw", gl.ID, nil)); !strings.Contains(body, `name="holding_id"`) {
		t.Error("the withdraw form doesn't offer the holdings")
	}
	if body := render(callHandler(ctx, h.GoalMoneyInputs)(httptest.NewRecorder(), "POST", "/goals/x/money-inputs?dir=withdraw", gl.ID, url.Values{
		"holding_id": {usdHolding}, "account_id": {bank.ID}, "amount": {"5"},
	})); !strings.Contains(body, `name="to_amount"`) || !strings.Contains(body, "240") {
		t.Errorf("withdrawing dollars to an EGP account doesn't ask what it receives: %s", body)
	}
	if body := render(callHandler(ctx, h.GoalDetails)(httptest.NewRecorder(), "GET", "/goals/x/details", gl.ID, nil)); !strings.Contains(body, "today&#39;s rates") && !strings.Contains(body, "today's rates") {
		t.Error("the details don't show the holdings")
	}
	if body := render(callHandler(ctx, h.GetGoalFulfillForm)(httptest.NewRecorder(), "GET", "/goals/x/fulfill-form", gl.ID, nil)); !strings.Contains(body, `name="convert_`+usdHolding+`"`) {
		t.Error("the buy form doesn't convert the holding")
	}

	paces, err := repo.GetGoalPaces(ctx, []models.Goal{*goal()})
	if err != nil {
		t.Fatal(err)
	}
	if p := paces[gl.ID]; p < 326.66 || p > 326.67 {
		t.Errorf("pace = %v, want 980 / 3", p)
	}

	// Deposits and withdraws are transfers: nothing spent, and they're
	// locked.
	txs, err := repo.GetTransactions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tx := range txs {
		if tx.Kind != models.TransferTxKind || !tx.IsGoalTx() || !tx.IsLocked() || tx.Goal == nil || tx.Goal.ID != gl.ID {
			t.Errorf("bad goal deposit %+v", tx)
		}
		if _, err := callHandler(ctx, h.GetEditTransaction)(httptest.NewRecorder(), "GET", "/transactions/x/edit", tx.ID, nil); err == nil {
			t.Error("a goal's transaction is editable")
		}
		body := render(callHandler(ctx, h.TransactionDetails)(httptest.NewRecorder(), "GET", "/transactions/x", tx.ID, nil))
		if !strings.Contains(body, "Goal deposit") && !strings.Contains(body, "Goal withdrawal") || !strings.Contains(body, "of E£ 800 saved") {
			t.Errorf("the details don't show the goal's progress: %s", body)
		}
	}

	// Withdrawing more than what's in it fails.
	c, err := money(httptest.NewRecorder(), "POST", "/goals/x/money?dir=withdraw", gl.ID, url.Values{
		"amount": {"5000"}, "account_id": {bank.ID},
	})
	if err != nil || c == nil {
		t.Fatalf("withdrawing too much: %v", err)
	}
	if p := goal().Progress(); p != 980 {
		t.Errorf("progress after a failed withdraw = %v, want 980", p)
	}
	if _, err := money(httptest.NewRecorder(), "POST", "/goals/x/money?dir=withdraw", gl.ID, url.Values{
		"amount": {"200"}, "account_id": {bank.ID},
	}); err != nil {
		t.Fatal(err)
	}
	if b := amountOf(bank.ID); b.Amount != 700 || b.SetAside != 300 {
		t.Errorf("bank after withdraw = %v (set aside %v), want 700 (300)", b.Amount, b.SetAside)
	}
	if p := goal().Progress(); p != 780 {
		t.Errorf("progress = %v, want 780", p)
	}

	for _, path := range []string{"/goals/x/money-form?dir=withdraw", "/goals/x/delete-form"} {
		fn := h.GetGoalMoneyForm
		if strings.HasSuffix(path, "delete-form") {
			fn = h.GetGoalDeleteForm
		}
		render(callHandler(ctx, fn)(httptest.NewRecorder(), "GET", path, gl.ID, nil))
	}
	render(callHandler(ctx, h.GetGoalFulfillForm)(httptest.NewRecorder(), "GET", "/goals/x/fulfill-form", gl.ID, nil))
	render(callHandler(ctx, h.Goals)(httptest.NewRecorder(), "GET", "/goals", "", nil))

	// Buying it: the holding converted, then an expense out of its account.
	if _, err := callHandler(ctx, h.FulfillGoal)(httptest.NewRecorder(), "PUT", "/goals/x/fulfill", gl.ID, url.Values{
		"amount": {"700"}, "description": {"Laptop"}, "scheduled_to": {today}, "convert_" + usdHolding: {"480"},
	}); err != nil {
		t.Fatal(err)
	}
	if a := amountOf(usdHolding); a.Amount != 0 {
		t.Errorf("holding after buying = %v, want 0", a.Amount)
	}
	gl = goal()
	if gl.FulfilledAt.IsZero() || gl.Balance() != 80 || gl.Progress() != 780 {
		t.Errorf("bought goal: fulfilled %v, balance %v, progress %v", gl.FulfilledAt, gl.Balance(), gl.Progress())
	}
	if b := amountOf(bank.ID); b.SetAside != 0 {
		t.Errorf("set aside for a bought goal = %v, want 0", b.SetAside)
	}
	if u := amountOf(usd.ID); u.SetAside != 0 {
		t.Errorf("usd set aside for a bought goal = %v, want 0", u.SetAside)
	}
	// It doesn't follow the rate anymore.
	fxrate.SetRates(map[string]map[string]float64{"usd": {"egp": 50}})
	if p := goal().Progress(); p != 780 {
		t.Errorf("bought goal's progress at a new rate = %v, want 780", p)
	}
	fxrate.SetRates(map[string]map[string]float64{"usd": {"egp": 48}})
	var purchase *models.Transaction
	txs, err = repo.GetTransactions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := range txs {
		if txs[i].IsGoalPurchase() {
			purchase = &txs[i]
		}
	}
	if purchase == nil || purchase.Amount != 700 || purchase.AnalysisAmount() != 700 {
		t.Fatalf("bad purchase %+v", purchase)
	}
	if _, err := callHandler(ctx, h.DeleteTransaction)(httptest.NewRecorder(), "DELETE", "/transactions/x", purchase.ID, nil); err == nil {
		t.Error("the goal's purchase got deleted out of the goal")
	}

	// Undoing it.
	if _, err := callHandler(ctx, h.UnfulfillGoal)(httptest.NewRecorder(), "PUT", "/goals/x/unfulfill", gl.ID, nil); err != nil {
		t.Fatal(err)
	}
	gl = goal()
	if !gl.FulfilledAt.IsZero() || gl.Balance() != 780 || gl.Account.Amount != 300 || gl.Holdings[0].Amount != 10 {
		t.Errorf("unfulfilled goal: fulfilled %v, balance %v (own %v, holdings %+v)", gl.FulfilledAt, gl.Balance(), gl.Account.Amount, gl.Holdings)
	}

	// Deleting it moves what's left, keeping the history.
	if _, err := callHandler(ctx, h.DeleteGoal)(httptest.NewRecorder(), "POST", "/goals/x/delete?dir=delete", gl.ID, url.Values{
		"account_id": {bank.ID},
	}); err != nil {
		t.Fatal(err)
	}
	if b := amountOf(bank.ID); b.Amount != 1480 {
		t.Errorf("bank after deleting the goal = %v, want 1480", b.Amount)
	}
	if a := amountOf(gl.Account.ID); !a.IsDeleted() || a.Amount != 0 {
		t.Errorf("goal account after deleting it: deleted %v, amount %v", a.DeletedAt, a.Amount)
	}
	if a := amountOf(usdHolding); !a.IsDeleted() || a.Amount != 0 {
		t.Errorf("goal holding after deleting it: deleted %v, amount %v", a.DeletedAt, a.Amount)
	}
	if gls, _ := repo.GetGoals(ctx); len(gls) != 0 {
		t.Errorf("got %d goals after deleting it", len(gls))
	}
	txs, err = repo.GetTransactions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// 3 deposits, 2 withdraws and a move out of each of its accounts.
	if len(txs) != 7 {
		t.Errorf("got %d transactions after deleting the goal, want its 7 kept", len(txs))
	}

	// Its name is free again.
	if _, err := create(httptest.NewRecorder(), "POST", "/goals", "", url.Values{
		"name": {"Laptop"}, "amount": {"800"}, "currency_id": {egp.ID},
	}); err != nil {
		t.Fatal(err)
	}
	goal()
}

func TestDeleteAccountKeepsHistory(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)

	a := newTestAccount(t, repo, ctx, "Old bank", "EGP", models.AccKindBankAcc, 100)
	b := newTestAccount(t, repo, ctx, "New bank", "EGP", models.AccKindBankAcc, 50)

	now := time.Now()
	tr := &models.Transaction{
		ID: idgen.New(), Kind: models.TransferTxKind, Account: *a, ToAccount: b,
		Amount: 30, ToAmount: 30, ScheduledTo: now, FulfilledAt: now, CreatedAt: now,
	}
	if err := repo.CreateTransaction(ctx, tr, nil, nil); err != nil {
		t.Fatal(err)
	}
	scheduled := &models.Transaction{
		ID: idgen.New(), Kind: models.ExpenseTxKind, Account: *a, Amount: 10,
		ScheduledTo: now.AddDate(0, 1, 0), CreatedAt: now,
	}
	if err := repo.CreateTransaction(ctx, scheduled, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetDefaultAccount(ctx, a.ID); err != nil {
		t.Fatal(err)
	}

	var page strings.Builder
	c, err := callHandler(ctx, h.GetAccountDeleteForm)(httptest.NewRecorder(), "GET", "/accounts/x/delete-form", a.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Render(ctx, &page); err != nil || !strings.Contains(page.String(), "Move it to") {
		t.Fatalf("delete form: %v", err)
	}

	if _, err := callHandler(ctx, h.DeleteAccount)(httptest.NewRecorder(), "POST", "/accounts/x/delete", a.ID, url.Values{
		"account_id": {b.ID},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetAccount(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Amount != 150 {
		t.Errorf("target balance = %v, want 150 (50 + 30 + the 70 moved)", got.Amount)
	}
	accs, err := repo.GetAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, acc := range accs {
		if acc.ID == a.ID {
			t.Error("the deleted account is still listed")
		}
		if acc.IsDefault {
			t.Errorf("%s is still the default", acc.Name)
		}
	}
	txs, err := repo.GetTransactions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 2 {
		t.Fatalf("got %d transactions, want the transfer and the moved balance (the scheduled one gone)", len(txs))
	}
	for _, tx := range txs {
		if !tx.IsLocked() || !tx.Account.IsDeleted() {
			t.Errorf("transaction of the deleted account isn't locked: %+v", tx.Account)
		}
	}

	// Its name is free again, and it's not found for edits.
	newTestAccount(t, repo, ctx, "Old bank", "EGP", models.AccKindBankAcc, 0)
	if _, err := callHandler(ctx, h.LiveAccount(h.GetEditAccount))(httptest.NewRecorder(), "GET", "/accounts/x/edit", a.ID, nil); err == nil {
		t.Error("the deleted account is editable")
	}
}

func TestGoalLoanPayments(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	// Planning reads the salary, off the user.
	usr.Email, usr.DefaultCurrency = "e2e@example.com", *egp
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 10_000)
	if _, err := callHandler(ctx, h.CreateGoal)(httptest.NewRecorder(), "POST", "/goals", "", url.Values{
		"name": {"Car"}, "amount": {"1200"}, "currency_id": {egp.ID},
	}); err != nil {
		t.Fatal(err)
	}
	goal := func() *models.Goal {
		t.Helper()
		gls, err := repo.GetGoals(ctx)
		if err != nil || len(gls) != 1 {
			t.Fatalf("goals %v, %v", gls, err)
		}
		return &gls[0]
	}
	gl := goal()

	// 300 down, the rest in 3 interest-free payments.
	plan := url.Values{"monthly": {"300"}, "mode": {"installments"}, "strategy": {"known"}, "down_payment": {"300"}, "down_payment_unit": {"amount"}}
	if _, err := callHandler(ctx, h.SetGoalPlan)(httptest.NewRecorder(), "PUT", "/goals/x/plan", gl.ID, plan); err != nil {
		t.Fatal(err)
	}
	today := userToday(t)
	if _, err := callHandler(ctx, h.MoveGoalMoney)(httptest.NewRecorder(), "POST", "/goals/x/money?dir=deposit", gl.ID, url.Values{
		"amount": {"300"}, "account_id": {bank.ID}, "scheduled_to": {today},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := callHandler(ctx, h.FulfillGoal)(httptest.NewRecorder(), "PUT", "/goals/x/fulfill", gl.ID, url.Values{
		"amount": {"300"}, "scheduled_to": {today}, "description": {"Car"},
		"schedule_loan": {"on"}, "loan_account_id": {bank.ID},
	}); err != nil {
		t.Fatal(err)
	}

	gl = goal()
	if gl.Loan.Payments != 3 || gl.Loan.Total != 900 || gl.Loan.Paid != 0 || !gl.PayingOff() || gl.PayoffPercentage() != 25 {
		t.Fatalf("loan %+v, paying off %v, %v%%", gl.Loan, gl.PayingOff(), gl.PayoffPercentage())
	}

	// The payments are the goal's transactions, still editable expenses.
	txs, err := repo.GetTransactions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var payments []models.Transaction
	for _, tx := range txs {
		if tx.IsLoanPayment() {
			payments = append(payments, tx)
		}
	}
	if len(payments) != 3 {
		t.Fatalf("%d loan payments listed", len(payments))
	}
	for _, tx := range payments {
		if tx.Goal == nil || tx.Goal.ID != gl.ID || tx.Kind != models.ExpenseTxKind || tx.IsLocked() {
			t.Errorf("bad loan payment %+v", tx)
		}
	}

	// Paying one pays the goal off further.
	if _, err := callHandler(ctx, h.FulfillTransaction)(httptest.NewRecorder(), "PUT", "/transactions/x/fulfill", payments[0].ID, nil); err != nil {
		t.Fatal(err)
	}
	if gl = goal(); gl.Loan.PaidPayments != 1 || gl.Loan.Paid != 300 || gl.PayoffPercentage() != 50 {
		t.Errorf("after a payment: loan %+v, %v%%", gl.Loan, gl.PayoffPercentage())
	}

	// Undoing the purchase deletes the unpaid ones, unlinks the paid one.
	if _, err := callHandler(ctx, h.UnfulfillGoal)(httptest.NewRecorder(), "PUT", "/goals/x/unfulfill", gl.ID, nil); err != nil {
		t.Fatal(err)
	}
	if gl = goal(); gl.HasLoan() || !gl.FulfilledAt.IsZero() {
		t.Errorf("after undo: loan %+v", gl.Loan)
	}
	if txs, err = repo.GetTransactions(ctx); err != nil {
		t.Fatal(err)
	}
	left := 0
	for _, tx := range txs {
		if tx.Description != "" && strings.HasPrefix(tx.Description, "Car (") {
			left++
			if tx.IsLoanPayment() || tx.FulfilledAt.IsZero() {
				t.Errorf("left payment %+v", tx)
			}
		}
	}
	if left != 1 {
		t.Errorf("%d payments left, want the paid one", left)
	}
}

// userToday is today for a new user, in the default time zone the forms'
// days are read in (whatever the machine's).
func userToday(t *testing.T) string {
	t.Helper()
	loc, err := models.LoadTimezone(models.DefaultTimezone)
	if err != nil {
		t.Fatal(err)
	}
	return time.Now().In(loc).Format(time.DateOnly)
}
