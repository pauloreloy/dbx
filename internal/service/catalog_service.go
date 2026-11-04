package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pauloeloy/databricks-governance-service/internal/domain"
	"github.com/pauloeloy/databricks-governance-service/internal/ports"
)

// CatalogService orquestra o ciclo de vida dos objetos e seus metadados.
type CatalogService struct {
	catalogsAPI ports.CatalogsAPI
	schemasAPI  ports.SchemasAPI
	tablesAPI   ports.TablesAPI
	viewsAPI    ports.ViewsAPI
	volumesAPI  ports.VolumesAPI
	ownershipAPI ports.OwnershipAPI
	tagsAPI     ports.TagsAPI
	identityAPI ports.IdentityAPI
	auditStore  ports.AuditStore
	registry    ports.WorkspaceRegistry
}

// NewCatalogService inicializa a camada de serviço do catálogo.
func NewCatalogService(
	catalogsAPI ports.CatalogsAPI,
	schemasAPI ports.SchemasAPI,
	tablesAPI ports.TablesAPI,
	viewsAPI ports.ViewsAPI,
	volumesAPI ports.VolumesAPI,
	ownershipAPI ports.OwnershipAPI,
	tagsAPI ports.TagsAPI,
	identityAPI ports.IdentityAPI,
	auditStore ports.AuditStore,
	registry ports.WorkspaceRegistry,
) *CatalogService {
	return &CatalogService{
		catalogsAPI:  catalogsAPI,
		schemasAPI:   schemasAPI,
		tablesAPI:    tablesAPI,
		viewsAPI:     viewsAPI,
		volumesAPI:   volumesAPI,
		ownershipAPI: ownershipAPI,
		tagsAPI:      tagsAPI,
		identityAPI:  identityAPI,
		auditStore:   auditStore,
		registry:     registry,
	}
}

// --- Catalogs ---

func (s *CatalogService) ListCatalogs(ctx context.Context, ws string, pageSize int, pageToken string) ([]domain.Catalog, string, error) {
	return s.catalogsAPI.ListCatalogs(ctx, ws, pageSize, pageToken)
}

func (s *CatalogService) GetCatalog(ctx context.Context, ws, name string) (*domain.Catalog, error) {
	return s.catalogsAPI.GetCatalog(ctx, ws, name)
}

func (s *CatalogService) CreateCatalog(ctx context.Context, ws string, cat domain.Catalog, actor, changeRef, requestID string) (*domain.Catalog, error) {
	if !domain.ValidateCreateIdentifier(cat.Name) {
		return nil, domain.NewBadRequestError(fmt.Sprintf("Nome de catálogo inválido '%s'. Deve iniciar com letra e conter apenas letras minúsculas, dígitos e sublinhados.", cat.Name))
	}

	created, err := s.catalogsAPI.CreateCatalog(ctx, ws, cat)
	if err != nil {
		return nil, err
	}

	s.recordAudit(ctx, ws, actor, domain.ActionCatalogCreate, changeRef, requestID, &domain.SecurableRef{
		Type: domain.SecurableCatalog,
		Name: cat.Name,
	}, nil, nil, map[string]interface{}{"catalog": created})

	return created, nil
}

func (s *CatalogService) UpdateCatalog(ctx context.Context, ws, name string, newName, comment *string, props map[string]string, actor, changeRef, requestID string) (*domain.Catalog, error) {
	if newName != nil && !domain.ValidateCreateIdentifier(*newName) {
		return nil, domain.NewBadRequestError(fmt.Sprintf("Novo nome de catálogo inválido '%s'", *newName))
	}

	before, _ := s.catalogsAPI.GetCatalog(ctx, ws, name)
	updated, err := s.catalogsAPI.UpdateCatalog(ctx, ws, name, newName, comment, props)
	if err != nil {
		return nil, err
	}

	s.recordAudit(ctx, ws, actor, domain.ActionCatalogUpdate, changeRef, requestID, &domain.SecurableRef{
		Type: domain.SecurableCatalog,
		Name: name,
	}, nil, map[string]interface{}{"catalog": before}, map[string]interface{}{"catalog": updated})

	return updated, nil
}

func (s *CatalogService) DeleteCatalog(ctx context.Context, ws, name, confirm string, force bool, actor, changeRef, requestID string) error {
	if confirm != name {
		return domain.NewConfirmationMismatchError(name, confirm)
	}

	before, _ := s.catalogsAPI.GetCatalog(ctx, ws, name)
	if err := s.catalogsAPI.DeleteCatalog(ctx, ws, name, force); err != nil {
		return err
	}

	s.recordAudit(ctx, ws, actor, domain.ActionCatalogDelete, changeRef, requestID, &domain.SecurableRef{
		Type: domain.SecurableCatalog,
		Name: name,
	}, nil, map[string]interface{}{"catalog": before}, nil)

	return nil
}

// --- Schemas ---

func (s *CatalogService) ListSchemas(ctx context.Context, ws, catalog string, pageSize int, pageToken string) ([]domain.DataSchema, string, error) {
	return s.schemasAPI.ListSchemas(ctx, ws, catalog, pageSize, pageToken)
}

func (s *CatalogService) GetSchema(ctx context.Context, ws, catalog, schema string) (*domain.DataSchema, error) {
	return s.schemasAPI.GetSchema(ctx, ws, catalog, schema)
}

func (s *CatalogService) CreateSchema(ctx context.Context, ws, catalog string, sch domain.DataSchema, actor, changeRef, requestID string) (*domain.DataSchema, error) {
	if !domain.ValidateCreateIdentifier(sch.Name) {
		return nil, domain.NewBadRequestError(fmt.Sprintf("Nome de schema inválido '%s'", sch.Name))
	}

	created, err := s.schemasAPI.CreateSchema(ctx, ws, catalog, sch)
	if err != nil {
		return nil, err
	}

	fullName := fmt.Sprintf("%s.%s", catalog, sch.Name)
	s.recordAudit(ctx, ws, actor, domain.ActionSchemaCreate, changeRef, requestID, &domain.SecurableRef{
		Type: domain.SecurableSchema,
		Name: fullName,
	}, nil, nil, map[string]interface{}{"schema": created})

	return created, nil
}

func (s *CatalogService) UpdateSchema(ctx context.Context, ws, catalog, schema string, newName, comment *string, props map[string]string, actor, changeRef, requestID string) (*domain.DataSchema, error) {
	if newName != nil && !domain.ValidateCreateIdentifier(*newName) {
		return nil, domain.NewBadRequestError(fmt.Sprintf("Novo nome de schema inválido '%s'", *newName))
	}

	fullName := fmt.Sprintf("%s.%s", catalog, schema)
	before, _ := s.schemasAPI.GetSchema(ctx, ws, catalog, schema)
	updated, err := s.schemasAPI.UpdateSchema(ctx, ws, catalog, schema, newName, comment, props)
	if err != nil {
		return nil, err
	}

	s.recordAudit(ctx, ws, actor, domain.ActionSchemaUpdate, changeRef, requestID, &domain.SecurableRef{
		Type: domain.SecurableSchema,
		Name: fullName,
	}, nil, map[string]interface{}{"schema": before}, map[string]interface{}{"schema": updated})

	return updated, nil
}

func (s *CatalogService) DeleteSchema(ctx context.Context, ws, catalog, schema, confirm string, force bool, actor, changeRef, requestID string) error {
	fullName := fmt.Sprintf("%s.%s", catalog, schema)
	if confirm != fullName {
		return domain.NewConfirmationMismatchError(fullName, confirm)
	}

	before, _ := s.schemasAPI.GetSchema(ctx, ws, catalog, schema)
	if err := s.schemasAPI.DeleteSchema(ctx, ws, catalog, schema, force); err != nil {
		return err
	}

	s.recordAudit(ctx, ws, actor, domain.ActionSchemaDelete, changeRef, requestID, &domain.SecurableRef{
		Type: domain.SecurableSchema,
		Name: fullName,
	}, nil, map[string]interface{}{"schema": before}, nil)

	return nil
}

// --- Tables ---

func (s *CatalogService) ListTables(ctx context.Context, ws, catalog, schema string, pageSize int, pageToken string) ([]domain.Table, string, error) {
	return s.tablesAPI.ListTables(ctx, ws, catalog, schema, pageSize, pageToken)
}

func (s *CatalogService) GetTable(ctx context.Context, ws, catalog, schema, table string) (*domain.Table, error) {
	return s.tablesAPI.GetTable(ctx, ws, catalog, schema, table)
}

func (s *CatalogService) DeleteTable(ctx context.Context, ws, catalog, schema, table, confirm string, actor, changeRef, requestID string) error {
	fullName := fmt.Sprintf("%s.%s.%s", catalog, schema, table)
	if confirm != fullName {
		return domain.NewConfirmationMismatchError(fullName, confirm)
	}

	before, _ := s.tablesAPI.GetTable(ctx, ws, catalog, schema, table)
	if err := s.tablesAPI.DeleteTable(ctx, ws, catalog, schema, table); err != nil {
		return err
	}

	s.recordAudit(ctx, ws, actor, domain.ActionTableDelete, changeRef, requestID, &domain.SecurableRef{
		Type: domain.SecurableTable,
		Name: fullName,
	}, nil, map[string]interface{}{"table": before}, nil)

	return nil
}

// --- Views ---

func (s *CatalogService) ListViews(ctx context.Context, ws, catalog, schema string, pageSize int, pageToken string) ([]domain.View, string, error) {
	return s.viewsAPI.ListViews(ctx, ws, catalog, schema, pageSize, pageToken)
}

func (s *CatalogService) GetView(ctx context.Context, ws, catalog, schema, view string) (*domain.View, error) {
	return s.viewsAPI.GetView(ctx, ws, catalog, schema, view)
}

func (s *CatalogService) DeleteView(ctx context.Context, ws, catalog, schema, view, confirm string, actor, changeRef, requestID string) error {
	fullName := fmt.Sprintf("%s.%s.%s", catalog, schema, view)
	if confirm != fullName {
		return domain.NewConfirmationMismatchError(fullName, confirm)
	}

	before, _ := s.viewsAPI.GetView(ctx, ws, catalog, schema, view)
	if err := s.viewsAPI.DeleteView(ctx, ws, catalog, schema, view); err != nil {
		return err
	}

	s.recordAudit(ctx, ws, actor, domain.ActionViewDelete, changeRef, requestID, &domain.SecurableRef{
		Type: domain.SecurableView,
		Name: fullName,
	}, nil, map[string]interface{}{"view": before}, nil)

	return nil
}

// --- Volumes ---

func (s *CatalogService) ListVolumes(ctx context.Context, ws, catalog, schema string, pageSize int, pageToken string) ([]domain.Volume, string, error) {
	return s.volumesAPI.ListVolumes(ctx, ws, catalog, schema, pageSize, pageToken)
}

func (s *CatalogService) GetVolume(ctx context.Context, ws, catalog, schema, volume string) (*domain.Volume, error) {
	return s.volumesAPI.GetVolume(ctx, ws, catalog, schema, volume)
}

func (s *CatalogService) CreateVolume(ctx context.Context, ws, catalog, schema string, vol domain.Volume, actor, changeRef, requestID string) (*domain.Volume, error) {
	if !domain.ValidateCreateIdentifier(vol.Name) {
		return nil, domain.NewBadRequestError(fmt.Sprintf("Nome de volume inválido '%s'", vol.Name))
	}

	created, err := s.volumesAPI.CreateVolume(ctx, ws, catalog, schema, vol)
	if err != nil {
		return nil, err
	}

	fullName := fmt.Sprintf("%s.%s.%s", catalog, schema, vol.Name)
	s.recordAudit(ctx, ws, actor, domain.ActionVolumeCreate, changeRef, requestID, &domain.SecurableRef{
		Type: domain.SecurableVolume,
		Name: fullName,
	}, nil, nil, map[string]interface{}{"volume": created})

	return created, nil
}

func (s *CatalogService) UpdateVolume(ctx context.Context, ws, catalog, schema, volume string, newName, comment *string, actor, changeRef, requestID string) (*domain.Volume, error) {
	if newName != nil && !domain.ValidateCreateIdentifier(*newName) {
		return nil, domain.NewBadRequestError(fmt.Sprintf("Novo nome de volume inválido '%s'", *newName))
	}

	fullName := fmt.Sprintf("%s.%s.%s", catalog, schema, volume)
	before, _ := s.volumesAPI.GetVolume(ctx, ws, catalog, schema, volume)
	updated, err := s.volumesAPI.UpdateVolume(ctx, ws, catalog, schema, volume, newName, comment)
	if err != nil {
		return nil, err
	}

	s.recordAudit(ctx, ws, actor, domain.ActionVolumeUpdate, changeRef, requestID, &domain.SecurableRef{
		Type: domain.SecurableVolume,
		Name: fullName,
	}, nil, map[string]interface{}{"volume": before}, map[string]interface{}{"volume": updated})

	return updated, nil
}

func (s *CatalogService) DeleteVolume(ctx context.Context, ws, catalog, schema, volume, confirm string, actor, changeRef, requestID string) error {
	fullName := fmt.Sprintf("%s.%s.%s", catalog, schema, volume)
	if confirm != fullName {
		return domain.NewConfirmationMismatchError(fullName, confirm)
	}

	before, _ := s.volumesAPI.GetVolume(ctx, ws, catalog, schema, volume)
	if err := s.volumesAPI.DeleteVolume(ctx, ws, catalog, schema, volume); err != nil {
		return err
	}

	s.recordAudit(ctx, ws, actor, domain.ActionVolumeDelete, changeRef, requestID, &domain.SecurableRef{
		Type: domain.SecurableVolume,
		Name: fullName,
	}, nil, map[string]interface{}{"volume": before}, nil)

	return nil
}

// --- Ownership ---

func (s *CatalogService) GetOwner(ctx context.Context, ws string, sec domain.SecurableRef) (string, error) {
	return s.ownershipAPI.GetOwner(ctx, ws, sec)
}

func (s *CatalogService) SetOwner(ctx context.Context, ws string, sec domain.SecurableRef, newOwner, actor, changeRef, requestID string) error {
	if s.identityAPI != nil {
		p, err := s.identityAPI.GetPrincipal(ctx, ws, newOwner)
		if err != nil || p == nil {
			return domain.NewPrincipalNotFoundError(newOwner)
		}
	}

	oldOwner, _ := s.ownershipAPI.GetOwner(ctx, ws, sec)
	if err := s.ownershipAPI.SetOwner(ctx, ws, sec, newOwner); err != nil {
		return err
	}

	s.recordAudit(ctx, ws, actor, domain.ActionOwnerChange, changeRef, requestID, &sec, []string{newOwner},
		map[string]interface{}{"owner": oldOwner},
		map[string]interface{}{"owner": newOwner},
	)

	return nil
}

// --- Tags ---

func (s *CatalogService) ListTags(ctx context.Context, ws string, sec domain.SecurableRef) (map[string]string, error) {
	return s.tagsAPI.ListTags(ctx, ws, sec)
}

func (s *CatalogService) SetTag(ctx context.Context, ws string, sec domain.SecurableRef, key, value, actor, changeRef, requestID string) error {
	if err := s.tagsAPI.SetTag(ctx, ws, sec, key, value); err != nil {
		return err
	}

	s.recordAudit(ctx, ws, actor, domain.ActionTagSet, changeRef, requestID, &sec, nil, nil, map[string]interface{}{key: value})
	return nil
}

func (s *CatalogService) DeleteTag(ctx context.Context, ws string, sec domain.SecurableRef, key, actor, changeRef, requestID string) error {
	if err := s.tagsAPI.DeleteTag(ctx, ws, sec, key); err != nil {
		return err
	}

	s.recordAudit(ctx, ws, actor, domain.ActionTagDelete, changeRef, requestID, &sec, nil, map[string]interface{}{"deleted_tag": key}, nil)
	return nil
}

func (s *CatalogService) recordAudit(
	ctx context.Context,
	ws, actor string,
	action domain.AuditAction,
	changeRef, requestID string,
	sec *domain.SecurableRef,
	targets []string,
	before, after map[string]interface{},
) {
	if s.auditStore == nil {
		return
	}

	var metastoreID *string
	if s.registry != nil {
		if wsInfo, err := s.registry.GetWorkspace(ctx, ws); err == nil && wsInfo != nil && wsInfo.MetastoreID != "" {
			mID := wsInfo.MetastoreID
			metastoreID = &mID
			if sec != nil {
				sec.MetastoreID = &mID
			}
		}
	}

	_ = s.auditStore.Record(ctx, domain.AuditEvent{
		ID:          "evt_" + uuid.New().String(),
		OccurredAt:  time.Now().UTC(),
		Workspace:   ws,
		MetastoreID: metastoreID,
		Actor:       actor,
		Action:      action,
		Outcome:     domain.OutcomeSuccess,
		ChangeRef:   changeRef,
		RequestID:   requestID,
		Securable:   sec,
		Targets:     targets,
		Before:      before,
		After:       after,
	})
}
