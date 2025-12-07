package smsparser

import (
	"strconv"
	"strings"
)

func amountStrToFloat(amt string) (float64, error) {
	amt = strings.ReplaceAll(strings.TrimSpace(amt), ",", "")
	amt = strings.TrimRight(strings.TrimRight(strings.TrimRight(amt, "-"), "+"), ".")
	res, err := strconv.ParseFloat(amt, 64)
	return res, err
}
