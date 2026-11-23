package models

import (
	"math"
	"testing"
)

func TestComputeInflation(t *testing.T) {
	rates := map[int]float64{2020: 5, 2021: 10, 2022: 20, 2023: -10}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

	r, err := ComputeInflation(rates, 100, 2020, 2023)
	if err != nil {
		t.Fatal(err)
	}
	// 100 × 1.1 × 1.2 × 0.9: 2020's own rate isn't walked.
	if !near(r.Worth, 118.8) || !near(r.Cumulative, 0.188) {
		t.Errorf("worth = %v, cumulative = %v", r.Worth, r.Cumulative)
	}
	if !near(r.PowerLost, 1-1/1.188) {
		t.Errorf("power lost = %v", r.PowerLost)
	}
	if !near(r.AvgPct, (math.Cbrt(1.188)-1)*100) {
		t.Errorf("avg = %v", r.AvgPct)
	}
	if len(r.Years) != 4 || r.Years[0].Pct != 0 || r.Years[0].Value != 100 || !near(r.Years[2].Value, 132) || r.Years[2].Pct != 20 {
		t.Errorf("years = %+v", r.Years)
	}
	if r.Back() {
		t.Error("forward read as back")
	}

	// Back in time: 118.8 of 2023 was 100 in 2020.
	back, err := ComputeInflation(rates, 118.8, 2023, 2020)
	if err != nil {
		t.Fatal(err)
	}
	if !near(back.Worth, 100) || !back.Back() || !near(back.Years[3].Value, 118.8) {
		t.Errorf("back = %+v", back)
	}

	same, err := ComputeInflation(rates, 50, 2022, 2022)
	if err != nil || same.Worth != 50 || same.Cumulative != 0 || same.AvgPct != 0 || len(same.Years) != 1 {
		t.Errorf("same year = %+v %v", same, err)
	}

	if _, err := ComputeInflation(rates, 100, 2019, 2023); err != nil {
		t.Errorf("2019's own rate isn't needed: %v", err)
	}
	if _, err := ComputeInflation(rates, 100, 2020, 2024); err == nil {
		t.Error("missing 2024 accepted")
	}
}
