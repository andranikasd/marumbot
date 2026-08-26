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
)

//go:embed templates/*.html
var templateFS embed.FS

// Config is everything the interface needs to stand up. Without PasswordHash
// the server refuses to start, so a misconfigured deployment has no admin
// interface rather than an open one.
type Config struct {
	User         string
	PasswordHash string
	Version      string
	Env          string
}

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
	pages, err := parsePages()
	if err != nil {
		return nil, err
	}
	return &Server{
		admin: a, cfg: cfg, key: sessionKey(cfg.PasswordHash),
		pages: pages, thr: newThrottle(), log: log, now: time.Now,
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
		"GET /{$}":            s.dashboard,
		"GET /loans":          s.loans,
		"GET /loans/{id}":     s.loan,
		"GET /users":          s.users,
		"GET /policies":       s.policies,
		"POST /policies":      s.addPolicy,
		"GET /commands":       s.commands,
		"GET /deliveries":     s.deliveries,
		"GET /reconciliation": s.reconciliation,
	}
	for pattern, h := range guarded {
		mux.Handle(pattern, s.requireSession(h))
	}
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
		next(w, r)
	})
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login.html", map[string]any{"Title": "Sign in"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	addr := clientAddr(r)
	if blocked, wait := s.thr.blocked(addr, s.now()); blocked {
		s.render(w, r, "login.html", map[string]any{
			"Title": "Sign in",
			"Error": fmt.Sprintf("Too many attempts. Try again in %s.", wait),
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
		s.log.Warn("admin sign-in refused", "addr", addr)
		s.render(w, r, "login.html", map[string]any{
			"Title": "Sign in", "Error": "Those details were not accepted.",
		})
		return
	}
	s.thr.succeed(addr)
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: issue(s.key, s.now()), Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil, MaxAge: int(sessionTTL.Seconds()),
	})
	s.log.Info("admin signed in", "addr", addr)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
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
		"Title": "Overview", "Nav": "dashboard",
		"O": overview, "Health": s.admin.Health(ctx),
	})
}

func (s *Server) loans(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Loans(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "loans.html", map[string]any{"Title": "Loans", "Nav": "loans", "Rows": rows})
}

func (s *Server) loan(w http.ResponseWriter, r *http.Request) {
	v, err := s.admin.Loan(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "loan.html", map[string]any{
		"Title": "Loan " + v.Loan.Name, "Nav": "loans", "V": v,
	})
}

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Users(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "users.html", map[string]any{"Title": "Users", "Nav": "users", "Rows": rows})
}

func (s *Server) policies(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Policies(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "policies.html", map[string]any{
		"Title": "Allocation policies", "Nav": "policies", "Rows": rows,
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
		redirectWith(w, r, "/policies", "error", "A policy needs a key, a bucket order and a source it was read from.")
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
			redirectWith(w, r, "/policies", "error", "Version must be a number.")
			return
		}
	}
	id := newUUID()
	if err := s.admin.AddPolicy(r.Context(), id, key, version, definition, excess, source); err != nil {
		redirectWith(w, r, "/policies", "error", err.Error())
		return
	}
	s.log.Info("allocation policy recorded", "key", key, "version", version)
	redirectWith(w, r, "/policies", "notice", fmt.Sprintf("Recorded %s v%d.", key, version))
}

func (s *Server) commands(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Commands(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "commands.html", map[string]any{"Title": "Command inbox", "Nav": "commands", "Rows": rows})
}

func (s *Server) deliveries(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Deliveries(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "deliveries.html", map[string]any{"Title": "Delivery outbox", "Nav": "deliveries", "Rows": rows})
}

func (s *Server) reconciliation(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Reconciliations(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "reconciliation.html", map[string]any{
		"Title": "Reconciliation", "Nav": "reconciliation", "Rows": rows,
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
		s.log.Error("no such admin page", "page", page)
		http.Error(w, "page not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Render into a buffer first: a template error halfway through would
	// otherwise leave a half-written page with a 200 already committed.
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, page, data); err != nil {
		s.log.Error("rendering admin page", "page", page, "err", err)
		http.Error(w, "the page could not be rendered", http.StatusInternalServerError)
		return
	}
	_, _ = buf.WriteTo(w)
}

// fail shows the operator what broke. This interface has one user, who is also
// the person who would read the log, so hiding the detail helps nobody.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("admin request failed", "path", r.URL.Path, "err", err)
	w.WriteHeader(http.StatusInternalServerError)
	s.render(w, r, "error.html", map[string]any{"Title": "Something broke", "Err": err.Error()})
}

func redirectWith(w http.ResponseWriter, r *http.Request, path, key, msg string) {
	u := path + "?" + key + "=" + urlEscape(msg)
	http.Redirect(w, r, u, http.StatusSeeOther)
}
