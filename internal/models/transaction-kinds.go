package models

import "github.com/omareloui/financial-plans/internal/db"

type TransactionKind struct {
	ID              string
	Kind            string
	IncreaseBalance bool
}

func DBTransactionKindToTransactionKind(dbKind *db.TransactionKind) *TransactionKind {
	return &TransactionKind{
		ID:              dbKind.ID,
		Kind:            dbKind.Kind,
		IncreaseBalance: dbKind.IncreaseBalance.Bool,
	}
}

func DBTransactionKindsToTransactionKinds(dbKinds []db.TransactionKind) []TransactionKind {
	kinds := make([]TransactionKind, len(dbKinds))
	for i, kind := range dbKinds {
		kinds[i] = *DBTransactionKindToTransactionKind(&kind)
	}
	return kinds
}
