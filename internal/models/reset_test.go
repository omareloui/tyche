package models

import (
	"net/url"
	"testing"
)

func TestParseResetOptions(t *testing.T) {
	t.Run("ignores unknown entities", func(t *testing.T) {
		o := ParseResetOptions(url.Values{"entity": {"sms", "nope"}})
		if !o.Has(ResetSMS) || len(o) != 1 {
			t.Fatalf("got %v", o)
		}
	})

	t.Run("accounts require transactions and recurrent", func(t *testing.T) {
		o := ParseResetOptions(url.Values{"entity": {"accounts"}})
		if !o.Has(ResetAccounts) || !o.Has(ResetTransactions) || !o.Has(ResetRecurrent) {
			t.Fatalf("got %v", o)
		}
		if o.IsAll() {
			t.Fatal("not everything is selected")
		}
	})

	t.Run("empty", func(t *testing.T) {
		if !ParseResetOptions(url.Values{}).IsEmpty() {
			t.Fatal("expected empty")
		}
	})

	t.Run("all", func(t *testing.T) {
		var all []string
		for _, info := range ResetEntities {
			all = append(all, string(info.Entity))
		}
		if !ParseResetOptions(url.Values{"entity": all}).IsAll() {
			t.Fatal("expected all")
		}
	})
}
