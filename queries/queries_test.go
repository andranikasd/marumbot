package queries

import (
	"strings"
	"testing"
)

// Every statement the store asks for must exist. A typo in a query name would
// otherwise surface as a panic on the first request that used it.
func TestEveryQueryLoads(t *testing.T) {
	want := []string{
		"CountsOverview", "ListUsers", "ListLoans", "GetLoan",
		"ListContractsForLoan", "ListSnapshotsForLoan", "ListEventsForLoan",
		"ListPolicies", "InsertPolicy", "ListCommands", "ListDeliveries",
		"ListReconciliationRuns", "GetLoanState", "CoveredEventIDs",
		"MenuUsers",
	}
	for _, name := range want {
		q, err := Lookup(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if strings.TrimSpace(q) == "" {
			t.Errorf("%s is empty", name)
		}
	}
}

func TestUnknownQueryIsAnError(t *testing.T) {
	if _, err := Lookup("NoSuchQuery"); err == nil {
		t.Error("an unknown query name must be reported, not silently empty")
	}
}

// The ledger tables are append-only. A statement that updates or deletes one
// would break that guarantee, so the whole query set is checked rather than
// relying on review.
func TestNoStatementMutatesTheLedger(t *testing.T) {
	protected := []string{"loan_events", "loan_snapshots", "billing_events"}
	for _, name := range Names() {
		q := strings.ToLower(Get(name))
		for _, table := range protected {
			for _, verb := range []string{"update " + table, "delete from " + table} {
				if strings.Contains(q, verb) {
					t.Errorf("%s performs %q; %s is append-only", name, verb, table)
				}
			}
		}
	}
}
