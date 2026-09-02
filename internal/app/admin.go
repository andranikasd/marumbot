package app

import (
	"context"
	"time"

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

	UsersByDay(context.Context) ([]DayCount, error)
	LoansByDay(context.Context) ([]DayCount, error)
	GetUser(context.Context, string) (UserRow, error)
	LoansByUser(context.Context, string) ([]LoanRow, error)
	BudgetsForUser(context.Context, string) ([]BudgetRow, error)
	ConversationState(context.Context, string) (*ConvoRow, error)
	CommandCounts(context.Context) ([]StatusCount, error)
	DeliveryCounts(context.Context) ([]StatusCount, error)
	CommandsForUser(ctx context.Context, userID string, limit int32) ([]CommandRow, error)
	DeliveriesForUser(ctx context.Context, userID string, limit int32) ([]DeliveryRow, error)
}

// EngineReader is what the admin needs to run the engine over a stored loan:
// the same read path the bot uses, so the operator sees exactly what the
// borrower is told.
type EngineReader interface {
	LoansForUser(ctx context.Context, userID string, limit int32) ([]UserLoan, error)
	Budget(ctx context.Context, userID string) (Budget, error)
}

// Admin is the read-mostly service behind the private web interface.
type Admin struct {
	store  AdminStore
	mod    Moderation
	engine EngineReader
}

// NewAdmin builds the read-mostly service behind the admin interface.
func NewAdmin(s AdminStore) *Admin { return &Admin{store: s} }

// WithModeration attaches the operator's write surface. Separate from the read
// store so a build that only reads cannot accidentally gain the ability to
// delete an account.
func (a *Admin) WithModeration(m Moderation) *Admin { a.mod = m; return a }

// WithEngine lets loan pages show the projection and the plan the borrower
// would get. Without it the pages show the stored facts only.
func (a *Admin) WithEngine(e EngineReader) *Admin { a.engine = e; return a }

// The moderation actions, each a thin pass-through that exists so the inbound
// handler never touches a store directly.

// PauseUser suspends an account without destroying anything.
func (a *Admin) PauseUser(ctx context.Context, userID string) error {
	return a.mod.SetUserAccess(ctx, userID, "paused")
}

// RestoreUser returns a suspended account to active use.
func (a *Admin) RestoreUser(ctx context.Context, userID string) error {
	return a.mod.SetUserAccess(ctx, userID, "active")
}

// RequestDeletion marks an account for erasure. Reversible until honoured.
func (a *Admin) RequestDeletion(ctx context.Context, userID string) error {
	return a.mod.RequestUserDeletion(ctx, userID)
}

// EraseUser honours a deletion request. Irreversible, and the only such action.
func (a *Admin) EraseUser(ctx context.Context, userID string) error {
	return a.mod.DeleteUser(ctx, userID)
}

// ArchiveLoan hides a loan; its ledger is kept so the balance stays checkable.
func (a *Admin) ArchiveLoan(ctx context.Context, loanID string) error {
	return a.mod.ArchiveLoanAdmin(ctx, loanID)
}

// RestoreLoan un-archives a loan.
func (a *Admin) RestoreLoan(ctx context.Context, loanID string) error {
	return a.mod.RestoreLoan(ctx, loanID)
}

// RenameLoan changes a loan's title and note.
func (a *Admin) RenameLoan(ctx context.Context, loanID, name, description string) error {
	return a.mod.RenameLoanAdmin(ctx, loanID, name, description)
}

// Queue returns the command inbox in detail, optionally filtered by status.
func (a *Admin) Queue(ctx context.Context, status string) ([]CommandDetail, error) {
	return a.mod.CommandsDetailed(ctx, status, 100)
}

// PurgeDead deletes every command that has exhausted its retries, returning
// how many. An operator's decision, taken once the evidence has been read.
func (a *Admin) PurgeDead(ctx context.Context) (int64, error) {
	return a.mod.PurgeDeadCommands(ctx)
}

// Retry puts a command back in the queue with a fresh attempt budget.
func (a *Admin) Retry(ctx context.Context, id string) error {
	return a.mod.RetryCommand(ctx, id)
}

// CommandCounts returns the inbox split by status.
func (a *Admin) CommandCounts(ctx context.Context) ([]StatusCount, error) {
	return call(ctx, "CommandCounts", a.store.CommandCounts)
}

// DeliveryCounts returns the outbox split by status.
func (a *Admin) DeliveryCounts(ctx context.Context) ([]StatusCount, error) {
	return call(ctx, "DeliveryCounts", a.store.DeliveryCounts)
}

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

// LoanView is everything the interface shows about one loan: the stored
// facts, and what the engine makes of them.
type LoanView struct {
	Loan      LoanDetail
	Contracts []ContractRow
	Snapshots []SnapshotRow
	Events    []EventRow
	// Projection is the schedule from the latest anchor, when the engine can
	// build one. Nil with a reason otherwise.
	Projection     *Projection
	ProjectionNote string
	// Plan is what the borrower is currently advised, when a budget exists.
	Plan     *PlanSummary
	PlanNote string
	// Support is the loan summarised as plain text an operator can paste to
	// the borrower. Empty when the engine cannot read the loan.
	Support string
	// Borrower is the loan as the Mini App shows it: the balance, when it
	// was stated, the next instalment. The operator sees what the borrower
	// sees before the record behind it. Nil when the engine cannot read it.
	Borrower *BorrowerView
}

// Loan returns one loan with its contracts, snapshots and ledger.
func (a *Admin) Loan(ctx context.Context, id string, now time.Time) (LoanView, error) {
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
	a.enrich(ctx, &v, now)
	return v, nil
}

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

// Moderation is the operator's write surface.
//
// Every action here is reversible except DeleteUser, and that one is deliberately
// two steps: a request, then an erasure. Erasure destroys the evidence for the
// decision that caused it, so it should never be one click away from a list.
type Moderation interface {
	SetUserAccess(ctx context.Context, userID, state string) error
	RequestUserDeletion(ctx context.Context, userID string) error
	DeleteUser(ctx context.Context, userID string) error
	ArchiveLoanAdmin(ctx context.Context, loanID string) error
	RestoreLoan(ctx context.Context, loanID string) error
	RenameLoanAdmin(ctx context.Context, loanID, name, description string) error
	CommandsDetailed(ctx context.Context, status string, limit int32) ([]CommandDetail, error)
	RetryCommand(ctx context.Context, id string) error
	PurgeDeadCommands(ctx context.Context) (int64, error)
}

// CommandDetail is one queue entry with enough context to explain a stuck one.
type CommandDetail struct {
	ID            string
	UpdateID      int64
	UserID        string
	Kind          string
	Status        string
	Attempts      int
	ReceivedAt    time.Time
	NextAttemptAt time.Time
	LeaseOwner    string
	LastError     string
	CompletedAt   *time.Time
	DueAgeS       int64
}

// AccessStates are the values users.access_state accepts.
var AccessStates = []string{"trial", "grace", "active", "paused"}
