package smsparser

import (
	"strconv"
	"strings"
	"unicode"
)

func amountStrToFloat(amt string) (float64, error) {
	amt = strings.Map(func(r rune) rune {
		if r == ',' || unicode.IsSpace(r) {
			return -1
		}
		return r
	}, amt)
	amt = strings.TrimRight(amt, "+-.")
	return strconv.ParseFloat(amt, 64)
}
