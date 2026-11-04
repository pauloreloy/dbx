package ports

import (
	"context"

	"github.com/pauloeloy/databricks-governance-service/internal/domain"
)

// GrantsAPI define as operações de governança de permissões.
type GrantsAPI interface {
	GetGrants(ctx context.Context, ws string, sec domain.SecurableRef, principalFilter *string) ([]domain.PrincipalPrivileges, error)
	GetEffectiveGrants(ctx context.Context, ws string, sec domain.SecurableRef, principalFilter *string) ([]domain.EffectiveGrant, error)
	UpdateGrants(ctx context.Context, ws string, sec domain.SecurableRef, changes []domain.PrivilegeChange, isRevoke bool) ([]domain.ChangeOutcome, []domain.PrincipalPrivileges, bool, error)
}

// CatalogsAPI define operações sobre catálogos.
type CatalogsAPI interface {
	ListCatalogs(ctx context.Context, ws string, pageSize int, pageToken string) ([]domain.Catalog, string, error)
	GetCatalog(ctx context.Context, ws, name string) (*domain.Catalog, error)
	CreateCatalog(ctx context.Context, ws string, cat domain.Catalog) (*domain.Catalog, error)
	UpdateCatalog(ctx context.Context, ws, name string, newName, comment *string, props map[string]string) (*domain.Catalog, error)
	DeleteCatalog(ctx context.Context, ws, name string, force bool) error
}

// SchemasAPI define operações sobre schemas.
type SchemasAPI interface {
	ListSchemas(ctx context.Context, ws, catalog string, pageSize int, pageToken string) ([]domain.DataSchema, string, error)
	GetSchema(ctx context.Context, ws, catalog, schema string) (*domain.DataSchema, error)
	CreateSchema(ctx context.Context, ws, catalog string, s domain.DataSchema) (*domain.DataSchema, error)
	UpdateSchema(ctx context.Context, ws, catalog, schema string, newName, comment *string, props map[string]string) (*domain.DataSchema, error)
	DeleteSchema(ctx context.Context, ws, catalog, schema string, force bool) error
}

// TablesAPI define operações de leitura e remoção de tabelas.
type TablesAPI interface {
	ListTables(ctx context.Context, ws, catalog, schema string, pageSize int, pageToken string) ([]domain.Table, string, error)
	GetTable(ctx context.Context, ws, catalog, schema, table string) (*domain.Table, error)
	DeleteTable(ctx context.Context, ws, catalog, schema, table string) error
}

// ViewsAPI define operações de leitura e remoção de views.
type ViewsAPI interface {
	ListViews(ctx context.Context, ws, catalog, schema string, pageSize int, pageToken string) ([]domain.View, string, error)
	GetView(ctx context.Context, ws, catalog, schema, view string) (*domain.View, error)
	DeleteView(ctx context.Context, ws, catalog, schema, view string) error
}

// VolumesAPI define operações sobre volumes.
type VolumesAPI interface {
	ListVolumes(ctx context.Context, ws, catalog, schema string, pageSize int, pageToken string) ([]domain.Volume, string, error)
	GetVolume(ctx context.Context, ws, catalog, schema, volume string) (*domain.Volume, error)
	CreateVolume(ctx context.Context, ws, catalog, schema string, v domain.Volume) (*domain.Volume, error)
	UpdateVolume(ctx context.Context, ws, catalog, schema, volume string, newName, comment *string) (*domain.Volume, error)
	DeleteVolume(ctx context.Context, ws, catalog, schema, volume string) error
}

// OwnershipAPI define operações sobre dono dos objetos.
type OwnershipAPI interface {
	GetOwner(ctx context.Context, ws string, sec domain.SecurableRef) (string, error)
	SetOwner(ctx context.Context, ws string, sec domain.SecurableRef, newOwner string) error
}

// TagsAPI define operações sobre tags dos objetos.
type TagsAPI interface {
	ListTags(ctx context.Context, ws string, sec domain.SecurableRef) (map[string]string, error)
	SetTag(ctx context.Context, ws string, sec domain.SecurableRef, key, value string) error
	DeleteTag(ctx context.Context, ws string, sec domain.SecurableRef, key string) error
}

// IdentityAPI define operações de consulta de identidades.
type IdentityAPI interface {
	ListPrincipals(ctx context.Context, ws string, pType *domain.PrincipalType, query *string, pageSize int, pageToken string) ([]domain.Principal, string, error)
	GetPrincipal(ctx context.Context, ws, name string) (*domain.Principal, error)
}

// DatabricksClient agrega todas as portas de integração com a API Databricks Unity Catalog e IAM.
type DatabricksClient interface {
	GrantsAPI
	CatalogsAPI
	SchemasAPI
	TablesAPI
	ViewsAPI
	VolumesAPI
	OwnershipAPI
	TagsAPI
	IdentityAPI
}

