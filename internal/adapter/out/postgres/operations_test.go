package postgres_test

import "testing"

func TestQueueStatusMatchesAdminCounters(t *testing.T) {
	s := testStore(t)
	want, err := s.Overview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.QueueStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.CommandsPending != want.CommandsPending || got.DeliveriesPending != want.DeliveriesPending {
		t.Fatal("queue-only query changed pending counts")
	}
	if got.OldestCommandAgeS < 0 || got.OldestDeliveryAgeS < 0 {
		t.Fatal("queue ages must not be negative")
	}
}
