package admin

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/internal/app"
)

func (f *pageSecurityTx) Identities(context.Context) ([]app.AdminIdentityView, error) {
	rows := []app.AdminIdentity{f.store.identity}
	for _, id := range f.store.directory {
		rows = append(rows, id)
	}
	var result []app.AdminIdentityView
	for _, id := range rows {
		result = append(result, app.AdminIdentityView{ID: id.ID, Username: id.Username, Roles: id.Roles, Version: id.Version, Enabled: id.Enabled, Enrolled: id.TOTPSecret != ""})
	}
	return result, nil
}

func (f *pageSecurityTx) Policies(context.Context) ([]app.AdminPolicy, error) {
	var result []app.AdminPolicy
	for _, p := range f.store.policies {
		result = append(result, p)
	}
	return result, nil
}

func (f *pageSecurityTx) Policy(_ context.Context, id string) (app.AdminPolicy, error) {
	p, ok := f.store.policies[id]
	if !ok {
		return p, app.ErrNotFound
	}
	return p, nil
}

func (f *pageSecurityTx) SavePolicy(_ context.Context, p app.AdminPolicy, expected int64) error {
	if f.store.policies == nil {
		f.store.policies = map[string]app.AdminPolicy{}
	}
	if f.store.policies[p.ID].Revision != expected {
		return app.ErrAdminConflict
	}
	f.store.policies[p.ID] = p
	return nil
}
func (*pageSecurityTx) PublishPolicy(context.Context, app.AdminPolicy) error { return nil }
func (f *pageSecurityTx) Controls(_ context.Context, kind string) ([]app.AdminControl, error) {
	var result []app.AdminControl
	for _, c := range f.store.controls {
		if c.Kind == kind {
			result = append(result, c)
		}
	}
	return result, nil
}

func (f *pageSecurityTx) Control(_ context.Context, kind, id string) (app.AdminControl, error) {
	c, ok := f.store.controls[kind+"/"+id]
	if !ok {
		return c, app.ErrNotFound
	}
	return c, nil
}

func (f *pageSecurityTx) SaveControl(_ context.Context, c app.AdminControl, expected int64) error {
	if f.store.controls == nil {
		f.store.controls = map[string]app.AdminControl{}
	}
	key := c.Kind + "/" + c.ID
	if f.store.controls[key].Revision != expected {
		return app.ErrAdminConflict
	}
	f.store.controls[key] = c
	return nil
}

func (*pageSecurityTx) CaseEvidenceOptions(context.Context, string, string) ([]app.AdminEvidenceOption, error) {
	return []app.AdminEvidenceOption{{Kind: "anchor", ID: "snapshot-1", Label: "Bank anchor"}}, nil
}

func (*pageSecurityTx) CaseEvidence(_ context.Context, c app.AdminCase) (bool, error) {
	return c.State == "open" || (c.Resolution == "anchor" && c.EvidenceID == "snapshot-1"), nil
}

func (f *pageSecurityTx) AuditEvents(context.Context) ([]app.AdminAudit, error) {
	return f.store.audit, nil
}

type managementHistory struct{}

func (managementHistory) PlanHistory(context.Context, string) ([]app.PlanVersion, int64, error) {
	return []app.PlanVersion{{ID: "report", Currency: "AMD", CreatedAt: "2026-09-03", Manifest: app.PlanManifest{Engine: "unavailable/old", InputHash: "original-hash"}}}, 1, nil
}

func (managementHistory) PlanVersion(context.Context, string, string) (app.PlanVersion, error) {
	return app.PlanVersion{Manifest: app.PlanManifest{Engine: "unavailable/old"}}, nil
}

func managementServer(t *testing.T, roles []app.AdminRole) (*Server, *pageSecurity, *http.Cookie) {
	t.Helper()
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	identity := pageIdentity(hash)
	identity.Roles = roles
	f := &pageSecurity{identity: identity}
	a := app.NewAdmin(fakeStore{}).WithModeration(fakeStore{}).WithSecurity(f, func() time.Time { return stamp }).WithHistory(managementHistory{})
	s, err := New(a, Config{User: "op", PasswordHash: hash, Now: func() time.Time { return stamp }, PolicySigningKey: []byte(strings.Repeat("k", 32))}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	s.saveSession(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil), browserSession{IdentityID: "op", Username: "op", Version: 1, Strong: true, StepUpAt: stamp, Expires: stamp.Add(sessionTTL), Purpose: "review support case"})
	return s, f, recorder.Result().Cookies()[0]
}

func managementRequest(s *Server, cookie *http.Cookie, method, path string, form url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestAdminManagementPagesAndRoleBoundaries(t *testing.T) {
	roles := []app.AdminRole{}
	for _, choice := range roleChoices {
		roles = append(roles, choice.Value)
	}
	s, f, cookie := managementServer(t, roles)
	f.policies = map[string]app.AdminPolicy{"p": {ID: "p", Key: "lender", Version: 1, State: app.AdminPolicyReviewed, Revision: 2, Author: "someone-else", Reviewer: "reviewer", Definition: []byte(`{"order":["costs","principal"]}`), Source: "source", Evidence: "fixture"}}
	body, _ := json.Marshal(app.AdminCase{ID: "c", UserID: fakeUser, LoanID: fakeLoan, State: "open", Category: "unknown", Note: "internal note"})
	flag, _ := json.Marshal(app.AdminFlag{Environment: "test", Profile: "profile", Reason: "review", PlanningEnabled: false})
	f.controls = map[string]app.AdminControl{"case/c": {Kind: "case", ID: "c", Revision: 1, Body: body}, "profile_flag/f": {Kind: "profile_flag", ID: "f", Revision: 1, Body: flag}}
	for path, want := range map[string]string{"/security": "Set purpose", "/identities": "Identity directory", "/identities/op": "cannot change your own", "/policies": "lender", "/policies/p": "Sign and publish", "/policies/new": "Save draft", "/cases": "unknown", "/cases/c": "Bank anchor", "/flags": "profile", "/audit": "review support case", "/history?user=" + fakeUser: "original-hash", "/entitlements": "Trial ends", "/corpus": "provisional", "/corpus/inecobank-consumer-M26-029210-original.json": "501300200"} {
		t.Run(path, func(t *testing.T) {
			w := managementRequest(s, cookie, http.MethodGet, path, nil)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), want) {
				t.Fatalf("status=%d want=%q body=%s", w.Code, want, w.Body.String())
			}
			if strings.Contains(w.Body.String(), f.identity.PasswordHash) || strings.Contains(w.Body.String(), f.identity.TOTPSecret) {
				t.Fatal("credential rendered")
			}
		})
	}
	w := managementRequest(s, cookie, http.MethodPost, "/users/"+fakeUser+"/history/report/replay", nil)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "not recalculated") {
		t.Fatal(w.Code, w.Body.String())
	}
	f.identity.Roles = []app.AdminRole{app.AdminRoleAdministrator}
	for _, path := range []string{"/cases", "/corpus", "/policies", "/history?user=" + fakeUser, "/entitlements"} {
		if w := managementRequest(s, cookie, http.MethodGet, path, nil); w.Code != http.StatusForbidden {
			t.Fatal("administrator gained financial access", path, w.Code)
		}
	}
	f.identity.Roles = []app.AdminRole{app.AdminRoleSupportReader}
	w = managementRequest(s, cookie, http.MethodGet, "/cases/c", nil)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "internal note") || strings.Contains(w.Body.String(), "snapshot-1") {
		t.Fatal("case redaction", w.Code, w.Body.String())
	}
}

func TestAdminManagementFormsPersistAndRejectStaleVersions(t *testing.T) {
	roles := []app.AdminRole{}
	for _, choice := range roleChoices {
		roles = append(roles, choice.Value)
	}
	s, f, cookie := managementServer(t, roles)
	form := url.Values{"revision": {"0"}, "environment": {"test"}, "profile": {"ineco"}, "reason": {"review failed"}}
	w := managementRequest(s, cookie, http.MethodPost, "/flags", form)
	if w.Code != http.StatusSeeOther {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := managementRequest(s, cookie, http.MethodPost, "/flags", form); w.Code != http.StatusConflict {
		t.Fatal("stale form accepted", w.Code)
	}
	form = url.Values{"revision": {"0"}, "username": {"new-operator"}, "password": {"new-operator-password"}, "roles": {"support_reader"}, "enabled": {"yes"}}
	w = managementRequest(s, cookie, http.MethodPost, "/identities", form)
	if w.Code != http.StatusSeeOther {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(f.directory) != 1 {
		t.Fatal("identity not created")
	}
	form = url.Values{"revision": {"0"}, "key": {"example"}, "version": {"1"}, "order": {"costs, interest, principal"}, "source": {"contract"}, "evidence": {"fixture"}, "excess": {"unknown"}}
	w = managementRequest(s, cookie, http.MethodPost, "/policies/save", form)
	if w.Code != http.StatusSeeOther {
		t.Fatal(w.Code, w.Body.String())
	}
	var p app.AdminPolicy
	for _, stored := range f.policies {
		p = stored
	}
	review := url.Values{"revision": {"1"}, "hash": {p.ContentHash}, "confirm": {"yes"}}
	w = managementRequest(s, cookie, http.MethodPost, "/policies/"+p.ID+"/review", review)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatal("self review accepted", w.Code)
	}
	f.policies[p.ID] = func() app.AdminPolicy { p.Author = "different-author"; return p }()
	w = managementRequest(s, cookie, http.MethodPost, "/policies/"+p.ID+"/review", review)
	if w.Code != http.StatusSeeOther {
		t.Fatal("independent review", w.Code, w.Body.String())
	}
	review.Set("revision", "2")
	w = managementRequest(s, cookie, http.MethodPost, "/policies/"+p.ID+"/publish", review)
	if w.Code != http.StatusSeeOther || f.policies[p.ID].Signature == "" {
		t.Fatal("publish", w.Code, w.Body.String())
	}
	form = url.Values{"revision": {"0"}, "user": {fakeUser}, "loan": {fakeLoan}, "category": {"unknown"}, "note": {"first note"}, "state": {"open"}}
	w = managementRequest(s, cookie, http.MethodPost, "/cases", form)
	if w.Code != http.StatusSeeOther {
		t.Fatal(w.Code, w.Body.String())
	}
	id := strings.Split(strings.TrimPrefix(w.Header().Get("Location"), "/cases/"), "?")[0]
	form.Set("revision", "1")
	form.Set("id", id)
	form.Set("state", "resolved")
	form.Set("evidence", "anchor:snapshot-1")
	form.Set("note", "anchor accepted")
	w = managementRequest(s, cookie, http.MethodPost, "/cases", form)
	if w.Code != http.StatusSeeOther {
		t.Fatal(w.Code, w.Body.String())
	}
	c := f.controls["case/"+id]
	if !strings.Contains(string(c.Body), "first note") || !strings.Contains(string(c.Body), "anchor accepted") {
		t.Fatal("notes overwritten")
	}
}

func TestAdminEmbeddedCorpusMatchesSource(t *testing.T) {
	entries, err := evidenceFS.ReadDir("evidence")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		want, err := os.ReadFile(filepath.Join("../../../../testdata/golden", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		got, err := evidenceFS.ReadFile("evidence/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("refresh embedded evidence snapshot: %s", entry.Name())
		}
	}
	if _, err := (embeddedCorpus{}).Fixtures(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := (embeddedCorpus{}).Fixture(context.Background(), "../../secrets"); err == nil {
		t.Fatal("arbitrary file read")
	}
}
