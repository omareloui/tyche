package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/csvimport"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
)

// csvImportsListed is how many past imports the import page lists.
const csvImportsListed = 10

var errCSVImportNotFound = resperr.New(http.StatusNotFound, "This import isn't there anymore, upload the file again.")

// CSVImportPage is where a bank's CSV export is uploaded into an account.
func (h *Handler) CSVImportPage(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	accs, err := h.repo.GetAccounts(r.Context())
	if err != nil {
		return nil, err
	}
	past, err := h.repo.ListCSVImports(r.Context(), csvImportsListed)
	if err != nil {
		return nil, err
	}
	return views.CSVImportPage(getClaims(r.Context()), b, views.CSVImportPageData{
		Accounts:  views.CSVImportAccounts(accs),
		AccountID: r.URL.Query().Get("account"),
		Past:      past,
	}), nil
}

// UploadCSV stores an uploaded CSV file as a pending import with its mapping
// (the one saved for the account's files with its header, else guessed), and
// goes on to its mapping and preview.
func (h *Handler) UploadCSV(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	r.Body = http.MaxBytesReader(w, r.Body, csvimport.MaxBytes+1<<20)
	if err := r.ParseMultipartForm(csvimport.MaxBytes + 1<<20); err != nil {
		return nil, uploadErr(err)
	}
	defer r.MultipartForm.RemoveAll()

	acc, err := h.csvImportAccount(r.Context(), r.FormValue("account_id"))
	if err != nil {
		return nil, err
	}

	f, fh, err := r.FormFile("file")
	if err != nil {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Choose a CSV file.")
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, csvimport.MaxBytes+1))
	if err != nil {
		return nil, uploadErr(err)
	}
	if len(content) > csvimport.MaxBytes {
		return nil, resperr.New(http.StatusRequestEntityTooLarge, fmt.Sprintf("The file is over %d MB, split it.", csvimport.MaxBytes>>20))
	}

	recs, delim, err := csvimport.Read(content, "")
	if err != nil {
		return nil, resperr.New(http.StatusUnprocessableEntity, err.Error())
	}
	m := csvimport.Guess(recs, delim)
	if saved, err := h.repo.GetCSVMapping(r.Context(), acc.ID, csvimport.HeaderSig(m.Header(recs))); err != nil {
		return nil, err
	} else if saved != nil && saved.Validate(saved.Columns(recs)) == nil {
		m = *saved
	}
	rows := len(m.DataRows(recs))
	if rows > csvimport.MaxRows {
		return nil, resperr.New(http.StatusUnprocessableEntity, fmt.Sprintf("The file has more than %d rows, split it.", csvimport.MaxRows))
	}

	imp := &csvimport.Import{AccountID: acc.ID, FileName: fileBaseName(fh.Filename), Content: content, Mapping: m, RowCount: rows}
	if err := h.repo.CreateCSVImport(r.Context(), imp); err != nil {
		return nil, err
	}
	w.Header().Set("HX-Redirect", "/import/"+imp.ID)
	return nil, nil
}

// CSVImport is a pending import's mapping and preview.
func (h *Handler) CSVImport(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	imp, acc, err := h.pendingCSVImport(r)
	if err != nil {
		return nil, err
	}
	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	data, err := h.csvImportData(r.Context(), imp, acc, imp.Mapping)
	if err != nil {
		return nil, err
	}
	return views.CSVImport(getClaims(r.Context()), b, data), nil
}

// PreviewCSVImport answers the mapping and preview of a changed mapping.
func (h *Handler) PreviewCSVImport(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	imp, acc, err := h.pendingCSVImport(r)
	if err != nil {
		return nil, err
	}
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "invalid form")
	}
	m := h.csvMappingFromForm(imp, r.PostForm)
	data, err := h.csvImportData(r.Context(), imp, acc, m)
	if err != nil {
		return nil, err
	}
	if data.MappingErr == "" {
		if err := h.repo.SetCSVImportMapping(r.Context(), imp.ID, &m); err != nil {
			return nil, err
		}
	}
	return views.CSVImportBody(data), nil
}

// ImportCSV saves the picked rows of a pending import, read again with the
// sent mapping, as transactions to review.
func (h *Handler) ImportCSV(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	imp, acc, err := h.pendingCSVImport(r)
	if err != nil {
		return nil, err
	}
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "invalid form")
	}
	m := h.csvMappingFromForm(imp, r.PostForm)
	data, err := h.csvImportData(r.Context(), imp, acc, m)
	if err != nil {
		return nil, err
	}
	if data.MappingErr != "" {
		return nil, resperr.New(http.StatusUnprocessableEntity, data.MappingErr)
	}

	picked := map[int]bool{}
	for _, v := range r.PostForm["line"] {
		if n, err := strconv.Atoi(v); err == nil {
			picked[n] = true
		}
	}
	var txs []repository.ImportedTx
	for _, pr := range data.Rows {
		if !picked[pr.Row.Line] || pr.Tx == nil || pr.Match.Status == csvimport.RowImported {
			continue
		}
		if err := pr.Tx.SanitizeAndValidate(h.sani, h.valid); err != nil {
			return nil, resperr.New(http.StatusUnprocessableEntity, fmt.Sprintf("Row %d can't be imported.", pr.Row.Line))
		}
		txs = append(txs, repository.ImportedTx{Tx: pr.Tx, Hash: pr.Row.Hash})
	}
	if len(txs) == 0 {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Pick the rows to import.")
	}

	imp.Mapping = m
	save := r.PostForm.Get("save_mapping") != ""
	if err := h.repo.ImportCSV(r.Context(), imp, txs, save, csvimport.HeaderSig(m.Header(data.Records))); err != nil {
		if errors.Is(err, repository.ErrCSVImported) {
			return nil, errCSVImportNotFound
		}
		return nil, err
	}
	w.Header().Set("HX-Redirect", "/transactions?"+url.Values{
		models.TxFilterSource: {string(models.TxSourceImport)},
		models.TxFilterStatus: {string(models.TxStatusReview)},
	}.Encode())
	return nil, nil
}

// pendingCSVImport is the pending import {id} with its account.
func (h *Handler) pendingCSVImport(r *http.Request) (*csvimport.Import, *models.Account, error) {
	imp, err := h.repo.GetCSVImport(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, errCSVImportNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if imp.Status != csvimport.ImportPending {
		return nil, nil, errCSVImportNotFound
	}
	acc, err := h.csvImportAccount(r.Context(), imp.AccountID)
	if err != nil {
		return nil, nil, err
	}
	return imp, acc, nil
}

// csvImportAccount is the account id when a file can be imported into it.
func (h *Handler) csvImportAccount(ctx context.Context, id string) (*models.Account, error) {
	if id == "" {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Pick the account the file is of.")
	}
	acc, err := h.repo.GetAccount(ctx, id)
	if err != nil || !views.CanImportCSVInto(acc) {
		return nil, errAccountNotFound
	}
	return acc, nil
}

// csvImportData reads the import's rows with the mapping m: their status
// (new, imported before, a duplicate of a transaction, or why they can't be
// imported) and the transaction each would be.
func (h *Handler) csvImportData(ctx context.Context, imp *csvimport.Import, acc *models.Account, m csvimport.Mapping) (views.CSVImportData, error) {
	data := views.CSVImportData{Import: imp, Account: acc, Mapping: m}
	recs, delim, err := csvimport.Read(imp.Content, m.Delimiter)
	if err != nil {
		data.MappingErr = err.Error()
		return data, nil
	}
	data.Records = recs
	data.Mapping.Delimiter = delim
	if err := data.Mapping.Validate(data.Mapping.Columns(recs)); err != nil {
		data.MappingErr = err.Error()
		return data, nil
	}

	rows := csvimport.ParseRows(recs, &data.Mapping, acc.ID, h.userToday(ctx))
	var from, to time.Time
	for _, row := range rows {
		if row.Err != "" {
			continue
		}
		if from.IsZero() || row.Date.Before(from) {
			from = row.Date
		}
		if row.Date.After(to) {
			to = row.Date
		}
	}
	var existing []csvimport.Existing
	if !from.IsZero() {
		if existing, err = h.repo.GetCSVDupCandidates(ctx, acc.ID, from, to); err != nil {
			return data, err
		}
	}
	matches := csvimport.MatchDuplicates(rows, existing, &acc.Currency)

	cps, err := h.repo.GetCounterparties(ctx)
	if err != nil {
		return data, err
	}
	loc, err := h.repo.GetTimezone(ctx)
	if err != nil {
		return data, err
	}

	data.Rows = make([]views.CSVPreviewRow, len(rows))
	for i := range rows {
		pr := views.CSVPreviewRow{Row: rows[i], Match: matches[i]}
		if pr.Row.Err == "" {
			tx, err := csvimport.Build(&pr.Row, acc, cps, loc, fxrate.GetFxRate)
			if err != nil {
				logger.FromCtx(ctx).Debug("building a csv row's transaction", zap.Error(err), zap.Int("line", pr.Row.Line))
				pr.Row.Err = "Can't convert " + pr.Row.Currency
				pr.Match = csvimport.Match{Status: csvimport.RowError}
			} else {
				pr.Tx = tx
			}
		}
		data.Rows[i] = pr
	}
	return data, nil
}

// csvDelimiterNames name the separators in forms.
var csvDelimiterNames = map[string]string{"comma": ",", "semicolon": ";", "tab": "\t", "pipe": "|"}

// csvMappingFromForm reads the mapping form. A new separator guesses the
// mapping again, the columns being others.
func (h *Handler) csvMappingFromForm(imp *csvimport.Import, v url.Values) csvimport.Mapping {
	delim := csvDelimiterNames[v.Get("delimiter")]
	if delim != "" && delim != imp.Mapping.Delimiter {
		if recs, _, err := csvimport.Read(imp.Content, delim); err == nil {
			return csvimport.Guess(recs, delim)
		}
	}

	col := func(name string) int {
		n, err := strconv.Atoi(v.Get(name))
		if err != nil || n < 0 {
			return csvimport.None
		}
		return n
	}
	m := csvimport.NewMapping()
	m.Delimiter = imp.Mapping.Delimiter
	m.SkipRows, _ = strconv.Atoi(v.Get("skip_rows"))
	m.HasHeader = v.Get("has_header") != ""
	m.Date = col("date")
	m.DateFormat = v.Get("date_format")
	m.Time = col("time")
	m.Split = v.Get("amount_mode") == "split"
	m.Amount, m.Debit, m.Credit = col("amount"), col("debit"), col("credit")
	m.Negative = csvimport.Sign(v.Get("negative"))
	m.Description = col("description")
	m.Counterparty = col("counterparty")
	m.Currency = col("currency")
	m.DecimalComma = v.Get("decimal_comma") != ""
	return m
}

// fileBaseName is the file's name without its folders, at most 120 bytes.
func fileBaseName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if len(name) > 120 {
		name = strings.ToValidUTF8(name[:120], "")
	}
	return name
}
