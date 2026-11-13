package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/formatters"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/receiptimg"
	"github.com/omareloui/tyche/internal/receiptocr"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/slicesutils"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

const (
	// maxReceiptBytes is the biggest image a scan takes (receipts.js resizes
	// them to a few hundred KB first, receiptimg the ones it couldn't).
	maxReceiptBytes = 8 << 20
	// receiptReadTimeout is how long reading a receipt may take.
	receiptReadTimeout = 50 * time.Second
)

var receiptTypes = []string{"image/jpeg", "image/png", "image/webp", "image/heic", "image/heif"}

// receiptScan is a scanned receipt: what was read, the stored image's id and
// the scans left this month.
type receiptScan struct {
	result    *receiptocr.Result
	receiptID string
	size      int64
	// cached tells it was read before, not counted.
	cached bool
	left   int
	limit  int
}

// errScan is a scan that failed for a reason to show the user.
type errScan struct{ msg string }

func (e errScan) Error() string { return e.msg }

// readReceiptUpload is the posted image (the first non-empty "receipt"
// file) and its type.
func readReceiptUpload(w http.ResponseWriter, r *http.Request) ([]byte, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxReceiptBytes+1<<20)
	if err := r.ParseMultipartForm(maxReceiptBytes); err != nil {
		return nil, "", errScan{"The image is too big or didn't upload, try again."}
	}
	for _, fh := range r.MultipartForm.File["receipt"] {
		if fh.Size == 0 {
			continue
		}
		f, err := fh.Open()
		if err != nil {
			return nil, "", err
		}
		data, err := io.ReadAll(io.LimitReader(f, maxReceiptBytes+1))
		f.Close()
		if err != nil {
			return nil, "", err
		}
		if len(data) > maxReceiptBytes {
			return nil, "", errScan{"The image is too big."}
		}
		ct := http.DetectContentType(data)
		if !strings.HasPrefix(ct, "image/") {
			// HEIC isn't sniffed: trust the browser's type for it.
			ct = strings.ToLower(fh.Header.Get("Content-Type"))
		}
		ct, _, _ = strings.Cut(ct, ";")
		for _, t := range receiptTypes {
			if ct == t {
				return data, ct, nil
			}
		}
		return nil, "", errScan{"Pick a photo (JPEG, PNG, WebP or HEIC)."}
	}
	return nil, "", errScan{"Pick a photo of the receipt."}
}

// scanReceipt reads the posted receipt within the tenant's monthly limit,
// keeping the scan for the usage and the image for its transaction.
func (h Handler) scanReceipt(w http.ResponseWriter, r *http.Request) (*receiptScan, error) {
	if h.ocr == nil {
		return nil, resperr.New(http.StatusNotFound, "Reading receipts is off")
	}
	ctx := r.Context()
	l := logger.FromCtx(ctx)
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	img, contentType, err := readReceiptUpload(w, r)
	if err != nil {
		return nil, err
	}
	// The upload's hash: the same file hits the cache before converting it.
	sum := sha256.Sum256(img)
	hash := hex.EncodeToString(sum[:])

	if scan, err := h.cachedReceiptScan(r, tenantID, hash); scan != nil || err != nil {
		return scan, err
	}

	left, limit, err := h.repo.ReceiptScansLeft(ctx, tenantID, h.ocrCfg.ScansPerMonth, time.Now())
	if err != nil {
		return nil, err
	}
	if left <= 0 {
		return nil, errScan{fmt.Sprintf("You've scanned %d receipts this month, the most you can. It starts again on the 1st.", limit)}
	}

	// Read and stored as a small WebP, whatever was uploaded (e.g. a HEIC
	// the browser couldn't shrink); as is when it can't be converted.
	if webp, err := receiptimg.Normalize(img, contentType); err != nil {
		l.Warn("converting a receipt's image, keeping it as is", zap.Error(err), zap.String("content_type", contentType), zap.Int("size", len(img)))
	} else {
		img, contentType = webp, receiptimg.ContentType
	}

	readCtx, cancel := context.WithTimeout(ctx, receiptReadTimeout)
	defer cancel()
	result, usage, readErr := h.ocr.Read(readCtx, img, contentType)
	scan := models.ReceiptScan{
		TenantID:     tenantID,
		Model:        h.ocr.Model(),
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		CostMicros:   usage.CostMicros(h.ocrCfg.InputPerM, h.ocrCfg.OutputPerM),
		OK:           readErr == nil,
	}
	if readErr != nil {
		scan.Error = readErr.Error()
	}
	if err := h.repo.RecordReceiptScan(ctx, scan); err != nil {
		l.Error("recording a receipt scan", zap.Error(err))
	}
	if scan.Billed() {
		left--
	}
	if errors.Is(readErr, receiptocr.ErrUnreadable) {
		l.Info("unreadable receipt", zap.Error(readErr))
		return nil, errScan{"Couldn't read a receipt in the photo. Try again closer, flat and well lit."}
	}
	if readErr != nil {
		l.Error("reading a receipt", zap.Error(readErr))
		return nil, errScan{"Couldn't read the receipt right now, try again in a bit."}
	}

	ocrJSON, _ := json.Marshal(result)
	id, err := h.repo.CreateReceipt(ctx, &repository.ScannedReceipt{
		Data:        img,
		ContentType: contentType,
		Hash:        hash,
		Merchant:    result.Merchant,
		OCRJSON:     string(ocrJSON),
	})
	if err != nil {
		return nil, err
	}
	l.Info("scanned a receipt", zap.String("receipt_id", id), zap.Int("items", len(result.Items)), zap.Int("charges", len(result.Charges)))
	return &receiptScan{result: result, receiptID: id, size: int64(len(img)), left: max(left, 0), limit: limit}, nil
}

// cachedReceiptScan answers a scan of an image already read (same sha256 of
// the upload, in the tenant's receipts) with what was read then: no call, not counted
// toward the limit. An unlinked receipt is reused, one of a transaction
// copied. nil when the image wasn't read before.
func (h Handler) cachedReceiptScan(r *http.Request, tenantID, hash string) (*receiptScan, error) {
	ctx := r.Context()
	l := logger.FromCtx(ctx)
	rec, err := h.repo.FindReceiptByHash(ctx, hash)
	if err != nil || rec == nil {
		return nil, err
	}
	var result receiptocr.Result
	if err := json.Unmarshal([]byte(rec.OcrJson), &result); err != nil {
		l.Warn("unreadable cached receipt result, reading it again", zap.Error(err), zap.String("receipt_id", rec.ID))
		return nil, nil
	}

	id := rec.ID
	if rec.TransactionID.Valid {
		if id, err = h.repo.CreateReceipt(ctx, &repository.ScannedReceipt{
			Data: rec.Data, ContentType: rec.ContentType, Hash: hash, Merchant: rec.Merchant, OCRJSON: rec.OcrJson,
		}); err != nil {
			return nil, err
		}
	}
	if err := h.repo.RecordReceiptScan(ctx, models.ReceiptScan{TenantID: tenantID, Model: h.ocr.Model(), OK: true, Cached: true}); err != nil {
		l.Error("recording a cached receipt scan", zap.Error(err))
	}
	left, limit, err := h.repo.ReceiptScansLeft(ctx, tenantID, h.ocrCfg.ScansPerMonth, time.Now())
	if err != nil {
		return nil, err
	}
	l.Info("scanned a receipt from the cache", zap.String("receipt_id", id), zap.String("from", rec.ID))
	return &receiptScan{result: &result, receiptID: id, size: int64(len(rec.Data)), left: left, limit: limit, cached: true}, nil
}

// scanError shows a scan's error in the slot selector, as a 422 swapped
// inline.
func scanError(w http.ResponseWriter, slot string, err error) error {
	var se errScan
	if !errors.As(err, &se) {
		return err
	}
	w.Header().Set("HX-Retarget", slot)
	w.Header().Set("HX-Reswap", "innerHTML")
	return resperr.New(http.StatusUnprocessableEntity, se.msg)
}

// receiptCurrency is the currency the receipt's lines are in: the one it's
// printed in, the account's when it doesn't tell or it's unknown (warning
// then that its amounts were taken as is).
func (h Handler) receiptCurrency(ctx context.Context, res *receiptocr.Result, acc *models.Account) (*models.Currency, string) {
	code := strings.ToUpper(strings.TrimSpace(res.Currency))
	if code == "" || acc == nil || acc.Currency.String == "" || strings.EqualFold(code, acc.Currency.String) {
		return &acc.Currency, ""
	}
	cur, err := h.repo.GetCurrencyByString(ctx, code)
	if err != nil {
		logger.FromCtx(ctx).Warn("the receipt's currency is unknown", zap.Error(err))
		return &acc.Currency, fmt.Sprintf("The receipt is in %s, the account in %s: the amounts were taken as is.", code, acc.Currency.String)
	}
	return cur, ""
}

// estimateTxAmount is what lines of total in cur come to in the account's
// currency (acc) on day, at that day's rate (else today's), with a notice
// telling it's an estimate; 0 without a rate.
func (h Handler) estimateTxAmount(ctx context.Context, total float64, cur, acc *models.Currency, day time.Time) (float64, string) {
	if h.fxJob == nil || total == 0 {
		return 0, ""
	}
	rate, dated, err := h.fxJob.RateOn(ctx, day, cur.String, acc.String)
	if err != nil || rate <= 0 {
		logger.FromCtx(ctx).Warn("no fx rate to estimate the amount", zap.Error(err))
		return 0, ""
	}
	when := "today's rate"
	if dated {
		when = "the rate of " + day.Format("Jan 2")
	}
	return acc.Round(total * rate), fmt.Sprintf("Amount estimated at %s (1 %s = %s %s): set what was charged.",
		when, cur.String, formatters.FormatRate(rate), acc.String)
}

// ScanReceiptIntoForm reads a receipt into the transaction form being
// filled: its lines and image, and the fields still empty (on a form with no
// amount yet: kind, amount, date, merchant, description, category).
func (h Handler) ScanReceiptIntoForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	formID := r.PathValue("id")
	if formID == "" {
		return nil, errs.ErrInvalidID
	}
	slot := "#" + views.TxScanErrorID(formID)
	scan, err := h.scanReceipt(w, r)
	if err != nil {
		return nil, scanError(w, slot, err)
	}
	ctx := r.Context()
	res := scan.result

	isEdit := formID != "0"
	tx, err := h.mapFDToTransaction(r, !isEdit && r.FormValue("is_recurrent") != "")
	if err != nil {
		return nil, err
	}
	accs, _, _, _, err := h.getTxDeps(ctx)
	if err != nil {
		return nil, err
	}
	if a := slicesutils.Find(accs, func(a models.Account) bool { return a.ID == tx.Account.ID }); a != nil {
		tx.Account = *a
	}

	lineCur, warning := h.receiptCurrency(ctx, res, &tx.Account)
	foreign := lineCur.ID != tx.Account.Currency.ID
	fresh := tx.Amount == 0
	var filled []string
	var estimate string
	if fresh {
		if tx.Kind != models.ExpenseTxKind && tx.Kind != models.RefundTxKind {
			tx.Kind = models.ExpenseTxKind
			tx.ToAccount, tx.ToAmount = nil, 0
		}
		// The day first: a foreign receipt's amount is estimated at its rate.
		dated := false
		if when := res.When(getClaims(ctx).Location()); !when.IsZero() {
			day := time.Date(when.Year(), when.Month(), when.Day(), 0, 0, 0, 0, time.UTC)
			if !day.After(h.userToday(ctx)) {
				tx.ScheduledTo, dated = day, true
			}
		}
		if total := res.Total; total > 0 {
			if !foreign {
				tx.Amount = tx.Account.Currency.Round(total)
				filled = append(filled, "amount")
			} else if amount, notice := h.estimateTxAmount(ctx, total, lineCur, &tx.Account.Currency, tx.ScheduledTo); amount > 0 {
				tx.Amount, estimate = amount, notice
				filled = append(filled, "amount")
			}
		}
		if dated {
			filled = append(filled, "date")
		}
	}
	if merchant := strings.TrimSpace(res.Merchant); merchant != "" && tx.Kind.CategoryKind() == models.ExpenseTxKind {
		if tx.Counterparty == nil || tx.Counterparty.ID == "" {
			cp := dfltCounterparty()
			cp.ID, cp.Name, cp.IsMerchant = idgen.New(), merchant, true
			if err := cp.SanitizeAndValidate(h.sani, h.valid); err != nil {
				logger.FromCtx(ctx).Warn("the receipt's merchant isn't a valid counterparty", zap.Error(err))
			} else {
				if cp, err = h.findOrCreateCounterparty(ctx, cp); err != nil {
					return nil, err
				}
				tx.Counterparty = cp
				filled = append(filled, "merchant")
				if tx.Category == nil || tx.Category.ID == "" {
					tx.Category = cp.DefaultCategoryFor(tx.Kind)
				}
			}
		}
		if tx.Description == "" {
			tx.Description = merchant
		}
	}
	tx.Items = res.TxItems()
	models.SetTxItemsCurrency(tx.Items, lineCur, &tx.Account.Currency)
	models.NormalizeTxItems(tx.Items, lineCur)
	tx.Items = models.SortTxItems(tx.Items)
	filled = append(filled, txItemsCountNotice(len(tx.Items)))
	if foreign {
		filled = append(filled, "in "+lineCur.String)
	}

	accs, knds, cats, cps, err := h.getTxDeps(ctx)
	if err != nil {
		return nil, err
	}
	fd := h.mapTransactionToFD(r.Context(), tx, new(errs.ValidationError))
	if isEdit {
		fd.RecurrenceFD = nil
	}
	fd.WishlistID = r.FormValue("wishlist_id")
	fd.Receipt = views.ReceiptFD{
		Enabled: true,
		ID:      scan.receiptID,
		URL:     (&models.Receipt{ID: scan.receiptID, Hash: "new"}).URL(),
		Size:    models.FormatBytes(scan.size),
		Notice:  "Read from the receipt: " + strings.Join(filled, ", ") + "." + cachedNotice(scan),
		Warning: strings.TrimSpace(warning + " " + estimate),
		Scanned: true,
		Left:    scan.left,
		Limit:   scan.limit,
	}
	return views.TxFormFields(fd, formID, accs, knds, cats, cps, &getClaims(ctx).DefaultCurrency), nil
}

// cachedNotice tells a scan was answered from an earlier one of the image.
func cachedNotice(scan *receiptScan) string {
	if scan.cached {
		return " Already read before, not counted."
	}
	return ""
}

func txItemsCountNotice(n int) string {
	if n == 1 {
		return "1 line"
	}
	return fmt.Sprintf("%d lines", n)
}

// ScanReceiptIntoTx reads a receipt for a saved transaction from its
// details: its lines (replacing the ones it had) and image.
func (h Handler) ScanReceiptIntoTx(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	tx, err := h.editableTx(r)
	if err != nil {
		return nil, err
	}
	scan, err := h.scanReceipt(w, r)
	if err != nil {
		return nil, scanError(w, "#txd-scan-error-"+tx.ID, err)
	}
	items := scan.result.TxItems()
	lineCur, warning := h.receiptCurrency(r.Context(), scan.result, &tx.Account)
	models.SetTxItemsCurrency(items, lineCur, &tx.Account.Currency)
	models.NormalizeTxItems(items, lineCur)
	items = models.SortTxItems(items)
	if err := h.repo.SetTxItems(r.Context(), tx.ID, items, scan.receiptID); err != nil {
		return nil, err
	}
	rec := &views.ReceiptFD{
		Notice:  "Read " + txItemsCountNotice(len(items)) + " from the receipt." + cachedNotice(scan),
		Warning: warning,
		Scanned: true,
		Left:    scan.left,
		Limit:   scan.limit,
	}
	if m := strings.TrimSpace(scan.result.Merchant); m != "" && tx.Counterparty == nil {
		rec.Notice += " Merchant: " + m + " (set it from Edit)."
	}
	return h.txItemsAnswer(w, r, tx.ID, nil, rec)
}

// EstimateItemsAmount answers the form's lines' "Use as the amount": their
// total, converted to the account's currency at the form's day rate when
// they're in another (an estimate, to set to what was charged).
func (h Handler) EstimateItemsAmount(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	tx, err := h.mapFDToTransaction(r, false)
	if err != nil {
		return nil, err
	}
	accs, _, _, _, err := h.getTxDeps(ctx)
	if err != nil {
		return nil, err
	}
	a := slicesutils.Find(accs, func(a models.Account) bool { return a.ID == tx.Account.ID })
	if a == nil {
		return views.ItemsAmountEstimate("", "Pick the account first.", true), nil
	}
	tx.Account = *a
	normalizeTxItems(tx)
	acc := &tx.Account.Currency
	cur := models.TxItemsCurrency(tx.Items, acc)
	total := math.Abs(models.TxItemsTotal(tx.Items))
	if total == 0 {
		return views.ItemsAmountEstimate("", "The lines add up to nothing yet.", true), nil
	}
	if !models.TxItemsForeign(tx.Items, acc) {
		return views.ItemsAmountEstimate(formatItemAmount(acc.Round(total)), "The amount is the lines' total.", false), nil
	}
	amount, notice := h.estimateTxAmount(ctx, total, cur, acc, tx.ScheduledTo)
	if amount == 0 {
		return views.ItemsAmountEstimate("", fmt.Sprintf("No %s → %s rate to estimate it: type what was charged.", cur.String, acc.String), true), nil
	}
	return views.ItemsAmountEstimate(formatItemAmount(amount), notice, true), nil
}

func formatItemAmount(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
