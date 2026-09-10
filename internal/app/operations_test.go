package app

import (
	"context"
	"errors"
	"testing"
)

type probeStore struct {
	version     int64
	unavailable bool
}

func (s probeStore) Ping(context.Context) error {
	if s.unavailable {
		return errors.New("offline")
	}
	return nil
}
func (s probeStore) MigrationVersion(context.Context) (int64, error) { return s.version, nil }
func (s probeStore) QueueStatus(context.Context) (OperationStatus, error) {
	return OperationStatus{CommandsPending: 2, DeliveriesPending: 3, OldestCommandAgeS: 4, OldestDeliveryAgeS: 5}, nil
}

func TestPublicProbesRequireCompatibleSchema(t *testing.T) {
	for _, tc := range []struct {
		s     probeStore
		ready bool
	}{{probeStore{RequiredSchemaVersion, false}, true}, {probeStore{RequiredSchemaVersion - 1, false}, false}, {probeStore{RequiredSchemaVersion, true}, false}} {
		o := Operations{Store: tc.s}
		if o.Health(t.Context()).DatabaseOK != tc.ready {
			t.Fatalf("readiness for %+v", tc.s)
		}
	}
	status, err := (&Operations{Store: probeStore{RequiredSchemaVersion, false}}).Status(t.Context())
	if err != nil || status.CommandsPending != 2 || status.OldestDeliveryAgeS != 5 {
		t.Fatal("public queue counters unavailable", err)
	}
}

type deadlineProbeStore struct{ probeStore }

func (deadlineProbeStore) Ping(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("probe has no deadline")
	}
	return nil
}

func (deadlineProbeStore) QueueStatus(ctx context.Context) (OperationStatus, error) {
	if _, ok := ctx.Deadline(); !ok {
		return OperationStatus{}, errors.New("probe has no deadline")
	}
	return OperationStatus{}, nil
}

func TestProbesBoundDatabaseWork(t *testing.T) {
	o := Operations{Store: deadlineProbeStore{probeStore{version: RequiredSchemaVersion}}}
	if !o.Health(context.Background()).DatabaseOK {
		t.Fatal("health did not bound its database calls")
	}
	if _, err := o.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
}
