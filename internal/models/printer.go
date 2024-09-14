package models

import (
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
)

var p *message.Printer = message.NewPrinter(language.English)

type nu interface {
	~int | ~int64 | ~int32 | ~int16 | ~int8 |
		~float64 | ~float32
}

func formatNum[T nu](num T) string {
	return p.Sprintf("%v", number.Decimal(num))
}

func formatAmount(amount float64, currency Currency) string {
	strBuilder := new(strings.Builder)
	if currency.Currency != "" {
		strBuilder.WriteString(currency.Symbol)
		strBuilder.WriteString(" ")
	}
	strBuilder.WriteString(formatNum(amount))
	return strBuilder.String()
}
