package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/csvimport"
	"github.com/omareloui/tyche/internal/formatters"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/statementocr"
)

// statementReadTimeout is how long reading a PDF statement may take.
const statementReadTimeout = 2 * time.Minute

// SetStatementReader turns importing PDF statements on (a nil reader keeps
// it off).
func (h *Handler) SetStatementReader(reader statementocr.Reader) {
	h.statements = reader
}

// uploadStatement reads a PDF statement into a pending import of its rows
// as a CSV, with the preset mapping: from there it's a CSV import. The same
// PDF read before (its sha256) reuses its rows without a call.
func (h *Handler) uploadStatement(w http.ResponseWriter, r *http.Request, acc *models.Account, fileName string, pdf []byte) error {
	ctx := r.Context()
	l := logger.FromCtx(ctx)
	if h.statements == nil {
		return resperr.New(http.StatusUnprocessableEntity, "Reading PDF statements is off: export a CSV from your bank instead.")
	}
	switch err := statementocr.Inspect(pdf); {
	case errors.Is(err, statementocr.ErrPasswordProtected):
		return resperr.New(http.StatusUnprocessableEntity, "The PDF is password protected: save a copy without the password (print it to PDF) and upload that.")
	case errors.Is(err, statementocr.ErrTooManyPages):
		return resperr.New(http.StatusUnprocessableEntity, fmt.Sprintf("The PDF has more than %d pages, split it.", statementocr.MaxPages))
	case err != nil:
		return resperr.New(http.StatusUnprocessableEntity, "Choose a CSV or PDF file.")
	}
	if r.FormValue("ai_consent") == "" {
		return resperr.New(http.StatusUnprocessableEntity, "Tick that the statement may be sent to the AI to be read.")
	}
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return err
	}
	hash := statementocr.CacheKey(pdf)
	imp := &csvimport.Import{AccountID: acc.ID, FileName: fileName, Source: csvimport.SourcePDF, SourceHash: hash, Mapping: statementocr.PresetMapping()}

	cached, err := h.repo.GetCSVImportBySourceHash(ctx, hash)
	if err != nil {
		return err
	}
	if cached != nil {
		imp.Content, imp.Warning = cached.Content, cached.Warning
		if err := h.repo.RecordReceiptScan(ctx, models.ReceiptScan{TenantID: tenantID, Model: h.statements.Model(), OK: true, Cached: true, Kind: models.ScanStatement}); err != nil {
			l.Error("recording a cached statement read", zap.Error(err))
		}
		l.Info("reused a read statement", zap.String("from_import", cached.ID))
	} else {
		if imp.Content, imp.Warning, err = h.readStatement(ctx, tenantID, acc, pdf); err != nil {
			return err
		}
	}

	recs, _, err := csvimport.Read(imp.Content, imp.Mapping.Delimiter)
	if err != nil {
		return resperr.New(http.StatusUnprocessableEntity, "Couldn't read the statement's rows, try again.")
	}
	imp.RowCount = len(imp.Mapping.DataRows(recs))
	if imp.RowCount > csvimport.MaxRows {
		return resperr.New(http.StatusUnprocessableEntity, fmt.Sprintf("The statement has more than %d rows, split it.", csvimport.MaxRows))
	}
	if err := h.repo.CreateCSVImport(ctx, imp); err != nil {
		return err
	}
	w.Header().Set("HX-Redirect", "/import/"+imp.ID)
	return nil
}

// readStatement has the AI read the PDF within the tenant's monthly limit,
// keeping the call for the usage: its rows as a CSV, and what was found off.
func (h *Handler) readStatement(ctx context.Context, tenantID string, acc *models.Account, pdf []byte) ([]byte, string, error) {
	l := logger.FromCtx(ctx)
	left, limit, err := h.repo.AICallsLeft(ctx, tenantID, models.ScanStatement, h.ocrCfg.StatementImportsPerMonth, time.Now())
	if err != nil {
		return nil, "", err
	}
	if left <= 0 {
		return nil, "", resperr.New(http.StatusTooManyRequests, fmt.Sprintf("You've had %d PDF statements read this month, the most you can. It starts again on the 1st.", limit))
	}

	readCtx, cancel := context.WithTimeout(ctx, statementReadTimeout)
	defer cancel()
	result, usage, readErr := h.statements.Read(readCtx, pdf)
	scan := models.ReceiptScan{
		TenantID:     tenantID,
		Model:        h.statements.Model(),
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		CostMicros:   usage.CostMicros(h.ocrCfg.Prices()),
		OK:           readErr == nil,
		Kind:         models.ScanStatement,
	}
	if readErr != nil {
		scan.Error = readErr.Error()
	}
	if err := h.repo.RecordReceiptScan(ctx, scan); err != nil {
		l.Error("recording a statement read", zap.Error(err))
	}
	switch {
	case errors.Is(readErr, statementocr.ErrUnreadable):
		l.Info("unreadable statement", zap.Error(readErr))
		return nil, "", resperr.New(http.StatusUnprocessableEntity, "Couldn't read a bank statement's transactions in the PDF.")
	case errors.Is(readErr, statementocr.ErrTooLong):
		return nil, "", resperr.New(http.StatusUnprocessableEntity, "The statement is too long to read at once, split it in a few PDFs.")
	case readErr != nil:
		l.Error("reading a statement", zap.Error(readErr))
		return nil, "", resperr.New(http.StatusBadGateway, "Couldn't read the statement right now, try again in a bit.")
	}

	// checked before merging the fees (which keeps the sums anyway)
	warning := statementWarning(result, acc)
	merge := result.MergeFxFees(acc.FxFeePercent)
	warning = joinWarnings(warning, feeWarning(merge, acc))
	content, err := result.ToCSV()
	if err != nil {
		return nil, "", err
	}
	l.Info("read a statement", zap.Int("rows", len(result.Rows)), zap.String("kind", string(result.Kind)),
		zap.Int("fees_merged", merge.Merged), zap.Int("fees_unmatched", merge.Unmatched))
	return content, warning, nil
}

// statementWarning is what reading a statement found off, "" when nothing:
// rows not adding up to the closing balance (some possibly missed), or a
// statement of another currency or account.
func statementWarning(res *statementocr.Result, acc *models.Account) string {
	var w []string
	if ok, diff := res.Check(); !ok {
		w = append(w, fmt.Sprintf("The rows read don't add up to the statement's closing balance (off by %s): some may be missing or misread, check them against the PDF.", formatters.FormatAmount(diff, "")))
	}
	if cur := strings.ToUpper(strings.TrimSpace(res.Currency)); cur != "" && acc.Currency.String != "" && !strings.EqualFold(cur, acc.Currency.String) {
		w = append(w, fmt.Sprintf("The statement is in %s, %s in %s.", cur, acc.Name, acc.Currency.String))
	}
	if hint := hintDigits(res.AccountHint); len(hint) >= 4 {
		if own := accountDigits(acc); len(own) > 0 && !slices.ContainsFunc(own, func(d string) bool { return strings.HasSuffix(d, hint[len(hint)-4:]) }) {
			w = append(w, fmt.Sprintf("The statement's account ends in %s, it may not be %s's.", hint[len(hint)-4:], acc.Name))
		}
	}
	return strings.Join(w, " ")
}

// feeWarning tells the fee lines left as rows of their own, and the fee
// told from the lines when the account has none set.
func feeWarning(m statementocr.FxFeeMerge, acc *models.Account) string {
	var w []string
	if m.Unmatched > 0 {
		w = append(w, fmt.Sprintf("%d foreign exchange fee lines couldn't be matched to their purchase: they're kept as their own rows.", m.Unmatched))
	}
	if m.Inferred && m.Merged > 0 {
		w = append(w, fmt.Sprintf("The foreign exchange fees look like %s%% of the purchases: set it as %s's foreign currency fee so purchases from SMS include it too.",
			formatters.FormatAmount(m.Percent, ""), acc.Name))
	}
	return strings.Join(w, " ")
}

func joinWarnings(ws ...string) string {
	var res []string
	for _, w := range ws {
		if w != "" {
			res = append(res, w)
		}
	}
	return strings.Join(res, " ")
}

// hintDigits are the digits of a printed account / card number or IBAN.
func hintDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// accountDigits are the digits of the account's card number and IBAN, the
// ones it has.
func accountDigits(acc *models.Account) []string {
	var all []string
	for _, s := range []string{acc.CardNumber, acc.Iban} {
		if d := hintDigits(s); d != "" {
			all = append(all, d)
		}
	}
	return all
}
