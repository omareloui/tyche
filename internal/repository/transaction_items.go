package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// receiptOrphanAge is how long a scanned receipt waits for its transaction
// to be saved before the daily upkeep deletes it.
const receiptOrphanAge = 24 * time.Hour

// ErrItemNotFound is a line that isn't the transaction's.
var ErrItemNotFound = errors.New("item not found")

func nullStrings(ids []string) []sql.NullString {
	out := make([]sql.NullString, len(ids))
	for i, id := range ids {
		out[i] = sql.NullString{String: id, Valid: true}
	}
	return out
}

func dbItemToTxItem(it queries.TransactionItem) models.TxItem {
	item := models.TxItem{
		ID:        it.ID,
		Kind:      models.TxItemKindOf(it.Kind),
		Name:      it.Name,
		Quantity:  it.Quantity,
		UnitPrice: it.UnitPrice,
		Total:     it.Total,
		Included:  it.Included,
		// SharedWith is only loaded for a split (loadTxItemShares).
		SharedByAll: it.SharedByAll,
	}
	if it.Rate.Valid {
		r := it.Rate.Float64
		item.Rate = &r
	}
	return item
}

// itemOwner is whose lines they are: a transaction's or a recurrent one's.
type itemOwner struct {
	txID, recurrentID string
}

// replaceItemsWithTxn writes items over the owner's lines, in order. A line
// kept (same id) without shares of its own keeps the ones it had in its
// transaction's split, as the forms don't send them.
func replaceItemsWithTxn(ctx context.Context, qtx *queries.Queries, owner itemOwner, items []models.TxItem) error {
	var err error
	if owner.txID != "" {
		if err := keepItemSharesWithTxn(ctx, qtx, owner.txID, items); err != nil {
			return err
		}
		err = qtx.DeleteTransactionItems(ctx, sql.NullString{String: owner.txID, Valid: true})
	} else {
		err = qtx.DeleteRecurrentTxItems(ctx, sql.NullString{String: owner.recurrentID, Valid: true})
	}
	if err != nil {
		logger.FromCtx(ctx).Error("deleting items", zap.Error(err), zap.Any("owner", owner))
		return err
	}
	for i := range items {
		if err := createItemWithTxn(ctx, qtx, owner, &items[i], i); err != nil {
			return err
		}
	}
	return nil
}

func createItemWithTxn(ctx context.Context, qtx *queries.Queries, owner itemOwner, it *models.TxItem, position int) error {
	if it.ID == "" {
		it.ID = idgen.New()
	}
	var rate sql.NullFloat64
	if it.Rate != nil {
		rate = sql.NullFloat64{Float64: *it.Rate, Valid: true}
	}
	if err := qtx.CreateTransactionItem(ctx, queries.CreateTransactionItemParams{
		ID:            it.ID,
		TransactionID: sql.NullString{String: owner.txID, Valid: owner.txID != ""},
		RecurrentID:   sql.NullString{String: owner.recurrentID, Valid: owner.recurrentID != ""},
		Kind:          int64(it.Kind),
		Name:          it.Name,
		Quantity:      it.Quantity,
		UnitPrice:     it.UnitPrice,
		Total:         it.Total,
		Rate:          rate,
		Included:      it.Included,
		SharedByAll:   it.SharedByAll && owner.txID != "",
		Position:      int64(position),
	}); err != nil {
		logger.FromCtx(ctx).Error("creating item", zap.Error(err), zap.Any("owner", owner))
		return err
	}
	if owner.txID == "" {
		return nil
	}
	return createItemSharesWithTxn(ctx, qtx, it.ID, it.SharedWith)
}

// writeTxExtrasWithTxn writes a saved transaction's lines (when set: nil
// leaves them as they are, e.g. a bulk edit) and links its scanned receipt.
func writeTxExtrasWithTxn(ctx context.Context, qtx *queries.Queries, tx *models.Transaction, created bool) error {
	if tx.Items != nil && (!created || len(tx.Items) > 0) {
		if err := replaceItemsWithTxn(ctx, qtx, itemOwner{txID: tx.ID}, tx.Items); err != nil {
			return err
		}
	}
	if tx.ReceiptID != "" {
		return linkReceiptWithTxn(ctx, qtx, tx.ID, tx.ReceiptID)
	}
	return nil
}

// linkReceiptWithTxn makes the receipt the transaction's, replacing the one
// it had. A receipt already another transaction's is left alone.
func linkReceiptWithTxn(ctx context.Context, qtx *queries.Queries, txID, receiptID string) error {
	l := logger.FromCtx(ctx)
	n, err := qtx.LinkReceipt(ctx, queries.LinkReceiptParams{
		TransactionID: sql.NullString{String: txID, Valid: true},
		ID:            receiptID,
	})
	if err != nil {
		l.Error("linking receipt", zap.Error(err), zap.String("receipt_id", receiptID))
		return err
	}
	if n == 0 {
		l.Warn("receipt to link not found", zap.String("receipt_id", receiptID))
		return nil
	}
	if err := qtx.UnlinkTransactionReceipts(ctx, queries.UnlinkTransactionReceiptsParams{
		TransactionID: sql.NullString{String: txID, Valid: true},
		KeepID:        receiptID,
	}); err != nil {
		l.Error("dropping the old receipt", zap.Error(err), zap.String("transaction_id", txID))
		return err
	}
	return nil
}

// loadTxItems sets the lines and the receipt of each transaction.
func loadTxItems(ctx context.Context, q *queries.Queries, txs []models.Transaction) error {
	if len(txs) == 0 {
		return nil
	}
	ids := make([]string, len(txs))
	at := make(map[string]int, len(txs))
	for i := range txs {
		ids[i] = txs[i].ID
		at[txs[i].ID] = i
	}
	rows, err := q.GetTransactionItemsOfTxs(ctx, nullStrings(ids))
	if err != nil {
		logger.FromCtx(ctx).Error("querying transaction items", zap.Error(err))
		return err
	}
	for _, row := range rows {
		if i, ok := at[row.TransactionID.String]; ok {
			txs[i].Items = append(txs[i].Items, dbItemToTxItem(row))
			txs[i].ItemsCount++
		}
	}
	return loadReceipts(ctx, q, txs, ids, at)
}

// loadTxItemCounts sets how many lines each transaction has and its receipt,
// for lists.
func loadTxItemCounts(ctx context.Context, q *queries.Queries, lists ...[]models.Transaction) error {
	var ids []string
	type ref struct{ list, i int }
	at := map[string]ref{}
	for li, txs := range lists {
		for i := range txs {
			ids = append(ids, txs[i].ID)
			at[txs[i].ID] = ref{li, i}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.CountTransactionItemsOfTxs(ctx, nullStrings(ids))
	if err != nil {
		logger.FromCtx(ctx).Error("counting transaction items", zap.Error(err))
		return err
	}
	for _, row := range rows {
		if r, ok := at[row.TransactionID.String]; ok {
			lists[r.list][r.i].ItemsCount = int(row.N)
		}
	}
	recs, err := q.GetReceiptsOfTxs(ctx, nullStrings(ids))
	if err != nil {
		logger.FromCtx(ctx).Error("querying receipts", zap.Error(err))
		return err
	}
	for _, rec := range recs {
		if r, ok := at[rec.TransactionID.String]; ok {
			lists[r.list][r.i].Receipt = &models.Receipt{ID: rec.ID, ContentType: rec.ContentType, Hash: rec.Hash, Size: rec.Size}
		}
	}
	return nil
}

func loadReceipts(ctx context.Context, q *queries.Queries, txs []models.Transaction, ids []string, at map[string]int) error {
	recs, err := q.GetReceiptsOfTxs(ctx, nullStrings(ids))
	if err != nil {
		logger.FromCtx(ctx).Error("querying receipts", zap.Error(err))
		return err
	}
	for _, rec := range recs {
		if i, ok := at[rec.TransactionID.String]; ok {
			txs[i].Receipt = &models.Receipt{ID: rec.ID, ContentType: rec.ContentType, Hash: rec.Hash, Size: rec.Size}
		}
	}
	return nil
}

// loadRecurrentTxItems sets the lines of each recurrent transaction.
func loadRecurrentTxItems(ctx context.Context, q *queries.Queries, rts []models.RecurrentTx) error {
	if len(rts) == 0 {
		return nil
	}
	at := make(map[string]int, len(rts))
	for i := range rts {
		at[rts[i].ID] = i
	}
	rows, err := q.GetRecurrentTxItems(ctx)
	if err != nil {
		logger.FromCtx(ctx).Error("querying recurrent tx items", zap.Error(err))
		return err
	}
	for _, row := range rows {
		if i, ok := at[row.RecurrentID.String]; ok {
			rts[i].Items = append(rts[i].Items, dbItemToTxItem(row))
		}
	}
	return nil
}

// recurrentTxItemsOf is the recurrent transaction's lines, an empty (not nil)
// list when none.
func recurrentTxItemsOf(ctx context.Context, q *queries.Queries, id string) ([]models.TxItem, error) {
	rows, err := q.GetRecurrentTxItemsOf(ctx, sql.NullString{String: id, Valid: true})
	if err != nil {
		logger.FromCtx(ctx).Error("querying recurrent tx items", zap.Error(err), zap.String("id", id))
		return nil, err
	}
	items := make([]models.TxItem, len(rows))
	for i, row := range rows {
		items[i] = dbItemToTxItem(row)
	}
	return items, nil
}

// writeRecurrentItemsWithTxn writes the recurrent transaction's lines and,
// on an edit, puts them on its scheduled occurrences not edited on their own.
func writeRecurrentItemsWithTxn(ctx context.Context, qtx *queries.Queries, rt *models.RecurrentTx, toOccurrences bool) error {
	if rt.Items == nil {
		return nil
	}
	if err := replaceItemsWithTxn(ctx, qtx, itemOwner{recurrentID: rt.ID}, rt.Items); err != nil {
		return err
	}
	if !toOccurrences {
		return nil
	}
	ids, err := qtx.GetEditableOccurrenceIDs(ctx, sql.NullString{String: rt.ID, Valid: true})
	if err != nil {
		logger.FromCtx(ctx).Error("querying the occurrences to edit", zap.Error(err), zap.String("id", rt.ID))
		return err
	}
	for _, id := range ids {
		if err := replaceItemsWithTxn(ctx, qtx, itemOwner{txID: id}, models.CopyTxItems(rt.Items)); err != nil {
			return err
		}
	}
	return nil
}

// AddTxItem appends a line to the transaction, an occurrence then keeping
// its lines when its recurrent transaction is edited.
func (r *Repository) AddTxItem(ctx context.Context, txID string, it *models.TxItem) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		items, err := qtx.GetTransactionItemsOf(ctx, sql.NullString{String: txID, Valid: true})
		if err != nil {
			return err
		}
		if err := createItemWithTxn(ctx, qtx, itemOwner{txID: txID}, it, len(items)); err != nil {
			return err
		}
		return qtx.MarkTransactionOverridden(ctx, txID)
	})
}

// DeleteTxItem deletes one of the transaction's lines.
func (r *Repository) DeleteTxItem(ctx context.Context, txID, itemID string) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		n, err := qtx.DeleteTransactionItem(ctx, queries.DeleteTransactionItemParams{
			ID: itemID, TransactionID: sql.NullString{String: txID, Valid: true},
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrItemNotFound
		}
		return qtx.MarkTransactionOverridden(ctx, txID)
	})
}

// SetTxItems replaces the transaction's lines, e.g. with a scanned receipt's,
// linking the receipt when receiptID is set.
func (r *Repository) SetTxItems(ctx context.Context, txID string, items []models.TxItem, receiptID string) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if items != nil {
			if err := replaceItemsWithTxn(ctx, qtx, itemOwner{txID: txID}, items); err != nil {
				return err
			}
			if err := qtx.MarkTransactionOverridden(ctx, txID); err != nil {
				return err
			}
		}
		if receiptID != "" {
			return linkReceiptWithTxn(ctx, qtx, txID, receiptID)
		}
		return nil
	})
}

// GetTxItems is the transaction's lines.
func (r *Repository) GetTxItems(ctx context.Context, txID string) ([]models.TxItem, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetTransactionItemsOf(ctx, sql.NullString{String: txID, Valid: true})
	if err != nil {
		return nil, err
	}
	items := make([]models.TxItem, len(rows))
	for i, row := range rows {
		items[i] = dbItemToTxItem(row)
	}
	return items, nil
}

// AddRecurrentTxItem appends a line to the recurrent transaction and its
// scheduled occurrences not edited on their own.
func (r *Repository) AddRecurrentTxItem(ctx context.Context, rt *models.RecurrentTx, it *models.TxItem) error {
	rt.Items = append(rt.Items, *it)
	return r.transact(ctx, func(qtx *queries.Queries) error {
		return writeRecurrentItemsWithTxn(ctx, qtx, rt, true)
	})
}

// DeleteRecurrentTxItem deletes a line of the recurrent transaction, and its
// copies on the scheduled occurrences not edited on their own.
func (r *Repository) DeleteRecurrentTxItem(ctx context.Context, rt *models.RecurrentTx, itemID string) error {
	kept := make([]models.TxItem, 0, len(rt.Items))
	for _, it := range rt.Items {
		if it.ID != itemID {
			kept = append(kept, it)
		}
	}
	if len(kept) == len(rt.Items) {
		return ErrItemNotFound
	}
	rt.Items = kept
	return r.transact(ctx, func(qtx *queries.Queries) error {
		return writeRecurrentItemsWithTxn(ctx, qtx, rt, true)
	})
}

// ScannedReceipt is a receipt image as stored when scanned.
type ScannedReceipt struct {
	Data        []byte
	ContentType string
	Hash        string
	Merchant    string
	OCRJSON     string
}

// CreateReceipt stores a scanned receipt, not linked to any transaction yet.
func (r *Repository) CreateReceipt(ctx context.Context, rec *ScannedReceipt) (string, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return "", err
	}
	id := idgen.New()
	if err := q.CreateReceipt(ctx, queries.CreateReceiptParams{
		ID:          id,
		Data:        rec.Data,
		ContentType: rec.ContentType,
		Hash:        rec.Hash,
		Merchant:    rec.Merchant,
		OcrJson:     rec.OCRJSON,
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		logger.FromCtx(ctx).Error("creating receipt", zap.Error(err))
		return "", err
	}
	return id, nil
}

// FindReceiptByHash is the receipt of the same image already read (its
// sha256), nil when none: an unlinked one first, else the latest.
func (r *Repository) FindReceiptByHash(ctx context.Context, hash string) (*queries.Receipt, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rec, err := q.GetReceiptByHash(ctx, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// GetReceipt is a receipt with its image.
func (r *Repository) GetReceipt(ctx context.Context, id string) (*queries.Receipt, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rec, err := q.GetReceipt(ctx, id)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// DeleteReceipt deletes a receipt, its transaction keeping its lines.
func (r *Repository) DeleteReceipt(ctx context.Context, id string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	n, err := q.DeleteReceipt(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// purgeOrphanReceipts deletes the scanned receipts whose transaction was
// never saved.
func purgeOrphanReceipts(ctx context.Context, q *queries.Queries, now time.Time) {
	n, err := q.PurgeOrphanReceipts(ctx, now.Add(-receiptOrphanAge).UTC())
	if err != nil {
		logger.FromCtx(ctx).Error("purging orphan receipts", zap.Error(err))
		return
	}
	if n > 0 {
		logger.FromCtx(ctx).Info("purged orphan receipts", zap.Int64("count", n))
	}
}
