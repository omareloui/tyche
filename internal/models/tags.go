package models

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

const (
	// MaxTagNameLen is a tag name's longest.
	MaxTagNameLen = 40
	// MaxTxTags is how many tags a transaction can carry.
	MaxTxTags = 20
)

// ErrTagDays is a trip ending before it starts, or with one day only.
var ErrTagDays = errors.New("a trip needs both its first and last day, in order")

// Tag groups transactions across categories and accounts, "for what" they
// were ("Dubai trip", "Wedding"). A tag with days is a trip: it can tag
// everything dated in them at once, and with AutoTag the new transactions
// (form, SMS, CSV) dated in them while not archived.
type Tag struct {
	ID    string
	Name  string `validate:"required,not_blank,max=40" conform:"trim"`
	Color string
	// StartsOn and EndsOn are a trip's first and last days (midnight UTC),
	// both or neither.
	StartsOn *time.Time `validate:"-"`
	EndsOn   *time.Time `validate:"-"`
	AutoTag  bool
	// Budget is what the tag should spend at most, in BudgetCurrency.
	Budget         *float64   `validate:"omitempty,gt=0"`
	BudgetCurrency *Currency  `validate:"-"`
	ArchivedAt     *time.Time `validate:"-"`
	// Note is Markdown, stored as typed.
	Note string `validate:"max=10000"`
	// Txs is how many transactions carry it, when read with them.
	Txs int `validate:"-"`
}

// Hex is its color's.
func (t *Tag) Hex() string { return CategoryColorToHex(t.Color) }

// IsTrip tells it has days.
func (t *Tag) IsTrip() bool { return t.StartsOn != nil && t.EndsOn != nil }

func (t *Tag) IsArchived() bool { return t.ArchivedAt != nil }

// Covers tells day (its date, whatever the zone) is one of the trip's days.
func (t *Tag) Covers(day time.Time) bool {
	if !t.IsTrip() {
		return false
	}
	d := DayOf(day)
	return !d.Before(*t.StartsOn) && !d.After(*t.EndsOn)
}

// AutoTags tells it tags the new transactions dated on day.
func (t *Tag) AutoTags(day time.Time) bool {
	return t.AutoTag && !t.IsArchived() && t.Covers(day)
}

// Days is how many days the trip lasts, 0 when not a trip.
func (t *Tag) Days() int {
	if !t.IsTrip() {
		return 0
	}
	return int(t.EndsOn.Sub(*t.StartsOn).Hours()/24) + 1
}

func (t *Tag) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	if err := sani.SanitizeStruct(t); err != nil {
		return errs.ErrSanitizer
	}
	if !slices.Contains(CategoryColors, t.Color) {
		t.Color = GetRandomCategoryColor()
	}
	if (t.StartsOn == nil) != (t.EndsOn == nil) || (t.IsTrip() && t.EndsOn.Before(*t.StartsOn)) {
		return ErrTagDays
	}
	if !t.IsTrip() {
		t.AutoTag = false
	}
	if t.Budget == nil {
		t.BudgetCurrency = nil
	}
	if valerr := valid.Validate(t); valerr != nil {
		return *valerr
	}
	return nil
}

// ParseTagNames reads a comma separated list of tag names (the tags input's
// value): trimmed, blanks and duplicates (any case) dropped.
func ParseTagNames(s string) []string {
	var out []string
	for _, n := range strings.Split(s, ",") {
		n = strings.TrimSpace(n)
		if n == "" || slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, n) }) {
			continue
		}
		out = append(out, n)
	}
	return out
}

// TagNames are the tags' names.
func TagNames(tags []Tag) []string {
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.Name
	}
	return out
}

// LiveTagNames are the names of the tags not archived, for the pickers'
// suggestions.
func LiveTagNames(tags []Tag) []string {
	var out []string
	for _, t := range tags {
		if !t.IsArchived() {
			out = append(out, t.Name)
		}
	}
	return out
}

// HasTag tells tags has the tag id.
func HasTag(tags []Tag, id string) bool {
	return slices.ContainsFunc(tags, func(t Tag) bool { return t.ID == id })
}

// AutoTaggable tells a new transaction can be auto tagged by a trip: not a
// transfer, balance correction, goal's money or loan payment, nor a
// recurrent transaction's occurrence.
func (t *Transaction) AutoTaggable() bool {
	switch t.Kind {
	case TransferTxKind, AdjustmentTxKind:
		return false
	}
	return t.LoanGoalID == "" && t.Goal == nil && !t.IsGoalTx() && t.RecurrentTxID == "" && t.RecurrentTx == nil
}
