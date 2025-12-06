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
