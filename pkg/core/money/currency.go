package money

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Currency identifies a unit of account, how finely it subdivides, and how
// finely it is actually settled.
//
// Exponent and SettlementUnit are different facts and both matter. ISO 4217
// gives AMD two decimal places — the luma — but the luma has not circulated
// for decades and Armenian lenders settle in whole drams. Storing at exponent 2
// keeps intermediate interest precise; settling at 100 minor units keeps the
// schedule matching the bank's.
type Currency struct {
	Code           string // ISO 4217 alphabetic code
	Exponent       uint8  // decimal places in the minor unit
	SettlementUnit int64  // minor units a payable amount is rounded to
	Name           string
}

// ErrUnknownCurrency is returned for a code not in the registry. Marum refuses
// unknown currencies rather than assuming two decimal places, because that
// assumption is wrong for the yen and for the dinar in opposite directions.
var ErrUnknownCurrency = errors.New("unknown currency")

// registry is keyed by ISO 4217 alphabetic code.
//
// Exponents follow ISO 4217. Settlement units are 1 (the minor unit itself)
// unless a market demonstrably settles more coarsely; each exception carries
// the reason, and any further exception needs a real statement to justify it.
var registry = map[string]Currency{
	// Home market. Settles in whole drams — the luma is obsolete.
	"AMD": {"AMD", 2, 100, "Armenian dram"},

	// Neighbours and common loan currencies in the region.
	"GEL": {"GEL", 2, 1, "Georgian lari"},
	"RUB": {"RUB", 2, 1, "Russian rouble"},
	"USD": {"USD", 2, 1, "US dollar"},
	"EUR": {"EUR", 2, 1, "Euro"},
	"GBP": {"GBP", 2, 1, "Pound sterling"},
	"CHF": {"CHF", 2, 1, "Swiss franc"},
	"AED": {"AED", 2, 1, "UAE dirham"},
	"TRY": {"TRY", 2, 1, "Turkish lira"},
	"CNY": {"CNY", 2, 1, "Chinese yuan"},
	"INR": {"INR", 2, 1, "Indian rupee"},
	"CAD": {"CAD", 2, 1, "Canadian dollar"},
	"AUD": {"AUD", 2, 1, "Australian dollar"},
	"PLN": {"PLN", 2, 1, "Polish zloty"},
	"CZK": {"CZK", 2, 1, "Czech koruna"},
	"SEK": {"SEK", 2, 1, "Swedish krona"},
	"NOK": {"NOK", 2, 1, "Norwegian krone"},
	"DKK": {"DKK", 2, 1, "Danish krone"},
	"ILS": {"ILS", 2, 1, "Israeli new shekel"},
	"UAH": {"UAH", 2, 1, "Ukrainian hryvnia"},
	"KZT": {"KZT", 2, 1, "Kazakhstani tenge"},

	// Zero-decimal currencies. A payment here is a whole unit; assuming two
	// decimal places would inflate every stored amount a hundredfold.
	"JPY": {"JPY", 0, 1, "Japanese yen"},
	"KRW": {"KRW", 0, 1, "South Korean won"},
	"ISK": {"ISK", 0, 1, "Icelandic krona"},
	"CLP": {"CLP", 0, 1, "Chilean peso"},
	"VND": {"VND", 0, 1, "Vietnamese dong"},
	"HUF": {"HUF", 2, 1, "Hungarian forint"},

	// Three-decimal currencies. The opposite error: two decimals would lose a
	// digit of every amount.
	"KWD": {"KWD", 3, 1, "Kuwaiti dinar"},
	"BHD": {"BHD", 3, 1, "Bahraini dinar"},
	"OMR": {"OMR", 3, 1, "Omani rial"},
	"JOD": {"JOD", 3, 1, "Jordanian dinar"},
	"TND": {"TND", 3, 1, "Tunisian dinar"},
	"IQD": {"IQD", 3, 1, "Iraqi dinar"},
	"LYD": {"LYD", 3, 1, "Libyan dinar"},
}

// AMD is the home-market currency and the only one with a lender-confirmed
// settlement rule so far.
var AMD = registry["AMD"]

// Lookup returns the currency for an ISO 4217 code.
func Lookup(code string) (Currency, error) {
	c, ok := registry[strings.ToUpper(strings.TrimSpace(code))]
	if !ok {
		return Currency{}, fmt.Errorf("%w: %q", ErrUnknownCurrency, code)
	}
	return c, nil
}

// MustLookup is Lookup for constants and tests.
func MustLookup(code string) Currency {
	c, err := Lookup(code)
	if err != nil {
		panic(err)
	}
	return c
}

// Codes lists every supported currency code, sorted.
func Codes() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DefaultPolicy is the rounding policy for a currency before any
// lender-specific rule is confirmed. Half-up is the common commercial default;
// the unit comes from the currency's settlement granularity.
func DefaultPolicy(c Currency) Policy {
	return Policy{Mode: HalfUp, Unit: c.SettlementUnit}
}

func (c Currency) String() string { return c.Code }
