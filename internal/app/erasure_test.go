package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type erasureStoreFake struct {
	AdminStore
	Moderation
	user  UserRow
	steps *[]string
}

func (s *erasureStoreFake) GetUser(context.Context, string) (UserRow, error) { return s.user, nil }
func (s *erasureStoreFake) DeleteUser(context.Context, string) error {
	*s.steps = append(*s.steps, "delete")
	return nil
}

type erasureJournalFake struct {
	subject string
	err     error
	steps   *[]string
}

func (j *erasureJournalFake) Record(_ context.Context, subject string) error {
	*j.steps = append(*j.steps, "record")
	j.subject = subject
	return j.err
}
func (j *erasureJournalFake) Subjects(context.Context) ([]string, error) { return nil, nil }

func TestEraseUserJournalsCanonicalStoredIDBeforeDeletion(t *testing.T) {
	const canonical = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	var steps []string
	a, _ := adminFixture()
	s := &erasureStoreFake{user: UserRow{ID: canonical, DeletionRequested: true}, steps: &steps}
	journal := &erasureJournalFake{steps: &steps}
	a.store = s
	a.WithModeration(s).WithErasureJournal(journal)
	if err := a.EraseUser(adminContext("auditor"), strings.ToUpper(canonical)); err != nil {
		t.Fatal(err)
	}
	if journal.subject != erasureSubject(canonical) {
		t.Fatal("journal hash used caller spelling instead of canonical stored ID")
	}
	if !reflect.DeepEqual(steps, []string{"record", "delete"}) {
		t.Fatalf("erasure ordering = %v", steps)
	}
}

func TestEraseUserFailsClosedWithoutDurableJournal(t *testing.T) {
	journalFailure := errors.New("journal unavailable")
	for _, tc := range []struct {
		name    string
		journal bool
		want    error
	}{
		{"missing", false, ErrErasureJournalUnavailable},
		{"failed", true, journalFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var steps []string
			a, _ := adminFixture()
			s := &erasureStoreFake{user: UserRow{ID: "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", DeletionRequested: true}, steps: &steps}
			a.store = s
			a.WithModeration(s)
			if tc.journal {
				a.WithErasureJournal(&erasureJournalFake{steps: &steps, err: journalFailure})
			}
			if err := a.EraseUser(adminContext("auditor"), s.user.ID); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			for _, step := range steps {
				if step == "delete" {
					t.Fatal("account deleted without durable journal intent")
				}
			}
		})
	}
}
