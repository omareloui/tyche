package smsparser

import "strings"

type fieldSpec struct {
	// re is what the field matches by default.
	re   string
	desc string
}

const (
	accountRe  = `[\w*#-]+`
	amountRe   = `\d(?:[\d,]|` + ws + `\d)*(?:\.\d+)?[+-]?`
	currencyRe = `جنيه|دولار|جم|[A-Za-z]{3}`
)

// fields are the fields a template can read from an sms.
var fields = map[string]fieldSpec{
	"account_id":         {accountRe, "The account (card number, account number, sms ref). Leading * and # are dropped."},
	"to_account_id":      {accountRe, "The receiving account of a transfer."},
	"amount":             {amountRe, "The amount, with , or space thousands separators and an optional trailing + or -."},
	"currency":           {currencyRe, "The currency code (EGP, USD…), or جنيه / جم / دولار."},
	"to_amount":          {amountRe, "The amount received in a transfer, defaults to the amount."},
	"to_currency":        {currencyRe, "The currency received in a transfer, defaults to the currency."},
	"available_amount":   {amountRe, "The available balance or limit after the transaction."},
	"available_currency": {currencyRe, "The currency of the available balance."},
	"counterparty":       {`.+?`, "Who the money went to or came from, created if no counterparty or alias matches. :merchant creates it as a merchant."},
	"date":               {dateRe, "The transaction date, day first (13/05/26, 07-11-2025, 07OCT25, 07 DEC 2025), or in the format given, e.g. {{date:MM/DD/YYYY}}."},
	"time":               {timeRe, "The transaction time (10:21, 10:21:05, 9:05 PM)."},
	"ref":                {`\S+`, "A reference number."},
	"note":               {`.+?`, "Text added to the transaction description, e.g. the ATM."},
}

// FieldInfo describes a field for the template docs.
type FieldInfo struct {
	Name, Default, Desc string
}

// FieldNames are the template fields in a stable order.
var FieldNames = []string{
	"account_id", "amount", "currency",
	"to_account_id", "to_amount", "to_currency",
	"counterparty", "date", "time", "ref", "note",
	"available_amount", "available_currency",
}

func Fields() []FieldInfo {
	infos := make([]FieldInfo, len(FieldNames))
	for i, name := range FieldNames {
		infos[i] = FieldInfo{Name: name, Default: fields[name].re, Desc: fields[name].desc}
	}
	return infos
}

func normalizeAccount(s string) string {
	return strings.TrimSpace(strings.TrimLeft(s, "*# "))
}

func normalizeCurrency(s string) string {
	s = strings.TrimSpace(s)
	if c := convertArabicCurrencyToEnglish(s); c != "" {
		return c
	}
	return strings.ToUpper(s)
}

func normalizeCounterparty(s string) string {
	return strings.TrimSpace(strings.TrimRight(s, "> "))
}
