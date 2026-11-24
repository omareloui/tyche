package handler

import (
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
)

// TestHoldingGains buys a foreign currency then sells part of it back,
// through the account details' and the summary's gains.
func TestHoldingGains(t *testing.T) {
	e := newPickEnv(t)
	def := e.acc.Currency
	curs, err := e.repo.GetCurrencies(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var other *models.Currency
	for i := range curs {
		if !strings.EqualFold(curs[i].String, def.String) && !curs[i].IsMetal() {
			other = &curs[i]
			break
		}
	}
	if other == nil {
		t.Fatal("no other currency seeded")
	}
	t.Cleanup(fxrate.SetRates(map[string]map[string]float64{
		strings.ToLower(other.String): {strings.ToLower(def.String): 50},
		strings.ToLower(def.String):   {strings.ToLower(other.String): 0.02},
	}))

	acc := &models.Account{ID: idgen.New(), Name: "Abroad", Currency: *other, Kind: e.acc.Kind, SmsRefs: []string{}}
	if err := e.repo.CreateAccount(e.ctx, acc); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().AddDate(0, 0, -10)
	for _, tx := range []*models.Transaction{
		// 100 bought for 4,800, then 20 sold for 1,000.
		{Account: e.acc, ToAccount: acc, Amount: 4800, ToAmount: 100},
		{Account: *acc, ToAccount: &e.acc, Amount: 20, ToAmount: 1000},
	} {
		tx.ID, tx.Kind, tx.ScheduledTo, tx.FulfilledAt = idgen.New(), models.TransferTxKind, day, day
		if err := e.repo.CreateTransaction(e.ctx, tx, nil, nil); err != nil {
			t.Fatal(err)
		}
		day = day.AddDate(0, 0, 1)
	}

	gains, _, err := e.h.holdingGains(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	g := gains[acc.ID]
	if g == nil || math.Abs(g.Units-80) > 1e-6 || math.Abs(g.AvgCost()-48) > 1e-6 ||
		math.Abs(g.Realized-40) > 1e-6 || math.Abs(g.Unrealized-160) > 1e-6 {
		t.Fatalf("gain = %+v", g)
	}

	r := httptest.NewRequest("GET", "/accounts/"+acc.ID+"/gain", nil).WithContext(e.ctx)
	r.SetPathValue("id", acc.ID)
	c, err := e.h.AccountGain(httptest.NewRecorder(), r)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := c.Render(e.ctx, &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "Cost and gain") || !strings.Contains(b.String(), "Gained selling") {
		t.Errorf("details gain:\n%s", b.String())
	}

	r = httptest.NewRequest("GET", "/accounts/summary/gains", nil).WithContext(e.ctx)
	if c, err = e.h.AccountsSummaryGains(httptest.NewRecorder(), r); err != nil {
		t.Fatal(err)
	}
	b.Reset()
	if err := c.Render(e.ctx, &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `id="acc-gain-`+strings.ToLower(other.String)+`"`) || !strings.Contains(b.String(), "+4.2%") {
		t.Errorf("summary gains:\n%s", b.String())
	}

	// The default currency's account has none.
	r = httptest.NewRequest("GET", "/accounts/"+e.acc.ID+"/gain", nil).WithContext(e.ctx)
	r.SetPathValue("id", e.acc.ID)
	if c, err = e.h.AccountGain(httptest.NewRecorder(), r); err != nil {
		t.Fatal(err)
	}
	b.Reset()
	if err := c.Render(e.ctx, &b); err != nil || b.Len() != 0 {
		t.Errorf("default currency's gain: %q, %v", b.String(), err)
	}
}
