package postgres_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/andranikasd/marumbot/internal/adapter/out/erasurejournal"
	"github.com/andranikasd/marumbot/internal/app"
)

func TestRestoreReconcilesIndependentErasureJournal(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	erased, unaffected := newUser(t, s), newUser(t, s)
	if err := s.RequestUserDeletion(ctx, erased); err != nil {
		t.Fatal(err)
	}
	// The database represents an older backup: the account still exists while
	// the independent journal already holds its later durable deletion intent.
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	journal, err := erasurejournal.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(erased))
	hash := hex.EncodeToString(sum[:])
	if err := journal.Record(ctx, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetUser(ctx, erased); err != nil {
		t.Fatalf("restored fixture missing before reconciliation: %v", err)
	}
	for range 2 {
		if err := app.ReconcileErasures(ctx, journal, s); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetUser(ctx, erased); !errors.Is(err, app.ErrNotFound) {
			t.Fatalf("erased account resurrected: %v", err)
		}
		if _, err := s.GetUser(ctx, unaffected); err != nil {
			t.Fatalf("unaffected account changed: %v", err)
		}
	}
	subjects, err := journal.Subjects(ctx)
	if err != nil || len(subjects) != 1 || subjects[0] != hash {
		t.Fatal("durable deletion intent was lost")
	}
}
