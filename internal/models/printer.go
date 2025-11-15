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
	return p.Sprintf("%v", number.Decimal(num, number.MaxFractionDigits(2)))
}

func FormatAmount(amount float64, curSymbol string) string {
	strBuilder := new(strings.Builder)
	if curSymbol != "" {
		strBuilder.WriteString(curSymbol)
		strBuilder.WriteString(" ")
	}
	strBuilder.WriteString(formatNum(amount))
	return strBuilder.String()
}
