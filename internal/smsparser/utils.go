package smsparser

import (
	"strconv"
	"strings"
)

func amountStrToFloat(amt string) float64 {
	amt = strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(amt), ",", ""), "-")
	res, _ := strconv.ParseFloat(amt, 64)
	return res
}
