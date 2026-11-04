package domain

import "time"

// Workspace representa as informações de um workspace registrado no serviço.
type Workspace struct {
	ID                   string `json:"id"`
	DisplayName          string `json:"display_name"`
	DatabricksWorkspaceID string `json:"databricks_workspace_id,omitempty"`
	MetastoreID          string `json:"metastore_id,omitempty"`
	Cloud                string `json:"cloud,omitempty"`
	Region               string `json:"region,omitempty"`
	Status               string `json:"status"`
}

// Catalog representa um catálogo do Unity Catalog.
type Catalog struct {
	Name        string            `json:"name"`
	Comment     *string           `json:"comment,omitempty"`
	Owner       string            `json:"owner,omitempty"`
	CatalogType string            `json:"catalog_type,omitempty"`
	StorageRoot *string           `json:"storage_root,omitempty"`
	Properties  map[string]string `json:"properties,omitempty"`
	CreatedAt   *time.Time        `json:"created_at,omitempty"`
	UpdatedAt   *time.Time        `json:"updated_at,omitempty"`
}

// DataSchema representa um schema do Unity Catalog.
type DataSchema struct {
	Name        string            `json:"name"`
	CatalogName string            `json:"catalog_name"`
	FullName    string            `json:"full_name"`
	Comment     *string           `json:"comment,omitempty"`
	Owner       string            `json:"owner,omitempty"`
	StorageRoot *string           `json:"storage_root,omitempty"`
	Properties  map[string]string `json:"properties,omitempty"`
	CreatedAt   *time.Time        `json:"created_at,omitempty"`
	UpdatedAt   *time.Time        `json:"updated_at,omitempty"`
}

// Column representa uma coluna em uma tabela ou view.
type Column struct {
	Name     string `json:"name"`
	TypeText string `json:"type_text"`
	Position int    `json:"position"`
	Comment  *string `json:"comment,omitempty"`
	Nullable bool   `json:"nullable"`
}

// Table representa uma tabela do Unity Catalog.
type Table struct {
	Name             string     `json:"name"`
	CatalogName      string     `json:"catalog_name"`
	SchemaName       string     `json:"schema_name"`
	FullName         string     `json:"full_name"`
	TableType        string     `json:"table_type,omitempty"`
	DataSourceFormat *string    `json:"data_source_format,omitempty"`
	StorageLocation  *string    `json:"storage_location,omitempty"`
	Comment          *string    `json:"comment,omitempty"`
	Owner            string     `json:"owner,omitempty"`
	Columns          []Column   `json:"columns,omitempty"`
	CreatedAt        *time.Time `json:"created_at,omitempty"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
}

// View representa uma view do Unity Catalog.
type View struct {
	Name           string     `json:"name"`
	CatalogName    string     `json:"catalog_name"`
	SchemaName     string     `json:"schema_name"`
	FullName       string     `json:"full_name"`
	ViewDefinition *string    `json:"view_definition,omitempty"`
	Comment        *string    `json:"comment,omitempty"`
	Owner          string     `json:"owner,omitempty"`
	Columns        []Column   `json:"columns,omitempty"`
	CreatedAt      *time.Time `json:"created_at,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
}

// Volume representa um volume do Unity Catalog.
type Volume struct {
	Name            string     `json:"name"`
	CatalogName     string     `json:"catalog_name"`
	SchemaName      string     `json:"schema_name"`
	FullName        string     `json:"full_name"`
	VolumeType      string     `json:"volume_type"`
	StorageLocation *string    `json:"storage_location,omitempty"`
	Comment         *string    `json:"comment,omitempty"`
	Owner           string     `json:"owner,omitempty"`
	CreatedAt       *time.Time `json:"created_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// PrincipalPrivileges representa os privilégios concedidos diretamente a um principal.
type PrincipalPrivileges struct {
	Principal  string      `json:"principal"`
	Privileges []Privilege `json:"privileges"`
}

// EffectivePrivilege detalha herança de um privilégio.
type EffectivePrivilege struct {
	Privilege   Privilege     `json:"privilege"`
	Inherited   bool          `json:"inherited"`
	InheritedFrom *SecurableRef `json:"inherited_from,omitempty"`
}

// EffectiveGrant representa privilégios efetivos de um principal.
type EffectiveGrant struct {
	Principal  string               `json:"principal"`
	Privileges []EffectivePrivilege `json:"privileges"`
}

// PrivilegeChange detalha a intenção de concessão ou revogação.
type PrivilegeChange struct {
	Principal  string      `json:"principal"`
	Privileges []Privilege `json:"privileges"`
}

// ChangeOutcome informa o que foi aplicado vs o que não foi alterado (no-op).
type ChangeOutcome struct {
	Principal string      `json:"principal"`
	Applied   []Privilege `json:"applied"`
	Unchanged []Privilege `json:"unchanged"`
}

// GrantChangeResult consolida a resposta de um grant/revoke.
type GrantChangeResult struct {
	Securable SecurableRef          `json:"securable"`
	Outcomes  []ChangeOutcome       `json:"outcomes"`
	Grants    []PrincipalPrivileges `json:"grants"`
	Changed   bool                  `json:"changed"`
	AuditID   string                `json:"audit_id"`
}
