package models

import (
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

// ItemCategory groups items, e.g. Dairy or Snacks: the items' own
// categories, apart from the budget ones (Category).
type ItemCategory struct {
	ID    string
	Name  string `validate:"required,not_blank,max=60" conform:"trim"`
	Color string `validate:"required"`
	// Items is how many items are in it, when read with them.
	Items int `validate:"-"`
}

// Hex is its color's.
func (c *ItemCategory) Hex() string { return CategoryColorToHex(c.Color) }

func (c *ItemCategory) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	if err := sani.SanitizeStruct(c); err != nil {
		return errs.ErrSanitizer
	}
	if !slices.Contains(CategoryColors, c.Color) {
		c.Color = GetRandomCategoryColor()
	}
	if valerr := valid.Validate(c); valerr != nil {
		return *valerr
	}
	return nil
}

// Item is a product of the catalog: the receipt lines (TxItem, kind
// TxItemProduct) bought as it are linked to it, so its price is followed
// across transactions and merchants. Its lines keep their own spelling.
type Item struct {
	ID   string
	Name string `validate:"required,not_blank,max=200" conform:"trim"`
	// Aliases are other names lines are matched by, e.g. a receipt's
	// spelling.
	Aliases []string `validate:"max=50,dive,not_blank,max=200"`
	// Category groups it on the items page (an item category, apart from
	// the budget ones); never used by an analysis.
	Category *ItemCategory `validate:"-"`
	// Image is its picture's, nil when it has none.
	Image     *ItemImage `validate:"-"`
	CreatedAt time.Time
}

// ItemImage is what's shown of an item's picture without reading it.
type ItemImage struct {
	Hash          string
	Width, Height int
}

// ImageURL is its picture's url, versioned by its hash (cached for good),
// "" when it has none.
func (it *Item) ImageURL() string {
	if it.Image == nil {
		return ""
	}
	return "/item-pictures/" + it.ID + "?v=" + it.Image.Hash
}

// ItemKey is what a name is matched by: lowercased, its whitespace runs
// folded to one space, the punctuation at its ends dropped ("MILK 1L." and
// "milk  1l" are one).
func ItemKey(name string) string {
	name = strings.Join(strings.Fields(strings.ToLower(name)), " ")
	return strings.TrimFunc(name, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(".,*-:;#_", r)
	})
}

// Key is its name's ItemKey.
func (it *Item) Key() string { return ItemKey(it.Name) }

// Keys are the ItemKeys of its name and aliases.
func (it *Item) Keys() []string {
	keys := []string{it.Key()}
	for _, a := range it.Aliases {
		if k := ItemKey(a); k != "" && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return keys
}

// CleanAliases trims the aliases, dropping blanks, the name's key and
// duplicates (by key), keeping the first spelling.
func (it *Item) CleanAliases() {
	seen := map[string]bool{it.Key(): true}
	out := []string{}
	for _, a := range it.Aliases {
		a = strings.TrimSpace(a)
		k := ItemKey(a)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, a)
	}
	it.Aliases = out
}

// Absorb adds the other item's name and aliases to the aliases (merging it
// into this one), and takes its category when this one has none.
func (it *Item) Absorb(other *Item) {
	it.Aliases = slices.Concat(it.Aliases, []string{other.Name}, other.Aliases)
	it.CleanAliases()
	if it.Category == nil {
		it.Category = other.Category
	}
}

func (it *Item) SanitizeAndValidate(sani *sanitizer.Sanitizer, valid *valid.Validator) error {
	if err := sani.SanitizeStruct(it); err != nil {
		return errs.ErrSanitizer
	}
	it.CleanAliases()
	if valerr := valid.Validate(it); valerr != nil {
		return *valerr
	}
	return nil
}

// ItemCandidate is a catalog item a line's name can be matched to.
type ItemCandidate struct {
	ID      string
	NameKey string
	Aliases []string
	// AtMerchant tells it was bought at the line's merchant before.
	AtMerchant bool
	CreatedAt  time.Time
}

// MatchItem is the candidate a line named name is: one whose name or an
// alias has its key, preferring the ones bought at the line's merchant, then
// a name over an alias, then the oldest. Nil when none.
func MatchItem(name string, candidates []ItemCandidate) *ItemCandidate {
	key := ItemKey(name)
	if key == "" {
		return nil
	}
	var best *ItemCandidate
	bestScore := -1
	for i := range candidates {
		c := &candidates[i]
		score := -1
		switch {
		case c.NameKey == key:
			score = 1
		case slices.ContainsFunc(c.Aliases, func(a string) bool { return ItemKey(a) == key }):
			score = 0
		default:
			continue
		}
		if c.AtMerchant {
			score += 2
		}
		if score > bestScore || (score == bestScore && c.CreatedAt.Before(best.CreatedAt)) {
			best, bestScore = c, score
		}
	}
	return best
}
