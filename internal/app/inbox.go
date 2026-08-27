package app

import (
	"context"
	"errors"
	"time"
)

// ErrNotLeased is returned when a worker tries to finish a command it no longer
// holds. It is not an error in the sense of something being broken: it means the
// lease expired and another worker took over, and the correct response is to
// drop the work rather than write a result twice.
var ErrNotLeased = errors.New("app: command is no longer leased by this worker")

// InboundCommand is one Telegram update, durably recorded before anything acts
// on it.
//
// Telegram retries an update until it is acknowledged, so the webhook must
// persist and return quickly rather than process inline: a slow handler turns
// into a duplicate delivery, and a handler that crashes mid-work turns into a
// lost one. Writing first and working later makes both survivable.
type InboundCommand struct {
	ID           string
	UpdateID     int64
	UserID       string
	Kind         string
	Payload      []byte
	TraceContext string
	Attempts     int
	ReceivedAt   time.Time
}

// Lease is a claim on a command, held for a bounded time.
//
// The Token is a fencing token. A worker that stalls long enough for its lease
// to expire can wake up and try to complete work another worker has already
// redone; the token makes that write fail instead of silently winning. A lease
// without one is not a lease, it is a suggestion.
type Lease struct {
	Command InboundCommand
	Token   string
	Until   time.Time
}

// InboxStore is the durable command inbox. Declared by the consumer.
type InboxStore interface {
	// Enqueue records an update. It is idempotent on the Telegram update id:
	// delivery is at-least-once, so the same update WILL arrive twice, and the
	// second arrival must be a no-op rather than a second command.
	Enqueue(ctx context.Context, c InboundCommand) (accepted bool, err error)

	// Lease claims up to n due commands for the given owner.
	Lease(ctx context.Context, owner string, n int, until time.Time) ([]Lease, error)

	// LeaseByID claims one specific command. The webhook uses this to answer
	// the message in front of it rather than whichever is oldest.
	LeaseByID(ctx context.Context, id, owner string, until time.Time) (Lease, bool, error)

	// Complete marks a command done. It must verify the fencing token.
	Complete(ctx context.Context, id, token string) error

	// Fail records an attempt that did not succeed and schedules a retry, or
	// marks the command dead once it has been tried enough times.
	Fail(ctx context.Context, id, token, code string, retryAt time.Time, dead bool) error
}

// MaxAttempts is how many times a command is retried before it is set aside.
//
// Dead is not deleted: a command that cannot be processed is evidence about a
// bug, and throwing it away destroys the only record of what the user actually
// sent. The admin interface lists them.
const MaxAttempts = 5

// RetryAfter is the backoff schedule, indexed by attempt number.
//
// It is deliberately short at the start and long at the end. Most failures are
// a transient database blip that a second later would clear; the ones that
// survive a minute are usually a bug, and retrying those fast only fills the log
// with the same error.
func RetryAfter(attempt int) time.Duration {
	switch {
	case attempt <= 1:
		return 5 * time.Second
	case attempt == 2:
		return 30 * time.Second
	case attempt == 3:
		return 2 * time.Minute
	default:
		return 10 * time.Minute
	}
}
