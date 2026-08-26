package app

import "context"

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

func NewAdmin(s AdminStore) *Admin { return &Admin{store: s} }

func (a *Admin) Overview(ctx context.Context) (Overview, error) { return a.store.Overview(ctx) }

func (a *Admin) Users(ctx context.Context) ([]UserRow, error) { return a.store.ListUsers(ctx, 200) }

func (a *Admin) Loans(ctx context.Context) ([]LoanRow, error) { return a.store.ListLoans(ctx, 200) }

func (a *Admin) Commands(ctx context.Context) ([]CommandRow, error) {
	return a.store.ListCommands(ctx, 200)
}

func (a *Admin) Deliveries(ctx context.Context) ([]DeliveryRow, error) {
	return a.store.ListDeliveries(ctx, 200)
}

func (a *Admin) Reconciliations(ctx context.Context) ([]ReconRow, error) {
	return a.store.ListReconciliationRuns(ctx, 200)
}

func (a *Admin) Policies(ctx context.Context) ([]PolicyRow, error) { return a.store.ListPolicies(ctx) }

func (a *Admin) AddPolicy(ctx context.Context, id, key string, version int32, definition []byte, excess, source string) error {
	return a.store.InsertPolicy(ctx, id, key, version, definition, excess, source)
}

// LoanView is everything the admin interface shows about one loan: the stored
// facts, and what they replay to.
type LoanView struct {
	Loan      LoanDetail
	Contracts []ContractRow
	Snapshots []SnapshotRow
	Events    []EventRow
}

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
type Health struct {
	DatabaseOK       bool
	DatabaseError    string
	MigrationVersion int64
}

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
