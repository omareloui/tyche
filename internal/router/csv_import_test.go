package router_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
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

// uploadCSV uploads a CSV file into the account, returning the pending
// import's id.
func uploadCSV(t *testing.T, ctx context.Context, h *handler.Handler, accountID, content string) string {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("account_id", accountID); err != nil {
		t.Fatal(err)
	}
	fw, err := mw.CreateFormFile("file", "statement.csv")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte(content))
	mw.Close()

	r := httptest.NewRequest("POST", "/import", &body).WithContext(ctx)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	if _, err := h.UploadCSV(w, r); err != nil {
		t.Fatal(err)
	}
	id, ok := strings.CutPrefix(w.Header().Get("HX-Redirect"), "/import/")
	if !ok {
		t.Fatalf("HX-Redirect = %q", w.Header().Get("HX-Redirect"))
	}
	return id
}

// csvPreview renders the import's mapping and preview as sent by form.
func csvPreview(t *testing.T, ctx context.Context, h *handler.Handler, id string, form url.Values) string {
	t.Helper()
	c, err := callHandler(ctx, h.PreviewCSVImport)(httptest.NewRecorder(), "POST", "/import/"+id+"/preview", id, form)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := c.Render(ctx, &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func csvMappingForm(extra url.Values) url.Values {
	form := url.Values{
		"delimiter": {"comma"}, "skip_rows": {"0"}, "has_header": {"1"},
		"date": {"0"}, "date_format": {"DD/MM/YYYY"}, "time": {""},
		"amount_mode": {"signed"}, "amount": {"2"}, "negative": {"out"},
		"description": {"1"}, "counterparty": {""}, "currency": {""},
	}
	for k, v := range extra {
		form[k] = v
	}
	return form
}

func TestCSVImport(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)

	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	card := newTestAccount(t, repo, ctx, "Card", "EGP", models.AccKindCreditCard, 0)

	cats, err := repo.GetCategories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var food *models.Category
	for i := range cats {
		if cats[i].HasKind(models.ExpenseTxKind) {
			food = &cats[i]
			break
		}
	}
	cp := &models.Counterparty{ID: idgen.New(), Name: "Carrefour", IsMerchant: true, Aliases: []string{"CRF MAADI"}}
	if food != nil {
		cp.DefaultCategory = food
	}
	if err := repo.CreateCounterparty(ctx, cp); err != nil {
		t.Fatal(err)
	}

	d := func(daysAgo int) time.Time { return utcToday().AddDate(0, 0, -daysAgo) }
	f := func(t time.Time) string { return t.Format("02/01/2006") }

	// an SMS-like transaction already there: the row of the next day is it
	sms := &models.Transaction{
		ID: idgen.New(), Account: *bank, Kind: models.ExpenseTxKind, Amount: 75,
		ScheduledTo: d(6), FulfilledAt: d(6),
	}
	if err := repo.CreateTransaction(ctx, sms, nil, nil); err != nil {
		t.Fatal(err)
	}

	csv := "Date,Description,Amount,Balance\n" +
		f(d(10)) + ",PURCHASE CRF MAADI 1234,\"-1,250.50\",100\n" +
		f(d(9)) + ",SALARY,20000,100\n" +
		f(d(5)) + ",UBER TRIP,-75,100\n" +
		f(d(4)) + ",COFFEE,-40,100\n" +
		f(d(4)) + ",COFFEE,-40,100\n" +
		"Total,,,\n"
	id := uploadCSV(t, ctx, h, bank.ID, csv)

	html := csvPreview(t, ctx, h, id, csvMappingForm(nil))
	if n := strings.Count(html, `data-status="new"`); n != 4 {
		t.Errorf("new rows = %d, want 4", n)
	}
	if n := strings.Count(html, `data-status="duplicate"`); n != 1 {
		t.Errorf("duplicate rows = %d, want 1", n)
	}
	if n := strings.Count(html, `data-status="error"`); n != 1 {
		t.Errorf("error rows = %d, want 1", n)
	}
	if !strings.Contains(html, "Carrefour") {
		t.Error("the preview doesn't show the counterparty found in the description")
	}

	// a bad mapping is told, not imported
	if html := csvPreview(t, ctx, h, id, csvMappingForm(url.Values{"date": {""}})); !strings.Contains(html, "Pick the date") {
		t.Error("a mapping without a date isn't refused")
	}

	// the new rows (2, 3, 5, 6), not the duplicate (4) nor the total (7)
	form := csvMappingForm(url.Values{"line": {"2", "3", "5", "6"}, "save_mapping": {"1"}})
	w := httptest.NewRecorder()
	if _, err := callHandler(ctx, h.ImportCSV)(w, "POST", "/import/"+id, id, form); err != nil {
		t.Fatal(err)
	}
	if loc := w.Header().Get("HX-Redirect"); !strings.Contains(loc, "source=import") {
		t.Errorf("HX-Redirect = %q", loc)
	}

	if n := countTxs(t, ctx, repo, models.TxFilter{Source: models.TxSourceImport}); n != 4 {
		t.Fatalf("imported txs = %d, want 4", n)
	}
	all, err := repo.GetTransactions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tx := range all {
		if !tx.FromImport {
			continue
		}
		if !tx.NeedsReview || !tx.FromImport || tx.FulfilledAt.IsZero() {
			t.Errorf("tx %s: review %v, import %v, fulfilled %v", tx.Description, tx.NeedsReview, tx.FromImport, tx.FulfilledAt)
		}
		if strings.Contains(tx.Description, "CRF") {
			if tx.Counterparty == nil || tx.Counterparty.ID != cp.ID || tx.Amount != 1250.5 || tx.Kind != models.ExpenseTxKind {
				t.Errorf("carrefour tx = %+v", tx)
			}
			if food != nil && (tx.Category == nil || tx.Category.ID != food.ID) {
				t.Errorf("carrefour category = %v, want %s", tx.Category, food.ID)
			}
		}
	}
	got, err := repo.GetAccount(ctx, bank.ID)
	if err != nil {
		t.Fatal(err)
	}
	// 1000 − 75 (the SMS) − 1250.5 + 20000 − 40 − 40
	if want := 1000 - 75 - 1250.5 + 20000 - 80; got.Amount != want {
		t.Errorf("balance = %v, want %v", got.Amount, want)
	}

	// done: the import is gone
	if _, err := callHandler(ctx, h.ImportCSV)(httptest.NewRecorder(), "POST", "/import/"+id, id, form); !isStatus(err, 404) {
		t.Errorf("importing twice: err = %v, want a 404", err)
	}

	// the same file again: its rows were imported, the saved mapping is picked
	again := uploadCSV(t, ctx, h, bank.ID, csv)
	imp, err := repo.GetCSVImport(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	if imp.Mapping.Description != 1 || imp.Mapping.Amount != 2 {
		t.Errorf("mapping of the same file = %+v", imp.Mapping)
	}
	html = csvPreview(t, ctx, h, again, csvMappingForm(nil))
	if n := strings.Count(html, `data-status="imported"`); n != 4 {
		t.Errorf("imported before = %d, want 4", n)
	}
	if _, err := callHandler(ctx, h.ImportCSV)(httptest.NewRecorder(), "POST", "/import/"+again, again,
		csvMappingForm(url.Values{"line": {"2", "3", "5", "6"}})); !isStatus(err, 422) {
		t.Errorf("importing rows imported before: err = %v, want a 422", err)
	}

	// a transfer to the card, read off both accounts' exports, is paired
	out := uploadCSV(t, ctx, h, bank.ID, "Date,Description,Amount\n"+f(d(2))+",CARD PAYMENT,-3000\n")
	if _, err := callHandler(ctx, h.ImportCSV)(httptest.NewRecorder(), "POST", "/import/"+out, out,
		csvMappingForm(url.Values{"line": {"2"}})); err != nil {
		t.Fatal(err)
	}
	in := uploadCSV(t, ctx, h, card.ID, "Date,Description,Amount\n"+f(d(2))+",PAYMENT RECEIVED,3000\n")
	if _, err := callHandler(ctx, h.ImportCSV)(httptest.NewRecorder(), "POST", "/import/"+in, in,
		csvMappingForm(url.Values{"line": {"2"}})); err != nil {
		t.Fatal(err)
	}
	if n := countTxs(t, ctx, repo, models.TxFilter{Source: models.TxSourceImport, Kind: models.TransferTxKind}); n != 1 {
		t.Errorf("paired transfers = %d, want 1", n)
	}

	// "added by hand" leaves the imported ones out
	if n := countTxs(t, ctx, repo, models.TxFilter{Source: models.TxSourceManual}); n != 1 {
		t.Errorf("by hand = %d txs, want only the one created", n)
	}
}

// countTxs is how many rows the transactions list shows for the filter.
func countTxs(t *testing.T, ctx context.Context, repo *repository.Repository, f models.TxFilter) int {
	t.Helper()
	_, n, err := repo.GetFilteredTransactionIDs(ctx, f, -1)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func isStatus(err error, code int) bool {
	re, ok := err.(*resperr.RespError)
	return ok && re.Code == code
}
