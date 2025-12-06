package smsparser

import (
	"errors"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type HSBCEnglishTransferParser struct{}

func (p *HSBCEnglishTransferParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "From HSBC:") &&
		strings.Contains(sms, "Transfer from") &&
		!strings.Contains(sms, "IPN") &&
		!strings.Contains(sms, "Salary")
}

func (p *HSBCEnglishTransferParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	result := &SMS{Bank: HSBC, TxKind: models.TransferTxKind}

	parts := strings.Fields(sms)
	partsLen := len(parts)

	if partsLen < 12 {
		return nil, errors.New("invalid HSBC English Transfer message")
	}

	// From HSBC: 07OCT25 Transfer from 123-456***-789 EGP 360.00- Your available balance is EGP 49,647.85
	// 0    1     2       3        4    5              6   7       8    9         10      11 12  13

	i := 2

	timestamp, err := time.Parse("02Jan06", parts[i])
	if err == nil {
		result.Timestamp = timestamp
	}
	i += 3

	result.AccountID = parts[i]
	i++

	result.Currency = parts[i]
	i++
	result.Amount, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}
	result.ToCurrency = result.Currency
	result.ToAmount = result.Amount
	i += 5

	result.AvailableBalanceCurrency = parts[i]
	i++
	result.AvailableBalance, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}

	result.Counterparty = "Transfer"

	return result, nil
}
