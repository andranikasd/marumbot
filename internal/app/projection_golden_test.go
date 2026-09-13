package app_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/projection"
)

func TestApprovedMonthlyProjectionGolden(t *testing.T) {
	var f struct {
		Currency       string `json:"currency"`
		Today          string `json:"today"`
		Start          string `json:"start"`
		Maturity       string `json:"maturity"`
		NextDue        string `json:"next_due"`
		Balance        int64  `json:"balance_minor"`
		Required       int64  `json:"required_minor"`
		Extra          int64  `json:"extra_minor"`
		BaselineMonths int    `json:"baseline_months"`
		BaselineFinish string `json:"baseline_finish"`
		ExtraMonths    int    `json:"extra_months"`
		ExtraFinish    string `json:"extra_finish"`
		FirstMonth     string `json:"first_month"`
	}
	raw, err := os.ReadFile("../../testdata/projection/approved-monthly.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	parse := func(s string) date.Date {
		d, e := date.Parse(s)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	cur := money.MustLookup(f.Currency)
	l := projection.Loan{ID: "loan", Name: "Example", AsOf: parse(f.Today), Balance: money.FromMinor(f.Balance, cur), Contract: model.Contract{LoanID: "loan", Version: 1, Currency: cur, EffectiveFrom: parse(f.Start), StartDate: parse(f.Start), MaturityDate: parse(f.Maturity), PaymentDay: 15, NotBeforeDue: parse(f.NextDue), HasScheduled: true, ScheduledPayment: money.FromMinor(f.Required, cur), Rounding: money.DefaultPolicy(cur)}}
	for _, tc := range []struct {
		extra  int64
		months int
		finish string
	}{{0, f.BaselineMonths, f.BaselineFinish}, {f.Extra, f.ExtraMonths, f.ExtraFinish}} {
		result, err := projection.Build(parse(f.Today), cur, []projection.Loan{l}, projection.Extra{Minor: tc.extra})
		if err != nil {
			t.Fatal(err)
		}
		if !result.Complete || result.StartMonth != f.FirstMonth || result.Finish != tc.finish || len(result.Months) != tc.months {
			t.Fatalf("wrong approved projection: %+v", result)
		}
		var paid int64
		for _, m := range result.Months {
			if m.Required != f.Required || m.Extra != tc.extra || m.Total != f.Required+tc.extra {
				t.Fatalf("wrong month %+v", m)
			}
			paid += m.Total
		}
		if paid != f.Balance {
			t.Fatal("principal counted twice or missing", paid)
		}
	}
}
