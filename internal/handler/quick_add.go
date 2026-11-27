package handler

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/smsingest"
	"github.com/omareloui/tyche/internal/txguess"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// quickAddTimeout is how long reading a text with AI may take.
const quickAddTimeout = 20 * time.Second

// txQuickDoneEvent tells the page (transactions.js) a quick add got saved:
// how many, the ids it may undo, and what was skipped.
const txQuickDoneEvent = "tx-quick-done"

// SetTxGuess turns reading quick add's texts with AI on (a nil guesser
// keeps it off). Each user still turns it on in /settings.
func (h *Handler) SetTxGuess(g txguess.Guesser) {
	h.txGuess = g
}

// errQuickAddOff is quick add asked for while it's off (no AI on the
// server, or not turned on in /settings).
var errQuickAddOff = resperr.New(http.StatusNotFound, "Quick add is off, turn it on in Settings → AI.")

// quickAddQuota is the user's quick adds with AI this month against the
// limit, nil when quick add is off (no AI, or not turned on in /settings).
func (h Handler) quickAddQuota(ctx context.Context) (*views.QuickAddQuota, error) {
	if h.txGuess == nil {
		return nil, nil
	}
	on, err := h.repo.GetAIQuickAdd(ctx)
	if err != nil || !on {
		return nil, err
	}
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	left, limit, err := h.repo.AICallsLeft(ctx, tenantID, models.ScanQuickAdd, h.ocrCfg.QuickAddsPerMonth, time.Now())
	if err != nil {
		return nil, err
	}
	return &views.QuickAddQuota{Used: limit - left, Limit: limit}, nil
}

// quickAddErr shows msg in the quick add's error slot, as a 422.
func quickAddErr(w http.ResponseWriter, msg string) error {
	w.Header().Set("HX-Retarget", "#"+views.QuickAddErrorID)
	w.Header().Set("HX-Reswap", "innerHTML")
	return resperr.New(http.StatusUnprocessableEntity, msg)
}

// quickDeps is what a quick add's transactions are built from.
type quickDeps struct {
	accs  []models.Account
	cats  []models.Category
	cps   []models.Counterparty
	def   *models.Account
	loc   *time.Location
	today time.Time
	now   time.Time
}

func (d *quickDeps) account(id string) *models.Account {
	i := slices.IndexFunc(d.accs, func(a models.Account) bool { return a.ID == id && !a.IsArchive })
	if i < 0 {
		return nil
	}
	return &d.accs[i]
}

// QuickAdd reads the text into transactions and saves them to review: a
// plain amount or "name amount" without AI (txguess.Cheap), anything else
// with it (opted in on /settings, within the month's limit). Answers the
// list, the balances and txQuickDoneEvent; nothing saved is a 422 in the
// quick add's error slot.
func (h Handler) QuickAdd(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	l := logger.FromCtx(ctx)

	quota, err := h.quickAddQuota(ctx)
	if err != nil {
		return nil, err
	}
	if quota == nil {
		return nil, errQuickAddOff
	}
	text := txguess.Clean(r.FormValue("text"))
	if text == "" {
		return nil, quickAddErr(w, "Type what you spent, like “coffee 85 cash”.")
	}
	d, in, err := h.quickAddInput(ctx, text)
	if err != nil {
		return nil, err
	}

	var guesses []txguess.Guess
	var skipped []txguess.Skipped
	model := ""
	if g, ok := txguess.Cheap(in, text); ok {
		guesses = []txguess.Guess{g}
	} else {
		var msg string
		guesses, skipped, model, msg, err = h.quickAddGuess(ctx, in, quota)
		if err != nil {
			return nil, err
		}
		if msg != "" {
			return nil, quickAddErr(w, msg)
		}
	}

	var txs []*models.Transaction
	for _, g := range guesses {
		tx, reason, err := h.quickAddTx(ctx, d, g)
		if err != nil {
			return nil, err
		}
		if reason != "" {
			skipped = append(skipped, txguess.Skipped{Text: g.Text, Reason: reason})
			continue
		}
		txs = append(txs, tx)
	}
	if len(txs) == 0 {
		msg := "Hmm, no amount in there. Try something like “coffee 85 cash”."
		if len(skipped) > 0 {
			msg = "Nothing to add yet: " + describeSkipped(skipped)
		}
		return nil, quickAddErr(w, msg)
	}

	added, err := h.repo.QuickAdd(ctx, text, model, txs)
	if err != nil {
		return nil, err
	}
	var ids []string
	merged := 0
	for _, a := range added {
		if a.Merged {
			merged++
		} else {
			ids = append(ids, a.ID)
		}
	}
	l.Info("quick add", zap.Int("created", len(ids)), zap.Int("merged", merged), zap.Int("skipped", len(skipped)), zap.Bool("ai", model != ""))

	skips := make([]map[string]string, len(skipped))
	for i, s := range skipped {
		skips[i] = map[string]string{"text": s.Text, "reason": s.Reason}
	}
	hxRetargetTxList(w)
	setHXTriggerJSON(w, map[string]any{
		txQuickDoneEvent: map[string]any{"created": len(added), "merged": merged, "ids": ids, "skipped": skips},
	})
	if err := h.getAndRenderBalancesOOB(w, r); err != nil {
		return nil, err
	}
	if model != "" {
		quota.Used++
	}
	if err := renderToBody(w, r, views.QuickAddQuotaOOB(quota)); err != nil {
		return nil, err
	}
	return h.transactionsList(r)
}

func describeSkipped(skipped []txguess.Skipped) string {
	parts := make([]string, len(skipped))
	for i, s := range skipped {
		if s.Text != "" {
			parts[i] = fmt.Sprintf("“%s”: %s", s.Text, strings.ToLower(s.Reason[:1])+s.Reason[1:])
		} else {
			parts[i] = s.Reason
		}
	}
	return strings.Join(parts, "; ") + "."
}

// quickAddInput is what the text's transactions can be: the live accounts,
// the categories and the most used counterparties.
func (h Handler) quickAddInput(ctx context.Context, text string) (*quickDeps, txguess.In, error) {
	accs, err := h.repo.GetAccounts(ctx)
	if err != nil {
		return nil, txguess.In{}, err
	}
	cats, err := h.repo.GetCategories(ctx)
	if err != nil {
		return nil, txguess.In{}, err
	}
	cps, err := h.repo.GetCounterparties(ctx)
	if err != nil {
		return nil, txguess.In{}, err
	}
	uses, err := h.repo.GetCounterpartyUses(ctx)
	if err != nil {
		return nil, txguess.In{}, err
	}
	claims := getClaims(ctx)
	loc, err := h.repo.GetTimezone(ctx)
	if err != nil || loc == nil {
		loc = claims.Location()
	}
	d := &quickDeps{accs: accs, cats: cats, cps: cps, loc: loc, today: h.userToday(ctx), now: time.Now()}
	if claims.DefaultAccount != nil {
		d.def = d.account(claims.DefaultAccount.ID)
	}

	in := txguess.In{Text: text, Today: d.today, Timezone: loc.String(), DefaultCurrency: claims.DefaultCurrency.String}
	if d.def != nil {
		in.DefaultAccount = d.def.Name
	}
	for _, a := range accs {
		if a.IsArchive || a.Kind.Enum == models.AccKindDebt || a.Kind.Enum == models.AccKindGamyaa {
			continue
		}
		in.Accounts = append(in.Accounts, txguess.Account{ID: a.ID, Name: a.Name, Currency: a.Currency.String, Kind: a.Kind.Enum.String()})
	}
	for _, c := range cats {
		in.Categories = append(in.Categories, txguess.Category{ID: c.ID, Name: c.Name, Kinds: c.Kinds})
	}
	live := make([]models.Counterparty, 0, len(cps))
	for _, cp := range cps {
		if !cp.IsArchived {
			live = append(live, cp)
		}
	}
	slices.SortStableFunc(live, func(a, b models.Counterparty) int { return cmp.Compare(uses[b.ID], uses[a.ID]) })
	for _, cp := range live[:min(len(live), txguess.MaxCounterparties)] {
		in.Counterparties = append(in.Counterparties, txguess.Counterparty{ID: cp.ID, Name: cp.Name, IsMerchant: cp.IsMerchant, Aliases: cp.Aliases})
	}
	return d, in, nil
}

// quickAddGuess has the AI read the text, when it's on and within the
// month's limit; msg tells the user why not otherwise.
func (h Handler) quickAddGuess(ctx context.Context, in txguess.In, quota *views.QuickAddQuota) (guesses []txguess.Guess, skipped []txguess.Skipped, model, msg string, err error) {
	if quota.Used >= quota.Limit {
		return nil, nil, "", fmt.Sprintf("You've used this month's %d quick adds with AI. Until the 1st, type an amount, or a name and an amount, like “coffee 85”.", quota.Limit), nil
	}
	l := logger.FromCtx(ctx)
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return nil, nil, "", "", err
	}

	gctx, cancel := context.WithTimeout(ctx, quickAddTimeout)
	defer cancel()
	answers, usage, guessErr := h.txGuess.Guess(gctx, in)
	scan := models.ReceiptScan{
		TenantID:      tenantID,
		Kind:          models.ScanQuickAdd,
		Model:         h.txGuess.Model(),
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
		l.Error("recording a quick add call", zap.Error(err))
	}
	if guessErr != nil {
		l.Error("reading a quick add", zap.Error(guessErr))
		return nil, nil, "", "Couldn't read that just now, give it another try in a moment.", nil
	}
	guesses, skipped = txguess.Parse(in, answers)
	return guesses, skipped, h.txGuess.Model(), "", nil
}

// quickAddTx builds a guess' transaction, to review: reason tells why it
// can't be when it can't.
func (h Handler) quickAddTx(ctx context.Context, d *quickDeps, g txguess.Guess) (tx *models.Transaction, reason string, err error) {
	acc := d.def
	if g.AccountID != "" {
		acc = d.account(g.AccountID)
	}
	if acc == nil {
		return nil, "No account to add it to: pick a default account in Settings", nil
	}
	kind := g.Kind.TxKind()

	day := g.Day
	if day.IsZero() {
		day = d.today
	}
	var at time.Time
	switch {
	case g.HasTime:
		at = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, d.loc).Add(g.Clock)
	case day.Equal(d.today):
		at = d.now.In(d.loc)
	default:
		at = day
	}
	tx = &models.Transaction{
		ID:          idgen.New(),
		Kind:        kind,
		ScheduledTo: at,
		Description: g.Description,
		Note:        g.Note,
		NeedsReview: true,
	}
	for _, name := range g.Tags {
		tx.Tags = append(tx.Tags, models.Tag{Name: name})
	}
	if !day.After(d.today) {
		tx.FulfilledAt = at
	}

	rate := func(from, to string) (float64, error) {
		if h.fxJob != nil {
			v, _, err := h.fxJob.RateOn(ctx, day, from, to)
			return v, err
		}
		return fxrate.GetFxRate(from, to)
	}
	amount := g.Amount
	if g.Grams && isMetal(g.Currency, acc) {
		amount /= models.GramsPerTroyOunce
	}
	convert := func(into *models.Account) (smsingest.Converted, string) {
		c, err := smsingest.Convert(amount, g.Currency, into, kind, rate)
		if err != nil {
			return c, fmt.Sprintf("No exchange rate from %s to %s", g.Currency, into.Currency.String)
		}
		return c, ""
	}
	var notes []string

	switch {
	case kind == models.TransferTxKind:
		to := d.account(g.ToAccountID)
		if to == nil || to.ID == acc.ID {
			return nil, "A transfer needs the account it goes to", nil
		}
		from, why := convert(acc)
		if why != "" {
			return nil, why, nil
		}
		into, why := convert(to)
		if why != "" {
			return nil, why, nil
		}
		tx.Account, tx.ToAccount = *acc, to
		tx.Amount, tx.ToAmount = from.Amount, into.Amount
		notes = append(notes, from.Note())
	case g.Kind.IsDebt():
		person, err := h.quickAddCounterparty(ctx, d, g, false)
		if err != nil {
			return nil, "", err
		}
		debt, err := h.debtAccountOf(ctx, person)
		if re := (*resperr.RespError)(nil); errors.As(err, &re) {
			return nil, re.Message, nil
		} else if err != nil {
			return nil, "", err
		}
		from, to := acc, debt
		if g.Kind == txguess.KindBorrow {
			from, to = debt, acc
		}
		out, why := convert(from)
		if why != "" {
			return nil, why, nil
		}
		in, why := convert(to)
		if why != "" {
			return nil, why, nil
		}
		tx.Account, tx.ToAccount = *from, to
		tx.Amount, tx.ToAmount = out.Amount, in.Amount
		notes = append(notes, out.Note())
	default:
		c, why := convert(acc)
		if why != "" {
			return nil, why, nil
		}
		tx.Account, tx.Amount = *acc, c.Amount
		if kind.CanBeForeign() {
			tx.Original, tx.FxFee = c.Original, c.Fee
		} else {
			notes = append(notes, c.Note())
		}
		cp, err := h.quickAddCounterparty(ctx, d, g, g.NewIsMerchant)
		if err != nil {
			return nil, "", err
		}
		tx.Counterparty = cp
		if i := slices.IndexFunc(d.cats, func(c models.Category) bool { return c.ID == g.CategoryID }); i >= 0 && d.cats[i].HasKind(kind.CategoryKind()) {
			tx.Category = &d.cats[i]
		} else if cp != nil {
			tx.Category = cp.DefaultCategoryFor(kind)
		}
	}
	tx.Description = strings.Join(slices.DeleteFunc(append([]string{tx.Description}, notes...), func(s string) bool { return s == "" }), " · ")
	if err := tx.SanitizeAndValidate(h.sani, h.valid); err != nil {
		logger.FromCtx(ctx).Info("quick add made an invalid transaction", zap.Error(err))
		return nil, "Couldn't make a transaction of it", nil
	}
	return tx, "", nil
}

// quickAddCounterparty is the guess' counterparty: one of the user's, or
// the new one found or created (merchant: a business), nil for none.
func (h Handler) quickAddCounterparty(ctx context.Context, d *quickDeps, g txguess.Guess, merchant bool) (*models.Counterparty, error) {
	if g.CounterpartyID != "" {
		if i := slices.IndexFunc(d.cps, func(c models.Counterparty) bool { return c.ID == g.CounterpartyID }); i >= 0 {
			cp := d.cps[i]
			return &cp, nil
		}
		return nil, nil
	}
	if g.NewCounterparty == "" {
		return nil, nil
	}
	return h.findOrCreateCounterparty(ctx, &models.Counterparty{ID: idgen.New(), Name: g.NewCounterparty, IsMerchant: merchant})
}

// isMetal tells an amount in cur (the account's when "") is a precious
// metal's.
func isMetal(cur string, acc *models.Account) bool {
	if cur == "" || strings.EqualFold(cur, acc.Currency.String) {
		return acc.Currency.IsMetal()
	}
	c := models.Currency{String: cur}
	return c.IsMetal()
}
