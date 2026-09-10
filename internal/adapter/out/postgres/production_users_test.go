package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
)

func TestConcurrentFirstContactCreatesOneAccountWithoutOrphans(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	const callers = 32
	for round := range 4 {
		in := freshUpsert()
		inputs := make([]app.UpsertUser, callers)
		accounts := make([]app.Account, callers)
		errs := make([]error, callers)
		start := make(chan struct{})
		var calls sync.WaitGroup
		for i := range callers {
			inputs[i] = in
			inputs[i].NewID = uuid.NewString()
			calls.Add(1)
			go func() {
				defer calls.Done()
				<-start
				accounts[i], errs[i] = s.UpsertByTelegram(ctx, inputs[i])
			}()
		}
		close(start)
		calls.Wait()
		created := 0
		for i, account := range accounts {
			if errs[i] != nil {
				t.Fatalf("round %d caller %d: %v", round, i, errs[i])
			}
			if account.ID == "" || account.ID != accounts[0].ID {
				t.Fatalf("round %d caller %d resolved a different account", round, i)
			}
			if account.Created {
				created++
			}
		}
		if created != 1 {
			t.Fatalf("round %d created %d accounts, want one", round, created)
		}
		for i, input := range inputs {
			_, err := s.GetUser(ctx, input.NewID)
			if input.NewID == accounts[0].ID {
				if err != nil {
					t.Fatalf("winning account unavailable: %v", err)
				}
			} else if !errors.Is(err, app.ErrNotFound) {
				t.Fatalf("round %d losing caller %d retained an account: %v", round, i, err)
			}
		}
	}
}

func TestPausedTelegramIdentityCannotAuthenticateUntilRestored(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in := freshUpsert()
	account, err := s.UpsertByTelegram(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"active", "paused", "active"} {
		if err := s.SetUserAccess(ctx, account.ID, state); err != nil {
			t.Fatal(err)
		}
		id, err := s.ByTelegramTag(ctx, in.UserTag)
		if state == "paused" {
			if err == nil || id != "" {
				t.Fatal("paused identity authenticated")
			}
		} else if err != nil || id != account.ID {
			t.Fatalf("active identity not restored: %v", err)
		}
	}
}
