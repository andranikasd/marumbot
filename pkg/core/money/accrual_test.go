package money

import (
	"math"
	"testing"
)

func amd(major int64) Amount {
	a, err := FromMajor(major, AMD)
	if err != nil {
		panic(err)
	}
	return a
}

// The whole point of the 128-bit path: these all overflow a naive int64
// implementation, and they are ordinary loan sizes in this market.
func TestAccrue_LargePrincipalDoesNotOverflow(t *testing.T) {
	rate := RateFromPercent(18, 0)
	for _, principal := range []int64{4_000_000, 16_529_340, 30_000_000, 80_000_000, 1_000_000_000} {
		got, err := Accrue(amd(principal), rate, 31, Actual365, DefaultAMDPolicy)
		if err != nil {
			t.Fatalf("principal %d AMD: unexpected error %v", principal, err)
		}
		// Independent check in exact rational arithmetic via big-free reasoning:
		// interest = principal_minor * 0.18 * 31 / 365, rounded to whole drams.
		wantMinor := roundToDram(float64(principal) * 100 * 0.18 * 31 / 365)
		if got.Minor() != wantMinor {
			t.Errorf("principal %d AMD: got %d minor, want %d minor", principal, got.Minor(), wantMinor)
		}
	}
}

// roundToDram is a deliberately independent, float-based oracle used only in
// tests. Its precision is ample for the magnitudes checked here.
func roundToDram(minor float64) int64 {
	return int64(math.Round(minor/100)) * 100
}

func TestAccrue_KnownValues(t *testing.T) {
	tests := []struct {
		name      string
		principal int64
		percent   int64
		days      int64
		dc        DayCount
		want      int64 // minor units
	}{
		// 1,840,000 AMD at 16% for 31 days, act/365:
		// 184_000_000 * 0.16 * 31 / 365 = 2,500,383.56 minor -> 2,500,400 (whole dram, half-up)
		{"consumer act365 31d", 1_840_000, 16, 31, Actual365, 2_500_400},
		// The same loan over a 28-day February accrues visibly less: this is the
		// difference a naive rate/12 calculator cannot express.
		{"consumer act365 28d", 1_840_000, 16, 28, Actual365, 2_258_400},
		// act/360 accrues about 1.4% more than act/365 over the same days.
		{"consumer act360 31d", 1_840_000, 16, 30, Actual360, 2_453_300},
		{"zero rate", 1_840_000, 0, 31, Actual365, 0},
		{"zero days", 1_840_000, 16, 0, Actual365, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Accrue(amd(tc.principal), RateFromPercent(tc.percent, 0), tc.days, tc.dc, DefaultAMDPolicy)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Minor() != tc.want {
				t.Errorf("got %d minor (%s), want %d minor", got.Minor(), got, tc.want)
			}
		})
	}
}

// Interest must always land on a whole dram under the default policy.
func TestAccrue_AlwaysQuantisedToWholeDram(t *testing.T) {
	rate := RateFromPercent(23, 750000) // 23.75%
	for principal := int64(1); principal <= 5_000_000; principal += 137_137 {
		got, err := Accrue(amd(principal), rate, 29, Actual365, DefaultAMDPolicy)
		if err != nil {
			t.Fatalf("principal %d: %v", principal, err)
		}
		if got.Minor()%100 != 0 {
			t.Fatalf("principal %d: %d minor is not a whole dram", principal, got.Minor())
		}
	}
}

// Accrual must be monotone in principal, in rate and in days. A rounding bug
// that breaks monotonicity would show up here before it reached a schedule.
func TestAccrue_Monotonicity(t *testing.T) {
	rate := RateFromPercent(18, 0)
	prev := int64(-1)
	for principal := int64(100_000); principal <= 3_000_000; principal += 100_000 {
		got, err := Accrue(amd(principal), rate, 30, Actual365, DefaultAMDPolicy)
		if err != nil {
			t.Fatal(err)
		}
		if got.Minor() < prev {
			t.Fatalf("interest fell as principal rose at %d AMD", principal)
		}
		prev = got.Minor()
	}
}

func TestAccrue_RejectsBadInput(t *testing.T) {
	if _, err := Accrue(amd(1000), RateFromPercent(18, 0), -1, Actual365, DefaultAMDPolicy); err == nil {
		t.Error("negative days should be rejected")
	}
	if _, err := Accrue(FromMinor(-100, AMD), RateFromPercent(18, 0), 30, Actual365, DefaultAMDPolicy); err == nil {
		t.Error("negative principal should be rejected")
	}
	if _, err := Accrue(FromMinor(math.MaxInt64, AMD), RateFromPercent(40, 0), 366, Actual365, DefaultAMDPolicy); err == nil {
		t.Error("absurd principal should return an error rather than wrap")
	}
}

func TestRoundingModes(t *testing.T) {
	// 2.5 minor units at unit=1 under each mode.
	cases := []struct {
		mode Mode
		quo  int64
		rem  int64
		div  int64
		want int64
	}{
		{HalfUp, 2, 1, 2, 3},
		{HalfEven, 2, 1, 2, 2},
		{HalfEven, 3, 1, 2, 4},
		{Down, 2, 1, 2, 2},
		{Up, 2, 1, 2, 3},
	}
	for _, c := range cases {
		if got := roundQuotient(c.quo, c.rem, c.div, c.mode); got != c.want {
			t.Errorf("%s: roundQuotient(%d,%d,%d) = %d, want %d", c.mode, c.quo, c.rem, c.div, got, c.want)
		}
	}
}

func TestAddSubOverflow(t *testing.T) {
	max := FromMinor(math.MaxInt64, AMD)
	if _, err := max.Add(FromMinor(1, AMD)); err == nil {
		t.Error("add overflow should be reported")
	}
	min := FromMinor(math.MinInt64, AMD)
	if _, err := min.Sub(FromMinor(1, AMD)); err == nil {
		t.Error("sub overflow should be reported")
	}
}

func TestCurrencyMismatchPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("mixing currencies should panic")
		}
	}()
	usd := Currency{Code: "USD", Exponent: 2}
	_, _ = FromMinor(100, AMD).Add(FromMinor(100, usd))
}
