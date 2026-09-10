package app

import "context"

// Each search parallelizes internally. Bound aggregate interactive, bot and
// admin computation, and let callers stop waiting when their request expires.
var plannerCapacity = make(chan struct{}, 2)

func acquirePlanner(ctx context.Context) (func(), error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case plannerCapacity <- struct{}{}:
	}
	return func() { <-plannerCapacity }, nil
}
