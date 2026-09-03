package app

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
)

func allocationNumber(n int64) *int64 { return &n }

func TestPaymentAllocationGoldenSplit(t *testing.T) {
	// Exact published components, supplied as a reported fact; never derived from
	// a lender policy. Loading the corpus keeps the UI amounts tied to evidence.
	data, err := os.ReadFile("../../testdata/golden/inecobank-consumer-M26-029210.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Instalment int64 `json:"instalment_minor"`
		Rows       []struct {
			Due       string `json:"due"`
			Principal int64  `json:"principal_minor"`
			Interest  int64  `json:"interest_minor"`
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	row := fixture.Rows[0]
	c := PaymentCommand{LoanID: "loan", Key: "0123456789abcdef", AmountMinor: row.Principal + row.Interest, TransactionDate: row.Due, ValueDate: row.Due, Allocation: &PaymentAllocation{allocationNumber(row.Principal), allocationNumber(row.Interest), allocationNumber(0)}}
	if err := c.validate(date.MustNew(2026, 9, 24)); err != nil {
		t.Fatal(err)
	}
	if c.AmountMinor != 12507940 || *c.Allocation.PrincipalMinor != 5603410 || *c.Allocation.InterestMinor != 6904530 {
		t.Fatal("golden changed")
	}
}

func TestPaymentAllocationValidation(t *testing.T) {
	today := date.MustNew(2026, 9, 3)
	base := PaymentCommand{LoanID: "loan", Key: "0123456789abcdef", AmountMinor: 100, TransactionDate: "2026-09-02", ValueDate: "2026-09-03"}
	for _, tt := range []struct {
		name       string
		allocation *PaymentAllocation
		valid      bool
	}{
		{"unknown", nil, true},
		{"principal only explicit zeros", &PaymentAllocation{allocationNumber(100), allocationNumber(0), allocationNumber(0)}, true},
		{"fees", &PaymentAllocation{allocationNumber(70), allocationNumber(20), allocationNumber(10)}, true},
		{"omitted fee", &PaymentAllocation{allocationNumber(80), allocationNumber(20), nil}, false},
		{"empty", &PaymentAllocation{}, false},
		{"negative", &PaymentAllocation{allocationNumber(101), allocationNumber(-1), allocationNumber(0)}, false},
		{"short", &PaymentAllocation{allocationNumber(70), allocationNumber(20), allocationNumber(0)}, false},
		{"overflow", &PaymentAllocation{allocationNumber(math.MaxInt64), allocationNumber(math.MaxInt64), allocationNumber(102)}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := base
			c.Allocation = tt.allocation
			err := c.validate(today)
			if (err == nil) != tt.valid {
				t.Fatalf("validation: %v", err)
			}
		})
	}
	base.Allocation = &PaymentAllocation{allocationNumber(100), allocationNumber(0), allocationNumber(0)}
	base.ValueDate = ""
	if err := base.validate(today); !errors.Is(err, ErrPaymentInvalid) {
		t.Fatal("accepted allocation without posting")
	}
	base.VoidOnly = true
	base.Replaces = "event"
	base.AmountMinor = 0
	if err := base.validate(today); !errors.Is(err, ErrPaymentInvalid) {
		t.Fatal("void carried allocation")
	}
}

func TestPaymentAllocationUnknownJSONAndLegacyHash(t *testing.T) {
	var c PaymentCommand
	if err := json.Unmarshal([]byte(`{"loan_id":"loan","idempotency_key":"0123456789abcdef","expected_version":0,"amount_minor":100,"transaction_date":"2026-09-02","value_date":"","extra":false}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Allocation != nil {
		t.Fatal("unknown became zero")
	}
	body, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	const legacy = `{"loan_id":"loan","idempotency_key":"0123456789abcdef","expected_version":0,"amount_minor":100,"transaction_date":"2026-09-02","value_date":"","extra":false}`
	if string(body) != legacy {
		t.Fatalf("legacy retry hash changed: %s", body)
	}
}

func TestPaymentActualsMonthBound(t *testing.T) {
	for _, month := range []string{"", "2026-1", "2026-13", "2026-09-01", "2026-00", "anything"} {
		if ValidatePaymentMonth(month) == nil {
			t.Fatalf("accepted %q", month)
		}
	}
	if err := ValidatePaymentMonth("2026-09"); err != nil {
		t.Fatal(err)
	}
}
