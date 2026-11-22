package repository

import (
	"context"
	"database/sql"
	"strings"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// itemResolver links the item lines an owner's write saves to the catalog
// (models.Item): the item picked in the form when it still exists, else the
// one its name matches (models.MatchItem, the owner's merchant's first),
// else a new one named after it. The catalog is read once per write.
type itemResolver struct {
	qtx    *queries.Queries
	owner  itemOwner
	loaded bool
	cands  []models.ItemCandidate
}

func (r *itemResolver) load(ctx context.Context) error {
	if r.loaded {
		return nil
	}
	var cp sql.NullString
	var err error
	if r.owner.txID != "" {
		cp, err = r.qtx.GetTransactionCounterpartyID(ctx, r.owner.txID)
	} else {
		cp, err = r.qtx.GetRecurrentTxCounterpartyID(ctx, r.owner.recurrentID)
	}
	if err != nil && err != sql.ErrNoRows {
		logger.FromCtx(ctx).Error("querying the lines' counterparty", zap.Error(err))
		return err
	}
	rows, err := r.qtx.GetItemCandidates(ctx, cp)
	if err != nil {
		logger.FromCtx(ctx).Error("querying the items to match", zap.Error(err))
		return err
	}
	r.cands = make([]models.ItemCandidate, len(rows))
	for i, row := range rows {
		aliases, _ := unmarshalStrings(row.Aliases)
		r.cands[i] = models.ItemCandidate{
			ID: row.ID, NameKey: row.NameKey, Aliases: aliases,
			AtMerchant: row.AtMerchant, CreatedAt: row.CreatedAt,
		}
	}
	r.loaded = true
	return nil
}

func (r *itemResolver) resolve(ctx context.Context, it *models.TxItem) error {
	if it.Kind.IsCharge() {
		it.ItemID = ""
		return nil
	}
	name := strings.TrimSpace(it.Name)
	if models.ItemKey(name) == "" {
		it.ItemID = ""
		return nil
	}
	if err := r.load(ctx); err != nil {
		return err
	}
	if it.ItemID != "" {
		for _, c := range r.cands {
			if c.ID == it.ItemID {
				return nil
			}
		}
		it.ItemID = ""
	}
	if c := models.MatchItem(name, r.cands); c != nil {
		it.ItemID = c.ID
		return nil
	}

	id, key := idgen.New(), models.ItemKey(name)
	if err := r.qtx.CreateItem(ctx, queries.CreateItemParams{
		ID: id, Name: name, NameKey: key, Aliases: []byte("[]"),
	}); err != nil {
		logger.FromCtx(ctx).Error("creating an item", zap.Error(err))
		return err
	}
	r.cands = append(r.cands, models.ItemCandidate{ID: id, NameKey: key, AtMerchant: true})
	it.ItemID = id
	return nil
}

// linkOwnerItemsWithTxn makes the owner's counterparty one selling its
// lines' items.
func linkOwnerItemsWithTxn(ctx context.Context, qtx *queries.Queries, owner itemOwner) error {
	var err error
	if owner.txID != "" {
		err = qtx.LinkCounterpartyItemsOfTx(ctx, owner.txID)
	} else {
		err = qtx.LinkCounterpartyItemsOfRecurrent(ctx, owner.recurrentID)
	}
	if err != nil {
		logger.FromCtx(ctx).Error("linking the merchant's items", zap.Error(err))
	}
	return err
}
