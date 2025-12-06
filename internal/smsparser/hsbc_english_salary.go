package smsparser

import (
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type HSBCEnglishSalaryParser struct{}

func (p *HSBCEnglishSalaryParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "From HSBC:") &&
		strings.Contains(sms, "Salary to") && strings.Contains(sms, "Your available balance is")
}

func (p *HSBCEnglishSalaryParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	result := &SMS{Bank: HSBC, TxKind: models.IncomeTxKind}

	parts := strings.Fields(sms)

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
	i += 5

	result.AvailableBalanceCurrency = parts[i]
	i++
	result.AvailableBalance, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}

	return result, nil
}
