package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

type countedProfileFlags struct {
	calls [][2]string
	flag  AdminFlag
	err   error
}

func (f *countedProfileFlags) AdminProfileFlag(_ context.Context, environment, profile string) (AdminFlag, error) {
	f.calls = append(f.calls, [2]string{environment, profile})
	return f.flag, f.err
}

func TestPositionsDeduplicatesProfileQueriesPerRequest(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint("missing=", missing), func(t *testing.T) {
			flags := &countedProfileFlags{flag: AdminFlag{PlanningEnabled: true}}
			if missing {
				flags.err = fmt.Errorf("lookup: %w", ErrNotFound)
				flags.flag.PlanningEnabled = false
			}
			w := Worker{ProfileFlags: flags, Environment: "dev"}
			loans := make([]UserLoan, 10)
			for i := range loans {
				loans[i] = paidLoan(t)
				loans[i].ID = fmt.Sprint(i)
				loans[i].Contract.AllocationPolicy = model.PolicyRef{Key: "same-lender", Version: i + 1}
			}
			want, wantOwed, wantRequired, wantCur, err := (&Worker{}).positions(t.Context(), loans)
			if err != nil {
				t.Fatal(err)
			}
			got, owed, required, cur, err := w.positions(t.Context(), loans)
			if err != nil || !reflect.DeepEqual(got, want) || owed != wantOwed || required != wantRequired || cur != wantCur {
				t.Fatalf("position result changed: %v", err)
			}
			if !reflect.DeepEqual(flags.calls, [][2]string{{"dev", "same-lender"}}) {
				t.Fatalf("profile queries = %v; want one for ten loans", flags.calls)
			}
			// A later request must observe a newly disabled flag, not a cached allow.
			flags.err, flags.flag.PlanningEnabled = nil, false
			_, _, _, _, err = w.positions(t.Context(), loans)
			var unsupported *plan.UnsupportedError
			if !errors.As(err, &unsupported) || len(flags.calls) != 2 || flags.calls[1] != [2]string{"dev", "same-lender"} {
				t.Fatalf("next request reused a flag: calls=%v err=%v", flags.calls, err)
			}
			w.Environment = "prod"
			flags.flag.PlanningEnabled = true
			if _, _, _, _, err := w.positions(t.Context(), loans); err != nil || len(flags.calls) != 3 || flags.calls[2] != [2]string{"prod", "same-lender"} {
				t.Fatalf("environment scope changed: calls=%v err=%v", flags.calls, err)
			}
		})
	}
}

func TestPositionsProfileQueryBoundaries(t *testing.T) {
	flags := &countedProfileFlags{flag: AdminFlag{PlanningEnabled: true}}
	w := Worker{ProfileFlags: flags, Environment: "dev"}
	// Flags still apply before zero-balance filtering. Empty keys skip the lookup.
	loans := []UserLoan{
		{Contract: model.Contract{AllocationPolicy: model.PolicyRef{Key: "a"}}},
		{Contract: model.Contract{AllocationPolicy: model.PolicyRef{Key: "b"}}},
		{Contract: model.Contract{AllocationPolicy: model.PolicyRef{Key: "a"}}},
		{},
	}
	if _, _, _, _, err := w.positions(t.Context(), loans); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(flags.calls, [][2]string{{"dev", "a"}, {"dev", "b"}}) {
		t.Fatalf("distinct profile queries = %v", flags.calls)
	}
	for _, cause := range []error{errors.New("database unavailable"), context.Canceled} {
		flags.calls, flags.err = nil, cause
		_, _, _, _, err := w.positions(t.Context(), loans)
		if !errors.Is(err, cause) || len(flags.calls) != 1 {
			t.Fatalf("lookup failure not preserved: calls=%v err=%v", flags.calls, err)
		}
	}
	flags.calls, flags.err, flags.flag.PlanningEnabled = nil, nil, false
	loans[0].UnreconciledPayments = true
	_, _, _, _, err := w.positions(t.Context(), loans)
	var unsupported *plan.UnsupportedError
	if !errors.As(err, &unsupported) || len(flags.calls) != 1 {
		t.Fatalf("disabled flag must precede reconciliation/zero-balance filtering: %v", err)
	}
	// With no flag port, the existing reconciliation gate still applies.
	w.ProfileFlags = nil
	if _, _, _, _, err := w.positions(t.Context(), loans); !errors.Is(err, ErrPaymentReconciliation) {
		t.Fatalf("nil flag port changed reconciliation: %v", err)
	}
}
