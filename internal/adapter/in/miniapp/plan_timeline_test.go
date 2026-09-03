package miniapp

import "testing"

func TestCSVFormulaEscaping(t *testing.T) {
	for _, s := range []string{"=1+1", "+cmd", "-cmd", "@fn", "\t=1", "\r=1", "  =1"} {
		if csvText(s) != "'"+s {
			t.Fatalf("formula not escaped: %q", s)
		}
	}
	for _, s := range []string{"", "Bank", "Վարկ"} {
		if csvText(s) != s {
			t.Fatalf("ordinary text changed: %q", s)
		}
	}
}
