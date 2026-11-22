package handler

import (
	"context"
	"net/http"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
)

// SubscriptionSuggestions is the recurrent transactions page's "Looks
// recurring" card (models.DetectSubscriptions), loaded after the page.
func (h Handler) SubscriptionSuggestions(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d, err := h.subscriptionSuggestions(r.Context())
	if err != nil {
		return nil, err
	}
	return views.SubscriptionSuggestions(d), nil
}

// subscriptionSuggestions are the candidates with their counterparty and
// currency, the ones whose counterparty or currency isn't in a live account
// any more left out (nothing to make them recurring with).
func (h Handler) subscriptionSuggestions(ctx context.Context) (*views.SubscriptionSuggestionsData, error) {
	cands, err := h.repo.GetSubscriptionCandidates(ctx, h.userToday(ctx))
	if err != nil {
		return nil, err
	}
	def := &getClaims(ctx).DefaultCurrency
	d := &views.SubscriptionSuggestionsData{Def: def}
	if len(cands) == 0 {
		return d, nil
	}

	accs, err := h.repo.GetAccounts(ctx)
	if err != nil {
		return nil, err
	}
	cps, err := h.repo.GetCounterparties(ctx)
	if err != nil {
		return nil, err
	}
	cpByID := make(map[string]models.Counterparty, len(cps))
	for _, cp := range cps {
		cpByID[cp.ID] = cp
	}

	for _, c := range cands {
		cp, ok := cpByID[c.CounterpartyID]
		if !ok || cp.IsArchived {
			continue
		}
		acc := subscriptionAccount(&c, accs)
		if acc == nil {
			continue
		}
		cur := acc.Currency
		d.Items = append(d.Items, views.SubscriptionSuggestion{SubscriptionCandidate: c, Counterparty: cp, Currency: cur})
		m := c.Monthly()
		if cur.String != def.String {
			m *= cur.FxRate(def.String)
		}
		d.Monthly += m
	}
	return d, nil
}

// subscriptionAccount is the account a candidate is charged from: its last
// charge's when still live, else one in its currency (the default first).
func subscriptionAccount(c *models.SubscriptionCandidate, accs []models.Account) *models.Account {
	var other *models.Account
	for i := range accs {
		a := &accs[i]
		if a.ID == c.AccountID {
			return a
		}
		if a.Currency.ID == c.CurrencyID && (other == nil || a.IsDefault) {
			other = a
		}
	}
	return other
}

// subscriptionOf is the suggestion of the request's counterparty in its
// currency, a 404 when there's none (any more).
func (h Handler) subscriptionOf(r *http.Request) (*views.SubscriptionSuggestion, error) {
	cpID, curID := r.PathValue("cp"), r.PathValue("cur")
	if cpID == "" || curID == "" {
		return nil, errs.ErrInvalidID
	}
	d, err := h.subscriptionSuggestions(r.Context())
	if err != nil {
		return nil, err
	}
	for i := range d.Items {
		if s := &d.Items[i]; s.CounterpartyID == cpID && s.CurrencyID == curID {
			return s, nil
		}
	}
	return nil, resperr.New(http.StatusNotFound, "This charge doesn't look recurring any more")
}

// SubscriptionForm is a suggestion's recurrent transaction form, in a
// drawer: an expense from its account, of its typical amount, at its
// counterparty (with its category), repeating as it does, starting on its
// next charge after today.
func (h Handler) SubscriptionForm(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	s, err := h.subscriptionOf(r)
	if err != nil {
		return nil, err
	}
	accs, knds, cats, cps, err := h.getTxDeps(r.Context())
	if err != nil {
		return nil, err
	}

	tx := defaultTransaction(accs)
	if acc := subscriptionAccount(&s.SubscriptionCandidate, accs); acc != nil {
		tx.Account = *acc
	}
	tx.Kind = models.ExpenseTxKind
	tx.Amount = s.Currency.Round(s.Amount)
	cp := s.Counterparty
	tx.Counterparty = &cp
	tx.Category = nil
	for i := range cats {
		if cats[i].ID == s.CategoryID {
			tx.Category = &cats[i]
			break
		}
	}
	if tx.Category == nil {
		tx.Category = cp.DefaultCategoryFor(tx.Kind)
	}
	if tx.Category == nil {
		tx.Category = &models.Category{}
	}
	// Tomorrow at the earliest: a charge due today may not have come yet,
	// and the first transaction of one starting today is made fulfilled.
	tx.ScheduledTo = s.NextFrom(h.userToday(r.Context()).AddDate(0, 0, 1))
	tx.RecurrentTx = &models.RecurrentTx{Unit: s.Unit, Every: s.Every}

	fd := h.mapTransactionToFD(r.Context(), tx, new(errs.ValidationError))
	return views.SubscriptionForm(fd, accs, knds, cats, cps, &getClaims(r.Context()).DefaultCurrency), nil
}

// DismissSubscription hides a suggestion until its amount changes, answering
// the suggestions left.
func (h Handler) DismissSubscription(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	s, err := h.subscriptionOf(r)
	if err != nil {
		return nil, err
	}
	if err := h.repo.DismissSubscription(r.Context(), s.CounterpartyID, s.CurrencyID, s.Amount); err != nil {
		return nil, err
	}
	return h.SubscriptionSuggestions(w, r)
}
