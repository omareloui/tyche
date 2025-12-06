package smsparser

import (
	"strconv"
	"strings"
)

func amountStrToFloat(amt string) (float64, error) {
	amt = strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(amt), ",", ""), "-")
	res, err := strconv.ParseFloat(amt, 64)
	return res, err
}
