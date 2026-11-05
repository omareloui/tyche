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

// TestWishGoal saves for a wish with a goal, the goal then deciding it:
// bought with the goal, saved for again when that's undone, let go when the
// goal is deleted.
func TestWishGoal(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)

	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	today := userToday(t)

	newWish := func(plan *models.InstallmentPlan) *models.WishlistItem {
		t.Helper()
		item := &models.WishlistItem{ID: idgen.New(), Name: "Phone", Price: 600, Currency: *egp, Plan: plan, CoolingDays: 7, CreatedAt: time.Now()}
		if err := repo.CreateWishlistItem(ctx, item); err != nil {
			t.Fatal(err)
		}
		return item
	}
	wish := func(id string) *models.WishlistItem {
		t.Helper()
		item, err := repo.GetWishlistItem(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return item
	}
	save := func(item *models.WishlistItem) (*httptest.ResponseRecorder, string) {
		t.Helper()
		w := httptest.NewRecorder()
		c, err := callHandler(ctx, h.SaveWishForGoal)(w, "POST", "/wishlist/x/goal", item.ID, url.Values{
			"name": {item.Name}, "amount": {"600"}, "currency_id": {egp.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		if err := c.Render(ctx, &b); err != nil {
			t.Fatal(err)
		}
		return w, b.String()
	}

	item := newWish(nil)
	if _, err := callHandler(ctx, h.GetWishGoal)(httptest.NewRecorder(), "GET", "/wishlist/x/goal", item.ID, nil); err != nil {
		t.Fatal(err)
	}
	save(item)
	item = wish(item.ID)
	if item.Status != models.WishlistSaving || item.GoalID == "" {
		t.Fatalf("got status %d goal %q, want saving with a goal", item.Status, item.GoalID)
	}
	gl, err := repo.GetGoal(ctx, item.GoalID)
	if err != nil {
		t.Fatal(err)
	}
	if gl.Name != "Phone" || gl.Price != 600 {
		t.Errorf("got goal %q %v", gl.Name, gl.Price)
	}
	// Decided, it can't be saved for again.
	if _, err := callHandler(ctx, h.GetWishGoal)(httptest.NewRecorder(), "GET", "/wishlist/x/goal", item.ID, nil); err == nil {
		t.Error("saving for a wish already saved for")
	}

	if _, err := callHandler(ctx, h.MoveGoalMoney)(httptest.NewRecorder(), "POST", "/goals/x/money?dir=deposit", gl.ID, url.Values{
		"amount": {"600"}, "account_id": {bank.ID}, "scheduled_to": {today},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := callHandler(ctx, h.FulfillGoal)(httptest.NewRecorder(), "PUT", "/goals/x/fulfill", gl.ID, url.Values{
		"amount": {"600"}, "description": {"Phone"}, "scheduled_to": {today},
	}); err != nil {
		t.Fatal(err)
	}
	if item = wish(item.ID); item.Status != models.WishlistBought || item.TransactionID == "" {
		t.Errorf("got status %d tx %q after buying the goal, want bought with its purchase", item.Status, item.TransactionID)
	}

	if _, err := callHandler(ctx, h.UnfulfillGoal)(httptest.NewRecorder(), "PUT", "/goals/x/unfulfill", gl.ID, nil); err != nil {
		t.Fatal(err)
	}
	if item = wish(item.ID); item.Status != models.WishlistSaving || item.TransactionID != "" {
		t.Errorf("got status %d tx %q after undoing it, want saving", item.Status, item.TransactionID)
	}

	if _, err := callHandler(ctx, h.DeleteGoal)(httptest.NewRecorder(), "POST", "/goals/x/delete?dir=delete", gl.ID, url.Values{
		"account_id": {bank.ID}, "scheduled_to": {today},
	}); err != nil {
		t.Fatal(err)
	}
	if item = wish(item.ID); item.Status != models.WishlistAbandoned || item.GoalID != "" {
		t.Errorf("got status %d goal %q after deleting the goal, want let go", item.Status, item.GoalID)
	}

	// A wish paid in installments goes on to the goal's plan, its loan
	// prefilled.
	plan := &models.InstallmentPlan{Strategy: models.StrategyKnownAmount, Months: 12, Amount: 60}
	w, body := save(newWish(plan))
	if got := w.Header().Get("HX-Retarget"); got != "#drawer" {
		t.Errorf("got HX-Retarget %q, want the plan's drawer", got)
	}
	if !strings.Contains(body, "Save plan") {
		t.Error("no plan form after saving for an installments wish")
	}
}
