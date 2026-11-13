package router_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/receiptocr"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

// fakeOCR answers result (or err) to every read, with a billed usage.
type fakeOCR struct {
	result *receiptocr.Result
	err    error
	reads  int
	// lastType is the type of the last image read.
	lastType string
}

func (f *fakeOCR) Model() string { return "fake-flash" }

func (f *fakeOCR) Read(ctx context.Context, img []byte, ct string) (*receiptocr.Result, receiptocr.Usage, error) {
	f.reads++
	f.lastType = ct
	usage := receiptocr.Usage{InputTokens: 1000, OutputTokens: 200}
	if f.err != nil {
		return nil, usage, f.err
	}
	return f.result, usage, nil
}

// jpeg is the first bytes of a JPEG, enough to be sniffed as one.
var jpeg = append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{1}, 64)...)

func TestReceipts(t *testing.T) {
	// Read before setupTenant leaves the package's dir.
	heic, err := os.ReadFile("../receiptimg/testdata/receipt.heic")
	if err != nil {
		t.Fatal(err)
	}
	repo, ctx, usr := setupTenant(t)
	usrID := models.TenantID(usr.OAuthProvider, usr.OAuthID)
	t.Setenv("ADMIN_TENANT_IDS", usrID)

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
	cafe := &models.Counterparty{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZC", Name: "Cilantro", Aliases: []string{"CILANTRO EG"}, IsMerchant: true}
	if err := repo.CreateCounterparty(ctx, cafe); err != nil {
		t.Fatal(err)
	}

	ocr := &fakeOCR{result: &receiptocr.Result{
		IsReceipt: true, Merchant: "CILANTRO EG", Date: time.Now().AddDate(0, 0, -1).Format(time.DateOnly), Time: "18:42",
		Currency: "EGP", Subtotal: 180, Total: 201.6,
		Items: []receiptocr.Item{{Name: "Latte", Quantity: 2, UnitPrice: 45, Total: 90}, {Name: "Muffin", Quantity: 1, UnitPrice: 90, Total: 90}},
		Charges: []receiptocr.Charge{
			{Kind: "vat", Name: "VAT", Rate: ptrF(14), Amount: 25.2, Included: true},
			{Kind: "service", Rate: ptrF(12), Amount: 21.6},
		},
	}}
	h := handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)
	h.SetReceiptOCR(ocr, config.ReceiptOCR{Model: "fake-flash", InputPerM: 0.75, OutputPerM: 3.75, ScansPerMonth: 3})
	srv := httptest.NewServer(router.New(h))
	t.Cleanup(srv.Close)

	send := func(req *http.Request, token string) (*http.Response, string) {
		t.Helper()
		req.Header.Set("HX-Request", "true")
		req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	do := func(method, path string, form url.Values) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return send(req, pair.Access.Encoded)
	}
	// Every upload is a new image (no cache hit) but for the ones of img.
	shot := 0
	var img []byte
	imgType := "image/jpeg"
	upload := func(path string, fields url.Values) (*http.Response, string) {
		t.Helper()
		data := img
		if data == nil {
			shot++
			data = append(append([]byte{}, jpeg...), byte(shot))
		}
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, vs := range fields {
			for _, v := range vs {
				_ = mw.WriteField(k, v)
			}
		}
		// The camera input empty, the gallery one with the photo.
		empty, _ := mw.CreateFormFile("receipt", "")
		_, _ = empty.Write(nil)
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", `form-data; name="receipt"; filename="receipt.jpg"`)
		hdr.Set("Content-Type", imgType)
		part, _ := mw.CreatePart(hdr)
		_, _ = part.Write(data)
		_ = mw.Close()
		req, _ := http.NewRequest("POST", srv.URL+path, &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		return send(req, pair.Access.Encoded)
	}

	newForm := url.Values{
		"kind": {models.ExpenseTxKind.String()}, "account_id": {bank.ID}, "amount": {""},
		"scheduled_to": {time.Now().Format(time.DateOnly)}, "description": {""},
	}

	var receiptID string
	t.Run("the transactions page offers scanning", func(t *testing.T) {
		_, body := do("GET", "/transactions", nil)
		for _, want := range []string{"data-receipt-quick", "Scan a receipt", "AI fills in the amount, merchant, date and items."} {
			if !strings.Contains(body, want) {
				t.Errorf("no %q in the page", want)
			}
		}
	})

	t.Run("scanning fills the new transaction's form", func(t *testing.T) {
		res, body := upload("/receipts/scan/0", newForm)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		for _, want := range []string{
			`value="201.6"`, `value="Latte"`, `value="Muffin"`,
			`name="counterparty_id" value="` + cafe.ID + `"`, `value="` + time.Now().AddDate(0, 0, -1).Format(time.DateOnly) + `"`,
			"2 of 3 scans left this month.", "Read from the receipt: amount, date, merchant, 4 lines.",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("form misses %q", want)
			}
		}
		i := strings.Index(body, `name="receipt_id" value="`)
		if i < 0 {
			t.Fatal("no receipt_id")
		}
		rest := body[i+len(`name="receipt_id" value="`):]
		receiptID = rest[:strings.Index(rest, `"`)]
		if receiptID == "" {
			t.Fatal("empty receipt_id")
		}
	})

	t.Run("a typed amount is kept", func(t *testing.T) {
		form := url.Values{}
		for k, v := range newForm {
			form[k] = v
		}
		form.Set("amount", "150")
		form.Set("description", "Breakfast")
		_, body := upload("/receipts/scan/0", form)
		if !strings.Contains(body, `value="150"`) || !strings.Contains(body, `value="Breakfast"`) || strings.Contains(body, `value="201.6"`) {
			t.Errorf("the typed fields weren't kept: %s", body)
		}
	})

	var txID string
	t.Run("saving keeps the lines and links the receipt", func(t *testing.T) {
		form := url.Values{
			"kind": {models.ExpenseTxKind.String()}, "account_id": {bank.ID}, "amount": {"201.6"},
			"scheduled_to": {time.Now().Format(time.DateOnly)}, "counterparty_id": {cafe.ID},
			"receipt_id":      {receiptID},
			"item_id":         {"", "", "", ""},
			"item_kind":       {"0", "0", "1", "2"},
			"item_name":       {"Latte", "Muffin", "VAT", ""},
			"item_quantity":   {"2", "", "", ""},
			"item_unit_price": {"45", "90", "", ""},
			"item_total":      {"", "", "25.2", ""},
			"item_rate":       {"", "", "14", "12"},
			"item_included":   {"", "", "on", ""},
		}
		res, body := do("POST", "/transactions", form)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		txs, err := repo.GetTransactions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, tx := range txs {
			if tx.Amount == 201.6 {
				txID = tx.ID
			}
		}
		if txID == "" {
			t.Fatal("transaction not created")
		}
		tx, err := repo.GetTransaction(ctx, txID)
		if err != nil {
			t.Fatal(err)
		}
		if len(tx.Items) != 4 || tx.Receipt == nil || tx.Receipt.ID != receiptID {
			t.Fatalf("items = %+v, receipt = %+v", tx.Items, tx.Receipt)
		}
		if tx.Items[0].Total != 90 || tx.Items[1].Quantity != 1 || !tx.Items[2].Included || tx.Items[3].Total != 21.6 {
			t.Errorf("items = %+v", tx.Items)
		}
		if got := models.TxItemsTotal(tx.Items); got != 201.6 {
			t.Errorf("lines total = %v", got)
		}
	})

	t.Run("the details show the lines, add and remove them", func(t *testing.T) {
		_, body := do("GET", "/transactions/"+txID+"/details", nil)
		for _, want := range []string{"Latte", "Subtotal", "· 14%", "Service 12%", "· included", "/receipts/" + receiptID, "Scan receipt", "1 KB", "max-h-[calc(100dvh-2rem)]"} {
			if !strings.Contains(body, want) {
				t.Errorf("details miss %q", want)
			}
		}
		res, body := do("POST", "/transactions/"+txID+"/items", url.Values{"kind": {"6"}, "total": {"20"}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Discount") {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		tx, _ := repo.GetTransaction(ctx, txID)
		if len(tx.Items) != 5 || tx.Items[4].Total != -20 {
			t.Fatalf("items = %+v", tx.Items)
		}
		if !strings.Contains(body, "The lines add up to") {
			t.Error("no mismatch note with the discount")
		}
		_, body = do("POST", "/transactions/"+txID+"/items", url.Values{"kind": {"0"}, "name": {""}, "total": {"5"}})
		if !strings.Contains(body, "Name the item.") {
			t.Error("an unnamed item isn't refused")
		}
		res, _ = do("DELETE", "/transactions/"+txID+"/items/"+tx.Items[4].ID, nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("delete status %d", res.StatusCode)
		}
		if tx, _ := repo.GetTransaction(ctx, txID); len(tx.Items) != 4 {
			t.Errorf("items after delete = %d", len(tx.Items))
		}
		if res, _ := do("DELETE", "/transactions/"+txID+"/items/nope", nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("deleting a missing line = %d", res.StatusCode)
		}
	})

	t.Run("the receipt image is served and listed on the row", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/receipts/"+receiptID, nil)
		res, body := send(req, pair.Access.Encoded)
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/jpeg" || !strings.HasPrefix(body, string(jpeg)) {
			t.Errorf("status %d type %q", res.StatusCode, res.Header.Get("Content-Type"))
		}
		_, list := do("GET", "/transactions", nil)
		if !strings.Contains(list, "4 lines") {
			t.Error("the row has no lines chip")
		}
	})

	t.Run("another tenant can't see the receipt", func(t *testing.T) {
		other := &models.User{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZB", OAuthID: "other", OAuthProvider: models.Google}
		op, err := jwt.NewPair(other)
		if err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequest("GET", srv.URL+"/receipts/"+receiptID, nil)
		if res, _ := send(req, op.Access.Encoded); res.StatusCode == http.StatusOK {
			t.Errorf("status %d, want an error", res.StatusCode)
		}
	})

	t.Run("a failed read is shown in place and counted", func(t *testing.T) {
		ocr.err = errors.Join(receiptocr.ErrUnreadable, errors.New("blurry"))
		res, body := upload("/receipts/scan/0", newForm)
		ocr.err = nil
		if res.StatusCode != http.StatusUnprocessableEntity || res.Header.Get("HX-Retarget") != "#tx-scan-error-0" || !strings.Contains(body, "Couldn't read a receipt") {
			t.Fatalf("status %d retarget %q: %s", res.StatusCode, res.Header.Get("HX-Retarget"), body)
		}
	})

	t.Run("the monthly limit stops scans, an admin raises it", func(t *testing.T) {
		// 1 + 1 + 1 failed (billed) = 3 of 3.
		reads := ocr.reads
		res, body := upload("/receipts/scan/0", newForm)
		if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "the most you can") {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if ocr.reads != reads {
			t.Error("a scan past the limit was read")
		}
		res, body = do("PUT", "/admin/receipts/limits/"+usrID, url.Values{"limit": {"5"}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Saved") || !strings.Contains(body, `value="5"`) {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if res, _ := upload("/receipts/scan/0", newForm); res.StatusCode != http.StatusOK {
			t.Errorf("status %d after raising the limit", res.StatusCode)
		}
		if res, _ := do("PUT", "/admin/receipts/limits/"+usrID, url.Values{"limit": {"-1"}}); res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("a negative limit = %d", res.StatusCode)
		}
	})

	t.Run("the same image again comes from the cache, not counted", func(t *testing.T) {
		img = append(append([]byte{}, jpeg...), 0xAB)
		defer func() { img = nil }()
		if res, body := upload("/receipts/scan/0", newForm); res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		reads := ocr.reads
		res, body := upload("/receipts/scan/0", newForm)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Already read before, not counted.") || !strings.Contains(body, `value="Latte"`) {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if ocr.reads != reads {
			t.Error("a cached image was read again")
		}
		left, _, err := repo.ReceiptScansLeft(ctx, usrID, 3, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		// Past the limit, the cache still answers.
		repoLimit := 0
		if err := repo.SetReceiptLimit(ctx, usrID, &repoLimit); err != nil {
			t.Fatal(err)
		}
		if res, body := upload("/receipts/scan/0", newForm); res.StatusCode != http.StatusOK {
			t.Errorf("cached scan past the limit = %d: %s", res.StatusCode, body)
		}
		ten := 10
		_ = repo.SetReceiptLimit(ctx, usrID, &ten)
		if after, _, _ := repo.ReceiptScansLeft(ctx, usrID, 3, time.Now()); after != left+5 {
			t.Errorf("scans left %d → %d after cached scans", left, after)
		}
	})

	t.Run("the admin page shows the usage", func(t *testing.T) {
		_, body := do("GET", "/admin/receipts", nil)
		for _, want := range []string{"Receipt scans", "fake-flash", "1 failed", "from cache", usrID, "Scans a day", "Tokens a month", "Cost a month"} {
			if !strings.Contains(body, want) {
				t.Errorf("admin page misses %q", want)
			}
		}
		t.Setenv("ADMIN_TENANT_IDS", "")
		if res, _ := do("GET", "/admin/receipts", nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("non admin status %d", res.StatusCode)
		}
		t.Setenv("ADMIN_TENANT_IDS", usrID)
	})

	t.Run("scanning from the details replaces the lines", func(t *testing.T) {
		res, body := upload("/transactions/"+txID+"/receipt", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Read 4 lines from the receipt.") {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		tx, _ := repo.GetTransaction(ctx, txID)
		if len(tx.Items) != 4 || tx.Receipt == nil || tx.Receipt.ID == receiptID {
			t.Errorf("items = %d, receipt = %+v (old %s)", len(tx.Items), tx.Receipt, receiptID)
		}
	})

	t.Run("a heic is read and stored as a webp", func(t *testing.T) {
		img, imgType = heic, "image/heic"
		defer func() { img, imgType = nil, "image/jpeg" }()
		if res, body := upload("/transactions/"+txID+"/receipt", nil); res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if ocr.lastType != "image/webp" {
			t.Errorf("read a %q, want image/webp", ocr.lastType)
		}
		tx, _ := repo.GetTransaction(ctx, txID)
		if tx.Receipt == nil || tx.Receipt.ContentType != "image/webp" {
			t.Fatalf("receipt = %+v", tx.Receipt)
		}
		req, _ := http.NewRequest("GET", srv.URL+tx.Receipt.URL(), nil)
		if res, _ := send(req, pair.Access.Encoded); res.Header.Get("Content-Type") != "image/webp" {
			t.Errorf("served as %q", res.Header.Get("Content-Type"))
		}
	})

	t.Run("lines in another currency convert at the transaction's rate", func(t *testing.T) {
		eur, err := repo.GetCurrencyByString(ctx, "EUR")
		if err != nil {
			t.Fatal(err)
		}
		form := url.Values{
			"kind": {models.ExpenseTxKind.String()}, "account_id": {bank.ID}, "amount": {"800"},
			"scheduled_to": {time.Now().Format(time.DateOnly)}, "description": {"Trattoria"},
			"items_currency_id": {eur.ID},
			"item_id":           {"", ""},
			"item_kind":         {"0", "0"},
			"item_name":         {"Pasta", "Wine"},
			"item_quantity":     {"", ""},
			"item_unit_price":   {"", ""},
			"item_total":        {"10", "5"},
			"item_rate":         {"", ""},
			"item_included":     {"", ""},
		}
		res, body := do("POST", "/transactions", form)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		txs, _ := repo.GetTransactions(ctx)
		var id string
		for _, tx := range txs {
			if tx.Description == "Trattoria" {
				id = tx.ID
			}
		}
		tx, err := repo.GetTransaction(ctx, id)
		if err != nil {
			t.Fatal(err, body)
		}
		if len(tx.Items) != 2 || tx.Items[0].Currency == nil || tx.Items[0].Currency.String != "EUR" {
			t.Fatalf("items = %+v", tx.Items)
		}
		_, body = do("GET", "/transactions/"+id+"/details", nil)
		for _, want := range []string{"Charged", "1 EUR = 53.33 EGP"} {
			if !strings.Contains(body, want) {
				t.Errorf("details miss %q", want)
			}
		}
		if strings.Contains(body, "The lines add up to") {
			t.Error("foreign lines show a mismatch")
		}
		_, edit := do("GET", "/transactions/"+id+"/edit", nil)
		if !strings.Contains(edit, `name="items_currency_id" value="`+eur.ID+`"`) {
			t.Error("the edit form doesn't keep the lines' currency")
		}

		// Added from the details, a line takes the lines' currency.
		do("POST", "/transactions/"+id+"/items", url.Values{"kind": {"0"}, "name": {"Water"}, "total": {"2"}})
		tx, _ = repo.GetTransaction(ctx, id)
		if len(tx.Items) != 3 || tx.Items[2].Currency == nil || tx.Items[2].Currency.ID != eur.ID {
			t.Errorf("added line = %+v", tx.Items[len(tx.Items)-1])
		}

		// Picking the account's currency stores none.
		form.Set("items_currency_id", bank.Currency.ID)
		form.Set("description", "Koshary")
		do("POST", "/transactions", form)
		txs, _ = repo.GetTransactions(ctx)
		for _, tx := range txs {
			if tx.Description == "Koshary" {
				full, _ := repo.GetTransaction(ctx, tx.ID)
				if full.Items[0].Currency != nil {
					t.Errorf("the account's currency was stored: %+v", full.Items[0].Currency)
				}
			}
		}
	})

	t.Run("a receipt in another currency puts the lines in it", func(t *testing.T) {
		ocr.result.Currency = "EUR"
		defer func() { ocr.result.Currency = "EGP" }()
		res, body := upload("/transactions/"+txID+"/receipt", nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		if strings.Contains(body, "taken as is") {
			t.Error("a known currency still warns")
		}
		tx, _ := repo.GetTransaction(ctx, txID)
		if tx.Items[0].Currency == nil || tx.Items[0].Currency.String != "EUR" {
			t.Errorf("lines' currency = %+v", tx.Items[0].Currency)
		}
		ocr.result.Currency = "XYZ"
		_, body = upload("/transactions/"+txID+"/receipt", nil)
		if !strings.Contains(body, "taken as is") {
			t.Error("an unknown currency doesn't warn")
		}
		if tx, _ := repo.GetTransaction(ctx, txID); tx.Items[0].Currency != nil {
			t.Errorf("an unknown currency's lines = %+v", tx.Items[0].Currency)
		}
	})

	t.Run("recurrent lines go to the occurrences, edited ones kept", func(t *testing.T) {
		start := time.Now().AddDate(0, 0, 1)
		rt := createTestRecurrentTx(t, ctx, repo, h, bank, "300", start, models.RecurrentUnitMonth, url.Values{
			"item_id": {""}, "item_kind": {"0"}, "item_name": {"Membership"}, "item_quantity": {""},
			"item_unit_price": {""}, "item_total": {"300"}, "item_rate": {""}, "item_included": {""},
		})
		if len(rt.Items) != 1 {
			t.Fatalf("recurrent items = %+v", rt.Items)
		}
		occs, err := repo.GetRecurrentTxTransactions(ctx, rt.ID)
		if err != nil || len(occs) < 2 {
			t.Fatalf("occurrences = %d (%v)", len(occs), err)
		}
		first, err := repo.GetTransaction(ctx, occs[0].ID)
		if err != nil || len(first.Items) != 1 || first.Items[0].Name != "Membership" {
			t.Fatalf("occurrence items = %+v (%v)", first.Items, err)
		}
		// One edited on its own keeps its lines.
		if res, _ := do("POST", "/transactions/"+occs[0].ID+"/items", url.Values{"kind": {"0"}, "name": {"Towel"}, "total": {"10"}}); res.StatusCode != http.StatusOK {
			t.Fatalf("add status %d", res.StatusCode)
		}
		res, body := do("POST", "/recurrent-tx/"+rt.ID+"/items", url.Values{"kind": {"5"}, "name": {"Locker"}, "total": {"25"}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Locker") {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		edited, _ := repo.GetTransaction(ctx, occs[0].ID)
		other, _ := repo.GetTransaction(ctx, occs[1].ID)
		if len(edited.Items) != 2 || edited.Items[1].Name != "Towel" {
			t.Errorf("edited occurrence = %+v", edited.Items)
		}
		if len(other.Items) != 2 || other.Items[1].Name != "Locker" {
			t.Errorf("other occurrence = %+v", other.Items)
		}
		got, _ := repo.GetRecurrentTx(ctx, rt.ID)
		if res, _ := do("DELETE", "/recurrent-tx/"+rt.ID+"/items/"+got.Items[1].ID, nil); res.StatusCode != http.StatusOK {
			t.Fatalf("delete status %d", res.StatusCode)
		}
		if other, _ := repo.GetTransaction(ctx, occs[1].ID); len(other.Items) != 1 {
			t.Errorf("other occurrence after delete = %+v", other.Items)
		}
	})

	t.Run("duplicating copies the lines, not the receipt", func(t *testing.T) {
		_, body := do("GET", "/transactions/"+txID+"/duplicate", nil)
		if !strings.Contains(body, `value="Latte"`) || strings.Contains(body, `name="receipt_id" value="0`) {
			t.Errorf("duplicate form: %s", body)
		}
	})
}

func ptrF(v float64) *float64 { return &v }
