package http

import (
	"net/http"
)

// NewRouter configura todas as rotas do microserviço e aplica a cadeia de middlewares.
func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()

	// Probes operacionais (sem auth, sem middleware restritivo)
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.HandleFunc("GET /readyz", h.Readyz)

	// Workspaces (D5)
	mux.HandleFunc("GET /v1/workspaces", h.ListWorkspaces)
	mux.HandleFunc("GET /v1/workspaces/{ws}", h.GetWorkspace)

	// Audit global (D10)
	mux.HandleFunc("GET /v1/audit/events", h.ListAuditEvents)
	mux.HandleFunc("GET /v1/audit/events/{event_id}", h.GetAuditEvent)

	// Catalogs
	mux.HandleFunc("GET /v1/workspaces/{ws}/catalogs", h.ListCatalogs)
	mux.HandleFunc("POST /v1/workspaces/{ws}/catalogs", h.CreateCatalog)
	mux.HandleFunc("GET /v1/workspaces/{ws}/catalogs/{catalog}", h.GetCatalog)
	mux.HandleFunc("DELETE /v1/workspaces/{ws}/catalogs/{catalog}", h.DeleteCatalog)

	// Catalog Grants (D7)
	mux.HandleFunc("GET /v1/workspaces/{ws}/catalogs/{catalog}/grants", h.GetCatalogGrants)
	mux.HandleFunc("GET /v1/workspaces/{ws}/catalogs/{catalog}/effective-grants", h.GetCatalogEffectiveGrants)
	mux.HandleFunc("POST /v1/workspaces/{ws}/catalogs/{catalog}/grants/grant", h.GrantCatalog)
	mux.HandleFunc("POST /v1/workspaces/{ws}/catalogs/{catalog}/grants/revoke", h.RevokeCatalog)

	// Schema Grants
	mux.HandleFunc("GET /v1/workspaces/{ws}/catalogs/{catalog}/schemas/{schema}/grants", h.GetSchemaGrants)
	mux.HandleFunc("GET /v1/workspaces/{ws}/catalogs/{catalog}/schemas/{schema}/effective-grants", h.GetSchemaEffectiveGrants)
	mux.HandleFunc("POST /v1/workspaces/{ws}/catalogs/{catalog}/schemas/{schema}/grants/grant", h.GrantSchema)
	mux.HandleFunc("POST /v1/workspaces/{ws}/catalogs/{catalog}/schemas/{schema}/grants/revoke", h.RevokeSchema)

	// Table Grants
	mux.HandleFunc("GET /v1/workspaces/{ws}/catalogs/{catalog}/schemas/{schema}/tables/{table}/grants", h.GetTableGrants)
	mux.HandleFunc("GET /v1/workspaces/{ws}/catalogs/{catalog}/schemas/{schema}/tables/{table}/effective-grants", h.GetTableEffectiveGrants)
	mux.HandleFunc("POST /v1/workspaces/{ws}/catalogs/{catalog}/schemas/{schema}/tables/{table}/grants/grant", h.GrantTable)
	mux.HandleFunc("POST /v1/workspaces/{ws}/catalogs/{catalog}/schemas/{schema}/tables/{table}/grants/revoke", h.RevokeTable)

	// Aplica a cadeia de middlewares: Recovery -> RequestID -> ChangeRef -> Actor
	var handler http.Handler = mux
	handler = ChangeRefMiddleware(handler)
	handler = ActorMiddleware(handler)
	handler = RequestIDMiddleware(handler)
	handler = RecoveryMiddleware(handler)

	return handler
}
