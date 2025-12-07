package smsparser

var arabicToEnglishCurrencyMap = map[string]string{
	"دولار": "USD",
	"جنيه":  "EGP",
	"جم":    "EGP",
}

func convertArabicCurrencyToEnglish(currency string) string {
	return arabicToEnglishCurrencyMap[currency]
}
