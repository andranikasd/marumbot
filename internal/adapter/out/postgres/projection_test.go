package postgres_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/projection"
)

func TestProjectionSettingsDurableIdentityAndIsolation(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	user, other := newUser(t, s), newUser(t, s)
	svc := app.BudgetCommands{Store: s}
	if err := s.SetBudget(ctx, user, "AMD", 100000, 15); err != nil {
		t.Fatal(err)
	}
	before, err := s.Budget(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := s.ProjectionSettings(ctx, user)
	if err != nil || empty.Enabled || empty.Version != 0 || len(empty.Currencies) != 0 {
		t.Fatal("new user silently opted in", err)
	}
	in := app.ProjectionUpdate{Enabled: true, Key: uuid.NewString(), Currencies: map[string]projection.Extra{"AMD": {Minor: 2500000, Overrides: map[string]int64{"2026-11": 0}}, "USD": {Minor: 10000}}}
	version, err := svc.SetProjectionSettings(ctx, user, in)
	if err != nil || version != 1 {
		t.Fatal("save", version, err)
	}
	version, err = svc.SetProjectionSettings(ctx, user, in)
	if err != nil || version != 1 {
		t.Fatal("durable retry", err)
	}
	in.Enabled = false
	if _, err = svc.SetProjectionSettings(ctx, user, in); !errors.Is(err, app.ErrConflict) {
		t.Fatal("changed retry accepted", err)
	}
	in.Key = uuid.NewString()
	if _, err = svc.SetProjectionSettings(ctx, user, in); !errors.Is(err, app.ErrConflict) {
		t.Fatal("stale source overwritten", err)
	}
	current, err := s.ProjectionSettings(ctx, user)
	if err != nil || !current.Enabled || !reflect.DeepEqual(current.Currencies, in.Currencies) {
		t.Fatal("source changed", err)
	}
	isolated, err := s.ProjectionSettings(ctx, other)
	if err != nil || isolated.Version != 0 {
		t.Fatal("cross-account source leak", err)
	}
	in.ExpectedVersion = 1
	in.Key = uuid.NewString()
	in.Enabled = true
	delete(in.Currencies["AMD"].Overrides, "2026-11")
	if version, err = svc.SetProjectionSettings(ctx, user, in); err != nil || version != 2 {
		t.Fatal("reset", version, err)
	}
	current, err = s.ProjectionSettings(ctx, user)
	if err != nil || len(current.Currencies["AMD"].Overrides) != 0 {
		t.Fatal("override reset failed", err)
	}
	after, err := s.Budget(ctx, user)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("projection mutated legacy budget", err)
	}
	if err = s.RequestUserDeletion(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteUser(ctx, user); err != nil {
		t.Fatal("source history blocked account erasure", err)
	}
	erased, err := s.ProjectionSettings(ctx, user)
	if err != nil || erased.Version != 0 || len(erased.Currencies) != 0 {
		t.Fatal("private source choices survived erasure", err)
	}
}
