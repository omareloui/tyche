package models

import (
	"slices"
	"strings"
	"time"
)

// MerchantGuess is the AI's decision on a merchant string of an sms no
// counterparty matched (merchantguess): matched to one of the user's
// merchants, the string added as its alias, or a merchant it Created (named
// cleanly, the string as its alias). The alias is what matches the next sms
// with the same string, no call needed.
type MerchantGuess struct {
	ID string
	// Raw is the merchant as the sms wrote it.
	Raw            string
	CounterpartyID string
	// CategoryID is the default category given to a merchant it created.
	CategoryID string
	Created    bool
	// RejectedAt is when the user said it's wrong: its alias removed, the
	// string never asked about again.
	RejectedAt *time.Time
	CreatedAt  time.Time
}

// MerchantGuessKey is how sms merchant strings compare (as
// smsingest.MatchCounterparty does).
func MerchantGuessKey(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// WithoutAlias is the aliases without raw (any case).
func WithoutAlias(aliases []string, raw string) []string {
	key := MerchantGuessKey(raw)
	return slices.DeleteFunc(slices.Clone(aliases), func(a string) bool { return MerchantGuessKey(a) == key })
}

// WithAlias is the aliases with raw added, unless already there (any case)
// or it's the name.
func WithAlias(name string, aliases []string, raw string) []string {
	key := MerchantGuessKey(raw)
	if key == "" || MerchantGuessKey(name) == key || slices.ContainsFunc(aliases, func(a string) bool { return MerchantGuessKey(a) == key }) {
		return aliases
	}
	return append(slices.Clone(aliases), strings.TrimSpace(raw))
}
