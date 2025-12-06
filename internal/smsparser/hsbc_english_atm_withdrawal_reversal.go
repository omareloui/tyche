package smsparser

import (
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

type HSBCEnglishATMReversalParser struct{}

func (p *HSBCEnglishATMReversalParser) CanParse(sms string) bool {
	return strings.HasPrefix(sms, "From HSBC:") &&
		strings.Contains(sms, "ATM Reversal of Cash Withdrawal from")
}

func (p *HSBCEnglishATMReversalParser) Parse(sms string) (*SMS, error) {
	if !p.CanParse(sms) {
		return nil, ErrCantParse
	}

	var err error
	result := &SMS{Bank: HSBC, TxKind: models.TransferTxKind, AccountID: "Cash"}

	parts := strings.Fields(sms)

	// From HSBC: 28JUL25 ATM Reversal of Cash Withdrawal from 123-456***-789 EGP 1,000.00+ Your available balance is EGP 28,905.35
	// 0    1     2       3   4        5  6    7          8    9              10  11        12   13        14      15 16  17

	i := 2

	timestamp, err := time.Parse("02Jan06", parts[i])
	if err == nil {
		result.Timestamp = timestamp
	}
	i += 7

	result.ToAccountID = parts[i]
	i++

	result.ToCurrency = parts[i]
	i++
	result.ToAmount, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}
	result.Currency = result.ToCurrency
	result.Amount = result.ToAmount
	i += 5

	result.AvailableBalanceCurrency = parts[i]
	i++
	result.AvailableBalance, err = amountStrToFloat(parts[i])
	if err != nil {
		return nil, err
	}

	return result, nil
}
