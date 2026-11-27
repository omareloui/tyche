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

	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/statementocr"
	"github.com/omareloui/tyche/internal/valid"
)

type fakeStatements struct {
	result *statementocr.Result
	reads  int
}

func (f *fakeStatements) Model() string { return "fake-flash" }

func (f *fakeStatements) Read(_ context.Context, _ []byte) (*statementocr.Result, statementocr.Usage, error) {
	f.reads++
	return f.result, statementocr.Usage{InputTokens: 1500, OutputTokens: 900}, nil
}

// uploadImportFile posts a file to /import with the extra fields, answering
// the pending import's id or the handler's error.
func uploadImportFile(t *testing.T, ctx context.Context, h *handler.Handler, accountID, name string, content []byte, fields url.Values) (string, error) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("account_id", accountID)
	for k, vs := range fields {
		for _, v := range vs {
			mw.WriteField(k, v)
		}
	}
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(content)
	mw.Close()

	r := httptest.NewRequest("POST", "/import", &body).WithContext(ctx)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	if _, err := h.UploadCSV(w, r); err != nil {
		return "", err
	}
	id, ok := strings.CutPrefix(w.Header().Get("HX-Redirect"), "/import/")
	if !ok {
		t.Fatalf("HX-Redirect = %q", w.Header().Get("HX-Redirect"))
	}
	return id, nil
}

func ptrNum(v float64) *float64 { return &v }

// pdfMappingForm is what the import page sends for a PDF's rows (the
// preset mapping's hidden fields).
func pdfMappingForm(extra url.Values) url.Values {
	m := statementocr.PresetMapping()
	form := url.Values{
		"delimiter": {"comma"}, "skip_rows": {"0"}, "has_header": {"1"},
		"date": {"0"}, "date_format": {m.DateFormat}, "time": {"1"},
		"amount_mode": {"split"}, "amount": {""}, "debit": {"3"}, "credit": {"4"},
		"description": {"2"}, "counterparty": {""}, "currency": {""},
		"original_amount": {"6"}, "original_currency": {"7"}, "fx_fee": {"8"},
	}
	for k, v := range extra {
		form[k] = v
	}
	return form
}

func TestStatementImport(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	d := func(daysAgo int) string { return tenantToday().AddDate(0, 0, -daysAgo).Format(time.DateOnly) }

	reader := &fakeStatements{result: &statementocr.Result{
		IsStatement: true, Kind: statementocr.KindAccount, Currency: "EGP", AccountHint: "EG680010021100000100067406979",
		OpeningBalance: ptrNum(7810.01), ClosingBalance: ptrNum(7810.01 - 85.5 + 70000 - 40000),
		Rows: []statementocr.Row{
			{Date: d(40), Description: "IPN Outward, Payee \"Seif\"", Debit: 85.5},
			{Date: d(39), Description: "IPN Inward", Credit: 70000},
			{Date: d(38), Description: "Online Transfer - Settlement of My Credit Card", Debit: 40000, Balance: ptrNum(37724.51)},
		},
	}}
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	h.SetReceiptOCR(nil, config.ReceiptOCR{StatementImportsPerMonth: 2})
	h.SetStatementReader(reader)

	bank := newTestAccount(t, repo, ctx, "CIB", "EGP", models.AccKindBankAcc, 50000)
	pdf := []byte("%PDF-1.7\n1 0 obj << /Type /Catalog >> endobj\n%%EOF")
	consent := url.Values{"ai_consent": {"1"}}

	// no consent to send it to the AI: refused, no call
	if _, err := uploadImportFile(t, ctx, h, bank.ID, "Statements_31OCT26.pdf", pdf, nil); !isStatus(err, 422) || reader.reads != 0 {
		t.Fatalf("without consent: err = %v, reads = %d", err, reader.reads)
	}
	// a password protected one: refused, no call
	locked := []byte("%PDF-1.7\ntrailer << /Root 1 0 R /Encrypt 5 0 R >>\n%%EOF")
	if _, err := uploadImportFile(t, ctx, h, bank.ID, "locked.pdf", locked, consent); !isStatus(err, 422) || reader.reads != 0 {
		t.Fatalf("password protected: err = %v, reads = %d", err, reader.reads)
	}

	id, err := uploadImportFile(t, ctx, h, bank.ID, "Statements_31OCT26.pdf", pdf, consent)
	if err != nil {
		t.Fatal(err)
	}
	if reader.reads != 1 {
		t.Fatalf("reads = %d, want 1", reader.reads)
	}

	// rows as a CSV, the balance check passing; past data suggested (no
	// transaction in the account, its balance set)
	c, err := callHandler(ctx, h.CSVImport)(httptest.NewRecorder(), "GET", "/import/"+id, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := c.Render(ctx, &b); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	if n := strings.Count(html, `data-status="new"`); n != 3 {
		t.Errorf("new rows = %d, want 3", n)
	}
	if strings.Contains(html, "don&#39;t add up") {
		t.Error("balance check warned on rows adding up")
	}
	if !pastSuggested(html) {
		t.Error("past data isn't suggested for an account without transactions")
	}

	// the same PDF again: no call, a cached usage row
	again, err := uploadImportFile(t, ctx, h, bank.ID, "copy.pdf", pdf, consent)
	if err != nil {
		t.Fatal(err)
	}
	if reader.reads != 1 {
		t.Errorf("the same PDF was read again (%d reads)", reader.reads)
	}
	imp, err := repo.GetCSVImport(ctx, again)
	if err != nil || len(imp.Content) == 0 {
		t.Fatalf("cached import = %+v, %v", imp, err)
	}

	// imported as past data: the balance doesn't move; two rows first, the
	// import opened again for the last one
	w := httptest.NewRecorder()
	if _, err := callHandler(ctx, h.ImportCSV)(w, "POST", "/import/"+id, id,
		pdfMappingForm(url.Values{"line": {"2", "3"}, "historical": {"1"}})); err != nil {
		t.Fatal(err)
	}
	if html := csvPreview(t, ctx, h, id, pdfMappingForm(nil)); strings.Count(html, `data-status="imported"`) != 2 || strings.Count(html, `data-status="new"`) != 1 {
		t.Error("the import opened again doesn't tell the rows imported and the one left")
	}
	if _, err := callHandler(ctx, h.ImportCSV)(httptest.NewRecorder(), "POST", "/import/"+id, id,
		pdfMappingForm(url.Values{"line": {"2", "3", "4"}, "historical": {"1"}})); err != nil {
		t.Fatal(err)
	}
	if imp, err := repo.GetCSVImport(ctx, id); err != nil || imp.ImportedCount != 3 {
		t.Errorf("imported count = %v (%v), want 3", imp, err)
	}
	if loc := w.Header().Get("HX-Redirect"); !strings.Contains(loc, "import="+id) {
		t.Errorf("HX-Redirect = %q", loc)
	}
	assertBalance(t, ctx, repo, bank.ID, 50000)
	if n := countTxs(t, ctx, repo, models.TxFilter{ImportID: id}); n != 3 {
		t.Fatalf("txs of the import = %d, want 3", n)
	}
	if n := countTxs(t, ctx, repo, models.TxFilter{ImportID: again}); n != 0 {
		t.Errorf("txs of the other import = %d, want 0", n)
	}

	// the balance chart still walks back through them: before them it was
	// lower by what they brought in (−85.5 + 70000 − 40000)
	acc, err := repo.GetAccount(ctx, bank.ID)
	if err != nil {
		t.Fatal(err)
	}
	hist, err := repo.GetAccountHistory(ctx, acc)
	if err != nil {
		t.Fatal(err)
	}
	at := func(daysAgo int) float64 {
		i := int(tenantToday().AddDate(0, 0, -daysAgo).Sub(hist.From).Hours() / 24)
		if i < 0 || i >= len(hist.Balance) {
			t.Fatalf("day %d ago out of the history (%v, %d days)", daysAgo, hist.From, len(hist.Balance))
		}
		return hist.Balance[i]
	}
	if got := at(0); got != 50000 {
		t.Errorf("today's balance in the chart = %v, want 50000", got)
	}
	if got, want := at(41), 50000-29914.5; got != want {
		t.Errorf("balance before the statement = %v, want %v", got, want)
	}

	// editing and deleting past data never moves the balance either
	ids, _, err := repo.GetFilteredTransactionIDs(ctx, models.TxFilter{ImportID: id}, -1)
	if err != nil {
		t.Fatal(err)
	}
	old, err := repo.GetTransaction(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if !old.Historical {
		t.Fatal("imported tx isn't past data")
	}
	edited := *old
	edited.Amount += 100
	if err := repo.UpdateTransaction(ctx, old, &edited); err != nil {
		t.Fatal(err)
	}
	assertBalance(t, ctx, repo, bank.ID, 50000)
	if got, _ := repo.GetTransaction(ctx, old.ID); got == nil || !got.Historical {
		t.Error("an edit dropped the past data flag")
	}
	if err := repo.DeleteTransaction(ctx, &edited); err != nil {
		t.Fatal(err)
	}
	assertBalance(t, ctx, repo, bank.ID, 50000)

	// over the monthly limit: refused before reading
	other := []byte("%PDF-1.7\n% another statement\n%%EOF")
	if _, err := uploadImportFile(t, ctx, h, bank.ID, "b.pdf", other, consent); err != nil {
		t.Fatal(err)
	}
	if _, err := uploadImportFile(t, ctx, h, bank.ID, "c.pdf", append(other, ' '), consent); !isStatus(err, 429) {
		t.Errorf("over the limit: err = %v, want a 429", err)
	}
}

func TestStatementWarning(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	reader := &fakeStatements{result: &statementocr.Result{
		IsStatement: true, Kind: statementocr.KindCard, Currency: "USD", AccountHint: "5229 **** **** 9545",
		OpeningBalance: ptrNum(19694.80), ClosingBalance: ptrNum(24715.73),
		Rows: []statementocr.Row{{Date: "2026-09-08", Description: "Uber Downtown", Debit: 85}},
	}}
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	h.SetReceiptOCR(nil, config.ReceiptOCR{StatementImportsPerMonth: 5})
	h.SetStatementReader(reader)
	card := newTestAccount(t, repo, ctx, "Card", "EGP", models.AccKindCreditCard, 0)

	id, err := uploadImportFile(t, ctx, h, card.ID, "Statements_Oct-26.pdf", []byte("%PDF-1.7\n%%EOF"), url.Values{"ai_consent": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	imp, err := repo.GetCSVImport(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(imp.Warning, "don't add up") || !strings.Contains(imp.Warning, "in USD") {
		t.Errorf("warning = %q", imp.Warning)
	}
}

func TestCSVImportHistoricalNotSuggested(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	f := func(daysAgo int) string { return tenantToday().AddDate(0, 0, -daysAgo).Format("02/01/2006") }
	first := &models.Transaction{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZE", Account: *bank, Kind: models.ExpenseTxKind, Amount: 10,
		ScheduledTo: tenantToday().AddDate(0, 0, -20), FulfilledAt: tenantToday().AddDate(0, 0, -20)}
	if err := repo.CreateTransaction(ctx, first, nil, nil); err != nil {
		t.Fatal(err)
	}

	// a row after the account's first transaction: not past data
	id := uploadCSV(t, ctx, h, bank.ID, "Date,Description,Amount\n"+f(30)+",OLD,-5\n"+f(3)+",NEW,-7\n")
	if html := csvPreview(t, ctx, h, id, csvMappingForm(nil)); pastSuggested(html) {
		t.Error("past data suggested for rows after the first transaction")
	}
	// all before it: suggested, unless unticked (kept on previewing again)
	old := uploadCSV(t, ctx, h, bank.ID, "Date,Description,Amount\n"+f(30)+",OLD,-5\n")
	if html := csvPreview(t, ctx, h, old, csvMappingForm(nil)); !pastSuggested(html) {
		t.Error("past data not suggested for rows before the first transaction")
	}

	// a regular import moves the balance
	if _, err := callHandler(ctx, h.ImportCSV)(httptest.NewRecorder(), "POST", "/import/"+id, id,
		csvMappingForm(url.Values{"line": {"3"}})); err != nil {
		t.Fatal(err)
	}
	assertBalance(t, ctx, repo, bank.ID, 1000-10-7)
}

// pastSuggested tells the preview's primary button imports as past data.
func pastSuggested(html string) bool {
	return strings.Contains(html, `name="historical" value="1" class="btn-primary`)
}

func assertBalance(t *testing.T, ctx context.Context, repo *repository.Repository, id string, want float64) {
	t.Helper()
	acc, err := repo.GetAccount(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if acc.Amount != want {
		t.Errorf("balance = %v, want %v", acc.Amount, want)
	}
}

func TestStatementImportForeignFees(t *testing.T) {
	repo, ctx, _ := setupTenant(t)
	d := func(daysAgo int) string { return tenantToday().AddDate(0, 0, -daysAgo).Format(time.DateOnly) }
	reader := &fakeStatements{result: &statementocr.Result{
		IsStatement: true, Kind: statementocr.KindCard, Currency: "EGP",
		Rows: []statementocr.Row{
			{Date: d(20), Description: "Disney Plus London", Debit: 170},
			{Date: d(19), Description: "WL STEAM PURCHASE BELLEVUE", Debit: 341.49, OriginalAmount: 6.36, OriginalCurrency: "USD"},
			{Date: d(18), Description: "aliexpress Luxembourg", Credit: 917.95},
			{Date: d(20), Description: "FOREIGN EXCHANGE FEES", Debit: 5.10, IsFxFee: true},
			{Date: d(18), Description: "FOREIGN EXCHANGE FEES REV", Credit: 27.53, IsFxFee: true},
		},
	}}
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, nil, nil)
	h.SetReceiptOCR(nil, config.ReceiptOCR{StatementImportsPerMonth: 5})
	h.SetStatementReader(reader)
	card := newTestAccount(t, repo, ctx, "Card", "EGP", models.AccKindCreditCard, 0)
	card.FxFeePercent = 3
	if err := repo.UpdateAccount(ctx, card); err != nil {
		t.Fatal(err)
	}

	id, err := uploadImportFile(t, ctx, h, card.ID, "Statements_Oct-26.pdf", []byte("%PDF-1.7\n%%EOF"), url.Values{"ai_consent": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := callHandler(ctx, h.ImportCSV)(httptest.NewRecorder(), "POST", "/import/"+id, id,
		pdfMappingForm(url.Values{"line": {"2", "3", "4"}})); err != nil {
		t.Fatal(err)
	}
	all, err := repo.GetTransactions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("txs = %d, want 3 (the fee lines merged)", len(all))
	}
	for _, tx := range all {
		switch {
		case strings.Contains(tx.Description, "Disney"):
			if tx.Amount != 175.10 || tx.FxFee != 5.10 || tx.Original != nil {
				t.Errorf("disney = %v fee %v original %+v", tx.Amount, tx.FxFee, tx.Original)
			}
		case strings.Contains(tx.Description, "STEAM"):
			if tx.Amount != 341.49 || !tx.IsForeign() || tx.Original.Amount != 6.36 || tx.Original.Currency != "USD" {
				t.Errorf("steam = %v original %+v", tx.Amount, tx.Original)
			}
		case strings.Contains(tx.Description, "aliexpress"):
			if tx.Amount != 945.48 || tx.Kind != models.IncomeTxKind {
				t.Errorf("refund = %v kind %v", tx.Amount, tx.Kind)
			}
		default:
			t.Errorf("unexpected tx %q", tx.Description)
		}
	}
	assertBalance(t, ctx, repo, card.ID, -175.10-341.49+945.48)
}
