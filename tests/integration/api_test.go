package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pauloeloy/databricks-governance-service/internal/adapters/memory"
	"github.com/pauloeloy/databricks-governance-service/internal/domain"
	"github.com/pauloeloy/databricks-governance-service/internal/service"
	transport "github.com/pauloeloy/databricks-governance-service/internal/transport/http"
)

func setupTestServer() http.Handler {
	workspaceRegistry := memory.NewMemoryWorkspaceRegistry([]domain.Workspace{
		{
			ID:                   "prod-br",
			DisplayName:          "Produção Brasil",
			DatabricksWorkspaceID: "7474652819690760",
			MetastoreID:          "meta-sa-east-1",
			Cloud:                "AWS",
			Region:               "sa-east-1",
			Status:               "ACTIVE",
		},
	})

	protectedRegistry := domain.NewProtectedRegistry(map[string][]string{
		"prod-br": {"sp-governance-service"},
	})

	auditStore := memory.NewMemoryAuditStore()
	idempotencyStore := memory.NewMemoryIdempotencyStore()
	databricksClient := memory.NewMemoryDatabricksClient()

	grantService := service.NewGrantService(
		databricksClient,
		databricksClient,
		protectedRegistry,
		auditStore,
		workspaceRegistry,
	)

	catalogService := service.NewCatalogService(
		databricksClient,
		databricksClient,
		databricksClient,
		databricksClient,
		databricksClient,
		databricksClient,
		databricksClient,
		databricksClient,
		auditStore,
		workspaceRegistry,
	)

	auditService := service.NewAuditService(auditStore)
	workspaceService := service.NewWorkspaceService(workspaceRegistry)

	handler := transport.NewHandler(
		grantService,
		catalogService,
		auditService,
		workspaceService,
		idempotencyStore,
	)

	return transport.NewRouter(handler)
}

func TestHealthProbes(t *testing.T) {
	router := setupTestServer()

	for _, path := range []string{"/healthz", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s returned status %d; want 200", path, rec.Code)
		}
	}
}

func TestMissingChangeRefHeaderOnMutation(t *testing.T) {
	router := setupTestServer()

	// Mutation POST sem X-Change-Ref deve retornar 400
	body := bytes.NewBufferString(`{"name": "test_cat"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/workspaces/prod-br/catalogs", body)
	req.Header.Set("Idempotency-Key", "idemp-001")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for missing X-Change-Ref, got %d", rec.Code)
	}

	var prob domain.ProblemError
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("failed to decode problem json: %v", err)
	}

	if prob.Type != "/problems/bad-request" {
		t.Errorf("expected type /problems/bad-request, got %s", prob.Type)
	}
}

func TestCatalogCreateAndIdempotency(t *testing.T) {
	router := setupTestServer()

	catBody := bytes.NewBufferString(`{"name": "sales_cat"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/workspaces/prod-br/catalogs", catBody)
	req.Header.Set("X-Change-Ref", "TICKET-123")
	req.Header.Set("Idempotency-Key", "idemp-sales-01")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 for catalog creation, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	if loc := rec.Header().Get("Location"); loc != "/v1/workspaces/prod-br/catalogs/sales_cat" {
		t.Errorf("unexpected Location header: %s", loc)
	}

	// Replay da mesma requisição com a mesma chave de idempotência
	reqReplay := httptest.NewRequest(http.MethodPost, "/v1/workspaces/prod-br/catalogs", bytes.NewBufferString(`{"name": "sales_cat"}`))
	reqReplay.Header.Set("X-Change-Ref", "TICKET-123")
	reqReplay.Header.Set("Idempotency-Key", "idemp-sales-01")
	recReplay := httptest.NewRecorder()

	router.ServeHTTP(recReplay, reqReplay)

	if recReplay.Code != http.StatusCreated {
		t.Fatalf("expected status 201 for replayed idempotent request, got %d", recReplay.Code)
	}
}

func TestPrivilegeNotApplicableRule(t *testing.T) {
	router := setupTestServer()

	// Tentativa de conceder USE_SCHEMA diretamente em TABLE (viola a matriz)
	grantBody := bytes.NewBufferString(`{
		"changes": [
			{
				"principal": "grp-analytics",
				"privileges": ["USE_SCHEMA"]
			}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/workspaces/prod-br/catalogs/main/schemas/default/tables/orders/grants/grant", grantBody)
	req.Header.Set("X-Change-Ref", "TICKET-999")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422 for non-applicable privilege, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var prob domain.ProblemError
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("failed to decode problem json: %v", err)
	}

	if prob.Type != "/problems/privilege-not-applicable" {
		t.Errorf("expected type /problems/privilege-not-applicable, got %s", prob.Type)
	}
}

func TestAntiLockoutProtectedPrincipal(t *testing.T) {
	router := setupTestServer()

	// Tentativa de revogar MANAGE do service principal protegido
	revokeBody := bytes.NewBufferString(`{
		"changes": [
			{
				"principal": "sp-governance-service",
				"privileges": ["MANAGE"]
			}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/workspaces/prod-br/catalogs/main/grants/revoke", revokeBody)
	req.Header.Set("X-Change-Ref", "SECURITY-01")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409 for protected principal revoke, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var prob domain.ProblemError
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("failed to decode problem json: %v", err)
	}

	if prob.Type != "/problems/protected-principal" {
		t.Errorf("expected type /problems/protected-principal, got %s", prob.Type)
	}
}

func TestDeleteWithoutConfirmation(t *testing.T) {
	router := setupTestServer()

	// DELETE sem parâmetro confirm correspondente
	req := httptest.NewRequest(http.MethodDelete, "/v1/workspaces/prod-br/catalogs/main?confirm=wrong_name", nil)
	req.Header.Set("X-Change-Ref", "TICKET-DEL")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422 for confirmation mismatch, got %d", rec.Code)
	}

	var prob domain.ProblemError
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("failed to decode problem json: %v", err)
	}

	if prob.Type != "/problems/confirmation-mismatch" {
		t.Errorf("expected type /problems/confirmation-mismatch, got %s", prob.Type)
	}
}

func TestGlobalAuditTrailLogging(t *testing.T) {
	router := setupTestServer()

	// 1. Executa um Grant válido
	grantBody := bytes.NewBufferString(`{
		"changes": [
			{
				"principal": "grp-analytics",
				"privileges": ["USE_CATALOG"]
			}
		]
	}`)
	reqGrant := httptest.NewRequest(http.MethodPost, "/v1/workspaces/prod-br/catalogs/main/grants/grant", grantBody)
	reqGrant.Header.Set("X-Change-Ref", "TICKET-AUDIT-TEST")
	recGrant := httptest.NewRecorder()
	router.ServeHTTP(recGrant, reqGrant)

	if recGrant.Code != http.StatusOK {
		t.Fatalf("expected status 200 for grant, got %d: %s", recGrant.Code, recGrant.Body.String())
	}

	// 2. Consulta a trilha de auditoria global
	reqAudit := httptest.NewRequest(http.MethodGet, "/v1/audit/events?change_ref=TICKET-AUDIT-TEST", nil)
	recAudit := httptest.NewRecorder()
	router.ServeHTTP(recAudit, reqAudit)

	if recAudit.Code != http.StatusOK {
		t.Fatalf("expected status 200 for audit events, got %d", recAudit.Code)
	}

	var auditResponse struct {
		Events []domain.AuditEvent `json:"events"`
	}
	if err := json.NewDecoder(recAudit.Body).Decode(&auditResponse); err != nil {
		t.Fatalf("failed to parse audit response: %v", err)
	}

	if len(auditResponse.Events) == 0 {
		t.Fatalf("expected at least 1 audit event for TICKET-AUDIT-TEST, got 0")
	}

	event := auditResponse.Events[0]
	if event.Action != domain.ActionGrant {
		t.Errorf("expected action GRANT, got %s", event.Action)
	}
	if event.Outcome != domain.OutcomeSuccess {
		t.Errorf("expected outcome SUCCESS, got %s", event.Outcome)
	}
	if event.Workspace != "prod-br" {
		t.Errorf("expected workspace prod-br, got %s", event.Workspace)
	}
	if event.MetastoreID == nil || *event.MetastoreID != "meta-sa-east-1" {
		t.Errorf("expected metastore_id meta-sa-east-1, got %v", event.MetastoreID)
	}
}
