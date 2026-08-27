package app

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

// People write money the way they write money. Refusing any of these forms
// makes the bot look pedantic about something it could simply have understood.
func TestParseAmountAcceptsHowPeopleWrite(t *testing.T) {
	amd := money.MustLookup("AMD")
	cases := map[string]int64{
		"100000":     10_000_000, // AMD has two decimal places
		"100 000":    10_000_000,
		"100,000":    10_000_000, // a comma is a thousands mark here
		"100'000":    10_000_000,
		" 100000 ":   10_000_000,
		"100000֏":    10_000_000,
		"100000 AMD": 10_000_000,
		"1000.50":    100_050,
		"1000,50":    100_050, // exactly two digits: a decimal after all
		"250000":     25_000_000,
	}
	for in, want := range cases {
		got, cur, ok := parseAmount(in, amd)
		if !ok {
			t.Errorf("parseAmount(%q) refused it", in)
			continue
		}
		if got != want {
			t.Errorf("parseAmount(%q) = %d minor, want %d", in, got, want)
		}
		if cur.Code != "AMD" {
			t.Errorf("parseAmount(%q) gave %s, want AMD", in, cur.Code)
		}
	}
}

// A trailing code overrides the default, because a user with a dollar loan
// writes one.
func TestParseAmountHonoursAnExplicitCurrency(t *testing.T) {
	amd := money.MustLookup("AMD")
	got, cur, ok := parseAmount("500 USD", amd)
	if !ok {
		t.Fatal("refused a dollar amount")
	}
	if cur.Code != "USD" {
		t.Errorf("currency = %s, want USD", cur.Code)
	}
	if got != 50_000 { // USD exponent 2
		t.Errorf("got %d minor, want 50000", got)
	}
}

// Anything that is not an amount must be refused, so an ordinary message during
// a pending question stays an ordinary message instead of being swallowed.
func TestParseAmountRefusesEverythingElse(t *testing.T) {
	amd := money.MustLookup("AMD")
	for _, in := range []string{
		"", "   ", "hello", "/loans", "a lot", "100k", "-500", "0",
		"📋 My loans", "what is my budget",
	} {
		if v, _, ok := parseAmount(in, amd); ok {
			t.Errorf("parseAmount(%q) accepted it as %d", in, v)
		}
	}
}

// A currency with no minor unit must not gain one.
func TestParseAmountRespectsTheExponent(t *testing.T) {
	jpy, err := money.Lookup("JPY")
	if err != nil {
		t.Skip("JPY is not in the registry")
	}
	got, _, ok := parseAmount("5000", jpy)
	if !ok {
		t.Fatal("refused a yen amount")
	}
	if got != 5000 {
		t.Errorf("got %d minor, want 5000: the yen has no minor unit", got)
	}
}
