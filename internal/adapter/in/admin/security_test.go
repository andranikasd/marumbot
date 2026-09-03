package admin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/internal/app"
)

type pageSecurity struct {
	mu        sync.Mutex
	identity  app.AdminIdentity
	lastOTP   int64
	audit     []app.AdminAudit
	policies  map[string]app.AdminPolicy
	controls  map[string]app.AdminControl
	directory map[string]app.AdminIdentity
}
type pageSecurityTx struct {
	app.AdminSecurityTransaction
	store  *pageSecurity
	closed bool
}

func (f *pageSecurity) BeginAdmin(context.Context) (app.AdminSecurityTransaction, error) {
	f.mu.Lock()
	return &pageSecurityTx{store: f}, nil
}

func (f *pageSecurity) BootstrapAdmin(_ context.Context, id app.AdminIdentity) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.identity.ID == "" {
		f.identity = id
	}
	return nil
}

func (f *pageSecurity) AdminIdentityByUsername(_ context.Context, name string) (app.AdminIdentity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.identity.Username != name {
		return app.AdminIdentity{}, app.ErrNotFound
	}
	return f.identity, nil
}

func (f *pageSecurity) ConsumeAdminOTP(_ context.Context, id string, version, counter int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id != f.identity.ID || version != f.identity.Version || counter <= f.lastOTP {
		return app.ErrAdminAccessDenied
	}
	f.lastOTP = counter
	return nil
}

func (f *pageSecurityTx) Identity(_ context.Context, id string) (app.AdminIdentity, error) {
	if entry, ok := f.store.directory[id]; ok {
		return entry, nil
	}
	if f.store.identity.ID != id {
		return app.AdminIdentity{}, app.ErrNotFound
	}
	return f.store.identity, nil
}

func (f *pageSecurityTx) SaveIdentity(_ context.Context, id app.AdminIdentity, expected int64) error {
	if id.ID != f.store.identity.ID {
		if f.store.directory == nil {
			f.store.directory = map[string]app.AdminIdentity{}
		}
		if f.store.directory[id.ID].Version != expected {
			return app.ErrAdminConflict
		}
		f.store.directory[id.ID] = id
		return nil
	}
	if expected != f.store.identity.Version {
		return app.ErrAdminConflict
	}
	f.store.identity = id
	return nil
}

func (f *pageSecurityTx) AppendAudit(_ context.Context, e app.AdminAudit) error {
	f.store.audit = append(f.store.audit, e)
	return nil
}

func (f *pageSecurityTx) Commit(context.Context) error {
	if !f.closed {
		f.closed = true
		f.store.mu.Unlock()
	}
	return nil
}
func (f *pageSecurityTx) Rollback(ctx context.Context) error { return f.Commit(ctx) }
func pageIdentity(hash string) app.AdminIdentity {
	return app.AdminIdentity{ID: "op", Username: "op", PasswordHash: hash, TOTPSecret: "JBSWY3DPEHPK3PXP", Version: 1, Enabled: true, Roles: []app.AdminRole{app.AdminRoleSupportReader, app.AdminRoleSupportOperator, app.AdminRoleFinancialVerifier, app.AdminRoleOperations}}
}

func TestAdminTOTPAndReplay(t *testing.T) {
	// RFC 6238 SHA-1 test vector at 59s, truncated to six digits.
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	now := time.Unix(59, 0)
	if code := totpCode(secret, 1); code != "287082" {
		t.Fatal(code)
	}
	if _, ok := verifyTOTP(secret, "287082", now); !ok {
		t.Fatal("valid TOTP rejected")
	}
	if _, ok := verifyTOTP(secret, "287082", now.Add(2*time.Minute)); ok {
		t.Fatal("expired TOTP accepted")
	}
	if _, ok := verifyTOTP("", "", now); ok {
		t.Fatal("empty TOTP accepted")
	}
}

func TestAdminHTTPStrongAuthEnrollmentAndRevocation(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	f := &pageSecurity{lastOTP: -1}
	a := app.NewAdmin(nil).WithSecurity(f, func() time.Time { return stamp })
	s, err := New(a, Config{User: "operator", PasswordHash: hash, Now: func() time.Time { return stamp }}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	request := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	legacy := &http.Cookie{Name: cookieName, Value: issue(s.key, stamp)}
	if w := request(http.MethodGet, "/api/audit", "", legacy); w.Code != http.StatusSeeOther {
		t.Fatal("legacy cookie accepted", w.Code)
	}
	w := request(http.MethodPost, "/login", url.Values{"user": {"operator"}, "password": {"correct-horse-battery"}}.Encode(), nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/setup" {
		t.Fatal("bootstrap unavailable", w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	if w = request(http.MethodGet, "/users", "", cookie); w.Code != http.StatusSeeOther {
		t.Fatal("enrollment cookie read data", w.Code)
	}
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/setup", nil)
	r.AddCookie(cookie)
	session, ok := s.session(r)
	if !ok {
		t.Fatal("session missing")
	}
	w = request(http.MethodPost, "/setup", "otp="+totpCode(session.EnrollmentSecret, stamp.Unix()/30), cookie)
	if w.Code != http.StatusSeeOther {
		t.Fatal("enroll", w.Code, w.Body.String())
	}
	cookie = w.Result().Cookies()[0]
	if len(f.identity.Roles) != 1 || f.identity.Roles[0] != app.AdminRoleAdministrator {
		t.Fatal("bootstrap gained financial role")
	}
	if w = request(http.MethodGet, "/loans", "", cookie); w.Code != http.StatusForbidden {
		t.Fatal("administrator read financial data", w.Code)
	}
	// Enrollment consumes its TOTP; login must wait for a new code.
	w = request(http.MethodPost, "/login", url.Values{"user": {"operator"}, "password": {"correct-horse-battery"}, "otp": {totpCode(f.identity.TOTPSecret, stamp.Unix()/30)}}.Encode(), nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatal("OTP replay accepted", w.Code)
	}
	f.identity.Version++
	if w = request(http.MethodGet, "/security", "", cookie); w.Code != http.StatusUnauthorized {
		t.Fatal("revoked session survived", w.Code)
	}
}

func TestAdminRequiresSecurityWiring(t *testing.T) {
	_, err := New(app.NewAdmin(nil), Config{PasswordHash: "configured", Now: func() time.Time { return stamp }}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if !errors.Is(err, app.ErrAdminSecurityUnavailable) {
		t.Fatal(err)
	}
}
