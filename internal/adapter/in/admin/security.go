package admin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // RFC 6238's interoperable TOTP algorithm, not a password hash.
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/andranikasd/marumbot/internal/app"
)

type browserSession struct {
	IdentityID       string
	Username         string
	Version          int64
	Strong           bool
	StepUpAt         time.Time
	Expires          time.Time
	EnrollmentSecret string
	Purpose          string
}
type sessions struct {
	mu     sync.Mutex
	values map[string]browserSession
}

func (s *Server) session(r *http.Request) (browserSession, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return browserSession{}, false
	}
	s.sessions.mu.Lock()
	defer s.sessions.mu.Unlock()
	v, ok := s.sessions.values[c.Value]
	if !ok || !s.now().Before(v.Expires) {
		delete(s.sessions.values, c.Value)
		return browserSession{}, false
	}
	return v, true
}

func (s *Server) saveSession(w http.ResponseWriter, r *http.Request, v browserSession) {
	token := randomToken(32)
	s.sessions.mu.Lock()
	for key, old := range s.sessions.values {
		if !s.now().Before(old.Expires) {
			delete(s.sessions.values, key)
		}
	}
	if c, err := r.Cookie(cookieName); err == nil {
		delete(s.sessions.values, c.Value)
	}
	s.sessions.values[token] = v
	s.sessions.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.cfg.Env == "prod" || r.TLS != nil, MaxAge: int(sessionTTL.Seconds())})
}

func randomToken(n int) string {
	b := make([]byte, n)
	// crypto/rand.Read is guaranteed to fill the buffer or terminate the process.
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func newTOTPSecret() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
}

func totpCode(secret string, counter int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return ""
	}
	var input [8]byte
	binary.BigEndian.PutUint64(input[:], uint64(counter))
	h := hmac.New(sha1.New, key)
	h.Write(input[:])
	sum := h.Sum(nil)
	offset := sum[len(sum)-1] & 15
	value := (binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff) % 1000000
	return fmt.Sprintf("%06d", value)
}

func verifyTOTP(secret, code string, now time.Time) (int64, bool) {
	if secret == "" || len(code) != 6 {
		return 0, false
	}
	counter := now.Unix() / 30
	for _, candidate := range []int64{counter, counter - 1, counter + 1} {
		if subtle.ConstantTimeCompare([]byte(code), []byte(totpCode(secret, candidate))) == 1 {
			return candidate, true
		}
	}
	return 0, false
}

func (s *Server) securityLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	addr := clientAddr(r)
	if blocked, wait := s.thr.blocked(addr, s.now()); blocked {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(wait.Seconds()))))
		http.Error(w, "try again later", http.StatusTooManyRequests)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	id, err := s.admin.LoginIdentity(r.Context(), r.PostFormValue("user"))
	hash := id.PasswordHash
	if hash == "" {
		hash = s.cfg.PasswordHash
	}
	passOK := verifyPassword(hash, r.PostFormValue("password"))
	if err != nil || !id.Enabled || !passOK {
		s.loginDenied(w, r)
		return
	}
	v := browserSession{IdentityID: id.ID, Username: id.Username, Version: id.Version, Expires: s.now().Add(sessionTTL)}
	if id.TOTPSecret == "" {
		v.EnrollmentSecret = newTOTPSecret()
		v.Expires = s.now().Add(5 * time.Minute)
		s.saveSession(w, r, v)
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	counter, ok := verifyTOTP(id.TOTPSecret, r.PostFormValue("otp"), s.now())
	if !ok || s.admin.ConsumeOTP(r.Context(), id.ID, id.Version, counter) != nil {
		s.loginDenied(w, r)
		return
	}
	if err := s.admin.RecordAuthentication(r.Context(), id.ID, "authenticated"); err != nil {
		s.loginDenied(w, r)
		return
	}
	v.Strong = true
	v.StepUpAt = s.now()
	s.thr.succeed(addr)
	s.saveSession(w, r, v)
	http.Redirect(w, r, "/security", http.StatusSeeOther)
}

func (s *Server) loginDenied(w http.ResponseWriter, r *http.Request) {
	_ = s.admin.RecordAuthentication(r.Context(), "", "denied")
	s.thr.fail(clientAddr(r), s.now())
	http.Error(w, "sign-in refused", http.StatusUnauthorized)
}

func sessionContext(r *http.Request, v browserSession) *http.Request {
	purpose := r.Header.Get("X-Admin-Purpose")
	if purpose == "" {
		purpose = v.Purpose
	}
	return r.WithContext(app.WithAdminSession(r.Context(), app.AdminSession{IdentityID: v.IdentityID, Version: v.Version, Strong: v.Strong, StepUpAt: v.StepUpAt, Purpose: purpose}))
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	v, ok := s.session(r)
	if !ok || v.Strong || v.EnrollmentSecret == "" {
		http.Error(w, "enrollment session required", http.StatusUnauthorized)
		return
	}
	if r.Method == http.MethodPost {
		if blocked, _ := s.thr.blocked(clientAddr(r), s.now()); blocked {
			http.Error(w, "try again later", http.StatusTooManyRequests)
			return
		}
		counter, ok := verifyTOTP(v.EnrollmentSecret, r.FormValue("otp"), s.now())
		if !ok {
			s.loginDenied(w, r)
			return
		}
		if err := s.admin.EnrollIdentity(sessionContext(r, v).Context(), v.EnrollmentSecret); err != nil {
			s.fail(w, r, err)
			return
		}
		v.Version++
		v.Strong = true
		v.StepUpAt = s.now()
		v.Expires = s.now().Add(sessionTTL)
		v.EnrollmentSecret = ""
		if err := s.admin.ConsumeOTP(r.Context(), v.IdentityID, v.Version, counter); err != nil {
			s.fail(w, r, err)
			return
		}
		s.saveSession(w, r, v)
		http.Redirect(w, r, "/security", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html><title>Set up authenticator</title><h1>Set up your authenticator</h1><p>Save this secret in your authenticator, then enter its code.</p><code>%s</code><form method="post"><input name="otp" inputmode="numeric" autocomplete="one-time-code" required><button>Verify and enable</button></form>`, v.EnrollmentSecret)
}

func (s *Server) stepUp(w http.ResponseWriter, r *http.Request) {
	v, ok := s.session(r)
	if !ok || !v.Strong {
		http.Error(w, "sign-in required", http.StatusUnauthorized)
		return
	}
	if blocked, _ := s.thr.blocked(clientAddr(r), s.now()); blocked {
		http.Error(w, "try again later", http.StatusTooManyRequests)
		return
	}
	id, err := s.admin.LoginIdentity(r.Context(), v.Username)
	counter, otpOK := verifyTOTP(id.TOTPSecret, r.FormValue("otp"), s.now())
	if err != nil || id.Version != v.Version || !id.Enabled || !verifyPassword(id.PasswordHash, r.FormValue("password")) || !otpOK {
		s.loginDenied(w, r)
		return
	}
	if err := s.admin.ConsumeOTP(r.Context(), id.ID, id.Version, counter); err != nil {
		s.loginDenied(w, r)
		return
	}
	if err := s.admin.RecordAuthentication(r.Context(), id.ID, "step_up"); err != nil {
		s.loginDenied(w, r)
		return
	}
	v.StepUpAt = s.now()
	s.saveSession(w, r, v)
	http.Redirect(w, r, "/security", http.StatusSeeOther)
}

func decodeAdminJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return false
	}
	return true
}

func writeAdminJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) identityAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID              string
		Username        string
		Password        string
		Roles           []app.AdminRole
		Enabled         bool
		ExpectedVersion int64
	}
	if !decodeAdminJSON(w, r, &in) {
		return
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		http.Error(w, "password must have at least 12 characters", http.StatusBadRequest)
		return
	}
	err = s.admin.ChangeIdentity(r.Context(), app.AdminIdentity{ID: in.ID, Username: in.Username, PasswordHash: hash, Roles: in.Roles, Enabled: in.Enabled}, in.ExpectedVersion)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) policyAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		p, err := s.admin.PolicyDraft(r.Context(), r.PathValue("id"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeAdminJSON(w, p)
		return
	}
	var in struct {
		Policy          app.AdminPolicy
		ExpectedVersion int64
		ContentHash     string
	}
	if !decodeAdminJSON(w, r, &in) {
		return
	}
	var err error
	switch {
	case strings.HasSuffix(r.URL.Path, "/review"):
		err = s.admin.ReviewPolicy(r.Context(), r.PathValue("id"), in.ExpectedVersion, in.ContentHash)
	case strings.HasSuffix(r.URL.Path, "/publish"):
		err = s.admin.PublishPolicy(r.Context(), r.PathValue("id"), in.ExpectedVersion, in.ContentHash)
	default:
		err = s.admin.DraftPolicy(r.Context(), in.Policy, in.ExpectedVersion)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) auditAPI(w http.ResponseWriter, r *http.Request) {
	entries, err := s.admin.Audit(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeAdminJSON(w, entries)
}

func (s *Server) historyAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		result, err := s.admin.ReplayHistoricalPlan(r.Context(), r.PathValue("id"), r.PathValue("report"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeAdminJSON(w, result)
		return
	}
	rows, revision, err := s.admin.HistoricalPlans(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeAdminJSON(w, struct {
		Plans    []app.PlanVersion
		Revision int64
	}{rows, revision})
}

func policySigner(key []byte) func(string) (string, error) {
	secret := append([]byte(nil), key...)
	return func(hash string) (string, error) {
		h := hmac.New(sha256.New, secret)
		h.Write([]byte("marum-admin-policy-v1:" + hash))
		return "hmac-sha256:" + base64.RawURLEncoding.EncodeToString(h.Sum(nil)), nil
	}
}

func (s *Server) caseAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		c, revision, err := s.admin.SupportCase(r.Context(), r.PathValue("id"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeAdminJSON(w, struct {
			Case     app.AdminCase
			Revision int64
		}{c, revision})
		return
	}
	var in struct {
		Case            app.AdminCase
		ExpectedVersion int64
	}
	if !decodeAdminJSON(w, r, &in) {
		return
	}
	if err := s.admin.SaveCase(r.Context(), in.Case, in.ExpectedVersion); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) flagAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Flag            app.AdminFlag
		ExpectedVersion int64
	}
	if !decodeAdminJSON(w, r, &in) {
		return
	}
	if err := s.admin.SetProfileFlag(r.Context(), in.Flag, in.ExpectedVersion); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) purpose(w http.ResponseWriter, r *http.Request) {
	v, ok := s.session(r)
	if !ok || !v.Strong {
		http.Error(w, "sign-in required", http.StatusUnauthorized)
		return
	}
	value := r.FormValue("purpose")
	if strings.TrimSpace(value) == "" || len(value) > 512 || !utf8.ValidString(value) {
		http.Error(w, "purpose required (up to 512 bytes)", http.StatusBadRequest)
		return
	}
	v.Purpose = value
	s.saveSession(w, r, v)
	http.Redirect(w, r, "/security", http.StatusSeeOther)
}
