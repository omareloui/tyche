package smsparser

import (
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type HSBCEnglishIPNOutwardTransferParser struct{}

func (p *HSBCEnglishIPNOutwardTransferParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "Your HSBC Account ********") &&
		strings.Contains(sms, "was debited with IPN outward transfer for")
}

func (p *HSBCEnglishIPNOutwardTransferParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	result := &SMS{Bank: HSBC, TxKind: models.ExpenseTxKind}

	parts := strings.Fields(sms)
	partsLen := len(parts)

	// Your HSBC Account ********1234 was debited with IPN outward transfer for EGP 6,731.73 on 29-07-2025 21:24 to 7967 with reference 4f38876c.
	// 0    1    2       3            4   5       6    7   8       9        10  11  12       13 14         15    16 17   18   19        20

	i := 3

	result.AccountID = parts[i][8:]
	i += 8

	result.Currency = parts[i]
	i++
	result.Amount, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}
	i += 2

	timestamp, err := time.Parse("02-01-2006 15:04", parts[i]+" "+parts[i+1])
	if err == nil {
		result.Timestamp = timestamp
	}
	i += 3

	result.Counterparty = parts[i]
	i++
	if parts[i] != "with" {
		for ; i < partsLen && parts[i] != "with"; i++ {
			result.Counterparty += " " + parts[i]
		}
	}

	i += 2

	result.Ref = strings.TrimRight(parts[i], ".")

	return result, nil
}
