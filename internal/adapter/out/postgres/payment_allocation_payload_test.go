package postgres

import (
	"encoding/json"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
)

func TestPaymentAllocationPayloadPreservesUnknownAndKnownZero(t *testing.T) {
	principal, interest, fees := int64(5603410), int64(6904530), int64(0)
	entry := app.PaymentEntry{TransactionDate: "2026-09-24", Hash: "hash"}
	unknown, err := paymentPayload(entry)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(unknown, &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["allocation"]; exists {
		t.Fatal("unknown allocation was persisted as known")
	}
	entry.Allocation = &app.PaymentAllocation{PrincipalMinor: &principal, InterestMinor: &interest, FeesMinor: &fees}
	known, err := paymentPayload(entry)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"transaction_date":"2026-09-24","trust":"user_entered","request_hash":"hash","allocation":{"principal_minor":5603410,"interest_minor":6904530,"fees_minor":0}}`
	if string(known) != want {
		t.Fatalf("payload: %s", known)
	}
}
