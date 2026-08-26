package date

import (
	"testing"
	"time"
)

func TestNew_RejectsImpossibleDates(t *testing.T) {
	bad := []struct {
		y int
		m time.Month
		d int
	}{
		{2026, time.February, 30},
		{2026, time.February, 29}, // 2026 is not a leap year
		{2026, time.April, 31},
		{2026, time.January, 0},
		{2026, time.Month(13), 1},
	}
	for _, c := range bad {
		if _, err := New(c.y, c.m, c.d); err == nil {
			t.Errorf("New(%d, %v, %d) should be rejected, not rolled over", c.y, c.m, c.d)
		}
	}
	if _, err := New(2024, time.February, 29); err != nil {
		t.Errorf("2024-02-29 is a real date: %v", err)
	}
}

// The behaviour lenders actually implement, and the one naive month arithmetic
// gets wrong: a loan due on the 31st visits the short months and comes back.
func TestOccurrence_AnchorDayDoesNotDrift(t *testing.T) {
	start := MustNew(2026, time.January, 31)
	want := []string{
		"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30",
		"2026-05-31", "2026-06-30", "2026-07-31",
	}
	for i, w := range want {
		got := Occurrence(start, 31, i).String()
		if got != w {
			t.Errorf("occurrence %d: got %s, want %s", i, got, w)
		}
	}
}

// A leap year must put the February occurrence on the 29th.
func TestOccurrence_LeapFebruary(t *testing.T) {
	start := MustNew(2024, time.January, 31)
	if got := Occurrence(start, 31, 1).String(); got != "2024-02-29" {
		t.Errorf("got %s, want 2024-02-29", got)
	}
	if got := Occurrence(MustNew(2026, time.January, 31), 31, 1).String(); got != "2026-02-28" {
		t.Errorf("non-leap February: got %s, want 2026-02-28", got)
	}
}

func TestOccurrence_CrossesYearBoundary(t *testing.T) {
	start := MustNew(2026, time.November, 30)
	cases := map[int]string{0: "2026-11-30", 1: "2026-12-30", 2: "2027-01-30", 3: "2027-02-28"}
	for n, want := range cases {
		if got := Occurrence(start, 30, n).String(); got != want {
			t.Errorf("n=%d: got %s, want %s", n, got, want)
		}
	}
}

// AddMonths is allowed to drift; that is the difference between it and
// Occurrence, and the test pins the distinction so nobody "fixes" one into
// the other.
func TestAddMonths_DriftsWhereOccurrenceDoesNot(t *testing.T) {
	d := MustNew(2026, time.January, 31)
	d = AddMonths(d, 1) // 2026-02-28
	d = AddMonths(d, 1) // 2026-03-28 — drifted
	if got := d.String(); got != "2026-03-28" {
		t.Fatalf("AddMonths chain: got %s, want 2026-03-28", got)
	}
	if got := Occurrence(MustNew(2026, time.January, 31), 31, 2).String(); got != "2026-03-31" {
		t.Fatalf("Occurrence: got %s, want 2026-03-31", got)
	}
}

func TestDaysBetween(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2026-01-01", "2026-01-31", 30},
		{"2026-01-31", "2026-03-01", 29}, // through a 28-day February
		{"2024-01-31", "2024-03-01", 30}, // through a 29-day February
		{"2026-12-31", "2027-01-01", 1},
		{"2026-03-01", "2026-01-31", -29}, // reversed
		{"2026-05-05", "2026-05-05", 0},
		{"2024-02-28", "2024-02-29", 1}, // the leap day itself
	}
	for _, c := range cases {
		a, b := mustParse(t, c.a), mustParse(t, c.b)
		if got := DaysBetween(a, b); got != c.want {
			t.Errorf("DaysBetween(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// A full year must be 365 days, or 366 in a leap year. If this ever fails, an
// entire year of interest is wrong.
func TestDaysBetween_FullYears(t *testing.T) {
	for y := 2020; y <= 2035; y++ {
		a := MustNew(y, time.January, 1)
		b := MustNew(y+1, time.January, 1)
		want := 365
		if IsLeapYear(y) {
			want = 366
		}
		if got := DaysBetween(a, b); got != want {
			t.Errorf("%d: got %d days, want %d", y, got, want)
		}
	}
}

func TestDays30360(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2026-01-01", "2026-02-01", 30},
		{"2026-01-31", "2026-02-28", 28}, // 31 becomes 30; 28 stays 28
		{"2026-01-01", "2027-01-01", 360},
		{"2026-01-31", "2026-03-31", 60},
	}
	for _, c := range cases {
		if got := Days30360(mustParse(t, c.a), mustParse(t, c.b)); got != c.want {
			t.Errorf("Days30360(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLeapYears(t *testing.T) {
	leap := map[int]bool{1900: false, 2000: true, 2024: true, 2025: false, 2026: false, 2100: false}
	for y, want := range leap {
		if got := IsLeapYear(y); got != want {
			t.Errorf("IsLeapYear(%d) = %v, want %v", y, got, want)
		}
	}
	if got := DaysInMonth(2100, time.February); got != 28 {
		t.Errorf("2100 is not a leap year: February has %d days", got)
	}
}

func TestCompareAndOrder(t *testing.T) {
	a, b := MustNew(2026, time.March, 1), MustNew(2026, time.March, 2)
	if !a.Before(b) || !b.After(a) || a.Equal(b) {
		t.Error("ordering is wrong")
	}
	same := MustNew(2026, time.March, 1)
	if a.Compare(same) != 0 {
		t.Error("two dates with the same value must compare equal")
	}
}

// The one sanctioned crossing into an instant must use the given zone and not
// silently fall back to UTC.
func TestAtLocal(t *testing.T) {
	yerevan, err := time.LoadLocation("Asia/Yerevan")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	ts := MustNew(2026, time.September, 5).AtLocal(9, 0, yerevan)
	if ts.Hour() != 9 || ts.Location().String() != "Asia/Yerevan" {
		t.Errorf("got %s, want 09:00 in Asia/Yerevan", ts)
	}
	if ts.UTC().Hour() != 5 { // Yerevan is UTC+4 year round
		t.Errorf("UTC hour = %d, want 5", ts.UTC().Hour())
	}
}

func TestParseRoundTrip(t *testing.T) {
	for _, s := range []string{"2026-01-01", "2024-02-29", "2026-12-31"} {
		d := mustParse(t, s)
		if d.String() != s {
			t.Errorf("round trip of %s produced %s", s, d.String())
		}
	}
	if _, err := Parse("2026-02-30"); err == nil {
		t.Error("Parse should reject an impossible date")
	}
	if _, err := Parse("05/09/2026"); err == nil {
		t.Error("Parse should require ISO 8601")
	}
}

func mustParse(t *testing.T, s string) Date {
	t.Helper()
	d, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return d
}
