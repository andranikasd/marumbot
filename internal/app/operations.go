package app

import "context"

// OperationsStore is the non-personal read surface for public service probes.
type OperationsStore interface {
	Ping(context.Context) error
	MigrationVersion(context.Context) (int64, error)
	Overview(context.Context) (Overview, error)
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
	if version < 22 {
		h.DatabaseError = "schema upgrade required"
		return h
	}
	h.DatabaseOK = true
	return h
}

func (o *Operations) Status(ctx context.Context) (OperationStatus, error) {
	v, err := o.Store.Overview(ctx)
	if err != nil {
		return OperationStatus{}, err
	}
	return OperationStatus{v.OldestCommandAgeS, v.OldestDeliveryAgeS, v.CommandsPending, v.DeliveriesPending}, nil
}
