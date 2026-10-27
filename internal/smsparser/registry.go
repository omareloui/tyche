package smsparser

import "strings"

var banksOrder = []Bank{HSBC, CIB, AAIB, QNB, ArabBank}

var senderToBank = map[string]Bank{
	"HSBC":      HSBC,
	"CIB":       CIB,
	"AAIB":      AAIB,
	"QNB":       QNB,
	"ARAB BANK": ArabBank,
}

// Default returns a registry with all the supported parsers registered.
func Default() *Registry {
	r := NewRegistry()

	r.Register(HSBC, &HSBCEnglishATMWithdrawalParser{})
	r.Register(HSBC, &HSBCEnglishATMReversalParser{})
	r.Register(HSBC, &HSBCEnglishCreditCardPurchaseParser{})
	r.Register(HSBC, &HSBCEnglishDebitCardPurchaseParser{})
	r.Register(HSBC, &HSBCEnglishIPNInwardTransferParser{})
	r.Register(HSBC, &HSBCEnglishIPNOutwardTransferParser{})
	r.Register(HSBC, &HSBCEnglishSalaryParser{})
	r.Register(HSBC, &HSBCEnglishTransferParser{})

	r.Register(CIB, &CIBArabicATMWithdrawalParser{})
	r.Register(CIB, &CIBArabicDebitCardPurchaseParser{})
	r.Register(CIB, &CIBArabicDebitToCreditTransferParser{})
	r.Register(CIB, &CIBArabicHoldReleaseParser{})
	r.Register(CIB, &CIBArabicIPNInwardTransferParser{})
	r.Register(CIB, &CIBArabicIPNOutwardTransferParser{})
	r.Register(CIB, &CIBArabicRefundParser{})
	r.Register(CIB, &CIBEnglishCreditCardPurchaseParser{})
	r.Register(CIB, &CIBEnglishDebitToCreditTransferParser{})
	r.Register(CIB, &CIBEnglishCreditCardChargeParser{})
	r.Register(CIB, &CIBEnglishCreditCardReversalParser{})
	r.Register(CIB, &CIBArabicCreditCardRefundParser{})
	r.Register(CIB, &CIBInformationalParser{})

	r.Register(AAIB, &AAIBEnglishCreditCardPurchaseParser{})

	r.Register(ArabBank, &ArabBankArabicSalaryParser{})

	return r
}

// BankFromSender maps an SMS sender id to a bank, NilBank if unknown.
func BankFromSender(sender string) Bank {
	return senderToBank[strings.ToUpper(strings.TrimSpace(sender))]
}

// ParseFromSender parses using the sender's bank parsers if the sender is
// known, otherwise tries every registered bank.
func (r *Registry) ParseFromSender(sender, sms string) (*SMS, error) {
	sms = strings.TrimSpace(sms)
	if bank := BankFromSender(sender); bank != NilBank {
		return r.Parse(bank, sms)
	}
	return r.ParseAny(sms)
}

// ParseAny tries the parsers of every registered bank.
func (r *Registry) ParseAny(sms string) (*SMS, error) {
	for _, bank := range banksOrder {
		for _, parser := range r.parsers[bank] {
			if parser.CanParse(sms) {
				return safeParse(parser, sms)
			}
		}
	}
	return nil, ErrCantParse
}
