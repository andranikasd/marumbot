package app

import (
	"context"

	"github.com/andranikasd/marumbot/internal/obs"
)

// call wraps a store access in the caller side of a boundary span. Without it
// the store node has no inbound edge and the graph shows an orphan.
func call[T any](ctx context.Context, op string, fn func(context.Context) (T, error)) (T, error) {
	ctx, span := obs.ComponentAdmin.Call(ctx, obs.ComponentStore, op)
	defer span.End()
	v, err := fn(ctx)
	if err != nil {
		span.RecordError(err)
	}
	return v, err
}

// AdminStore is the read surface the admin interface needs. It is declared
// here, by the consumer, and satisfied by the Postgres adapter.
type AdminStore interface {
	Overview(context.Context) (Overview, error)
	ListUsers(context.Context, int32) ([]UserRow, error)
	ListLoans(context.Context, int32) ([]LoanRow, error)
	GetLoan(context.Context, string) (LoanDetail, error)
	ContractsForLoan(context.Context, string) ([]ContractRow, error)
	SnapshotsForLoan(context.Context, string) ([]SnapshotRow, error)
	EventsForLoan(context.Context, string) ([]EventRow, error)
	ListPolicies(context.Context) ([]PolicyRow, error)
	InsertPolicy(ctx context.Context, id, key string, version int32, definition []byte, excess, source string) error
	ListCommands(context.Context, int32) ([]CommandRow, error)
	ListDeliveries(context.Context, int32) ([]DeliveryRow, error)
	ListReconciliationRuns(context.Context, int32) ([]ReconRow, error)
	Ping(context.Context) error
	MigrationVersion(context.Context) (int64, error)
}

// Admin is the read-mostly service behind the private web interface.
type Admin struct{ store AdminStore }

// NewAdmin builds the read-mostly service behind the admin interface.
func NewAdmin(s AdminStore) *Admin { return &Admin{store: s} }

// Overview returns the dashboard counters.
func (a *Admin) Overview(ctx context.Context) (Overview, error) {
	return call(ctx, "Overview", a.store.Overview)
}

// Users lists the most recent accounts.
func (a *Admin) Users(ctx context.Context) ([]UserRow, error) {
	return call(ctx, "ListUsers", func(c context.Context) ([]UserRow, error) { return a.store.ListUsers(c, 200) })
}

// Loans lists loans with their derived reliability.
func (a *Admin) Loans(ctx context.Context) ([]LoanRow, error) {
	return call(ctx, "ListLoans", func(c context.Context) ([]LoanRow, error) { return a.store.ListLoans(c, 200) })
}

// Commands lists the most recent inbox entries.
func (a *Admin) Commands(ctx context.Context) ([]CommandRow, error) {
	return call(ctx, "ListCommands", func(c context.Context) ([]CommandRow, error) { return a.store.ListCommands(c, 200) })
}

// Deliveries lists the most recent outbound messages.
func (a *Admin) Deliveries(ctx context.Context) ([]DeliveryRow, error) {
	return call(ctx, "ListDeliveries", func(c context.Context) ([]DeliveryRow, error) { return a.store.ListDeliveries(c, 200) })
}

// Reconciliations lists recent drift measurements.
func (a *Admin) Reconciliations(ctx context.Context) ([]ReconRow, error) {
	return call(ctx, "ListReconciliations", func(c context.Context) ([]ReconRow, error) { return a.store.ListReconciliationRuns(c, 200) })
}

// Policies lists every recorded allocation policy.
func (a *Admin) Policies(ctx context.Context) ([]PolicyRow, error) {
	return call(ctx, "ListPolicies", a.store.ListPolicies)
}

// AddPolicy records a lender payment allocation policy.
func (a *Admin) AddPolicy(ctx context.Context, id, key string, version int32, definition []byte, excess, source string) error {
	return a.store.InsertPolicy(ctx, id, key, version, definition, excess, source)
}

// LoanView is everything the admin interface shows about one loan: the stored
// facts, and what they replay to.
// LoanView is everything the interface shows about one loan.
type LoanView struct {
	Loan      LoanDetail
	Contracts []ContractRow
	Snapshots []SnapshotRow
	Events    []EventRow
}

// Loan returns one loan with its contracts, snapshots and ledger.
func (a *Admin) Loan(ctx context.Context, id string) (LoanView, error) {
	var v LoanView
	var err error
	if v.Loan, err = a.store.GetLoan(ctx, id); err != nil {
		return v, err
	}
	if v.Contracts, err = a.store.ContractsForLoan(ctx, id); err != nil {
		return v, err
	}
	if v.Snapshots, err = a.store.SnapshotsForLoan(ctx, id); err != nil {
		return v, err
	}
	if v.Events, err = a.store.EventsForLoan(ctx, id); err != nil {
		return v, err
	}
	return v, nil
}

// Health is what the status endpoint and the dashboard both report.
// Health is what the status endpoint and the dashboard both report.
type Health struct {
	DatabaseOK       bool
	DatabaseError    string
	MigrationVersion int64
}

// Health reports whether the database is reachable and at which schema.
func (a *Admin) Health(ctx context.Context) Health {
	h := Health{DatabaseOK: true}
	if err := a.store.Ping(ctx); err != nil {
		h.DatabaseOK, h.DatabaseError = false, err.Error()
		return h
	}
	if v, err := a.store.MigrationVersion(ctx); err == nil {
		h.MigrationVersion = v
	}
	return h
}
