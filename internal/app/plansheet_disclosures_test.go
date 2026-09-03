package app

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func TestPlanSheetPreservesExcludedLoanDisclosure(t *testing.T) {
	in := cacheInput(t)
	in.Loans[0].OptionalExcluded = true
	goal := plan.Goal{Kind: plan.LeastInterest}
	report, err := plan.Search(in, goal)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := sheetFromReport(in, goal, report, in.Loans[0].Balance, in.Cash.Monthly, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheet.ExcludedLoans) != 1 || sheet.ExcludedLoans[0].ID != in.Loans[0].ID || sheet.ExcludedLoans[0].Name != in.Loans[0].Name || sheet.ExcludedLoans[0].Reason != "required_only" {
		t.Fatal("original exclusion not disclosed")
	}
	if len(sheet.Months) == 0 || sheet.Months[0].RequiredMinor <= 0 {
		t.Fatal("excluded debt disappeared from required payments")
	}
	in.Loans[0].OptionalExcluded = false
	next, err := sheetFromReport(in, goal, report, in.Loans[0].Balance, in.Cash.Monthly, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.ExcludedLoans) != 0 || len(sheet.ExcludedLoans) != 1 {
		t.Fatal("disclosure leaked between sheets")
	}
}
