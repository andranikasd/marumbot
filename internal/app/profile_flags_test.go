package app

import (
	"context"
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

type scopedFlag struct {
	environment, profile string
	enabled              bool
}

func (f *scopedFlag) AdminProfileFlag(_ context.Context, environment, profile string) (AdminFlag, error) {
	f.environment = environment
	f.profile = profile
	return AdminFlag{PlanningEnabled: f.enabled}, nil
}

func TestProfileKillSwitchStopsPlanningBeforeProjection(t *testing.T) {
	flags := &scopedFlag{}
	w := Worker{ProfileFlags: flags, Environment: "dev"}
	_, _, _, _, err := w.positions(t.Context(), []UserLoan{{Contract: model.Contract{AllocationPolicy: model.PolicyRef{Key: "lender-a", Version: 2}}}})
	var unsupported *plan.UnsupportedError
	if !errors.As(err, &unsupported) {
		t.Fatal("disabled profile remained plannable", err)
	}
	if flags.environment != "dev" || flags.profile != "lender-a" {
		t.Fatal("flag scope changed")
	}
}
