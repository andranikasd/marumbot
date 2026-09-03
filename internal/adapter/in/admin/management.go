package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/andranikasd/marumbot/internal/app"
)

const (
	unknownValue = "unknown"
	formYes      = "yes"
)

var roleChoices = []struct {
	Value app.AdminRole
	Label string
}{
	{app.AdminRoleSupportReader, "Support reader"},
	{app.AdminRoleSupportOperator, "Support operator"},
	{app.AdminRoleFinancialVerifier, "Financial verifier"},
	{app.AdminRolePolicyPublisher, "Policy publisher"},
	{app.AdminRoleOperations, "Operations"},
	{app.AdminRoleBillingOperator, "Billing operator"},
	{app.AdminRoleSecurityAuditor, "Security auditor"},
	{app.AdminRoleAdministrator, "Administrator"},
}
var caseCategories = []string{"input_error", "posting_date", "allocation_policy", "fee", "rounding", "schedule_reissue", "bank_correction", "engine_defect", unknownValue}

func (s *Server) managementPage(w http.ResponseWriter, r *http.Request, page, title, nav string, data map[string]any) {
	data[keyTitle] = title
	data[keyNav] = nav
	data["Notice"] = r.URL.Query().Get("notice")
	s.render(w, r, page, data)
}

func (s *Server) managementError(w http.ResponseWriter, r *http.Request, err error) {
	status, message := http.StatusInternalServerError, "The operation could not be completed. No successful change has been confirmed."
	switch {
	case errors.Is(err, app.ErrAdminPurposeRequired):
		status, message = http.StatusForbidden, "Set an access purpose on Identity and purpose, then return to this page."
	case errors.Is(err, app.ErrAdminStepUpRequired):
		status, message = http.StatusForbidden, "Confirm your password and a new authenticator code on Identity and purpose, then retry within five minutes."
	case errors.Is(err, app.ErrAdminAccessDenied):
		status, message = http.StatusForbidden, "Your assigned roles do not permit this action. Authors cannot approve or publish their own policies, and administrators cannot change their own roles."
	case errors.Is(err, app.ErrAdminConflict), errors.Is(err, app.ErrConflict):
		status, message = http.StatusConflict, "The saved version changed, or this version already exists. Reload the record before reviewing or saving again."
	case errors.Is(err, app.ErrAdminEvidenceRequired):
		status, message = http.StatusUnprocessableEntity, "The operation needs valid source evidence and required fields. Policy approval must be independent; case closure must cite a stored resolution."
	case errors.Is(err, app.ErrNotFound):
		status, message = http.StatusNotFound, "This record was not found. Return to its list and refresh."
	case errors.Is(err, app.ErrAdminSecurityUnavailable):
		status, message = http.StatusServiceUnavailable, "This operation is not configured. Publication needs a signing key; contact the deployment administrator."
	case errors.Is(err, app.ErrHistoricalEngine):
		status, message = http.StatusConflict, app.ErrHistoricalEngine.Error()+". This report was not recalculated using current rules."
	}
	w.WriteHeader(status)
	s.render(w, r, "error.html", map[string]any{keyTitle: "Action needs attention", "Err": message})
}

func formRevision(w http.ResponseWriter, r *http.Request) (int64, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return 0, false
	}
	value, err := strconv.ParseInt(r.PostFormValue("revision"), 10, 64)
	if err != nil || value < 0 {
		http.Error(w, "Invalid saved version; reload the record", http.StatusBadRequest)
		return 0, false
	}
	return value, true
}

func savedRedirect(w http.ResponseWriter, r *http.Request, path, message string) {
	http.Redirect(w, r, path+"?notice="+url.QueryEscape(message), http.StatusSeeOther)
}

func (s *Server) identitiesPage(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Identities(r.Context())
	if err != nil {
		s.managementError(w, r, err)
		return
	}
	selected := app.AdminIdentityView{Enabled: true}
	if id := r.PathValue("id"); id != "" {
		found := false
		for _, row := range rows {
			if row.ID == id {
				selected = row
				found = true
				break
			}
		}
		if !found {
			s.managementError(w, r, app.ErrNotFound)
			return
		}
	}
	s.managementPage(w, r, "identities.html", "Admin identities", "identities", map[string]any{"Identities": rows, "Identity": selected, "RoleChoices": roleChoices})
}

func (s *Server) saveIdentityPage(w http.ResponseWriter, r *http.Request) {
	revision, ok := formRevision(w, r)
	if !ok {
		return
	}
	id := r.PostFormValue("id")
	if revision == 0 {
		id = newUUID()
	}
	var hash string
	if password := r.PostFormValue("password"); password != "" {
		var err error
		hash, err = HashPassword(password)
		if err != nil {
			http.Error(w, "Use a password of at least 12 characters. Passwords are never echoed back.", http.StatusUnprocessableEntity)
			return
		}
	}
	roles := make([]app.AdminRole, 0, len(r.PostForm["roles"]))
	for _, role := range r.PostForm["roles"] {
		roles = append(roles, app.AdminRole(role))
	}
	err := s.admin.ChangeIdentity(r.Context(), app.AdminIdentity{ID: id, Username: strings.TrimSpace(r.PostFormValue("username")), PasswordHash: hash, Roles: roles, Enabled: r.PostFormValue("enabled") == formYes}, revision)
	if err != nil {
		s.managementError(w, r, err)
		return
	}
	savedRedirect(w, r, "/identities/"+id, "Identity saved. Existing sessions for this identity are invalidated.")
}

func (s *Server) registryPage(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.PolicyRegistry(r.Context())
	if err != nil {
		s.managementError(w, r, err)
		return
	}
	active, err := s.admin.Policies(r.Context())
	if err != nil && !errors.Is(err, app.ErrAdminAccessDenied) {
		s.managementError(w, r, err)
		return
	}
	s.managementPage(w, r, "registry.html", "Lender policies", "policies", map[string]any{"Policies": rows, "ActivePolicies": active})
}

func (s *Server) policyDetailPage(w http.ResponseWriter, r *http.Request) {
	p := app.AdminPolicy{Version: 1, Excess: unknownValue, State: app.AdminPolicyDraft}
	var err error
	if id := r.PathValue("id"); id != "" {
		p, err = s.admin.PolicyDraft(r.Context(), id)
	} else {
		err = s.admin.CheckAccess(r.Context(), app.AdminCapabilityPolicyReview, "new_policy")
	}
	if err != nil {
		s.managementError(w, r, err)
		return
	}
	var definition struct {
		Order []string `json:"order"`
	}
	_ = json.Unmarshal(p.Definition, &definition)
	s.managementPage(w, r, "policy_editor.html", "Policy review", "policies", map[string]any{"Policy": p, "Order": strings.Join(definition.Order, ", "), "ExcessRules": []string{unknownValue, "reduce_principal", "hold_as_advance", "requires_bank_request"}})
}

func (s *Server) savePolicyPage(w http.ResponseWriter, r *http.Request) {
	revision, ok := formRevision(w, r)
	if !ok {
		return
	}
	p := app.AdminPolicy{ID: newUUID()}
	if revision > 0 {
		var err error
		p, err = s.admin.PolicyDraft(r.Context(), r.PostFormValue("id"))
		if err != nil {
			s.managementError(w, r, err)
			return
		}
	}
	version, err := strconv.ParseInt(r.PostFormValue("version"), 10, 32)
	if err != nil || version < 1 {
		http.Error(w, "Policy version must be a positive integer", http.StatusUnprocessableEntity)
		return
	}
	p.Key = strings.TrimSpace(r.PostFormValue("key"))
	p.Version = int32(version)
	p.Source = strings.TrimSpace(r.PostFormValue("source"))
	p.Evidence = strings.TrimSpace(r.PostFormValue("evidence"))
	p.Excess = r.PostFormValue("excess")
	order := []string{}
	for _, bucket := range strings.Split(r.PostFormValue("order"), ",") {
		bucket = strings.TrimSpace(bucket)
		if bucket != "" {
			order = append(order, bucket)
		}
	}
	if len(order) == 0 {
		http.Error(w, "Enter the settlement buckets in the order stated by the source", http.StatusUnprocessableEntity)
		return
	}
	// Preserve non-order metadata from existing API-authored definitions.
	definition := map[string]json.RawMessage{}
	if len(p.Definition) > 0 {
		if err := json.Unmarshal(p.Definition, &definition); err != nil {
			http.Error(w, "This definition cannot be edited with the allocation-order form", http.StatusUnprocessableEntity)
			return
		}
	}
	definition["order"], _ = json.Marshal(order)
	p.Definition, err = json.Marshal(definition)
	if err != nil {
		s.managementError(w, r, err)
		return
	}
	if err := s.admin.DraftPolicy(r.Context(), p, revision); err != nil {
		s.managementError(w, r, err)
		return
	}
	savedRedirect(w, r, "/policies/"+p.ID, "Draft saved. Any previous approval has been cleared.")
}

func (s *Server) advancePolicyPage(w http.ResponseWriter, r *http.Request) {
	revision, ok := formRevision(w, r)
	if !ok {
		return
	}
	if r.PostFormValue("confirm") != formYes {
		http.Error(w, "Confirm that you reviewed this policy and its evidence", http.StatusUnprocessableEntity)
		return
	}
	id := r.PathValue("id")
	var err error
	message := "Independent review recorded."
	if strings.HasSuffix(r.URL.Path, "/publish") {
		err = s.admin.PublishPolicy(r.Context(), id, revision, r.PostFormValue("hash"))
		message = "Policy published with a signature."
	} else {
		err = s.admin.ReviewPolicy(r.Context(), id, revision, r.PostFormValue("hash"))
	}
	if err != nil {
		s.managementError(w, r, err)
		return
	}
	savedRedirect(w, r, "/policies/"+id, message)
}

func (s *Server) casesPage(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Cases(r.Context())
	if err != nil {
		s.managementError(w, r, err)
		return
	}
	s.managementPage(w, r, "cases.html", "Support cases", "cases", map[string]any{"Cases": rows, "Categories": caseCategories, "QueryUser": r.URL.Query().Get("user"), "QueryLoan": r.URL.Query().Get("loan")})
}

func (s *Server) caseDetailPage(w http.ResponseWriter, r *http.Request) {
	v, err := s.admin.CaseDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		s.managementError(w, r, err)
		return
	}
	var options []app.AdminEvidenceOption
	grants, err := s.admin.GrantedCapabilities(r.Context())
	if err != nil {
		s.managementError(w, r, err)
		return
	}
	if grants[app.AdminCapabilityReconciliation] {
		options, err = s.admin.CaseEvidenceChoices(r.Context(), v.Case)
		if err != nil {
			s.managementError(w, r, err)
			return
		}
	}
	s.managementPage(w, r, "case_detail.html", "Case review", "cases", map[string]any{"CaseView": v, "Categories": caseCategories, "EvidenceOptions": options})
}

func (s *Server) saveCasePage(w http.ResponseWriter, r *http.Request) {
	revision, ok := formRevision(w, r)
	if !ok {
		return
	}
	c := app.AdminCase{ID: newUUID(), State: "open"}
	if revision > 0 {
		v, err := s.admin.CaseDetail(r.Context(), r.PostFormValue("id"))
		if err != nil {
			s.managementError(w, r, err)
			return
		}
		c = v.Case
	}
	if revision == 0 {
		c.UserID = r.PostFormValue("user")
		c.LoanID = r.PostFormValue("loan")
	}
	c.Note = strings.TrimSpace(r.PostFormValue("note"))
	c.Category = r.PostFormValue("category")
	c.State = r.PostFormValue("state")
	c.Resolution = ""
	c.EvidenceID = ""
	if c.State == "resolved" {
		var ok bool
		c.Resolution, c.EvidenceID, ok = strings.Cut(r.PostFormValue("evidence"), ":")
		if !ok {
			http.Error(w, "Choose stored resolution evidence", http.StatusUnprocessableEntity)
			return
		}
	}
	if err := s.admin.SaveCase(r.Context(), c, revision); err != nil {
		s.managementError(w, r, err)
		return
	}
	savedRedirect(w, r, "/cases/"+c.ID, "Case saved. Previous revisions remain in the audit history.")
}

func (s *Server) flagsPage(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.ProfileFlags(r.Context())
	if err != nil {
		s.managementError(w, r, err)
		return
	}
	s.managementPage(w, r, "flags.html", "Profile flags", "flags", map[string]any{"Flags": rows})
}

func (s *Server) saveFlagPage(w http.ResponseWriter, r *http.Request) {
	revision, ok := formRevision(w, r)
	if !ok {
		return
	}
	flag := app.AdminFlag{Environment: strings.TrimSpace(r.PostFormValue("environment")), Profile: strings.TrimSpace(r.PostFormValue("profile")), PlanningEnabled: r.PostFormValue("enabled") == formYes, Reason: strings.TrimSpace(r.PostFormValue("reason"))}
	if err := s.admin.SetProfileFlag(r.Context(), flag, revision); err != nil {
		s.managementError(w, r, err)
		return
	}
	savedRedirect(w, r, "/flags", "Profile flag saved.")
}

func (s *Server) auditPage(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Audit(r.Context())
	if err != nil {
		s.managementError(w, r, err)
		return
	}
	s.managementPage(w, r, "audit.html", "Audit and access", "audit", map[string]any{"Audit": rows})
}

func (s *Server) historyPage(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("id")
	if user == "" {
		user = r.URL.Query().Get("user")
	}
	data := map[string]any{"UserID": user}
	if user != "" {
		rows, revision, err := s.admin.HistoricalPlans(r.Context(), user)
		if err != nil {
			s.managementError(w, r, err)
			return
		}
		data["History"], data["Revision"] = rows, revision
	} else if err := s.admin.CheckAccess(r.Context(), app.AdminCapabilityFinancialRead, "history"); err != nil {
		s.managementError(w, r, err)
		return
	}
	if r.Method == http.MethodPost {
		result, err := s.admin.ReplayHistoricalPlan(r.Context(), user, r.PathValue("report"))
		if err != nil {
			s.managementError(w, r, err)
			return
		}
		data["Replay"], data["ReportID"] = result, r.PathValue("report")
	}
	s.managementPage(w, r, "history.html", "Calculation history", "history", data)
}

func (s *Server) entitlementsPage(w http.ResponseWriter, r *http.Request) {
	rows, err := s.admin.Entitlements(r.Context())
	if err != nil {
		s.managementError(w, r, err)
		return
	}
	s.managementPage(w, r, "entitlements.html", "Entitlements", "entitlements", map[string]any{"Entitlements": rows})
}

func selectedRole(roles []app.AdminRole, role app.AdminRole) bool {
	return slices.Contains(roles, role)
}

func capabilityNavigation(grants map[app.AdminCapability]bool) map[string]bool {
	return map[string]bool{"identities": grants[app.AdminCapabilityIdentityRead], "identityEdit": grants[app.AdminCapabilityRoleChanges], "policyRead": grants[app.AdminCapabilityPolicyRead], "policyEdit": grants[app.AdminCapabilityPolicyReview], "policyPublish": grants[app.AdminCapabilityPolicyPublish], "cases": grants[app.AdminCapabilitySupportRead] || grants[app.AdminCapabilityFinancialRead], "caseNotes": grants[app.AdminCapabilitySupportNotes], "caseResolve": grants[app.AdminCapabilityReconciliation], "caseFinancial": grants[app.AdminCapabilityFinancialRead], "flags": grants[app.AdminCapabilityFeatureFlags], "audit": grants[app.AdminCapabilityAuditRead], "history": grants[app.AdminCapabilityFinancialRead], "replay": grants[app.AdminCapabilityFinancialRead] && grants[app.AdminCapabilitySafeReplay], "notifications": grants[app.AdminCapabilityNotifications], "entitlements": grants[app.AdminCapabilityEntitlements], "corpus": grants[app.AdminCapabilityFixtureReview]}
}

func (s *Server) securityManagementPage(w http.ResponseWriter, r *http.Request) {
	v, _ := s.session(r)
	s.managementPage(w, r, "security.html", "Identity and purpose", "security", map[string]any{"Purpose": v.Purpose, "StepUpAt": v.StepUpAt, "SigningReady": len(s.cfg.PolicySigningKey) >= 32})
}

// Keep the display text in one place, without rendering arbitrary errors.
func roleLabel(role app.AdminRole) string {
	for _, choice := range roleChoices {
		if choice.Value == role {
			return choice.Label
		}
	}
	return fmt.Sprint(role)
}
