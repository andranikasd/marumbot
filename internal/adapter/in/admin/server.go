package admin

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/internal/obs"
)

//go:embed templates/*.html
var templateFS embed.FS

// Template data keys, named so a typo is a compile error rather than a blank
// page.
const (
	keyTitle = "Title"
	keyNav   = "Nav"
	keyRows  = "Rows"
)

// Config is everything the interface needs to stand up. Without PasswordHash
// the server refuses to start, so a misconfigured deployment has no admin
// interface rather than an open one.
type Config struct {
	User         string
	PasswordHash string
	Version      string
	Env          string

	// Now supplies the current instant. Session expiry is a business decision,
	// so the clock is injected rather than read here - the same reason every
	// other component takes one.
	Now func() time.Time
}

// Server is the private admin interface.
type Server struct {
	admin *app.Admin
	cfg   Config
	key   []byte
	pages map[string]*template.Template
	thr   *throttle
	log   *slog.Logger
	now   func() time.Time
}

// New builds the interface. It returns an error rather than a disabled server
// when misconfigured, so the caller decides whether that is fatal.
func New(a *app.Admin, cfg Config, log *slog.Logger) (*Server, error) {
	if cfg.PasswordHash == "" {
		return nil, fmt.Errorf("admin interface needs MARUM_ADMIN_PASSWORD_HASH")
	}
	if cfg.Now == nil {
		return nil, fmt.Errorf("admin interface needs a clock")
	}
	pages, err := parsePages()
	if err != nil {
		return nil, err
	}
	return &Server{
		admin: a, cfg: cfg, key: sessionKey(cfg.PasswordHash),
		pages: pages, thr: newThrottle(), log: log, now: cfg.Now,
	}, nil
}

// parsePages builds one template set per page.
//
// Every page defines a template called "content", so parsing them all into a
// single set makes the last file parsed win and every page renders as that one.
// Each page therefore gets its own set containing the layout and itself.
func parsePages() (map[string]*template.Template, error) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("reading admin templates: %w", err)
	}
	pages := map[string]*template.Template{}
	for _, e := range entries {
		name := e.Name()
		if name == "layout.html" || name == "style.html" || !strings.HasSuffix(name, ".html") {
			continue
		}
		t, err := template.New(name).Funcs(funcs()).
			ParseFS(templateFS, "templates/layout.html", "templates/"+name)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", name, err)
		}
		pages[name] = t
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no admin templates found")
	}
	return pages, nil
}

// Handler returns the routed interface.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", s.loginForm)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("GET /style.css", s.stylesheet)

	guarded := map[string]http.HandlerFunc{
		"GET /{$}":                  s.dashboard,
		"GET /loans":                s.loans,
		"GET /loans/{id}":           s.loan,
		"GET /users":                s.users,
		"GET /policies":             s.policies,
		"POST /policies":            s.addPolicy,
		"GET /commands":             s.commands,
		"POST /commands/{id}/retry": s.retryCommand,
		"POST /commands/purge-dead": s.purgeDead,
		"POST /users/{id}/pause":    s.pauseUser,
		"POST /users/{id}/restore":  s.restoreUser,
		"POST /users/{id}/erase":    s.eraseUser,
		"POST /loans/{id}/archive":  s.archiveLoan,
		"POST /loans/{id}/restore":  s.restoreLoan,
		"GET /deliveries":           s.deliveries,
		"GET /reconciliation":       s.reconciliation,
	}
	for pattern, h := range guarded {
		mux.Handle(pattern, s.requireSession(h))
	}
	_ = obs.ComponentAdmin // component spans are opened per handler below
	return securityHeaders(mux)
}

// securityHeaders locks the page down. The interface loads no third-party
// script, font or image, so the policy can be absolute rather than negotiated.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireSession(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil || !valid(s.key, c.Value, s.now()) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		// SERVER, and under the admin service name: this is the entry point of
		// the admin component, so the graph shows traffic arriving at it
		// rather than at an undifferentiated "marum".
		ctx, span := obs.ComponentAdmin.Enter(r.Context(), strings.TrimPrefix(r.URL.Path, "/"))
		defer span.End()
		next(w, r.WithContext(ctx))
	})
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login.html", map[string]any{keyTitle: "Sign in"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	addr := clientAddr(r)
	if blocked, wait := s.thr.blocked(addr, s.now()); blocked {
		s.render(w, r, "login.html", map[string]any{
			keyTitle: "Sign in",
			"Error":  fmt.Sprintf("Too many attempts. Try again in %s.", wait),
		})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	user, pass := r.PostFormValue("user"), r.PostFormValue("password")
	// Both checks always run: failing early on the username would leak which
	// half was wrong through the response time.
	userOK := user == s.cfg.User
	passOK := verifyPassword(s.cfg.PasswordHash, pass)
	if !userOK || !passOK {
		s.thr.fail(addr, s.now())
		s.log.WarnContext(r.Context(), "admin sign-in refused", "addr", addr)
		s.render(w, r, "login.html", map[string]any{
			keyTitle: "Sign in", "Error": "Those details were not accepted.",
		})
		return
	}
	s.thr.succeed(addr)
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: issue(s.key, s.now()), Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil, MaxAge: int(sessionTTL.Seconds()),
	})
	s.log.InfoContext(r.Context(), "admin signed in", "addr", addr)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// The moderation actions.
//
// All POST, never GET: a link that suspends an account is a link a crawler can
// follow, and a preview fetch must never change anything. Each redirects back
// to the list it came from, so the operator sees the result rather than a
// success page telling them about it.
func (s *Server) act(w http.ResponseWriter, r *http.Request, back string, fn func(string) error) {
	if err := fn(r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

func (s *Server) pauseUser(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, "/users", func(id string) error { return s.admin.PauseUser(r.Context(), id) })
}

func (s *Server) restoreUser(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, "/users", func(id string) error { return s.admin.RestoreUser(r.Context(), id) })
}

// eraseUser is the only irreversible action here, so it takes two steps: the
// first records a request, the second honours it. One click that destroys an
// account and the evidence for destroying it is a click somebody makes by
// accident exactly once.
func (s *Server) eraseUser(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("confirm") != "erase" {
		s.act(w, r, "/users", func(id string) error { return s.admin.RequestDeletion(r.Context(), id) })
		return
	}
	s.act(w, r, "/users", func(id string) error { return s.admin.EraseUser(r.Context(), id) })
}

func (s *Server) archiveLoan(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, "/loans", func(id string) error { return s.admin.ArchiveLoan(r.Context(), id) })
}

func (s *Server) restoreLoan(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, "/loans", func(id string) error { return s.admin.RestoreLoan(r.Context(), id) })
}

// purgeDead is the one bulk deletion in the admin. It touches only rows already
// marked dead, and the count removed is shown on the redirect so the operator
// sees what happened rather than a page that merely reloaded.
func (s *Server) purgeDead(w http.ResponseWriter, r *http.Request) {
	n, err := s.admin.PurgeDead(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/commands?purged=%d", n), http.StatusSeeOther)
}

func (s *Server) retryCommand(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, "/commands", func(id string) error { return s.admin.Retry(r.Context(), id) })
}

func (s *Server) stylesheet(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	b, err := templateFS.ReadFile("templates/style.html")
	if err != nil {
		http.Error(w, "stylesheet unavailable", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(b)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	overview, err := s.admin.Overview(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "dashboard.html", map[string]any{
		keyTitle: "Overview", keyNav: "dashboard",
		"O": overview, "Health": s.admin.Health(ctx),
	})
}

func (s *Server) loans(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Loans(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "loans.html", map[string]any{keyTitle: "Loans", keyNav: "loans", keyRows: rows})
}

func (s *Server) loan(w http.ResponseWriter, r *http.Request) {
	v, err := s.admin.Loan(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "loan.html", map[string]any{
		keyTitle: "Loan " + v.Loan.Name, keyNav: "loans", "V": v,
	})
}

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Users(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "users.html", map[string]any{keyTitle: "Users", keyNav: "users", keyRows: rows})
}

func (s *Server) policies(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Policies(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "policies.html", map[string]any{
		keyTitle: "Allocation policies", keyNav: "policies", keyRows: rows,
		"Notice": r.URL.Query().Get("notice"), "Error": r.URL.Query().Get("error"),
	})
}

func (s *Server) addPolicy(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	key := r.PostFormValue("policy_key")
	source := r.PostFormValue("source")
	excess := r.PostFormValue("excess_rule")
	order := r.PostForm["order"]

	if key == "" || source == "" || len(order) == 0 {
		redirectBack(w, r, "error", "A policy needs a key, a bucket order and a source it was read from.")
		return
	}
	definition, err := json.Marshal(map[string]any{"order": order})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	version := int32(1)
	if v := r.PostFormValue("version"); v != "" {
		if _, scanErr := fmt.Sscanf(v, "%d", &version); scanErr != nil {
			redirectBack(w, r, "error", "Version must be a number.")
			return
		}
	}
	id := newUUID()
	if err := s.admin.AddPolicy(r.Context(), id, key, version, definition, excess, source); err != nil {
		redirectBack(w, r, "error", err.Error())
		return
	}
	s.log.InfoContext(r.Context(), "allocation policy recorded", "key", key, "version", version)
	redirectBack(w, r, "notice", fmt.Sprintf("Recorded %s v%d.", key, version))
}

func (s *Server) commands(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	rows, err := s.admin.Queue(r.Context(), status)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "commands.html", map[string]any{
		keyTitle: "Command inbox", keyNav: "commands", keyRows: rows,
		"Status": status, "Statuses": []string{"", "pending", "leased", "completed", "dead"},
		"Purged": r.URL.Query().Get("purged"),
	})
}

func (s *Server) deliveries(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Deliveries(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "deliveries.html", map[string]any{keyTitle: "Delivery outbox", keyNav: "deliveries", keyRows: rows})
}

func (s *Server) reconciliation(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Reconciliations(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "reconciliation.html", map[string]any{
		keyTitle: "Reconciliation", keyNav: "reconciliation", keyRows: rows,
	})
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, page string, data map[string]any) {
	data["Env"] = s.cfg.Env
	data["Version"] = s.cfg.Version
	data["SignedIn"] = false
	if c, err := r.Cookie(cookieName); err == nil && valid(s.key, c.Value, s.now()) {
		data["SignedIn"] = true
	}
	t, ok := s.pages[page]
	if !ok {
		s.log.ErrorContext(r.Context(), "no such admin page", "page", page)
		http.Error(w, "page not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Render into a buffer first: a template error halfway through would
	// otherwise leave a half-written page with a 200 already committed.
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, page, data); err != nil {
		s.log.ErrorContext(r.Context(), "rendering admin page", "page", page, "err", err)
		http.Error(w, "the page could not be rendered", http.StatusInternalServerError)
		return
	}
	_, _ = buf.WriteTo(w)
}

// fail shows the operator what broke. This interface has one user, who is also
// the person who would read the log, so hiding the detail helps nobody.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.log.ErrorContext(r.Context(), "admin request failed", "path", r.URL.Path, "err", err)
	w.WriteHeader(http.StatusInternalServerError)
	s.render(w, r, "error.html", map[string]any{keyTitle: "Something broke", "Err": err.Error()})
}

// redirectBack returns the operator to the page they submitted from, carrying
// a message. Post-redirect-get, so a refresh does not resubmit the form.
func redirectBack(w http.ResponseWriter, r *http.Request, key, msg string) {
	http.Redirect(w, r, r.URL.Path+"?"+key+"="+urlEscape(msg), http.StatusSeeOther)
}
