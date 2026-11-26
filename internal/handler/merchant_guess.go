package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/brandicons"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/merchantguess"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/smsingest"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// merchantGuessTimeout is how long matching a batch's merchants may hold
// the ingest up: a call takes 3–10s, and the app waits 20s for the whole
// batch (TycheApi.kt).
const merchantGuessTimeout = 15 * time.Second

// SetMerchantGuess turns matching the sms' unknown merchants with AI on (a
// nil guesser keeps it off). Each user still turns it on in /settings.
func (h *Handler) SetMerchantGuess(g merchantguess.Guesser) {
	h.merchantGuess = g
}

// smsText is an sms to read.
type smsText struct {
	sender, body string
	receivedAt   time.Time
}

// unknownMerchants are the merchant strings the sms would create a merchant
// named after (a {{counterparty:merchant}} no counterparty matches), each
// once, but the ones the user said were guessed wrong.
func (sctx *smsContext) unknownMerchants(msgs []smsText) []string {
	var names []string
	seen := map[string]bool{}
	for _, msg := range msgs {
		m, err := sctx.registry.Parse(msg.sender, msg.body)
		if err != nil || !m.SMS.AutoCreateCounterparty || !m.SMS.CounterpartyIsMerchant {
			continue
		}
		tx, err := smsingest.BuildTransaction(m.SMS, sctx.accounts, sctx.counterparties, msg.receivedAt, sctx.loc, fxrate.GetFxRate)
		if err != nil || tx.Counterparty == nil || tx.Counterparty.ID != "" {
			continue
		}
		raw := strings.TrimSpace(m.SMS.Counterparty)
		key := models.MerchantGuessKey(raw)
		if key == "" || seen[key] || sctx.rejected[key] {
			continue
		}
		seen[key] = true
		names = append(names, raw)
		if len(names) == merchantguess.MaxNames {
			break
		}
	}
	return names
}

// resolveMerchants has the AI match the sms' unknown merchants to the
// user's (adding the string as an alias) or create them with a clean name
// and a default category, before their transactions are built: these then
// match through the alias like any other. It never fails: off, over the
// limit or on any error, the sms go on as they would without it.
func (h *Handler) resolveMerchants(ctx context.Context, sctx *smsContext, msgs []smsText) {
	if h.merchantGuess == nil || !sctx.aiMerchants {
		return
	}
	names := sctx.unknownMerchants(msgs)
	if len(names) == 0 {
		return
	}
	l := logger.FromCtx(ctx)
	defer func() {
		if p := recover(); p != nil {
			l.Error("matching merchants panicked", zap.Any("message", p))
		}
	}()
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		l.Error("matching merchants: no tenant", zap.Error(err))
		return
	}
	left, _, err := h.repo.AICallsLeft(ctx, tenantID, models.ScanMerchant, h.ocrCfg.MerchantGuessesPerMonth, time.Now())
	if err != nil {
		l.Error("matching merchants: counting the calls left", zap.Error(err))
		return
	}
	if left <= 0 {
		l.Info("merchant matching over the monthly limit", zap.Int("names", len(names)))
		return
	}
	cats, err := h.repo.GetCategories(ctx)
	if err != nil {
		l.Error("matching merchants: fetching categories", zap.Error(err))
		return
	}

	in := merchantguess.In{Names: names}
	for _, cp := range sctx.counterparties {
		if cp.IsMerchant {
			in.Merchants = append(in.Merchants, merchantguess.Merchant{ID: cp.ID, Name: cp.Name, Aliases: cp.Aliases})
		}
	}
	catByID := map[string]*models.Category{}
	for i := range cats {
		if cats[i].HasKind(models.ExpenseTxKind) {
			in.Categories = append(in.Categories, merchantguess.Category{ID: cats[i].ID, Name: cats[i].Name})
			catByID[cats[i].ID] = &cats[i]
		}
	}

	gctx, cancel := context.WithTimeout(ctx, merchantGuessTimeout)
	defer cancel()
	answers, usage, guessErr := h.merchantGuess.Guess(gctx, in)
	scan := models.ReceiptScan{
		TenantID:      tenantID,
		Kind:          models.ScanMerchant,
		Model:         h.merchantGuess.Model(),
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
		l.Error("recording a merchant matching call", zap.Error(err))
	}
	if guessErr != nil {
		l.Error("matching merchants", zap.Int("names", len(names)), zap.Error(guessErr))
		return
	}

	writes := h.merchantWrites(sctx, merchantguess.Parse(in, answers), catByID)
	if len(writes) == 0 {
		l.Info("matched no merchants", zap.Int("names", len(names)))
		return
	}
	saved, err := h.repo.SaveMerchantGuesses(ctx, writes)
	if err != nil {
		l.Error("saving merchant matches", zap.Int("decisions", len(writes)), zap.Error(err))
		return
	}
	sctx.applyMerchantGuesses(saved, writes)

	created := 0
	for _, g := range saved {
		if g.Created {
			created++
		}
	}
	l.Info("matched merchants", zap.Int("names", len(names)), zap.Int("matched", len(saved)-created), zap.Int("created", created))
}

// merchantWrites turns the decisions into what to save: a new merchant
// named like a counterparty already there is that merchant (or dropped when
// it's not a merchant: names are unique), two strings named alike make one.
func (h *Handler) merchantWrites(sctx *smsContext, decisions []merchantguess.Decision, catByID map[string]*models.Category) []repository.MerchantGuessWrite {
	byName := map[string]*models.Counterparty{}
	for i := range sctx.counterparties {
		byName[models.MerchantGuessKey(sctx.counterparties[i].Name)] = &sctx.counterparties[i]
	}
	known := append(models.CounterpartyLabels(sctx.counterparties), models.CounterpartyLabelSuggestions...)

	var writes []repository.MerchantGuessWrite
	created := map[string]string{}
	for _, d := range decisions {
		if d.MatchID != "" {
			writes = append(writes, repository.MerchantGuessWrite{Raw: d.Raw, CounterpartyID: d.MatchID})
			continue
		}
		cp := &models.Counterparty{
			ID:         idgen.New(),
			Name:       d.NewName,
			IsMerchant: true,
			Labels:     models.MatchLabelCasing(d.Labels, known),
		}
		if err := cp.SanitizeAndValidate(h.sani, h.valid); err != nil {
			continue
		}
		// after sanitizing, which doesn't take a category's kinds
		if cat, ok := catByID[d.CategoryID]; ok {
			cp.DefaultCategory = cat
		}
		key := models.MerchantGuessKey(cp.Name)
		if id, ok := created[key]; ok {
			writes = append(writes, repository.MerchantGuessWrite{Raw: d.Raw, CounterpartyID: id})
			continue
		}
		if other, ok := byName[key]; ok {
			if other.IsMerchant {
				writes = append(writes, repository.MerchantGuessWrite{Raw: d.Raw, CounterpartyID: other.ID})
			}
			continue
		}
		cp.Aliases = models.WithAlias(cp.Name, nil, d.Raw)
		cp.Icon = brandicons.Default().Detect(cp.Name, cp.Aliases)
		cp.Color = brandicons.Default().ColorFor(cp.Icon, "")
		created[key] = cp.ID
		writes = append(writes, repository.MerchantGuessWrite{Raw: d.Raw, New: cp})
	}
	return writes
}

// applyMerchantGuesses puts the saved guesses in the context: the new
// merchants, the aliases added, the guesses marking the transactions.
func (sctx *smsContext) applyMerchantGuesses(saved []models.MerchantGuess, writes []repository.MerchantGuessWrite) {
	news := map[string]*models.Counterparty{}
	for _, w := range writes {
		if w.New != nil {
			news[w.New.ID] = w.New
		}
	}
	for _, g := range saved {
		if cp, ok := news[g.CounterpartyID]; ok && g.Created {
			sctx.counterparties = append(sctx.counterparties, *cp)
		} else {
			for i := range sctx.counterparties {
				if cp := &sctx.counterparties[i]; cp.ID == g.CounterpartyID {
					cp.Aliases = models.WithAlias(cp.Name, cp.Aliases, g.Raw)
				}
			}
		}
		sctx.guesses[models.MerchantGuessKey(g.Raw)] = g
	}
}

// markMerchantGuess marks a transaction whose merchant an AI guess gave it.
func (sctx *smsContext) markMerchantGuess(tx *models.Transaction, raw string) {
	if tx.Counterparty == nil || tx.Counterparty.ID == "" {
		return
	}
	if g, ok := sctx.guesses[models.MerchantGuessKey(raw)]; ok && g.CounterpartyID == tx.Counterparty.ID {
		tx.MerchantGuessID = g.ID
	}
}

// RejectTxMerchantGuess undoes the AI's merchant of a transaction ("Wrong
// match"): the sms' string is no longer an alias of the merchant, and the
// counterparty's picker opens in its place to pick the right one.
func (h Handler) RejectTxMerchantGuess(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	tx, err := h.pickableTx(r)
	if err != nil {
		return nil, err
	}
	g, err := h.repo.GetTxMerchantGuess(ctx, tx.ID)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, resperr.New(http.StatusNotFound, "No AI match to undo")
	}
	if err := h.repo.RejectMerchantGuess(ctx, g.ID); err != nil {
		return nil, err
	}
	logger.FromCtx(ctx).Info("rejected a merchant guess", zap.String("guess_id", g.ID), zap.String("tx_id", tx.ID), zap.Bool("created", g.Created))

	cps, err := h.repo.GetCounterparties(ctx)
	if err != nil {
		return nil, err
	}
	return views.TxCounterpartyPicker(tx, cps), nil
}
