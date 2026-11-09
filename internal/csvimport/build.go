package csvimport

import (
	"strings"
	"time"
	"unicode"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsingest"
)

// minNameLen is the least length of a counterparty's name (or alias) found
// inside a row's text.
const minNameLen = 3

// Build makes the transaction of a row into acc, fulfilled on its day (at its
// time of day in loc when it has one, else midnight UTC like a day picked in
// a form) and flagged for review: an expense or an income by its sign, its
// counterparty the one named by the row (else found in its description) with
// that one's default category. Rows in another currency are converted at
// today's rate.
func Build(row *Row, acc *models.Account, counterparties []models.Counterparty, loc *time.Location, fxRate smsingest.FxRateFunc) (*models.Transaction, error) {
	kind := models.IncomeTxKind
	if row.IsOut() {
		kind = models.ExpenseTxKind
	}

	at := row.Date
	if row.HasTime {
		y, m, d := row.Date.Date()
		at = time.Date(y, m, d, 0, 0, 0, 0, loc).Add(row.Clock)
	}

	amount, note, err := smsingest.Convert(abs(row.Amount), row.Currency, acc, fxRate)
	if err != nil {
		return nil, err
	}

	tx := &models.Transaction{
		ID:          idgen.New(),
		Account:     *acc,
		Kind:        kind,
		Amount:      amount,
		ScheduledTo: at,
		FulfilledAt: at,
		NeedsReview: true,
	}

	cp := smsingest.MatchCounterparty(row.Counterparty, counterparties)
	if cp == nil {
		cp = FindCounterparty(row.Counterparty+" "+row.Description, counterparties)
	}
	if cp != nil {
		tx.Counterparty = cp
		tx.Category = cp.DefaultCategoryFor(kind)
	}

	var notes []string
	if row.Counterparty != "" && cp == nil {
		notes = append(notes, row.Counterparty)
	}
	if row.Description != "" {
		notes = append(notes, row.Description)
	}
	if note != "" {
		notes = append(notes, note)
	}
	tx.Description = strings.Join(notes, " · ")
	return tx, nil
}

// FindCounterparty is the counterparty whose name (or alias) is in text as
// whole words, the longest when many are, nil when none is.
func FindCounterparty(text string, counterparties []models.Counterparty) *models.Counterparty {
	text = " " + wordsOf(text) + " "
	var best *models.Counterparty
	bestLen := 0
	for i := range counterparties {
		for _, name := range append([]string{counterparties[i].Name}, counterparties[i].Aliases...) {
			w := wordsOf(name)
			if len(w) < minNameLen || len(w) <= bestLen {
				continue
			}
			if strings.Contains(text, " "+w+" ") {
				cp := counterparties[i]
				best, bestLen = &cp, len(w)
			}
		}
	}
	return best
}

// wordsOf lowers s and keeps its letters and digits, words separated by one
// space.
func wordsOf(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
