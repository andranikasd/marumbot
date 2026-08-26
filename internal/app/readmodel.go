// Package app holds the use cases and the read model. Adapters depend on this
// package; it depends on none of them.
package app

import "time"

// The admin read model. These types live here rather than in the database
// adapter so the inbound admin interface never imports the outbound Postgres
// one - adapters do not know about each other.

// Overview is the dashboard snapshot of the whole system.
type Overview struct {
	Users, Loans, Events, Snapshots, Policies int64
	CommandsPending, CommandsDead             int64
	DeliveriesPending, DeliveriesDead         int64
	OccurrencesScheduled                      int64
	OldestCommandAgeS, OldestDeliveryAgeS     int64
}

// UserRow is one account, without any Telegram identifier.
type UserRow struct {
	ID          string
	Locale      string
	Timezone    string
	AccessState string
	TrialEndsAt time.Time
	CreatedAt   time.Time
	DeletedAt   *time.Time
	LoanCount   int64
}

// LoanRow is a loan in a list, with the reliability its state carries.
type LoanRow struct {
	ID             string
	UserID         string
	Name           string
	Lender         *string
	Currency       string
	CreatedAt      time.Time
	ArchivedAt     *time.Time
	EventCount     int64
	SnapshotCount  int64
	Reliability    *string
	PrincipalMinor *int64
	BalanceAsOf    *time.Time
}

// LoanDetail is a single loan, without its history.
type LoanDetail struct {
	ID           string
	UserID       string
	Name         string
	Lender       *string
	Currency     string
	NextEventSeq int64
	CreatedAt    time.Time
	ArchivedAt   *time.Time
}

// ContractRow is one version of a loan agreement.
type ContractRow struct {
	ID              string
	Version         int32
	EffectiveFrom   time.Time
	EffectiveUntil  *time.Time
	NominalRate     float64
	DayCount        string
	RepaymentType   string
	StartDate       time.Time
	MaturityDate    time.Time
	PaymentDay      int16
	ScheduledMinor  *int64
	RoundingMode    string
	RoundingUnit    int32
	PolicyVersionID string
}

// SnapshotRow is one dated observation of the lender state.
type SnapshotRow struct {
	ID                    string
	AsOf                  time.Time
	CapturedAt            time.Time
	Trust                 string
	PrincipalMinor        int64
	AccruedInterestMinor  int64
	UnpaidInterestMinor   int64
	CurrentFeesMinor      int64
	OverdueFeesMinor      int64
	PenaltiesMinor        int64
	OverduePrincipalMinor int64
	AdvanceCreditMinor    int64
	NextInstalmentMinor   *int64
	NextDueDate           *time.Time
	RemainingInstalments  *int16
	SourceNote            *string
}

// EventRow is one entry in a loan ledger.
type EventRow struct {
	ID            string
	RecordedSeq   int64
	Kind          string
	ValueDate     time.Time
	RecordedAt    time.Time
	AmountMinor   *int64
	BankOrder     *int32
	BankReference *string
	VoidsEventID  *string
	ContractVerID string
	Covered       bool
}

// PolicyRow is a versioned payment allocation policy.
type PolicyRow struct {
	ID         string
	Key        string
	Version    int32
	ExcessRule string
	Definition []byte
	Source     string
	CreatedAt  time.Time
}

// CommandRow is one entry in the durable Telegram inbox.
type CommandRow struct {
	ID          string
	UpdateID    int64
	UserID      *string
	Kind        string
	Status      string
	Attempts    int16
	NextAttempt time.Time
	LeaseOwner  *string
	LeaseUntil  *time.Time
	ReceivedAt  time.Time
	CompletedAt *time.Time
	LastError   *string
}

// DeliveryRow is one outbound message and its delivery state.
type DeliveryRow struct {
	ID          string
	UserID      string
	Kind        string
	Status      string
	Priority    int16
	ScheduledAt time.Time
	NextAttempt time.Time
	Attempts    int16
	MessageID   *int64
	SentAt      *time.Time
	LastError   *string
}

// ReconRow is the drift measured at one new confirmed snapshot.
type ReconRow struct {
	ID             string
	LoanID         string
	PrincipalMinor int64
	InterestMinor  int64
	FeeMinor       int64
	PenaltyMinor   int64
	EngineVersion  string
	CreatedAt      time.Time
}
