package app

import (
	"context"
	"time"
)

// RequiredSchemaVersion is the earliest schema this binary may serve.
const RequiredSchemaVersion int64 = 26

// OperationsStore is the non-personal read surface for public service probes.
type OperationsStore interface {
	Ping(context.Context) error
	MigrationVersion(context.Context) (int64, error)
	QueueStatus(context.Context) (OperationStatus, error)
}

// Operations exposes readiness and queue aggregates only. Administrative pages
// retain their role checks; public probes never impersonate an administrator.
type Operations struct{ Store OperationsStore }

// OperationStatus contains public queue counts and ages, never account data.
type OperationStatus struct {
	OldestCommandAgeS, OldestDeliveryAgeS int64
	CommandsPending, DeliveriesPending    int64
}

func (o *Operations) Health(ctx context.Context) Health {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	h := Health{}
	if err := o.Store.Ping(ctx); err != nil {
		h.DatabaseError = "database unavailable"
		return h
	}
	version, err := o.Store.MigrationVersion(ctx)
	if err != nil {
		h.DatabaseError = "schema unavailable"
		return h
	}
	h.MigrationVersion = version
	if version < RequiredSchemaVersion {
		h.DatabaseError = "schema upgrade required"
		return h
	}
	h.DatabaseOK = true
	return h
}

func (o *Operations) Status(ctx context.Context) (OperationStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return o.Store.QueueStatus(ctx)
}
