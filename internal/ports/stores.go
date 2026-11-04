package ports

import (
	"context"
	"time"

	"github.com/pauloeloy/databricks-governance-service/internal/domain"
)

// AuditFilter define parâmetros para consulta do log de auditoria global.
type AuditFilter struct {
	Workspace *string
	Actor     *string
	Securable *string
	Target    *string
	ChangeRef *string
	Action    *domain.AuditAction
	Outcome   *domain.AuditOutcome
	Since     *time.Time
	Until     *time.Time
}

// AuditStore define o armazenamento persistente de auditoria (D10).
type AuditStore interface {
	Record(ctx context.Context, event domain.AuditEvent) error
	ListEvents(ctx context.Context, filter AuditFilter, pageSize int, pageToken string) ([]domain.AuditEvent, string, error)
	GetEvent(ctx context.Context, id string) (*domain.AuditEvent, error)
}

// IdempotencyRecord armazena a resposta em cache de uma chave de idempotência.
type IdempotencyRecord struct {
	StatusCode int
	Body       []byte
	CreatedAt  time.Time
}

// IdempotencyStore gerencia atomicidade e cache de idempotência (D9).
type IdempotencyStore interface {
	Acquire(ctx context.Context, scope, key string, ttl time.Duration) (acquired bool, cached *IdempotencyRecord, err error)
	Commit(ctx context.Context, scope, key string, statusCode int, responseBody []byte) error
	Release(ctx context.Context, scope, key string) error
}

// WorkspaceRegistry gerencia a resolução de aliases de workspace (D5).
type WorkspaceRegistry interface {
	ListWorkspaces(ctx context.Context, pageSize int, pageToken string) ([]domain.Workspace, string, error)
	GetWorkspace(ctx context.Context, alias string) (*domain.Workspace, error)
}
