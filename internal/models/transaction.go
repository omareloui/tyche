package models

import (
	"time"

	"github.com/omareloui/financial-planner/internal/db"
)

type Transaction struct {
	ID          string  `validate:"required"`
	Amount      float64 `validate:"required,min=1"`
	Currency    Currency
	Kind        TransactionKind
	Description string
	FulfilledOn time.Time
	ScheduledTo time.Time `validate:"required"`
	CreatedAt   time.Time
}

func (t *Transaction) ViewAmount() string {
	return formatAmount(t.Amount, t.Currency.Enum)
}

func DBTransactionRowToTransaction[T db.GetTransactionRow | db.GetTransactionsRow](dbTx *T) *Transaction {
	_dbTx := db.GetTransactionsRow(*dbTx)
	return &Transaction{
		ID:          _dbTx.ID,
		Amount:      _dbTx.Amount,
		Description: _dbTx.Description.String,
		FulfilledOn: _dbTx.FulfilledOn.Time,
		ScheduledTo: _dbTx.ScheduledTo,
		CreatedAt:   _dbTx.CreatedAt.(time.Time),
		Currency: Currency{
			ID:        _dbTx.CurrencyID,
			Enum:      CurrencyEnum(_dbTx.Enum.Int64),
			EgpFxRate: _dbTx.EgpFxRate.Float64,
		},
		Kind: TransactionKind{
			ID:              _dbTx.KindID,
			Kind:            _dbTx.Kind.String,
			IncreaseBalance: _dbTx.IncreaseBalance.Bool,
		},
	}
}
