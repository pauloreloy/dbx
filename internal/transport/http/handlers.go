package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/pauloeloy/databricks-governance-service/internal/domain"
	"github.com/pauloeloy/databricks-governance-service/internal/ports"
	"github.com/pauloeloy/databricks-governance-service/internal/service"
)

// Handler reúne os serviços necessários para atender as rotas HTTP.
type Handler struct {
	grantService     *service.GrantService
	catalogService   *service.CatalogService
	auditService     *service.AuditService
	workspaceService *service.WorkspaceService
	idempotencyStore ports.IdempotencyStore
}

func NewHandler(
	grantService *service.GrantService,
	catalogService *service.CatalogService,
	auditService *service.AuditService,
	workspaceService *service.WorkspaceService,
	idempotencyStore ports.IdempotencyStore,
) *Handler {
	return &Handler{
		grantService:     grantService,
		catalogService:   catalogService,
		auditService:     auditService,
		workspaceService: workspaceService,
		idempotencyStore: idempotencyStore,
	}
}

// --- Health Probes ---

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "UP"})
}

func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "READY"})
}

// --- Workspaces ---

func (h *Handler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	pageToken := r.URL.Query().Get("page_token")

	list, nextToken, err := h.workspaceService.ListWorkspaces(r.Context(), pageSize, pageToken)
	if err != nil {
		WriteProblem(w, r, err)
		return
	}

	res := map[string]interface{}{
		"workspaces": list,
	}
	if nextToken != "" {
		res["next_page_token"] = nextToken
	}
	WriteJSON(w, http.StatusOK, res)
}

func (h *Handler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	ws := r.PathValue("ws")
	workspace, err := h.workspaceService.GetWorkspace(r.Context(), ws)
	if err != nil {
		WriteProblem(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, workspace)
}

// --- Audit ---

func (h *Handler) ListAuditEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := ports.AuditFilter{}

	if ws := q.Get("workspace"); ws != "" {
		filter.Workspace = &ws
	}
	if actor := q.Get("actor"); actor != "" {
		filter.Actor = &actor
	}
	if changeRef := q.Get("change_ref"); changeRef != "" {
		filter.ChangeRef = &changeRef
	}

	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	pageToken := q.Get("page_token")

	events, nextToken, err := h.auditService.ListEvents(r.Context(), filter, pageSize, pageToken)
	if err != nil {
		WriteProblem(w, r, err)
		return
	}

	res := map[string]interface{}{
		"events": events,
	}
	if nextToken != "" {
		res["next_page_token"] = nextToken
	}
	WriteJSON(w, http.StatusOK, res)
}

func (h *Handler) GetAuditEvent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("event_id")
	event, err := h.auditService.GetEvent(r.Context(), id)
	if err != nil {
		WriteProblem(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, event)
}

// --- Catalogs ---

func (h *Handler) ListCatalogs(w http.ResponseWriter, r *http.Request) {
	ws := r.PathValue("ws")
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	pageToken := r.URL.Query().Get("page_token")

	catalogs, nextToken, err := h.catalogService.ListCatalogs(r.Context(), ws, pageSize, pageToken)
	if err != nil {
		WriteProblem(w, r, err)
		return
	}

	res := map[string]interface{}{
		"catalogs": catalogs,
	}
	if nextToken != "" {
		res["next_page_token"] = nextToken
	}
	WriteJSON(w, http.StatusOK, res)
}

func (h *Handler) GetCatalog(w http.ResponseWriter, r *http.Request) {
	ws := r.PathValue("ws")
	catalog := r.PathValue("catalog")

	cat, err := h.catalogService.GetCatalog(r.Context(), ws, catalog)
	if err != nil {
		WriteProblem(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, cat)
}

type CatalogCreateRequest struct {
	Name        string            `json:"name"`
	Comment     *string           `json:"comment"`
	StorageRoot *string           `json:"storage_root"`
	Properties  map[string]string `json:"properties"`
}

func (h *Handler) CreateCatalog(w http.ResponseWriter, r *http.Request) {
	ws := r.PathValue("ws")
	idempKey := r.Header.Get("Idempotency-Key")
	if idempKey == "" {
		WriteProblem(w, r, domain.NewBadRequestError("O cabeçalho 'Idempotency-Key' é obrigatório para operações DDL"))
		return
	}

	// Idempotência check
	if h.idempotencyStore != nil {
		acquired, cached, err := h.idempotencyStore.Acquire(r.Context(), ws, idempKey, 24*time.Hour)
		if err == nil && !acquired && cached != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(cached.StatusCode)
			_, _ = w.Write(cached.Body)
			return
		}
	}

	var req CatalogCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteProblem(w, r, domain.NewBadRequestError("JSON inválido no corpo da requisição"))
		return
	}

	cat := domain.Catalog{
		Name:        req.Name,
		Comment:     req.Comment,
		StorageRoot: req.StorageRoot,
		Properties:  req.Properties,
	}

	created, err := h.catalogService.CreateCatalog(
		r.Context(), ws, cat,
		GetActor(r.Context()),
		r.Header.Get("X-Change-Ref"),
		GetRequestID(r.Context()),
	)
	if err != nil {
		WriteProblem(w, r, err)
		return
	}

	location := fmt.Sprintf("/v1/workspaces/%s/catalogs/%s", ws, created.Name)
	w.Header().Set("Location", location)

	respBody, _ := json.Marshal(created)
	if h.idempotencyStore != nil {
		_ = h.idempotencyStore.Commit(r.Context(), ws, idempKey, http.StatusCreated, respBody)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(respBody)
}

func (h *Handler) DeleteCatalog(w http.ResponseWriter, r *http.Request) {
	ws := r.PathValue("ws")
	catalog := r.PathValue("catalog")
	confirm := r.URL.Query().Get("confirm")
	force := r.URL.Query().Get("force") == "true"

	err := h.catalogService.DeleteCatalog(
		r.Context(), ws, catalog, confirm, force,
		GetActor(r.Context()),
		r.Header.Get("X-Change-Ref"),
		GetRequestID(r.Context()),
	)
	if err != nil {
		WriteProblem(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Grants Handler Genérico ---

type GrantChangeBody struct {
	Changes []domain.PrivilegeChange `json:"changes"`
}

func (h *Handler) handleGrants(w http.ResponseWriter, r *http.Request, sec domain.SecurableRef) {
	var filter *string
	if pf := r.URL.Query().Get("principal"); pf != "" {
		filter = &pf
	}

	grants, err := h.grantService.GetGrants(r.Context(), r.PathValue("ws"), sec, filter)
	if err != nil {
		WriteProblem(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"securable": sec,
		"grants":    grants,
	})
}

func (h *Handler) handleEffectiveGrants(w http.ResponseWriter, r *http.Request, sec domain.SecurableRef) {
	var filter *string
	if pf := r.URL.Query().Get("principal"); pf != "" {
		filter = &pf
	}

	effGrants, err := h.grantService.GetEffectiveGrants(r.Context(), r.PathValue("ws"), sec, filter)
	if err != nil {
		WriteProblem(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"securable": sec,
		"grants":    effGrants,
	})
}

func (h *Handler) handleGrantMutation(w http.ResponseWriter, r *http.Request, sec domain.SecurableRef, isRevoke bool) {
	ws := r.PathValue("ws")
	var req GrantChangeBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteProblem(w, r, domain.NewBadRequestError("JSON inválido no corpo da requisição"))
		return
	}

	if len(req.Changes) == 0 {
		WriteProblem(w, r, domain.NewBadRequestError("O array 'changes' deve conter ao menos 1 item"))
		return
	}

	result, err := h.grantService.ApplyChanges(
		r.Context(), ws, sec, req.Changes, isRevoke,
		GetActor(r.Context()),
		r.Header.Get("X-Change-Ref"),
		GetRequestID(r.Context()),
	)
	if err != nil {
		WriteProblem(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

// Rotas de Catalog Grants
func (h *Handler) GetCatalogGrants(w http.ResponseWriter, r *http.Request) {
	h.handleGrants(w, r, domain.SecurableRef{Type: domain.SecurableCatalog, Name: r.PathValue("catalog")})
}
func (h *Handler) GetCatalogEffectiveGrants(w http.ResponseWriter, r *http.Request) {
	h.handleEffectiveGrants(w, r, domain.SecurableRef{Type: domain.SecurableCatalog, Name: r.PathValue("catalog")})
}
func (h *Handler) GrantCatalog(w http.ResponseWriter, r *http.Request) {
	h.handleGrantMutation(w, r, domain.SecurableRef{Type: domain.SecurableCatalog, Name: r.PathValue("catalog")}, false)
}
func (h *Handler) RevokeCatalog(w http.ResponseWriter, r *http.Request) {
	h.handleGrantMutation(w, r, domain.SecurableRef{Type: domain.SecurableCatalog, Name: r.PathValue("catalog")}, true)
}

// Rotas de Schema Grants
func (h *Handler) GetSchemaGrants(w http.ResponseWriter, r *http.Request) {
	fullName := fmt.Sprintf("%s.%s", r.PathValue("catalog"), r.PathValue("schema"))
	h.handleGrants(w, r, domain.SecurableRef{Type: domain.SecurableSchema, Name: fullName})
}
func (h *Handler) GetSchemaEffectiveGrants(w http.ResponseWriter, r *http.Request) {
	fullName := fmt.Sprintf("%s.%s", r.PathValue("catalog"), r.PathValue("schema"))
	h.handleEffectiveGrants(w, r, domain.SecurableRef{Type: domain.SecurableSchema, Name: fullName})
}
func (h *Handler) GrantSchema(w http.ResponseWriter, r *http.Request) {
	fullName := fmt.Sprintf("%s.%s", r.PathValue("catalog"), r.PathValue("schema"))
	h.handleGrantMutation(w, r, domain.SecurableRef{Type: domain.SecurableSchema, Name: fullName}, false)
}
func (h *Handler) RevokeSchema(w http.ResponseWriter, r *http.Request) {
	fullName := fmt.Sprintf("%s.%s", r.PathValue("catalog"), r.PathValue("schema"))
	h.handleGrantMutation(w, r, domain.SecurableRef{Type: domain.SecurableSchema, Name: fullName}, true)
}

// Rotas de Table Grants
func (h *Handler) GetTableGrants(w http.ResponseWriter, r *http.Request) {
	fullName := fmt.Sprintf("%s.%s.%s", r.PathValue("catalog"), r.PathValue("schema"), r.PathValue("table"))
	h.handleGrants(w, r, domain.SecurableRef{Type: domain.SecurableTable, Name: fullName})
}
func (h *Handler) GetTableEffectiveGrants(w http.ResponseWriter, r *http.Request) {
	fullName := fmt.Sprintf("%s.%s.%s", r.PathValue("catalog"), r.PathValue("schema"), r.PathValue("table"))
	h.handleEffectiveGrants(w, r, domain.SecurableRef{Type: domain.SecurableTable, Name: fullName})
}
func (h *Handler) GrantTable(w http.ResponseWriter, r *http.Request) {
	fullName := fmt.Sprintf("%s.%s.%s", r.PathValue("catalog"), r.PathValue("schema"), r.PathValue("table"))
	h.handleGrantMutation(w, r, domain.SecurableRef{Type: domain.SecurableTable, Name: fullName}, false)
}
func (h *Handler) RevokeTable(w http.ResponseWriter, r *http.Request) {
	fullName := fmt.Sprintf("%s.%s.%s", r.PathValue("catalog"), r.PathValue("schema"), r.PathValue("table"))
	h.handleGrantMutation(w, r, domain.SecurableRef{Type: domain.SecurableTable, Name: fullName}, true)
}
