package admin

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"sort"
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
	keyQuery = "Query"
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
		"GET /search":               s.search,
		"GET /loans":                s.loans,
		"GET /loans/{id}":           s.loan,
		"POST /loans/{id}/rename":   s.renameLoan,
		"GET /users":                s.users,
		"GET /users/{id}":           s.user,
		"GET /engine":               s.engine,
		"POST /engine":              s.engine,
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
	// A detail page may ask to be returned to rather than the list. Only a
	// local path is honoured, so the parameter cannot bounce the operator to
	// another site.
	if b := r.URL.Query().Get("back"); strings.HasPrefix(b, "/") && !strings.HasPrefix(b, "//") {
		back = b
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
	if r.FormValue("sure") != "yes" {
		s.fail(w, r, fmt.Errorf("erasure needs the confirmation box ticked"))
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
	if r.FormValue("sure") != "yes" {
		http.Redirect(w, r, "/commands?status=dead", http.StatusSeeOther)
		return
	}
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
	d, err := s.admin.Dashboard(r.Context(), s.now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "dashboard.html", map[string]any{keyTitle: "Overview", keyNav: "dashboard", "D": d})
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	res, err := s.admin.Search(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	title := "Search"
	if res.Query != "" {
		title = fmt.Sprintf("Search “%s”", res.Query)
	}
	s.render(w, r, "search.html", map[string]any{keyTitle: title, keyNav: "", "R": res, keyQuery: res.Query})
}

// loanFilter is what the loans list can be narrowed by. Filtering happens in
// memory over the page the store returns: the lists are capped at a few
// hundred rows, and a filter that reached the database would need an index
// per column for no operational gain yet.
type loanFilter struct {
	Q, Currency, Reliability, Archived string
}

func (f loanFilter) keep(l app.LoanRow) bool {
	if f.Q != "" {
		q := strings.ToLower(f.Q)
		if !strings.Contains(strings.ToLower(l.Name), q) && !strings.HasPrefix(l.ID, q) && !strings.HasPrefix(l.UserID, q) {
			return false
		}
	}
	if f.Currency != "" && l.Currency != f.Currency {
		return false
	}
	if f.Reliability != "" && (l.Reliability == nil || *l.Reliability != f.Reliability) {
		return false
	}
	switch f.Archived {
	case "only":
		return l.ArchivedAt != nil
	case "all":
		return true
	default:
		return l.ArchivedAt == nil
	}
}

func (s *Server) loans(w http.ResponseWriter, r *http.Request) {
	all, err := s.admin.Loans(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	q := r.URL.Query()
	f := loanFilter{Q: q.Get("q"), Currency: q.Get("currency"), Reliability: q.Get("reliability"), Archived: q.Get("archived")}
	var rows []app.LoanRow
	curs, rels := map[string]bool{}, map[string]bool{}
	for _, l := range all {
		curs[l.Currency] = true
		if l.Reliability != nil {
			rels[*l.Reliability] = true
		}
		if f.keep(l) {
			rows = append(rows, l)
		}
	}
	s.render(w, r, "loans.html", map[string]any{
		keyTitle: "Loans", keyNav: "loans", keyRows: rows, "Total": len(all), "F": f,
		"Currencies": keys(curs), "Reliabilities": keys(rels),
	})
}

func (s *Server) renameLoan(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	name := strings.TrimSpace(r.PostFormValue("name"))
	description := strings.TrimSpace(r.PostFormValue("description"))
	var problem string
	switch {
	case name == "":
		problem = "A loan needs a name."
	case len([]rune(name)) > 60:
		problem = "Names are at most 60 characters."
	case len([]rune(description)) > 200:
		problem = "Descriptions are at most 200 characters."
	}
	if problem != "" {
		// Re-render the page with the typed values and the complaint beside
		// the field, rather than an error page the operator has to back out of.
		s.loanPage(w, r, id, map[string]any{"NameError": problem, "TypedName": name, "TypedDescription": description})
		return
	}
	if err := s.admin.RenameLoan(r.Context(), id, name, description); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/loans/"+id+"?notice="+urlEscape("Renamed."), http.StatusSeeOther)
}

func (s *Server) user(w http.ResponseWriter, r *http.Request) {
	v, err := s.admin.User(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "user.html", map[string]any{
		keyTitle: "User " + shortID(v.User.ID), keyNav: "users", "V": v, "Notice": r.URL.Query().Get("notice"),
	})
}

// engine is the playground. GET shows an example already filled in, so the
// first thing an operator sees is a working schedule rather than a blank
// form; POST projects whatever was typed.
func (s *Server) engine(w http.ResponseWriter, r *http.Request) {
	in := app.PlaygroundInput{
		Currency: "AMD", Principal: "3000000", Rate: "18", Method: "annuity", DayCount: "act365",
		Start: "2026-01-15", Maturity: "2031-01-15", Day: "15", Unit: "10",
	}
	var view app.PlaygroundView
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		f := r.PostFormValue
		in = app.PlaygroundInput{
			Currency: f("currency"), Principal: f("principal"), Rate: f("rate"), Method: f("method"),
			DayCount: f("day_count"), Start: f("start"), Maturity: f("maturity"), Day: f("day"),
			Unit: f("unit"), Balance: f("balance"), From: f("from"), Bank: f("bank"),
		}
	}
	view = app.Playground(in)
	s.render(w, r, "engine.html", map[string]any{
		keyTitle: "Engine playground", keyNav: "engine", "P": view,
		"Currencies": []string{"AMD", "USD", "EUR", "RUB"},
		"DayCounts":  []string{"act365", "act360", "30_360", "act_act"},
	})
}

// keys returns a set's members sorted, for a stable select list.
func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (s *Server) loan(w http.ResponseWriter, r *http.Request) {
	s.loanPage(w, r, r.PathValue("id"), nil)
}

// loanPage renders one loan, with any extra data a form round-trip carries.
// The schedule folds when it is long enough that the panels below it would
// otherwise be a screen's scroll away.
func (s *Server) loanPage(w http.ResponseWriter, r *http.Request, id string, extra map[string]any) {
	v, err := s.admin.Loan(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data := map[string]any{
		keyTitle: v.Loan.Name, keyNav: "loans", "V": v,
		"TypedName": v.Loan.Name, "TypedDescription": v.Loan.Description,
		"Notice": r.URL.Query().Get("notice"), "NameError": "",
		"FoldSchedule": v.Projection != nil && len(v.Projection.Rows) > 24,
	}
	for k, val := range extra {
		data[k] = val
	}
	s.render(w, r, "loan.html", data)
}

type userFilter struct{ Q, State, Deletion string }

func (f userFilter) keep(u app.UserRow) bool {
	if f.Q != "" && !strings.HasPrefix(u.ID, strings.ToLower(f.Q)) {
		return false
	}
	if f.State != "" && u.AccessState != f.State {
		return false
	}
	switch f.Deletion {
	case "requested":
		return u.DeletionRequested
	case "none":
		return !u.DeletionRequested
	}
	return true
}

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	all, err := s.admin.Users(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	q := r.URL.Query()
	f := userFilter{Q: q.Get("q"), State: q.Get("state"), Deletion: q.Get("deletion")}
	var rows []app.UserRow
	for _, u := range all {
		if f.keep(u) {
			rows = append(rows, u)
		}
	}
	s.render(w, r, "users.html", map[string]any{
		keyTitle: "Users", keyNav: "users", keyRows: rows, "Total": len(all), "F": f,
		"States": []string{"trial", "grace", "active", "paused"},
	})
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
	counts, _ := s.admin.CommandCounts(r.Context())
	s.render(w, r, "commands.html", map[string]any{
		keyTitle: "Command inbox", keyNav: "commands", keyRows: rows,
		"Status": status, "Statuses": withAll(counts, []string{"pending", "leased", "completed", "dead"}),
		"Purged": r.URL.Query().Get("purged"),
	})
}

func (s *Server) deliveries(w http.ResponseWriter, r *http.Request) {
	all, err := s.admin.Deliveries(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	status := r.URL.Query().Get("status")
	var rows []app.DeliveryRow
	for _, d := range all {
		if status == "" || d.Status == status {
			rows = append(rows, d)
		}
	}
	counts, _ := s.admin.DeliveryCounts(r.Context())
	s.render(w, r, "deliveries.html", map[string]any{
		keyTitle: "Delivery outbox", keyNav: "deliveries", keyRows: rows,
		"Status": status, "Statuses": withAll(counts, []string{"pending", "sent", "dead"}),
	})
}

// withAll turns status counts into the pill row: "all" first with the total,
// then every known status in a fixed order, then anything unexpected the
// database reports, so a new status shows up rather than being hidden.
func withAll(counts []app.StatusCount, known []string) []app.StatusCount {
	by := map[string]int64{}
	var total int64
	for _, c := range counts {
		by[c.Status] = c.N
		total += c.N
	}
	out := []app.StatusCount{{Status: "", N: total}}
	seen := map[string]bool{}
	for _, k := range known {
		out = append(out, app.StatusCount{Status: k, N: by[k]})
		seen[k] = true
	}
	for _, c := range counts {
		if !seen[c.Status] {
			out = append(out, c)
		}
	}
	return out
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
	// The clock goes in as data so relative times ("3m ago") are computed
	// against the injected instant rather than a wall the template reads.
	data["Now"] = s.now()
	data["SignedIn"] = false
	data["Badges"] = app.Overview{}
	if _, ok := data[keyQuery]; !ok {
		data[keyQuery] = ""
	}
	if _, ok := data[keyNav]; !ok {
		data[keyNav] = ""
	}
	if c, err := r.Cookie(cookieName); err == nil && valid(s.key, c.Value, s.now()) {
		data["SignedIn"] = true
		// The sidebar carries live queue counts. One counting query per page
		// is cheap, and a dead command showing up wherever the operator is
		// looking is the point of having a sidebar.
		if o, err := s.admin.Overview(r.Context()); err == nil {
			data["Badges"] = o
		}
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
