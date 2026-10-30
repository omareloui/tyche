package models

import "testing"

func TestTransactionAnalysisAmount(t *testing.T) {
	tests := []struct {
		name string
		tx   Transaction
		want float64
	}{
		{"plain", Transaction{Amount: 100}, 100},
		{"reimbursed expense", Transaction{Amount: 1055.92, Reimbursed: 510}, 545.92},
		{"reimbursement", Transaction{Amount: 510, ReimbursesID: "01J0000000000000000000000"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tx.AnalysisAmount(); got-tt.want > 1e-9 || tt.want-got > 1e-9 {
				t.Errorf("AnalysisAmount() = %v, want %v", got, tt.want)
			}
		})
	}
}
