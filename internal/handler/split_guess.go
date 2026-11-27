package handler

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/splitguess"
	"github.com/omareloui/tyche/internal/txguess"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// splitGuessTimeout is how long reading a split from a sentence may take.
const splitGuessTimeout = 20 * time.Second

// SetSplitGuess turns describing a split bill in a sentence on (a nil
// guesser keeps it off).
func (h *Handler) SetSplitGuess(g splitguess.Guesser) {
	h.splitGuess = g
}

// splitDescribe is the split panel's "Describe the split" box, nil when it's
// off (no AI on the server).
func (h Handler) splitDescribe(ctx context.Context) (*views.TxSplitDescribe, error) {
	if h.splitGuess == nil {
		return nil, nil
	}
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	left, limit, err := h.repo.AICallsLeft(ctx, tenantID, models.ScanSplit, h.ocrCfg.SplitGuessesPerMonth, time.Now())
	if err != nil {
		return nil, err
	}
	return &views.TxSplitDescribe{Used: limit - left, Limit: limit}, nil
}

// describeSplit reads who had what from the sentence (describe) and sets the
// split: the people named who aren't in it added (found among the user's
// people, else created), the items mentioned shared as said, the others
// left as they were. A sentence that can't be read answers the panel with
// why, the text kept.
func (h Handler) describeSplit(r *http.Request, s *models.TxSplit) (templ.Component, error) {
	ctx := r.Context()
	l := logger.FromCtx(ctx)
	raw := r.FormValue("describe")
	text := splitguess.Clean(raw)

	refuse := func(msg string) (templ.Component, error) {
		return h.splitAnswer(r, s, views.TxSplitMsg{DescribeError: msg, DescribeText: text})
	}
	if h.splitGuess == nil {
		return nil, resperr.New(http.StatusNotFound, "Describing the split is off.")
	}
	if splitguess.TooLong(raw) {
		return refuse(fmt.Sprintf("That's a lot at once: keep it under %d characters.", splitguess.MaxTextLen))
	}
	if text == "" {
		return refuse("Say who had what, like “Sara had the pizza, drinks for everyone”.")
	}

	in := splitguess.In{Text: text}
	for _, it := range s.Tx.Items {
		if !it.Kind.IsCharge() {
			in.Lines = append(in.Lines, splitguess.Line{ID: it.ID, Name: it.Label(), Quantity: it.Quantity})
		}
	}
	if len(in.Lines) == 0 {
		return refuse("Add the bill's items first: the split goes by them.")
	}
	for _, p := range s.People {
		in.People = append(in.People, splitguess.Person{ID: p.ID, Name: p.Name})
	}

	quota, err := h.splitDescribe(ctx)
	if err != nil {
		return nil, err
	}
	if quota.Used >= quota.Limit {
		return refuse(fmt.Sprintf("You've used this month's %d split descriptions. Until the 1st, pick who had each item below.", quota.Limit))
	}
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	gctx, cancel := context.WithTimeout(ctx, splitGuessTimeout)
	defer cancel()
	ans, usage, guessErr := h.splitGuess.Guess(gctx, in)
	scan := models.ReceiptScan{
		TenantID:      tenantID,
		Kind:          models.ScanSplit,
		Model:         h.splitGuess.Model(),
		InputTokens:   usage.InputTokens,
		OutputTokens:  usage.OutputTokens,
		SearchQueries: usage.SearchQueries,
		CostMicros:    usage.CostMicros(h.ocrCfg.Prices()),
		OK:            guessErr == nil,
	}
	if guessErr != nil {
		scan.Error = guessErr.Error()
	}
	if err := h.repo.RecordReceiptScan(ctx, scan); err != nil {
		l.Error("recording a split call", zap.Error(err))
	}
	if guessErr != nil {
		l.Error("reading a split", zap.Error(guessErr))
		return refuse("Couldn't read that just now, give it another try in a moment.")
	}
	out := splitguess.Parse(in, ans)
	if len(out.Shares) == 0 {
		return refuse("Couldn't tell who had which item: name the items, like “Ahmed had the burger”.")
	}

	people := make([]string, 0, len(s.People)+len(out.NewPeople))
	for _, p := range s.People {
		people = append(people, p.ID)
	}
	if len(people)+len(out.NewPeople) > maxSplitPeople {
		return refuse("That's too many people for one bill.")
	}
	newIDs, added, created, err := h.splitNewPeople(ctx, s.People, out.NewPeople)
	if err != nil {
		return nil, err
	}
	byRef := map[string]string{}
	for i, id := range newIDs {
		byRef[splitguess.NewRef(i)] = id
		if !slices.Contains(people, id) {
			people = append(people, id)
		}
	}

	shares := map[string]models.SplitItemShare{}
	charges := map[string]models.ChargeSplitMode{}
	for _, it := range s.Tx.Items {
		if it.Kind.IsCharge() {
			charges[it.ID] = it.SplitMode
			continue
		}
		sh, ok := out.Shares[it.ID]
		if !ok {
			shares[it.ID] = models.SplitItemShare{All: it.SharedByAll, With: it.SharedWith}
			continue
		}
		for i := range sh.With {
			if id, ok := byRef[sh.With[i].Person]; ok {
				sh.With[i].Person = id
			}
		}
		shares[it.ID] = sh
	}

	if err := h.repo.SaveTxSplit(ctx, s.Tx.ID, people, shares, charges); err != nil {
		l.Error("saving split", zap.Error(err))
		return nil, err
	}
	l.Info("described split", zap.Int("items", len(out.Shares)), zap.Int("of", len(in.Lines)), zap.Int("added", len(added)), zap.Int("created", len(created)))

	s, err = h.repo.GetTxSplit(ctx, s.Tx.ID)
	if err != nil {
		return nil, err
	}
	return h.splitAnswer(r, s, views.TxSplitMsg{Notice: views.SplitDescribedNotice(len(out.Shares), len(in.Lines), added, created)})
}

// splitNewPeople are the counterparties of the names said who weren't in
// the split, in order: someone in it after all (a first name of theirs),
// one of the user's people by name, alias or unique first name
// (unarchived), else a new person; added are the names of the ones found
// out of the split, created of the new ones.
func (h Handler) splitNewPeople(ctx context.Context, in []models.SplitPerson, names []string) (ids, added, created []string, err error) {
	if len(names) == 0 {
		return nil, nil, nil, nil
	}
	l := logger.FromCtx(ctx)
	cps, err := h.repo.GetCounterparties(ctx)
	if err != nil {
		l.Error("fetching counterparties", zap.Error(err))
		return nil, nil, nil, err
	}
	var people []txguess.Counterparty
	byID := map[string]models.Counterparty{}
	for _, cp := range cps {
		if cp.IsMerchant {
			continue
		}
		people = append(people, txguess.Counterparty{ID: cp.ID, Name: cp.Name, Aliases: cp.Aliases})
		byID[cp.ID] = cp
	}
	var split []txguess.Counterparty
	for _, p := range in {
		split = append(split, txguess.Counterparty{ID: p.ID, Name: p.Name})
	}
	for _, name := range names {
		if id := txguess.FindCounterparty(name, false, split); id != "" {
			ids = append(ids, id)
			continue
		}
		if id := txguess.FindCounterparty(name, false, people); id != "" {
			cp := byID[id]
			if cp.IsArchived {
				if err := h.repo.SetCounterpartyArchived(ctx, id, false); err != nil {
					l.Error("unarchiving counterparty", zap.Error(err))
					return nil, nil, nil, err
				}
			}
			ids = append(ids, id)
			added = append(added, cp.Name)
			continue
		}
		cp, err := h.findOrCreateCounterparty(ctx, &models.Counterparty{Name: name})
		if err != nil {
			return nil, nil, nil, err
		}
		ids = append(ids, cp.ID)
		created = append(created, cp.Name)
	}
	return ids, added, created, nil
}
