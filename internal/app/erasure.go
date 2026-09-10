package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

var ErrErasureJournalUnavailable = errors.New("independent erasure journal is not configured")

// ErasureJournal survives database replacement. Record must be durable before return.
type ErasureJournal interface {
	Record(context.Context, string) error
	Subjects(context.Context) ([]string, error)
}

func (a *Admin) WithErasureJournal(j ErasureJournal) *Admin { a.erasure = j; return a }
func erasureSubject(user string) string {
	sum := sha256.Sum256([]byte(user))
	return hex.EncodeToString(sum[:])
}

type ErasureRestoreStore interface {
	ReconcileErasure(context.Context, []string) error
}

// ReconcileErasures reapplies verified deletion intents before serving restored data.
func ReconcileErasures(ctx context.Context, j ErasureJournal, s ErasureRestoreStore) error {
	subjects, err := j.Subjects(ctx)
	if err != nil {
		return err
	}
	for len(subjects) > 0 {
		n := min(1000, len(subjects))
		if err := s.ReconcileErasure(ctx, subjects[:n]); err != nil {
			return err
		}
		subjects = subjects[n:]
	}
	return nil
}
