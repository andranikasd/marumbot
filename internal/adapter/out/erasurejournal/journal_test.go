package erasurejournal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func privateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRecordIsIdempotentAndListsHashes(t *testing.T) {
	dir := privateDir(t)
	j, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, hash := range []string{b, a, a} {
		if err := j.Record(context.Background(), hash); err != nil {
			t.Fatal(err)
		}
	}
	subjects, err := j.Subjects(context.Background())
	if err != nil || len(subjects) != 2 || subjects[0] != a || subjects[1] != b {
		t.Fatalf("unexpected journal result: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, a))
	if err != nil || string(body) != a+"\n" {
		t.Fatal("intent not durably represented")
	}
}

func TestCanceledJournalOperations(t *testing.T) {
	j, err := New(privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := j.Record(ctx, strings.Repeat("a", 64)); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled write accepted")
	}
	if _, err := j.Subjects(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled read accepted")
	}
}

func TestJournalRejectsMalformedSubjects(t *testing.T) {
	j, err := New(privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, hash := range []string{"", "../outside", strings.Repeat("A", 64), strings.Repeat("a", 63), strings.Repeat("g", 64)} {
		if err := j.Record(context.Background(), hash); !errors.Is(err, ErrJournal) {
			t.Fatal("invalid subject accepted")
		}
	}
}

func TestJournalFailsClosedOnUnexpectedEntries(t *testing.T) {
	for _, kind := range []string{"tampered", "oversize", "symlink", "directory", "unknown name", "public file"} {
		t.Run(kind, func(t *testing.T) {
			dir := privateDir(t)
			j, err := New(dir)
			if err != nil {
				t.Fatal(err)
			}
			hash := strings.Repeat("a", 64)
			path := filepath.Join(dir, hash)
			switch kind {
			case "symlink":
				err = os.Symlink(filepath.Join(t.TempDir(), "missing"), path)
			case "directory":
				err = os.Mkdir(path, 0o700)
			case "unknown name":
				err = os.WriteFile(filepath.Join(dir, "unexpected"), nil, 0o600)
			case "public file":
				err = os.WriteFile(path, []byte(hash+"\n"), 0o644)
			case "oversize":
				err = os.WriteFile(path, []byte(strings.Repeat("a", 1024)), 0o600)
			default:
				err = os.WriteFile(path, []byte(strings.Repeat("b", 64)+"\n"), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := j.Subjects(context.Background()); !errors.Is(err, ErrJournal) {
				t.Fatal("invalid journal entry accepted")
			}
			if kind != "unknown name" {
				if err := j.Record(context.Background(), hash); !errors.Is(err, ErrJournal) {
					t.Fatal("invalid existing intent accepted")
				}
			}
		})
	}
}

func TestJournalRequiresExistingPrivateDirectory(t *testing.T) {
	root := t.TempDir()
	if _, err := New(filepath.Join(root, "missing")); !errors.Is(err, ErrJournal) {
		t.Fatal("missing directory accepted")
	}
	public := filepath.Join(root, "public")
	if err := os.Mkdir(public, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := New(public); !errors.Is(err, ErrJournal) {
		t.Fatal("public directory accepted")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	if _, err := New(link); !errors.Is(err, ErrJournal) {
		t.Fatal("symlink directory accepted")
	}
}
