package models

import (
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
	CreatedAt       time.Time
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
// skipping blanks and case-insensitive duplicates, and takes its default
// category if this merchant has none.
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

	if cp.IsMerchant && cp.DefaultCategory == nil {
		cp.DefaultCategory = other.DefaultCategory
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
