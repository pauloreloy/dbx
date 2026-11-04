package domain

import "time"

// AuditAction categoriza as ações de mutação no serviço.
type AuditAction string

const (
	ActionGrant         AuditAction = "GRANT"
	ActionRevoke        AuditAction = "REVOKE"
	ActionCatalogCreate AuditAction = "CATALOG_CREATE"
	ActionCatalogUpdate AuditAction = "CATALOG_UPDATE"
	ActionCatalogDelete AuditAction = "CATALOG_DELETE"
	ActionSchemaCreate  AuditAction = "SCHEMA_CREATE"
	ActionSchemaUpdate  AuditAction = "SCHEMA_UPDATE"
	ActionSchemaDelete  AuditAction = "SCHEMA_DELETE"
	ActionTableDelete   AuditAction = "TABLE_DELETE"
	ActionViewDelete    AuditAction = "VIEW_DELETE"
	ActionVolumeCreate  AuditAction = "VOLUME_CREATE"
	ActionVolumeUpdate  AuditAction = "VOLUME_UPDATE"
	ActionVolumeDelete  AuditAction = "VOLUME_DELETE"
	ActionOwnerChange   AuditAction = "OWNER_CHANGE"
	ActionTagSet        AuditAction = "TAG_SET"
	ActionTagDelete     AuditAction = "TAG_DELETE"
)

// AuditOutcome indica o resultado da operação.
type AuditOutcome string

const (
	OutcomeSuccess AuditOutcome = "SUCCESS"
	OutcomeDenied  AuditOutcome = "DENIED"
	OutcomeFailed  AuditOutcome = "FAILED"
)

// AuditEvent representa o registro imutável de auditoria.
type AuditEvent struct {
	ID          string                 `json:"id"`
	OccurredAt  time.Time              `json:"occurred_at"`
	Workspace   string                 `json:"workspace"`
	MetastoreID *string                `json:"metastore_id,omitempty"`
	Actor       string                 `json:"actor"`
	Action      AuditAction            `json:"action"`
	Outcome     AuditOutcome           `json:"outcome"`
	ChangeRef   string                 `json:"change_ref"`
	RequestID   string                 `json:"request_id"`
	Securable   *SecurableRef          `json:"securable,omitempty"`
	Targets     []string               `json:"targets,omitempty"`
	Before      map[string]interface{} `json:"before,omitempty"`
	After       map[string]interface{} `json:"after,omitempty"`
	Error       *AuditError            `json:"error,omitempty"`
}

// AuditError detalha erros no log de auditoria.
type AuditError struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
}
