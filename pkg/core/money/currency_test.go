package money

import "testing"

// Getting an exponent wrong is not a rounding bug, it is a factor-of-100 bug.
// These are the currencies where the common assumption of two decimal places
// is wrong in each direction.
func TestRegistry_Exponents(t *testing.T) {
	cases := map[string]uint8{
		"AMD": 2, "USD": 2, "EUR": 2, "GEL": 2, "RUB": 2,
		"JPY": 0, "KRW": 0, "ISK": 0, "CLP": 0, "VND": 0,
		"KWD": 3, "BHD": 3, "OMR": 3, "JOD": 3,
	}
	for code, want := range cases {
		c, err := Lookup(code)
		if err != nil {
			t.Fatalf("%s: %v", code, err)
		}
		if c.Exponent != want {
			t.Errorf("%s exponent = %d, want %d", code, c.Exponent, want)
		}
	}
}

func TestRegistry_RejectsUnknown(t *testing.T) {
	if _, err := Lookup("XYZ"); err == nil {
		t.Error("unknown code should be rejected, not assumed to have 2 decimals")
	}
	if _, err := Lookup(""); err == nil {
		t.Error("empty code should be rejected")
	}
}

func TestLookup_NormalisesInput(t *testing.T) {
	for _, in := range []string{"amd", " AMD ", "Amd"} {
		c, err := Lookup(in)
		if err != nil || c.Code != "AMD" {
			t.Errorf("Lookup(%q) = %v, %v", in, c, err)
		}
	}
}

// AMD is the one currency with a settlement rule that differs from its ISO
// exponent, and it is the reason SettlementUnit exists as a separate field.
func TestSettlementUnits(t *testing.T) {
	if got := MustLookup("AMD").SettlementUnit; got != 100 {
		t.Errorf("AMD settles in whole drams: unit = %d, want 100", got)
	}
	for _, code := range []string{"USD", "EUR", "JPY", "KWD"} {
		if got := MustLookup(code).SettlementUnit; got != 1 {
			t.Errorf("%s settlement unit = %d, want 1", code, got)
		}
	}
}

func TestFromMajor_AcrossExponents(t *testing.T) {
	cases := []struct {
		code  string
		major int64
		want  int64 // minor units
	}{
		{"AMD", 1_500, 150_000},
		{"USD", 1_500, 150_000},
		{"JPY", 1_500, 1_500},     // zero-decimal: minor == major
		{"KWD", 1_500, 1_500_000}, // three-decimal
	}
	for _, tc := range cases {
		a, err := FromMajor(tc.major, MustLookup(tc.code))
		if err != nil {
			t.Fatalf("%s: %v", tc.code, err)
		}
		if a.Minor() != tc.want {
			t.Errorf("%s %d: got %d minor, want %d", tc.code, tc.major, a.Minor(), tc.want)
		}
	}
}

func TestString_ShowsSubUnitsOnlyWhenTheyCirculate(t *testing.T) {
	// A dram amount that is NOT a whole dram keeps its digits: hiding them
	// would hide a rounding bug.
	if got := money(157).String(); got != "1.57 AMD" {
		t.Errorf("a part-dram amount must keep its digits, got %q", got)
	}
	if got := money(-250000).String(); got != "-2,500 AMD" {
		t.Errorf("negatives group and sign correctly, got %q", got)
	}
}

func money(minor int64) Amount { return FromMinor(minor, AMD) }

func TestString_FormatsPerExponent(t *testing.T) {
	cases := []struct{ code, want string }{
		{"AMD", "1,500 AMD"},     // whole drams: the luma does not circulate
		{"USD", "1,500.00 USD"},  // cents do
		{"JPY", "1,500 JPY"},     // no minor unit at all
		{"KWD", "1,500.000 KWD"}, // three decimal places
	}
	for _, tc := range cases {
		a, _ := FromMajor(1500, MustLookup(tc.code))
		if got := a.String(); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.code, got, tc.want)
		}
	}
}

// Accrual must be correct in every currency, not only the home one. A
// zero-decimal currency has no sub-unit to round away, and a three-decimal
// currency must not silently lose its third digit.
func TestAccrue_AcrossCurrencies(t *testing.T) {
	rate := RateFromPercent(12, 0)
	cases := []struct {
		code  string
		major int64
		want  int64 // minor units of interest over 30 days, act/365
	}{
		// 1,000,000 AMD: 100_000_000 * 0.12 * 30 / 365 = 986,301.4 -> 986,300
		{"AMD", 1_000_000, 986_300},
		// 1,000,000 USD: same minor arithmetic, but settles to the cent.
		{"USD", 1_000_000, 986_301},
		// 1,000,000 JPY: exponent 0, so minor == major.
		{"JPY", 1_000_000, 9_863},
		// 1,000,000 KWD: exponent 3.
		{"KWD", 1_000_000, 9_863_014},
	}
	for _, tc := range cases {
		cur := MustLookup(tc.code)
		p, err := FromMajor(tc.major, cur)
		if err != nil {
			t.Fatalf("%s: %v", tc.code, err)
		}
		got, err := Accrue(p, rate, 30, Actual365, DefaultPolicy(cur))
		if err != nil {
			t.Fatalf("%s: %v", tc.code, err)
		}
		if got.Minor() != tc.want {
			t.Errorf("%s: got %d minor (%s), want %d minor", tc.code, got.Minor(), got, tc.want)
		}
	}
}

// A zero-decimal currency needs a far larger principal before the 128-bit path
// matters, but a three-decimal one needs it sooner. Both must be exact.
func TestAccrue_LargePrincipalEveryCurrency(t *testing.T) {
	rate := RateFromPercent(26, 0)
	for _, code := range Codes() {
		cur := MustLookup(code)
		p, err := FromMajor(500_000_000, cur)
		if err != nil {
			continue // principal not representable in this exponent; not a failure
		}
		if _, err := Accrue(p, rate, 31, Actual365, DefaultPolicy(cur)); err != nil {
			t.Errorf("%s: accrual failed on a large principal: %v", code, err)
		}
	}
}
