package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/pauloeloy/databricks-governance-service/internal/domain"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	actorKey     contextKey = "actor"
)

// RequestIDMiddleware assegura que toda requisição e resposta tenham um X-Request-Id.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = "req_" + uuid.New().String()
		}
		w.Header().Set("X-Request-Id", reqID)

		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ChangeRefMiddleware exige o header X-Change-Ref em métodos de escrita (D9).
func ChangeRefMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ignora rotas de healthz e readyz
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}

		// Valida métodos mutativos
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			changeRef := strings.TrimSpace(r.Header.Get("X-Change-Ref"))
			if changeRef == "" {
				WriteProblem(w, r, domain.NewBadRequestError(
					"O cabeçalho 'X-Change-Ref' é obrigatório para todas as operações de escrita (auditoria/ticket)",
					domain.FieldError{
						Field:   "X-Change-Ref",
						Message: "Cabeçalho ausente ou vazio",
						Code:    "missing-change-ref",
					},
				))
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// ActorMiddleware extrai a identidade do chamador a partir de autenticação JWT/mTLS.
func ActorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor := "anonymous"
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			// Em produção: decodificar claim 'sub' do JWT validado
			actor = "authenticated-service"
		} else if userHeader := r.Header.Get("X-Consumer-Username"); userHeader != "" {
			actor = userHeader
		}

		ctx := context.WithValue(r.Context(), actorKey, actor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RecoveryMiddleware captura panics e responde com Problem RFC 9457 (500).
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				WriteProblem(w, r, domain.NewInternalError("Erro inesperado processando a requisição"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// GetRequestID extrai o ID de correlação do contexto.
func GetRequestID(ctx context.Context) string {
	if val, ok := ctx.Value(requestIDKey).(string); ok {
		return val
	}
	return "unknown"
}

// GetActor extrai o identificador do ator do contexto.
func GetActor(ctx context.Context) string {
	if val, ok := ctx.Value(actorKey).(string); ok {
		return val
	}
	return "anonymous"
}
