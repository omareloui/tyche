package smsparser

import (
	"errors"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

var (
	ErrCantParse = errors.New("can't parse this text")
	// ErrNotTransaction is returned for known messages that aren't
	// transactions, like statements and reminders.
	ErrNotTransaction = errors.New("not a transaction")
)

type Bank uint16

const (
	NilBank Bank = iota
	HSBC
	CIB
	AAIB
	QNB
	ArabBank
)

type SMS struct {
	Bank         Bank
	Amount       float64
	ToAmount     float64
	AccountID    string
	ToAccountID  string
	TxKind       models.TxKind
	Currency     string
	ToCurrency   string
	Ref          string
	Counterparty string
	// AutoCreateCounterparty creates Counterparty on ingest if no existing
	// counterparty (or alias) matches it.
	AutoCreateCounterparty   bool
	AvailableBalance         float64
	AvailableBalanceCurrency string
	Timestamp                time.Time
}

type Parser interface {
	CanParse(sms string) bool
	Parse(sms string) (*SMS, error)
}

type Registry struct {
	parsers map[Bank][]Parser
}

func NewRegistry() *Registry {
	return &Registry{
		parsers: make(map[Bank][]Parser),
	}
}

func (r *Registry) Register(bank Bank, parser Parser) {
	r.parsers[bank] = append(r.parsers[bank], parser)
}

func (r *Registry) Parse(bank Bank, sms string) (*SMS, error) {
	parsers, ok := r.parsers[bank]
	if !ok {
		return nil, errors.New("no parsers registered for bank")
	}

	for _, parser := range parsers {
		if parser.CanParse(sms) {
			return parser.Parse(sms)
		}
	}

	return nil, errors.New("no suitable parser found for SMS format")
}
