// Package erasurejournal retains deletion intents outside PostgreSQL backups.
// The latest journal must be backed up independently and reconciled before a
// restored database serves traffic. Entries contain subject hashes, never IDs.
package erasurejournal

import (
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"sync"
	"syscall"
)

// ErrJournal reports an invalid or unavailable journal without exposing hashes.
var ErrJournal = errors.New("erasure journal unavailable or invalid")

// Journal owns immutable per-subject files in an existing private directory.
type Journal struct {
	dir      string
	identity os.FileInfo
	mu       sync.Mutex
}

// New opens no persistent descriptors and never creates a directory implicitly.
func New(dir string) (*Journal, error) {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, ErrJournal
	}
	j := &Journal{dir: dir, identity: info}
	root, err := j.open()
	if err != nil {
		return nil, err
	}
	if err := root.Close(); err != nil {
		return nil, ErrJournal
	}
	return j, nil
}

func (j *Journal) open() (*os.Root, error) {
	root, err := os.OpenRoot(j.dir)
	if err != nil {
		return nil, ErrJournal
	}
	info, err := root.Stat(".")
	link, linkErr := os.Lstat(j.dir)
	if err != nil || linkErr != nil || !link.IsDir() || !os.SameFile(info, j.identity) || info.Mode().Perm()&0o077 != 0 {
		_ = root.Close()
		return nil, ErrJournal
	}
	return root, nil
}

func validHash(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	for _, c := range hash {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func verified(root *os.Root, hash string) (*os.File, error) {
	file, err := root.OpenFile(hash, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrJournal
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != 65 || info.Mode().Perm()&0o077 != 0 {
		_ = file.Close()
		return nil, ErrJournal
	}
	body, err := io.ReadAll(io.LimitReader(file, 66))
	if err != nil || string(body) != hash+"\n" {
		_ = file.Close()
		return nil, ErrJournal
	}
	return file, nil
}

// Record returns success only after both entry contents and directory metadata
// are synced. Repeating an existing verified intent is safe and re-syncs it.
func (j *Journal) Record(ctx context.Context, subjectHash string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validHash(subjectHash) {
		return ErrJournal
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := j.open()
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	file, err := root.OpenFile(subjectHash, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		file, err = verified(root, subjectHash)
	} else if err == nil {
		_, err = io.WriteString(file, subjectHash+"\n")
	}
	if err != nil {
		if file != nil {
			_ = file.Close()
		}
		return ErrJournal
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil || closeErr != nil {
		return ErrJournal
	}
	dir, err := root.Open(".")
	if err != nil {
		return ErrJournal
	}
	syncErr = dir.Sync()
	closeErr = dir.Close()
	if syncErr != nil || closeErr != nil {
		return ErrJournal
	}
	return ctx.Err()
}

// Subjects returns every validated intent. Any unexpected entry fails closed;
// malformed, partial or symlinked entries are never silently skipped.
func (j *Journal) Subjects(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	root, err := j.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	dir, err := root.Open(".")
	if err != nil {
		return nil, ErrJournal
	}
	defer func() { _ = dir.Close() }()
	var subjects []string
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, err := dir.ReadDir(128)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, ErrJournal
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if !validHash(entry.Name()) {
				return nil, ErrJournal
			}
			file, err := verified(root, entry.Name())
			if err != nil {
				return nil, err
			}
			if err := file.Close(); err != nil {
				return nil, ErrJournal
			}
			subjects = append(subjects, entry.Name())
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	sort.Strings(subjects)
	return subjects, nil
}
