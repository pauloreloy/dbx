package service

import (
	"context"

	"github.com/pauloeloy/databricks-governance-service/internal/domain"
	"github.com/pauloeloy/databricks-governance-service/internal/ports"
)

// AuditService gerencia a consulta da trilha global de auditoria (D10).
type AuditService struct {
	auditStore ports.AuditStore
}

// NewAuditService inicializa o serviço de auditoria.
func NewAuditService(auditStore ports.AuditStore) *AuditService {
	return &AuditService{auditStore: auditStore}
}

func (s *AuditService) ListEvents(ctx context.Context, filter ports.AuditFilter, pageSize int, pageToken string) ([]domain.AuditEvent, string, error) {
	if s.auditStore == nil {
		return []domain.AuditEvent{}, "", nil
	}
	return s.auditStore.ListEvents(ctx, filter, pageSize, pageToken)
}

func (s *AuditService) GetEvent(ctx context.Context, id string) (*domain.AuditEvent, error) {
	if s.auditStore == nil {
		return nil, domain.NewNotFoundError("AuditEvent", id)
	}
	return s.auditStore.GetEvent(ctx, id)
}

// WorkspaceService gerencia a consulta do registry de workspaces (D5).
type WorkspaceService struct {
	registry ports.WorkspaceRegistry
}

// NewWorkspaceService inicializa o serviço de workspaces.
func NewWorkspaceService(registry ports.WorkspaceRegistry) *WorkspaceService {
	return &WorkspaceService{registry: registry}
}

func (s *WorkspaceService) ListWorkspaces(ctx context.Context, pageSize int, pageToken string) ([]domain.Workspace, string, error) {
	if s.registry == nil {
		return []domain.Workspace{}, "", nil
	}
	return s.registry.ListWorkspaces(ctx, pageSize, pageToken)
}

func (s *WorkspaceService) GetWorkspace(ctx context.Context, alias string) (*domain.Workspace, error) {
	if s.registry == nil {
		return nil, domain.NewNotFoundError("Workspace", alias)
	}
	return s.registry.GetWorkspace(ctx, alias)
}
