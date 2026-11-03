package models

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

type Counterparty struct {
	ID      string   `validate:"required"`
	Name    string   `validate:"required,not_blank" conform:"trim,titlecase"`
	Aliases []string `validate:"dive,not_blank"`
	Icon    string   `conform:"trim,lower"`
	Color   string
	// IsMerchant enables auto-detecting the brand icon.
	IsMerchant bool
	// DefaultCategory is used for new transactions without a category.
	DefaultCategory *Category `validate:"-"`
	// IsArchived hides it from the pickers, its transactions keep it.
	IsArchived bool
	// Rating is a merchant's stars, 1 to 5, 0 when unrated.
	Rating int `validate:"min=0,max=5"`
	// Note is free text about any counterparty.
	Note string `validate:"max=1000" conform:"trim"`
	// Locations are a merchant's branches, in order.
	Locations []CounterpartyLocation `validate:"dive"`
	CreatedAt time.Time
}

// MaxRating is the most stars a merchant can get.
const MaxRating = 5

// CounterpartyLocation is a merchant's branch.
type CounterpartyLocation struct {
	ID      string
	Label   string `validate:"required,not_blank,max=100" conform:"trim"`
	Address string `validate:"max=300" conform:"trim"`
	MapsURL string `validate:"omitempty,http_url,max=2000" conform:"trim"`
}

// MapsLink is the location's maps url, else a maps search of its address
// (or label).
func (l *CounterpartyLocation) MapsLink() string {
	if l.MapsURL != "" {
		return l.MapsURL
	}
	q := l.Address
	if q == "" {
		q = l.Label
	}
	return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(q)
}

// DefaultCategoryFor returns the default category if it applies to the
// transaction kind, nil otherwise.
func (cp *Counterparty) DefaultCategoryFor(kind TxKind) *Category {
	if cp == nil || cp.DefaultCategory == nil || cp.DefaultCategory.ID == "" {
		return nil
	}
	catKind := kind.CategoryKind()
	if !cp.DefaultCategory.HasKind(catKind) {
		return nil
	}
	return &Category{ID: cp.DefaultCategory.ID, Kinds: []TxKind{catKind}}
}

// Absorb adds the other counterparty's name and aliases to the aliases,
// skipping blanks and case-insensitive duplicates, appends its note, and takes
// its default category and rating if this merchant has none. Locations are
// moved separately.
func (cp *Counterparty) Absorb(other *Counterparty) {
	seen := map[string]bool{strings.ToLower(strings.TrimSpace(cp.Name)): true}
	aliases := []string{}
	for _, a := range slices.Concat(cp.Aliases, []string{other.Name}, other.Aliases) {
		a = strings.TrimSpace(a)
		key := strings.ToLower(a)
		if a == "" || seen[key] {
			continue
		}
		seen[key] = true
		aliases = append(aliases, a)
	}
	cp.Aliases = aliases

	switch {
	case other.Note == "" || other.Note == cp.Note:
	case cp.Note == "":
		cp.Note = other.Note
	default:
		cp.Note += "\n\n" + other.Note
	}

	if cp.IsMerchant && cp.DefaultCategory == nil {
		cp.DefaultCategory = other.DefaultCategory
	}
	if cp.IsMerchant && cp.Rating == 0 {
		cp.Rating = other.Rating
	}
}

func (cp *Counterparty) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	if err := sani.SanitizeStruct(cp); err != nil {
		return errs.ErrSanitizer
	}
	if valerr := valid.Validate(cp); valerr != nil {
		return *valerr
	}
	return nil
}
