package models_test

import (
	"errors"
	"math"
	"testing"

	"github.com/omareloui/tyche/internal/models"
)

func approx(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestInstallmentScheduleStrategies(t *testing.T) {
	t.Run("reducing, equal payments", func(t *testing.T) {
		s, err := models.InstallmentPlan{Strategy: models.StrategyReducing, Months: 12, Rate: 12}.Schedule(100000)
		if err != nil {
			t.Fatal(err)
		}
		if !s.IsEven() || !approx(s.FirstPayment(), 8884.88, 0.01) || !approx(s.TotalInterest, 6618.55, 0.05) {
			t.Errorf("payment %v, interest %v", s.FirstPayment(), s.TotalInterest)
		}
		if !approx(s.Rows[11].Left, 0, 0.01) || !approx(s.Rows[0].Interest, 1000, 0.01) {
			t.Errorf("rows = %+v", s.Rows)
		}
		// 1% a month compounds to 12.68% a year.
		if !approx(s.APR, 12.68, 0.01) {
			t.Errorf("APR = %v", s.APR)
		}
	})

	t.Run("reducing, decreasing payments", func(t *testing.T) {
		s, err := models.InstallmentPlan{Strategy: models.StrategyDecreasing, Months: 12, Rate: 12}.Schedule(120000)
		if err != nil {
			t.Fatal(err)
		}
		if s.IsEven() || !approx(s.FirstPayment(), 11200, 0.01) || !approx(s.LastPayment(), 10100, 0.01) || !approx(s.TotalInterest, 7800, 0.01) {
			t.Errorf("first %v, last %v, interest %v", s.FirstPayment(), s.LastPayment(), s.TotalInterest)
		}
		if !approx(s.APR, 12.68, 0.01) {
			t.Errorf("APR = %v", s.APR)
		}
	})

	t.Run("flat rate", func(t *testing.T) {
		s, err := models.InstallmentPlan{Strategy: models.StrategyFlat, Months: 36, Rate: 10}.Schedule(100000)
		if err != nil {
			t.Fatal(err)
		}
		if !s.IsEven() || !approx(s.TotalInterest, 30000, 0.01) || !approx(s.FirstPayment(), 3611.11, 0.01) {
			t.Errorf("payment %v, interest %v", s.FirstPayment(), s.TotalInterest)
		}
		// A 10% flat rate costs about 18% a year on a reducing balance.
		if s.APR < 18 || s.APR > 21 {
			t.Errorf("APR = %v", s.APR)
		}
	})

	t.Run("flat monthly rate (credit card plan)", func(t *testing.T) {
		s, err := models.InstallmentPlan{Strategy: models.StrategyFlat, Months: 6, Rate: 1.5, RatePeriod: models.RatePerMonth}.Schedule(6000)
		if err != nil {
			t.Fatal(err)
		}
		if !approx(s.TotalInterest, 540, 0.01) || !approx(s.FirstPayment(), 1090, 0.01) {
			t.Errorf("payment %v, interest %v", s.FirstPayment(), s.TotalInterest)
		}
	})

	t.Run("known amount", func(t *testing.T) {
		s, err := models.InstallmentPlan{Months: 12, Amount: 1100}.Schedule(12000)
		if err != nil {
			t.Fatal(err)
		}
		if !approx(s.TotalPaid, 13200, 0.01) || !approx(s.TotalInterest, 1200, 0.01) || s.APR <= 0 {
			t.Errorf("paid %v, interest %v, APR %v", s.TotalPaid, s.TotalInterest, s.APR)
		}
	})

	t.Run("known amount, interest-free", func(t *testing.T) {
		s, err := models.InstallmentPlan{Months: 4}.Schedule(1000)
		if err != nil {
			t.Fatal(err)
		}
		if !approx(s.FirstPayment(), 250, 0.01) || s.TotalInterest != 0 || s.APR != 0 {
			t.Errorf("payment %v, interest %v, APR %v", s.FirstPayment(), s.TotalInterest, s.APR)
		}
	})

	t.Run("known amount, months from the amount", func(t *testing.T) {
		s, err := models.InstallmentPlan{Amount: 300}.Schedule(1000)
		if err != nil {
			t.Fatal(err)
		}
		if s.Plan.Months != 4 || !approx(s.FirstPayment(), 250, 0.01) || !approx(s.TotalPaid, 1000, 0.01) {
			t.Errorf("months %d, payment %v", s.Plan.Months, s.FirstPayment())
		}
	})
}

func TestInstallmentScheduleExtras(t *testing.T) {
	t.Run("down payment, fees and insurance", func(t *testing.T) {
		s, err := models.InstallmentPlan{
			Strategy: models.StrategyFlat, Months: 12, Rate: 10,
			DownPayment: 20, DownPaymentPct: true,
			AdminFee: 1, AdminFeePct: true,
			MonthlyFee:    50,
			InsuranceRate: 1.2,
		}.Schedule(100000)
		if err != nil {
			t.Fatal(err)
		}
		// 80,000 financed, 800 admin fee on the day, 80 insurance + 50 fee a month.
		if s.DownPayment != 20000 || s.UpfrontFees != 800 || s.Financed != 80000 || !approx(s.PaidOnDay(), 20800, 0.01) {
			t.Errorf("schedule = %+v", s)
		}
		if !approx(s.TotalInterest, 8000, 0.01) || !approx(s.TotalFees, 800+12*130, 0.01) {
			t.Errorf("interest %v, fees %v", s.TotalInterest, s.TotalFees)
		}
		if !approx(s.FirstPayment(), 80000.0/12+8000.0/12+130, 0.01) {
			t.Errorf("payment = %v", s.FirstPayment())
		}
		if !approx(s.TotalPaid, 100000+8000+800+1560, 0.01) {
			t.Errorf("paid = %v", s.TotalPaid)
		}
	})

	t.Run("financed admin fee", func(t *testing.T) {
		s, err := models.InstallmentPlan{Strategy: models.StrategyReducing, Months: 12, AdminFee: 500, AdminFeeFinanced: true}.Schedule(12000)
		if err != nil {
			t.Fatal(err)
		}
		if s.UpfrontFees != 0 || s.Financed != 12500 || !approx(s.FirstPayment(), 12500.0/12, 0.01) {
			t.Errorf("schedule = %+v", s)
		}
	})

	t.Run("balloon", func(t *testing.T) {
		s, err := models.InstallmentPlan{Strategy: models.StrategyReducing, Months: 12, BalloonPct: 30}.Schedule(120000)
		if err != nil {
			t.Fatal(err)
		}
		if !approx(s.FirstPayment(), 7000, 0.01) || !approx(s.LastPayment(), 43000, 0.01) || !s.IsEven() || !approx(s.MaxPayment(), 7000, 0.01) {
			t.Errorf("first %v, last %v", s.FirstPayment(), s.LastPayment())
		}
	})

	t.Run("balloon with interest", func(t *testing.T) {
		s, err := models.InstallmentPlan{Strategy: models.StrategyReducing, Months: 24, Rate: 12, BalloonPct: 40}.Schedule(100000)
		if err != nil {
			t.Fatal(err)
		}
		if !approx(s.Rows[23].Left, 0, 0.01) {
			t.Errorf("left at the end = %v", s.Rows[23].Left)
		}
	})

	t.Run("grace months", func(t *testing.T) {
		s, err := models.InstallmentPlan{Strategy: models.StrategyReducing, Months: 12, Rate: 12, GraceMonths: 3}.Schedule(100000)
		if err != nil {
			t.Fatal(err)
		}
		if s.Rows[0].Month != 4 || !approx(s.Rows[0].Interest, 100000*math.Pow(1.01, 3)*0.01, 0.01) || !approx(s.Rows[11].Left, 0, 0.01) {
			t.Errorf("first row = %+v", s.Rows[0])
		}
		f, _ := models.InstallmentPlan{Strategy: models.StrategyFlat, Months: 12, Rate: 12, GraceMonths: 3}.Schedule(100000)
		if !approx(f.TotalInterest, 15000, 0.01) {
			t.Errorf("flat with grace interest = %v", f.TotalInterest)
		}
	})

	t.Run("errors", func(t *testing.T) {
		cases := []struct {
			plan  models.InstallmentPlan
			price float64
			want  error
		}{
			{models.InstallmentPlan{Strategy: models.StrategyFlat, Rate: 10}, 1000, models.ErrInstallmentMonths},
			{models.InstallmentPlan{}, 1000, models.ErrInstallmentKnownAmount},
			{models.InstallmentPlan{Months: 12, DownPayment: 1200}, 1000, models.ErrInstallmentDownPayment},
			{models.InstallmentPlan{Months: 12, DownPayment: 50, DownPaymentPct: true, BalloonPct: 60}, 1000, models.ErrInstallmentDownPayment},
			{models.InstallmentPlan{Months: 12, Amount: 50}, 1000, models.ErrInstallmentTooLow},
		}
		for _, c := range cases {
			if _, err := c.plan.Schedule(c.price); !errors.Is(err, c.want) {
				t.Errorf("%+v: err = %v, want %v", c.plan, err, c.want)
			}
		}
	})
}
