package formatters

import (
	"math"
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

func FormatNum[T nu](num T) string {
	return p.Sprintf("%v", number.Decimal(num, number.MaxFractionDigits(2)))
}

// FormatRate formats an exchange rate, keeping 3 significant digits for
// rates under 1 (e.g. 0.000123) instead of rounding them to 0.
func FormatRate(rate float64) string {
	frac := 2
	if rate > 0 && rate < 1 {
		frac = min(12, int(math.Ceil(-math.Log10(rate)))+2)
	}
	return p.Sprintf("%v", number.Decimal(rate, number.MaxFractionDigits(frac)))
}

func FormatAmount(amount float64, curSymbol string) string {
	strBuilder := new(strings.Builder)
	if curSymbol != "" {
		strBuilder.WriteString(curSymbol)
		strBuilder.WriteString(" ")
	}
	strBuilder.WriteString(FormatNum(amount))
	return strBuilder.String()
}
