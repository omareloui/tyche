package formatters

import "testing"

func TestFormatAmountNegativeZero(t *testing.T) {
	cases := []struct {
		amount float64
		want   string
	}{
		{-0.001, "E£ 0"},
		{-0.0, "E£ 0"},
		{-0.004, "E£ 0"},
		{-0.006, "E£ -0.01"},
		{-12.5, "E£ -12.5"},
		{0.001, "E£ 0"},
	}
	for _, c := range cases {
		if got := FormatAmount(c.amount, "E£"); got != c.want {
			t.Errorf("FormatAmount(%v) = %q, want %q", c.amount, got, c.want)
		}
	}
}
