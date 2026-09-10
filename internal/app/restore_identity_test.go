package app

import (
	"context"
	"errors"
	"testing"
)

type restoreAdmins struct {
	err     error
	checked bool
}

func (a *restoreAdmins) VerifyAdminSecrets(context.Context) error { a.checked = true; return a.err }

type emptyRestoreUsers struct{}

func (emptyRestoreUsers) MenuUsers(context.Context, string, int32) ([]MenuUser, error) {
	return nil, nil
}

func TestRestoreChecksAdminCredentialsWithoutBorrowers(t *testing.T) {
	for _, failure := range []error{nil, errors.New("invalid admin ciphertext")} {
		admins := &restoreAdmins{err: failure}
		err := VerifyRestoredData(t.Context(), emptyRestoreUsers{}, nil, nil, admins)
		if !admins.checked || !errors.Is(err, failure) {
			t.Fatal("admin verification skipped or error lost")
		}
	}
	if err := VerifyRestoredData(t.Context(), emptyRestoreUsers{}, nil, nil, nil); !errors.Is(err, ErrAdminSecurityUnavailable) {
		t.Fatal("missing verifier accepted")
	}
}
