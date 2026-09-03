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
func (s probeStore) Overview(context.Context) (Overview, error) {
	return Overview{CommandsPending: 2, DeliveriesPending: 3, OldestCommandAgeS: 4, OldestDeliveryAgeS: 5}, nil
}

func TestPublicProbesRequireCompatibleSchema(t *testing.T) {
	for _, tc := range []struct {
		s     probeStore
		ready bool
	}{{probeStore{22, false}, true}, {probeStore{21, false}, false}, {probeStore{22, true}, false}} {
		o := Operations{Store: tc.s}
		if o.Health(t.Context()).DatabaseOK != tc.ready {
			t.Fatalf("readiness for %+v", tc.s)
		}
	}
	status, err := (&Operations{Store: probeStore{22, false}}).Status(t.Context())
	if err != nil || status.CommandsPending != 2 || status.OldestDeliveryAgeS != 5 {
		t.Fatal("public queue counters unavailable", err)
	}
}
