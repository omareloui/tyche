package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/csvimport"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
)

// csvImportPendingAge is how long a CSV file uploaded but not imported is
// kept.
const csvImportPendingAge = 24 * time.Hour

// CreateCSVImport stores an uploaded CSV file, pending until imported.
func (r *Repository) CreateCSVImport(ctx context.Context, imp *csvimport.Import) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	mapping, err := json.Marshal(imp.Mapping)
	if err != nil {
		return err
	}
	if imp.ID == "" {
		imp.ID = idgen.New()
	}
	if err := q.CreateCSVImport(ctx, queries.CreateCSVImportParams{
		ID: imp.ID, AccountID: imp.AccountID, FileName: imp.FileName, Content: imp.Content,
		Mapping: mapping, RowCount: int64(imp.RowCount),
		SourceKind: string(csvImportSource(imp.Source)),
		SourceHash: sql.NullString{String: imp.SourceHash, Valid: imp.SourceHash != ""},
		Warning:    imp.Warning,
	}); err != nil {
		logger.FromCtx(ctx).Error("creating csv import", zap.Error(err))
		return err
	}
	logger.FromCtx(ctx).Info("created csv import", zap.String("id", imp.ID), zap.Int("rows", imp.RowCount))
	return nil
}

// GetCSVImport is the import id, sql.ErrNoRows when there's none.
func (r *Repository) GetCSVImport(ctx context.Context, id string) (*csvimport.Import, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := q.GetCSVImport(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbCSVImport(ctx, &row), nil
}

// GetCSVImportBySourceHash is the latest import of the PDF with this
// sha256 still holding its rows, nil when there's none.
func (r *Repository) GetCSVImportBySourceHash(ctx context.Context, hash string) (*csvimport.Import, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := q.GetCSVImportBySourceHash(ctx, sql.NullString{String: hash, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		logger.FromCtx(ctx).Error("getting a csv import by its source", zap.Error(err))
		return nil, err
	}
	return dbCSVImport(ctx, &row), nil
}

// GetAccountFirstTrackedDay is the day of the account's first fulfilled
// transaction moving its balance (past data aside), zero when there's none.
func (r *Repository) GetAccountFirstTrackedDay(ctx context.Context, accountID string) (time.Time, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return time.Time{}, err
	}
	day, err := q.GetAccountFirstTrackedDay(ctx, accountID)
	if err != nil {
		logger.FromCtx(ctx).Error("getting an account's first tracked day", zap.Error(err), zap.String("id", accountID))
		return time.Time{}, err
	}
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return time.Time{}, nil
	}
	return t, nil
}

func dbCSVImport(ctx context.Context, row *queries.CsvImport) *csvimport.Import {
	imp := &csvimport.Import{
		ID: row.ID, AccountID: row.AccountID, FileName: row.FileName, Content: row.Content,
		Source: csvImportSource(csvimport.Source(row.SourceKind)), SourceHash: row.SourceHash.String, Warning: row.Warning,
		Kept: row.Content != nil,
		Status: csvimport.ImportStatus(row.Status), RowCount: int(row.RowCount),
		ImportedCount: int(row.ImportedCount), CreatedAt: row.CreatedAt, ImportedAt: row.ImportedAt.Time,
		Mapping: csvimport.NewMapping(),
	}
	if err := json.Unmarshal(row.Mapping, &imp.Mapping); err != nil {
		logger.FromCtx(ctx).Warn("reading a csv import's mapping", zap.Error(err), zap.String("id", row.ID))
	}
	return imp
}

func csvImportSource(s csvimport.Source) csvimport.Source {
	if s == csvimport.SourcePDF {
		return s
	}
	return csvimport.SourceCSV
}

// ListCSVImports are the files uploaded (imported or pending), the latest
// first, at most limit (-1 for all).
func (r *Repository) ListCSVImports(ctx context.Context, limit int) ([]csvimport.Import, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCSVImports(ctx, int64(limit))
	if err != nil {
		logger.FromCtx(ctx).Error("listing csv imports", zap.Error(err))
		return nil, err
	}
	res := make([]csvimport.Import, len(rows))
	for i, row := range rows {
		res[i] = csvimport.Import{
			ID: row.ID, AccountID: row.AccountID, AccountName: row.AccountName, FileName: row.FileName,
			Source: csvImportSource(csvimport.Source(row.SourceKind)), Kept: row.Kept,
			Status: csvimport.ImportStatus(row.Status), RowCount: int(row.RowCount),
			ImportedCount: int(row.ImportedCount), CreatedAt: row.CreatedAt, ImportedAt: row.ImportedAt.Time,
		}
	}
	return res, nil
}

// SetCSVImportMapping keeps the mapping picked for a pending import.
func (r *Repository) SetCSVImportMapping(ctx context.Context, id string, m *csvimport.Mapping) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	mapping, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return q.UpdateCSVImportMapping(ctx, queries.UpdateCSVImportMappingParams{ID: id, Mapping: mapping})
}

// GetCSVMapping is the mapping saved for the account's files with this
// header, nil when there's none.
func (r *Repository) GetCSVMapping(ctx context.Context, accountID, headerSig string) (*csvimport.Mapping, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := q.GetCSVMapping(ctx, queries.GetCSVMappingParams{AccountID: accountID, HeaderSig: headerSig})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m := csvimport.NewMapping()
	if err := json.Unmarshal(raw, &m); err != nil {
		logger.FromCtx(ctx).Warn("reading a saved csv mapping", zap.Error(err))
		return nil, nil
	}
	return &m, nil
}

// GetCSVDupCandidates are the fulfilled transactions moving the account
// between since and until (days, inclusive), as rows may already be them.
func (r *Repository) GetCSVDupCandidates(ctx context.Context, accountID string, since, until time.Time) ([]csvimport.Existing, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetCSVDupCandidates(ctx, queries.GetCSVDupCandidatesParams{
		AccountID: sql.NullString{String: accountID, Valid: true},
		Since:     since.AddDate(0, 0, -csvimport.MaybeDays-1),
		Until:     until.AddDate(0, 0, csvimport.MaybeDays+2),
	})
	if err != nil {
		logger.FromCtx(ctx).Error("querying csv duplicate candidates", zap.Error(err))
		return nil, err
	}
	res := make([]csvimport.Existing, len(rows))
	for i, row := range rows {
		y, m, d := row.ScheduledTo.Date()
		res[i] = csvimport.Existing{
			ID:      row.ID,
			Day:     time.Date(y, m, d, 0, 0, 0, 0, time.UTC),
			Out:     row.Side == 1,
			Moved:   row.Moved,
			Hash:    row.ImportHash,
			Foreign: row.PaidForeign,
		}
	}
	return res, nil
}


// ImportedTx is a transaction made from a CSV row, with the row's hash.
type ImportedTx struct {
	Tx   *models.Transaction
	Hash string
	// Target, when set, is the transaction the row updates (its amount, fee
	// and what it was paid in, models.MergeImported) instead of a new one.
	Target *models.Transaction
}

// ImportCSV saves the transactions of an import in one database transaction,
// each like an SMS's (paying the recurrent occurrence it likely is, paired
// with the other leg of a transfer), marks the import done and, with
// headerSig, saves its mapping for the account's next files.
func (r *Repository) ImportCSV(ctx context.Context, imp *csvimport.Import, txs []ImportedTx, saveMapping bool, headerSig string) error {
	l := logger.FromCtx(ctx)
	mapping, err := json.Marshal(imp.Mapping)
	if err != nil {
		return err
	}
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if _, err := qtx.GetCSVImport(ctx, imp.ID); err != nil {
			return err
		}
		n := 0
		for _, it := range txs {
			// imported meanwhile (the form sent twice): not again
			if it.Hash != "" {
				found, err := qtx.ImportHashExists(ctx, queries.ImportHashExistsParams{
					ImportHash: sql.NullString{String: it.Hash, Valid: true}, AccountID: it.Tx.Account.ID,
				})
				if err != nil {
					return err
				}
				if found {
					continue
				}
			}
			n++
			if it.Target != nil {
				if err := r.applyImportedWithTxn(ctx, qtx, it.Target, it.Tx, imp.ID, it.Hash); err != nil {
					return err
				}
				continue
			}
			if err := r.createIngestedTransactionWithTxn(ctx, qtx, it.Tx); err != nil {
				return err
			}
			if err := qtx.SetTransactionImport(ctx, queries.SetTransactionImportParams{
				ID:         it.Tx.ID,
				ImportID:   sql.NullString{String: imp.ID, Valid: true},
				ImportHash: sql.NullString{String: it.Hash, Valid: it.Hash != ""},
			}); err != nil {
				l.Error("linking a transaction to its csv import", zap.Error(err), zap.String("id", it.Tx.ID))
				return err
			}
		}
		if err := qtx.MarkCSVImportImported(ctx, queries.MarkCSVImportImportedParams{
			ID: imp.ID, Mapping: mapping, ImportedCount: int64(n),
		}); err != nil {
			l.Error("marking a csv import imported", zap.Error(err), zap.String("id", imp.ID))
			return err
		}
		if saveMapping {
			if err := qtx.SaveCSVMapping(ctx, queries.SaveCSVMappingParams{
				ID: idgen.New(), AccountID: imp.AccountID, HeaderSig: headerSig, Mapping: mapping,
			}); err != nil {
				l.Error("saving a csv mapping", zap.Error(err))
				return err
			}
		}
		l.Info("imported csv", zap.String("id", imp.ID), zap.Int("count", n))
		return nil
	})
}

// purgePendingCSVImports deletes the files uploaded but not imported after
// csvImportPendingAge, keeping the latest read of each PDF statement (the
// cache of its rows).
func purgePendingCSVImports(ctx context.Context, q *queries.Queries, now time.Time) {
	n, err := q.PurgePendingCSVImports(ctx, now.Add(-csvImportPendingAge).UTC())
	if err != nil {
		logger.FromCtx(ctx).Error("purging pending csv imports", zap.Error(err))
		return
	}
	if n > 0 {
		logger.FromCtx(ctx).Info("purged pending csv imports", zap.Int64("count", n))
	}
}
