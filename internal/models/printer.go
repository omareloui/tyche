package models

import (
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
)

var p *message.Printer = message.NewPrinter(language.English)

type nu interface {
	~int | ~int64 | ~int32 | ~int16 | ~int8
}

func formatNum[T nu](num T) string {
	return p.Sprintf("%v", number.Decimal(num))
}
