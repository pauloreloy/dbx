package databricks

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	sdk "github.com/databricks/databricks-sdk-go"
	"github.com/databricks/databricks-sdk-go/apierr"
	"github.com/databricks/databricks-sdk-go/service/catalog"
	"github.com/databricks/databricks-sdk-go/service/iam"
	"github.com/pauloeloy/databricks-governance-service/internal/domain"
	"github.com/pauloeloy/databricks-governance-service/internal/ports"
)

// DatabricksSDKAdapter implementa as portas de comunicação com a API real da Databricks via SDK oficial.
type DatabricksSDKAdapter struct {
	factory *ClientFactory
}

// Garante em tempo de compilação que o adapter implementa todas as portas requeridas.
var (
	_ ports.GrantsAPI        = (*DatabricksSDKAdapter)(nil)
	_ ports.CatalogsAPI      = (*DatabricksSDKAdapter)(nil)
	_ ports.SchemasAPI       = (*DatabricksSDKAdapter)(nil)
	_ ports.TablesAPI        = (*DatabricksSDKAdapter)(nil)
	_ ports.ViewsAPI         = (*DatabricksSDKAdapter)(nil)
	_ ports.VolumesAPI       = (*DatabricksSDKAdapter)(nil)
	_ ports.OwnershipAPI     = (*DatabricksSDKAdapter)(nil)
	_ ports.TagsAPI          = (*DatabricksSDKAdapter)(nil)
	_ ports.IdentityAPI      = (*DatabricksSDKAdapter)(nil)
	_ ports.DatabricksClient = (*DatabricksSDKAdapter)(nil)
)

// NewDatabricksSDKAdapter instancia o adapter com a fábrica de clientes configurada.
func NewDatabricksSDKAdapter(factory *ClientFactory) *DatabricksSDKAdapter {
	return &DatabricksSDKAdapter{
		factory: factory,
	}
}

func (a *DatabricksSDKAdapter) getClient(ctx context.Context, ws string) (*sdk.WorkspaceClient, error) {
	client, err := a.factory.GetClient(ctx, ws)
	if err != nil {
		return nil, mapError(err, ws)
	}
	return client, nil
}

// mapError mapeia erros retornados pelo SDK (*apierr.APIError) para *domain.ProblemError estruturado com UpstreamError.
func mapError(err error, ws string) error {
	if err == nil {
		return nil
	}

	var probErr *domain.ProblemError
	if errors.As(err, &probErr) {
		return probErr
	}

	var apiErr *apierr.APIError
	if errors.As(err, &apiErr) {
		status := apiErr.StatusCode
		if status == 0 {
			status = http.StatusInternalServerError
		}

		upstream := &domain.UpstreamError{
			ErrorCode:  apiErr.ErrorCode,
			Message:    apiErr.Message,
			HTTPStatus: status,
		}

		problem := &domain.ProblemError{
			Status:    status,
			Detail:    apiErr.Message,
			Workspace: ws,
			Upstream:  upstream,
		}

		switch status {
		case http.StatusNotFound:
			problem.Type = "/problems/not-found"
			problem.Title = "Recurso não encontrado"
		case http.StatusConflict:
			problem.Type = "/problems/conflict"
			problem.Title = "Conflito de recurso"
		case http.StatusBadRequest:
			problem.Type = "/problems/bad-request"
			problem.Title = "Requisição inválida"
		case http.StatusForbidden:
			problem.Type = "/problems/forbidden"
			problem.Title = "Acesso negado pela política"
		case http.StatusUnauthorized:
			problem.Type = "/problems/unauthorized"
			problem.Title = "Não autorizado"
		case http.StatusUnprocessableEntity:
			problem.Type = "/problems/unprocessable-entity"
			problem.Title = "Entidade não processável"
		default:
			problem.Type = "/problems/upstream-error"
			problem.Title = "Erro retornado pelo Databricks"
		}

		return problem
	}

	return domain.NewInternalError(err.Error())
}

// -----------------------------------------------------------------------------
// GrantsAPI
// -----------------------------------------------------------------------------

func (a *DatabricksSDKAdapter) GetGrants(ctx context.Context, ws string, sec domain.SecurableRef, principalFilter *string) ([]domain.PrincipalPrivileges, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	req := catalog.GetGrantRequest{
		SecurableType: string(securableTypeForSecurable(sec.Type)),
		FullName:      sec.Name,
	}
	if principalFilter != nil {
		req.Principal = *principalFilter
	}

	resp, err := client.Grants.Get(ctx, req)
	if err != nil {
		return nil, mapError(err, ws)
	}

	var res []domain.PrincipalPrivileges
	for _, pa := range resp.PrivilegeAssignments {
		if principalFilter != nil && pa.Principal != *principalFilter {
			continue
		}
		var privs []domain.Privilege
		for _, p := range pa.Privileges {
			privs = append(privs, domain.Privilege(p))
		}
		res = append(res, domain.PrincipalPrivileges{
			Principal:  pa.Principal,
			Privileges: privs,
		})
	}

	return res, nil
}

func (a *DatabricksSDKAdapter) GetEffectiveGrants(ctx context.Context, ws string, sec domain.SecurableRef, principalFilter *string) ([]domain.EffectiveGrant, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	req := catalog.GetEffectiveRequest{
		SecurableType: string(securableTypeForSecurable(sec.Type)),
		FullName:      sec.Name,
	}
	if principalFilter != nil {
		req.Principal = *principalFilter
	}

	resp, err := client.Grants.GetEffective(ctx, req)
	if err != nil {
		return nil, mapError(err, ws)
	}

	var res []domain.EffectiveGrant
	for _, epa := range resp.PrivilegeAssignments {
		if principalFilter != nil && epa.Principal != *principalFilter {
			continue
		}
		var effPrivs []domain.EffectivePrivilege
		for _, ep := range epa.Privileges {
			isInherited := ep.InheritedFromName != ""
			var ref *domain.SecurableRef
			if isInherited {
				ref = &domain.SecurableRef{
					Type: domain.SecurableType(ep.InheritedFromType),
					Name: ep.InheritedFromName,
				}
			}
			effPrivs = append(effPrivs, domain.EffectivePrivilege{
				Privilege:     domain.Privilege(ep.Privilege),
				Inherited:     isInherited,
				InheritedFrom: ref,
			})
		}
		res = append(res, domain.EffectiveGrant{
			Principal:  epa.Principal,
			Privileges: effPrivs,
		})
	}

	return res, nil
}

func (a *DatabricksSDKAdapter) UpdateGrants(ctx context.Context, ws string, sec domain.SecurableRef, changes []domain.PrivilegeChange, isRevoke bool) ([]domain.ChangeOutcome, []domain.PrincipalPrivileges, bool, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, nil, false, err
	}

	// 1. Obtém os privilégios atuais para calcular o que é alterado vs inalterado (no-op)
	currentGrants, err := a.GetGrants(ctx, ws, sec, nil)
	if err != nil {
		return nil, nil, false, err
	}

	currentMap := make(map[string][]domain.Privilege)
	for _, cg := range currentGrants {
		currentMap[cg.Principal] = cg.Privileges
	}

	var sdkChanges []catalog.PermissionsChange
	var outcomes []domain.ChangeOutcome
	overallChanged := false

	for _, ch := range changes {
		currentPrivs := currentMap[ch.Principal]
		var applied []domain.Privilege
		var unchanged []domain.Privilege
		var toAdd []catalog.Privilege
		var toRemove []catalog.Privilege

		if !isRevoke {
			for _, p := range ch.Privileges {
				if slices.Contains(currentPrivs, p) {
					unchanged = append(unchanged, p)
				} else {
					applied = append(applied, p)
					toAdd = append(toAdd, catalog.Privilege(p))
					overallChanged = true
				}
			}
		} else {
			for _, p := range ch.Privileges {
				if slices.Contains(currentPrivs, p) {
					applied = append(applied, p)
					toRemove = append(toRemove, catalog.Privilege(p))
					overallChanged = true
				} else {
					unchanged = append(unchanged, p)
				}
			}
		}

		if len(toAdd) > 0 || len(toRemove) > 0 {
			sdkChanges = append(sdkChanges, catalog.PermissionsChange{
				Principal: ch.Principal,
				Add:       toAdd,
				Remove:    toRemove,
			})
		}

		outcomes = append(outcomes, domain.ChangeOutcome{
			Principal: ch.Principal,
			Applied:   applied,
			Unchanged: unchanged,
		})
	}

	// 2. Executa a chamada no Databricks se houver alterações a aplicar
	if len(sdkChanges) > 0 {
		_, err = client.Grants.Update(ctx, catalog.UpdatePermissions{
			SecurableType: string(securableTypeForSecurable(sec.Type)),
			FullName:      sec.Name,
			Changes:       sdkChanges,
		})
		if err != nil {
			return nil, nil, false, mapError(err, ws)
		}
	}

	// 3. Obtém o estado resultante
	updatedGrants, err := a.GetGrants(ctx, ws, sec, nil)
	if err != nil {
		return outcomes, nil, overallChanged, err
	}

	return outcomes, updatedGrants, overallChanged, nil
}

// -----------------------------------------------------------------------------
// CatalogsAPI
// -----------------------------------------------------------------------------

func (a *DatabricksSDKAdapter) ListCatalogs(ctx context.Context, ws string, pageSize int, pageToken string) ([]domain.Catalog, string, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, "", err
	}

	infos, err := client.Catalogs.ListAll(ctx, catalog.ListCatalogsRequest{})
	if err != nil {
		return nil, "", mapError(err, ws)
	}

	var catalogs []domain.Catalog
	for _, info := range infos {
		catalogs = append(catalogs, catalogFromSDK(info))
	}

	return catalogs, "", nil
}

func (a *DatabricksSDKAdapter) GetCatalog(ctx context.Context, ws, name string) (*domain.Catalog, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	info, err := client.Catalogs.GetByName(ctx, name)
	if err != nil {
		return nil, mapError(err, ws)
	}

	c := catalogFromSDK(*info)
	return &c, nil
}

func (a *DatabricksSDKAdapter) CreateCatalog(ctx context.Context, ws string, cat domain.Catalog) (*domain.Catalog, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	req := catalog.CreateCatalog{
		Name:       cat.Name,
		Properties: cat.Properties,
	}
	if cat.Comment != nil {
		req.Comment = *cat.Comment
	}
	if cat.StorageRoot != nil {
		req.StorageRoot = *cat.StorageRoot
	}

	info, err := client.Catalogs.Create(ctx, req)
	if err != nil {
		return nil, mapError(err, ws)
	}

	c := catalogFromSDK(*info)
	return &c, nil
}

func (a *DatabricksSDKAdapter) UpdateCatalog(ctx context.Context, ws, name string, newName, comment *string, props map[string]string) (*domain.Catalog, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	req := catalog.UpdateCatalog{
		Name:       name,
		Properties: props,
	}
	if newName != nil {
		req.NewName = *newName
	}
	if comment != nil {
		req.Comment = *comment
	}

	info, err := client.Catalogs.Update(ctx, req)
	if err != nil {
		return nil, mapError(err, ws)
	}

	c := catalogFromSDK(*info)
	return &c, nil
}

func (a *DatabricksSDKAdapter) DeleteCatalog(ctx context.Context, ws, name string, force bool) error {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return err
	}

	err = client.Catalogs.Delete(ctx, catalog.DeleteCatalogRequest{
		Name:  name,
		Force: force,
	})
	return mapError(err, ws)
}

// -----------------------------------------------------------------------------
// SchemasAPI
// -----------------------------------------------------------------------------

func (a *DatabricksSDKAdapter) ListSchemas(ctx context.Context, ws, catalogName string, pageSize int, pageToken string) ([]domain.DataSchema, string, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, "", err
	}

	infos, err := client.Schemas.ListAll(ctx, catalog.ListSchemasRequest{
		CatalogName: catalogName,
	})
	if err != nil {
		return nil, "", mapError(err, ws)
	}

	var schemas []domain.DataSchema
	for _, info := range infos {
		schemas = append(schemas, schemaFromSDK(info))
	}

	return schemas, "", nil
}

func (a *DatabricksSDKAdapter) GetSchema(ctx context.Context, ws, catalogName, schema string) (*domain.DataSchema, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	fullName := fmt.Sprintf("%s.%s", catalogName, schema)
	info, err := client.Schemas.GetByFullName(ctx, fullName)
	if err != nil {
		return nil, mapError(err, ws)
	}

	s := schemaFromSDK(*info)
	return &s, nil
}

func (a *DatabricksSDKAdapter) CreateSchema(ctx context.Context, ws, catalogName string, s domain.DataSchema) (*domain.DataSchema, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	req := catalog.CreateSchema{
		CatalogName: catalogName,
		Name:        s.Name,
		Properties:  s.Properties,
	}
	if s.Comment != nil {
		req.Comment = *s.Comment
	}
	if s.StorageRoot != nil {
		req.StorageRoot = *s.StorageRoot
	}

	info, err := client.Schemas.Create(ctx, req)
	if err != nil {
		return nil, mapError(err, ws)
	}

	created := schemaFromSDK(*info)
	return &created, nil
}

func (a *DatabricksSDKAdapter) UpdateSchema(ctx context.Context, ws, catalogName, schema string, newName, comment *string, props map[string]string) (*domain.DataSchema, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	fullName := fmt.Sprintf("%s.%s", catalogName, schema)
	req := catalog.UpdateSchema{
		FullName:   fullName,
		Properties: props,
	}
	if newName != nil {
		req.NewName = *newName
	}
	if comment != nil {
		req.Comment = *comment
	}

	info, err := client.Schemas.Update(ctx, req)
	if err != nil {
		return nil, mapError(err, ws)
	}

	updated := schemaFromSDK(*info)
	return &updated, nil
}

func (a *DatabricksSDKAdapter) DeleteSchema(ctx context.Context, ws, catalogName, schema string, force bool) error {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return err
	}

	fullName := fmt.Sprintf("%s.%s", catalogName, schema)
	err = client.Schemas.Delete(ctx, catalog.DeleteSchemaRequest{
		FullName: fullName,
		Force:    force,
	})
	return mapError(err, ws)
}

// -----------------------------------------------------------------------------
// TablesAPI
// -----------------------------------------------------------------------------

func (a *DatabricksSDKAdapter) ListTables(ctx context.Context, ws, catalogName, schema string, pageSize int, pageToken string) ([]domain.Table, string, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, "", err
	}

	infos, err := client.Tables.ListAll(ctx, catalog.ListTablesRequest{
		CatalogName: catalogName,
		SchemaName:  schema,
	})
	if err != nil {
		return nil, "", mapError(err, ws)
	}

	var tables []domain.Table
	for _, info := range infos {
		if isView(info) {
			continue
		}
		tables = append(tables, tableFromSDK(info))
	}

	return tables, "", nil
}

func (a *DatabricksSDKAdapter) GetTable(ctx context.Context, ws, catalogName, schema, table string) (*domain.Table, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	fullName := fmt.Sprintf("%s.%s.%s", catalogName, schema, table)
	info, err := client.Tables.GetByFullName(ctx, fullName)
	if err != nil {
		return nil, mapError(err, ws)
	}
	if isView(*info) {
		return nil, domain.NewNotFoundError("Table", fullName)
	}

	t := tableFromSDK(*info)
	return &t, nil
}

func (a *DatabricksSDKAdapter) DeleteTable(ctx context.Context, ws, catalogName, schema, table string) error {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return err
	}

	fullName := fmt.Sprintf("%s.%s.%s", catalogName, schema, table)
	err = client.Tables.DeleteByFullName(ctx, fullName)
	return mapError(err, ws)
}

// -----------------------------------------------------------------------------
// ViewsAPI
// -----------------------------------------------------------------------------

func (a *DatabricksSDKAdapter) ListViews(ctx context.Context, ws, catalogName, schema string, pageSize int, pageToken string) ([]domain.View, string, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, "", err
	}

	infos, err := client.Tables.ListAll(ctx, catalog.ListTablesRequest{
		CatalogName: catalogName,
		SchemaName:  schema,
	})
	if err != nil {
		return nil, "", mapError(err, ws)
	}

	var views []domain.View
	for _, info := range infos {
		if isView(info) {
			views = append(views, viewFromSDK(info))
		}
	}

	return views, "", nil
}

func (a *DatabricksSDKAdapter) GetView(ctx context.Context, ws, catalogName, schema, view string) (*domain.View, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	fullName := fmt.Sprintf("%s.%s.%s", catalogName, schema, view)
	info, err := client.Tables.GetByFullName(ctx, fullName)
	if err != nil {
		return nil, mapError(err, ws)
	}
	if !isView(*info) {
		return nil, domain.NewNotFoundError("View", fullName)
	}

	v := viewFromSDK(*info)
	return &v, nil
}

func (a *DatabricksSDKAdapter) DeleteView(ctx context.Context, ws, catalogName, schema, view string) error {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return err
	}

	fullName := fmt.Sprintf("%s.%s.%s", catalogName, schema, view)
	err = client.Tables.DeleteByFullName(ctx, fullName)
	return mapError(err, ws)
}

// -----------------------------------------------------------------------------
// VolumesAPI
// -----------------------------------------------------------------------------

func (a *DatabricksSDKAdapter) ListVolumes(ctx context.Context, ws, catalogName, schema string, pageSize int, pageToken string) ([]domain.Volume, string, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, "", err
	}

	infos, err := client.Volumes.ListAll(ctx, catalog.ListVolumesRequest{
		CatalogName: catalogName,
		SchemaName:  schema,
	})
	if err != nil {
		return nil, "", mapError(err, ws)
	}

	var volumes []domain.Volume
	for _, info := range infos {
		volumes = append(volumes, volumeFromSDK(info))
	}

	return volumes, "", nil
}

func (a *DatabricksSDKAdapter) GetVolume(ctx context.Context, ws, catalogName, schema, volume string) (*domain.Volume, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	fullName := fmt.Sprintf("%s.%s.%s", catalogName, schema, volume)
	info, err := client.Volumes.ReadByName(ctx, fullName)
	if err != nil {
		return nil, mapError(err, ws)
	}

	v := volumeFromSDK(*info)
	return &v, nil
}

func (a *DatabricksSDKAdapter) CreateVolume(ctx context.Context, ws, catalogName, schema string, v domain.Volume) (*domain.Volume, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	req := catalog.CreateVolumeRequestContent{
		CatalogName: catalogName,
		SchemaName:  schema,
		Name:        v.Name,
		VolumeType:  catalog.VolumeType(v.VolumeType),
	}
	if v.Comment != nil {
		req.Comment = *v.Comment
	}
	if v.StorageLocation != nil {
		req.StorageLocation = *v.StorageLocation
	}

	info, err := client.Volumes.Create(ctx, req)
	if err != nil {
		return nil, mapError(err, ws)
	}

	created := volumeFromSDK(*info)
	return &created, nil
}

func (a *DatabricksSDKAdapter) UpdateVolume(ctx context.Context, ws, catalogName, schema, volume string, newName, comment *string) (*domain.Volume, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	fullName := fmt.Sprintf("%s.%s.%s", catalogName, schema, volume)
	req := catalog.UpdateVolumeRequestContent{
		Name: fullName,
	}
	if newName != nil {
		req.NewName = *newName
	}
	if comment != nil {
		req.Comment = *comment
	}

	info, err := client.Volumes.Update(ctx, req)
	if err != nil {
		return nil, mapError(err, ws)
	}

	updated := volumeFromSDK(*info)
	return &updated, nil
}

func (a *DatabricksSDKAdapter) DeleteVolume(ctx context.Context, ws, catalogName, schema, volume string) error {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return err
	}

	fullName := fmt.Sprintf("%s.%s.%s", catalogName, schema, volume)
	err = client.Volumes.DeleteByName(ctx, fullName)
	return mapError(err, ws)
}

// -----------------------------------------------------------------------------
// OwnershipAPI
// -----------------------------------------------------------------------------

func (a *DatabricksSDKAdapter) GetOwner(ctx context.Context, ws string, sec domain.SecurableRef) (string, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return "", err
	}

	switch sec.Type {
	case domain.SecurableCatalog:
		info, err := client.Catalogs.GetByName(ctx, sec.Name)
		if err != nil {
			return "", mapError(err, ws)
		}
		return info.Owner, nil

	case domain.SecurableSchema:
		info, err := client.Schemas.GetByFullName(ctx, sec.Name)
		if err != nil {
			return "", mapError(err, ws)
		}
		return info.Owner, nil

	case domain.SecurableTable, domain.SecurableView:
		info, err := client.Tables.GetByFullName(ctx, sec.Name)
		if err != nil {
			return "", mapError(err, ws)
		}
		return info.Owner, nil

	case domain.SecurableVolume:
		info, err := client.Volumes.ReadByName(ctx, sec.Name)
		if err != nil {
			return "", mapError(err, ws)
		}
		return info.Owner, nil

	default:
		return "", domain.NewBadRequestError(fmt.Sprintf("tipo de securable não suportado para ownership: %s", sec.Type))
	}
}

func (a *DatabricksSDKAdapter) SetOwner(ctx context.Context, ws string, sec domain.SecurableRef, newOwner string) error {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return err
	}

	switch sec.Type {
	case domain.SecurableCatalog:
		_, err := client.Catalogs.Update(ctx, catalog.UpdateCatalog{
			Name:  sec.Name,
			Owner: newOwner,
		})
		return mapError(err, ws)

	case domain.SecurableSchema:
		_, err := client.Schemas.Update(ctx, catalog.UpdateSchema{
			FullName: sec.Name,
			Owner:    newOwner,
		})
		return mapError(err, ws)

	case domain.SecurableTable, domain.SecurableView:
		err := client.Tables.Update(ctx, catalog.UpdateTableRequest{
			FullName: sec.Name,
			Owner:    newOwner,
		})
		return mapError(err, ws)

	case domain.SecurableVolume:
		_, err := client.Volumes.Update(ctx, catalog.UpdateVolumeRequestContent{
			Name:  sec.Name,
			Owner: newOwner,
		})
		return mapError(err, ws)

	default:
		return domain.NewBadRequestError(fmt.Sprintf("tipo de securable não suportado para ownership: %s", sec.Type))
	}
}

// -----------------------------------------------------------------------------
// TagsAPI
// -----------------------------------------------------------------------------

func (a *DatabricksSDKAdapter) ListTags(ctx context.Context, ws string, sec domain.SecurableRef) (map[string]string, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	entityType := entityTypeForSecurable(sec.Type)
	assignments, err := client.EntityTagAssignments.ListAll(ctx, catalog.ListEntityTagAssignmentsRequest{
		EntityType: entityType,
		EntityName: sec.Name,
	})
	if err != nil {
		return nil, mapError(err, ws)
	}

	tags := make(map[string]string)
	for _, as := range assignments {
		tags[as.TagKey] = as.TagValue
	}

	return tags, nil
}

func (a *DatabricksSDKAdapter) SetTag(ctx context.Context, ws string, sec domain.SecurableRef, key, value string) error {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return err
	}

	entityType := entityTypeForSecurable(sec.Type)
	assignment := catalog.EntityTagAssignment{
		EntityType: entityType,
		EntityName: sec.Name,
		TagKey:     key,
		TagValue:   value,
	}

	_, err = client.EntityTagAssignments.Create(ctx, catalog.CreateEntityTagAssignmentRequest{
		TagAssignment: assignment,
	})
	if err != nil {
		// Se a tag já existir (conflito ou bad request de unicidade), tenta o Update
		var apiErr *apierr.APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusConflict || apiErr.StatusCode == http.StatusBadRequest) {
			_, updateErr := client.EntityTagAssignments.Update(ctx, catalog.UpdateEntityTagAssignmentRequest{
				TagAssignment: assignment,
			})
			if updateErr == nil {
				return nil
			}
		}
		return mapError(err, ws)
	}

	return nil
}

func (a *DatabricksSDKAdapter) DeleteTag(ctx context.Context, ws string, sec domain.SecurableRef, key string) error {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return err
	}

	entityType := entityTypeForSecurable(sec.Type)
	err = client.EntityTagAssignments.Delete(ctx, catalog.DeleteEntityTagAssignmentRequest{
		EntityType: entityType,
		EntityName: sec.Name,
		TagKey:     key,
	})
	return mapError(err, ws)
}

// -----------------------------------------------------------------------------
// IdentityAPI
// -----------------------------------------------------------------------------

func (a *DatabricksSDKAdapter) ListPrincipals(ctx context.Context, ws string, pType *domain.PrincipalType, query *string, pageSize int, pageToken string) ([]domain.Principal, string, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, "", err
	}

	var principals []domain.Principal

	if pType == nil || *pType == domain.PrincipalTypeServicePrincipal {
		sps, err := client.ServicePrincipals.ListAll(ctx, iam.ListServicePrincipalsRequest{})
		if err != nil {
			return nil, "", mapError(err, ws)
		}
		for _, sp := range sps {
			name := sp.ApplicationId
			if name == "" {
				name = sp.DisplayName
			}
			principals = append(principals, domain.Principal{
				Name:   name,
				Type:   domain.PrincipalTypeServicePrincipal,
				ID:     sp.Id,
				Active: sp.Active,
			})
		}
	}

	if pType == nil || *pType == domain.PrincipalTypeUser {
		users, err := client.Users.ListAll(ctx, iam.ListUsersRequest{})
		if err != nil {
			return nil, "", mapError(err, ws)
		}
		for _, u := range users {
			principals = append(principals, domain.Principal{
				Name:   u.UserName,
				Type:   domain.PrincipalTypeUser,
				ID:     u.Id,
				Active: u.Active,
			})
		}
	}

	if pType == nil || *pType == domain.PrincipalTypeGroup {
		groups, err := client.Groups.ListAll(ctx, iam.ListGroupsRequest{})
		if err != nil {
			return nil, "", mapError(err, ws)
		}
		for _, g := range groups {
			principals = append(principals, domain.Principal{
				Name:   g.DisplayName,
				Type:   domain.PrincipalTypeGroup,
				ID:     g.Id,
				Active: true,
			})
		}
	}

	if query != nil && *query != "" {
		q := strings.ToLower(*query)
		var filtered []domain.Principal
		for _, p := range principals {
			if strings.Contains(strings.ToLower(p.Name), q) {
				filtered = append(filtered, p)
			}
		}
		principals = filtered
	}

	return principals, "", nil
}

func (a *DatabricksSDKAdapter) GetPrincipal(ctx context.Context, ws, name string) (*domain.Principal, error) {
	client, err := a.getClient(ctx, ws)
	if err != nil {
		return nil, err
	}

	// 1. Tenta identificar como Service Principal
	sps, err := client.ServicePrincipals.ListAll(ctx, iam.ListServicePrincipalsRequest{})
	if err == nil {
		for _, sp := range sps {
			if strings.EqualFold(sp.ApplicationId, name) || strings.EqualFold(sp.DisplayName, name) {
				pName := sp.ApplicationId
				if pName == "" {
					pName = sp.DisplayName
				}
				return &domain.Principal{
					Name:   pName,
					Type:   domain.PrincipalTypeServicePrincipal,
					ID:     sp.Id,
					Active: sp.Active,
				}, nil
			}
		}
	}

	// 2. Tenta identificar como Usuário
	u, err := client.Users.GetByUserName(ctx, name)
	if err == nil && u != nil && u.UserName != "" {
		return &domain.Principal{
			Name:   u.UserName,
			Type:   domain.PrincipalTypeUser,
			ID:     u.Id,
			Active: u.Active,
		}, nil
	}

	// 3. Tenta identificar como Grupo
	g, err := client.Groups.GetByDisplayName(ctx, name)
	if err == nil && g != nil && g.DisplayName != "" {
		return &domain.Principal{
			Name:   g.DisplayName,
			Type:   domain.PrincipalTypeGroup,
			ID:     g.Id,
			Active: true,
		}, nil
	}

	return nil, domain.NewPrincipalNotFoundError(name)
}

// -----------------------------------------------------------------------------
// Funções Utilitárias Internas de Conversão
// -----------------------------------------------------------------------------

func isView(info catalog.TableInfo) bool {
	tt := strings.ToUpper(string(info.TableType))
	return strings.Contains(tt, "VIEW")
}

func entityTypeForSecurable(secType domain.SecurableType) string {
	switch secType {
	case domain.SecurableCatalog:
		return "catalogs"
	case domain.SecurableSchema:
		return "schemas"
	case domain.SecurableTable, domain.SecurableView:
		return "tables"
	case domain.SecurableVolume:
		return "volumes"
	default:
		return strings.ToLower(string(secType)) + "s"
	}
}

func securableTypeForSecurable(secType domain.SecurableType) catalog.SecurableType {
	switch secType {
	case domain.SecurableCatalog:
		return catalog.SecurableTypeCatalog
	case domain.SecurableSchema:
		return catalog.SecurableTypeSchema
	case domain.SecurableTable, domain.SecurableView:
		return catalog.SecurableTypeTable
	case domain.SecurableVolume:
		return catalog.SecurableTypeVolume
	default:
		return catalog.SecurableType(secType)
	}
}

func catalogFromSDK(info catalog.CatalogInfo) domain.Catalog {
	c := domain.Catalog{
		Name:        info.Name,
		Owner:       info.Owner,
		CatalogType: string(info.CatalogType),
		Properties:  info.Properties,
	}
	if info.Comment != "" {
		c.Comment = &info.Comment
	}
	if info.StorageRoot != "" {
		c.StorageRoot = &info.StorageRoot
	}
	if info.CreatedAt > 0 {
		t := time.UnixMilli(info.CreatedAt).UTC()
		c.CreatedAt = &t
	}
	if info.UpdatedAt > 0 {
		t := time.UnixMilli(info.UpdatedAt).UTC()
		c.UpdatedAt = &t
	}
	return c
}

func schemaFromSDK(info catalog.SchemaInfo) domain.DataSchema {
	s := domain.DataSchema{
		Name:        info.Name,
		CatalogName: info.CatalogName,
		FullName:    info.FullName,
		Owner:       info.Owner,
		Properties:  info.Properties,
	}
	if s.FullName == "" {
		s.FullName = fmt.Sprintf("%s.%s", info.CatalogName, info.Name)
	}
	if info.Comment != "" {
		s.Comment = &info.Comment
	}
	if info.StorageRoot != "" {
		s.StorageRoot = &info.StorageRoot
	}
	if info.CreatedAt > 0 {
		t := time.UnixMilli(info.CreatedAt).UTC()
		s.CreatedAt = &t
	}
	if info.UpdatedAt > 0 {
		t := time.UnixMilli(info.UpdatedAt).UTC()
		s.UpdatedAt = &t
	}
	return s
}

func tableFromSDK(info catalog.TableInfo) domain.Table {
	t := domain.Table{
		Name:        info.Name,
		CatalogName: info.CatalogName,
		SchemaName:  info.SchemaName,
		FullName:    info.FullName,
		TableType:   string(info.TableType),
		Owner:       info.Owner,
	}
	if t.FullName == "" {
		t.FullName = fmt.Sprintf("%s.%s.%s", info.CatalogName, info.SchemaName, info.Name)
	}
	if info.Comment != "" {
		t.Comment = &info.Comment
	}
	if info.DataSourceFormat != "" {
		dsf := string(info.DataSourceFormat)
		t.DataSourceFormat = &dsf
	}
	if info.StorageLocation != "" {
		t.StorageLocation = &info.StorageLocation
	}
	if info.CreatedAt > 0 {
		tm := time.UnixMilli(info.CreatedAt).UTC()
		t.CreatedAt = &tm
	}
	if info.UpdatedAt > 0 {
		tm := time.UnixMilli(info.UpdatedAt).UTC()
		t.UpdatedAt = &tm
	}
	for _, col := range info.Columns {
		c := domain.Column{
			Name:     col.Name,
			TypeText: col.TypeText,
			Position: col.Position,
			Nullable: col.Nullable,
		}
		if col.Comment != "" {
			c.Comment = &col.Comment
		}
		t.Columns = append(t.Columns, c)
	}
	return t
}

func viewFromSDK(info catalog.TableInfo) domain.View {
	v := domain.View{
		Name:        info.Name,
		CatalogName: info.CatalogName,
		SchemaName:  info.SchemaName,
		FullName:    info.FullName,
		Owner:       info.Owner,
	}
	if v.FullName == "" {
		v.FullName = fmt.Sprintf("%s.%s.%s", info.CatalogName, info.SchemaName, info.Name)
	}
	if info.Comment != "" {
		v.Comment = &info.Comment
	}
	if info.ViewDefinition != "" {
		v.ViewDefinition = &info.ViewDefinition
	}
	if info.CreatedAt > 0 {
		tm := time.UnixMilli(info.CreatedAt).UTC()
		v.CreatedAt = &tm
	}
	if info.UpdatedAt > 0 {
		tm := time.UnixMilli(info.UpdatedAt).UTC()
		v.UpdatedAt = &tm
	}
	for _, col := range info.Columns {
		c := domain.Column{
			Name:     col.Name,
			TypeText: col.TypeText,
			Position: col.Position,
			Nullable: col.Nullable,
		}
		if col.Comment != "" {
			c.Comment = &col.Comment
		}
		v.Columns = append(v.Columns, c)
	}
	return v
}

func volumeFromSDK(info catalog.VolumeInfo) domain.Volume {
	v := domain.Volume{
		Name:        info.Name,
		CatalogName: info.CatalogName,
		SchemaName:  info.SchemaName,
		FullName:    info.FullName,
		VolumeType:  string(info.VolumeType),
		Owner:       info.Owner,
	}
	if v.FullName == "" {
		v.FullName = fmt.Sprintf("%s.%s.%s", info.CatalogName, info.SchemaName, info.Name)
	}
	if info.Comment != "" {
		v.Comment = &info.Comment
	}
	if info.StorageLocation != "" {
		v.StorageLocation = &info.StorageLocation
	}
	if info.CreatedAt > 0 {
		t := time.UnixMilli(info.CreatedAt).UTC()
		v.CreatedAt = &t
	}
	if info.UpdatedAt > 0 {
		t := time.UnixMilli(info.UpdatedAt).UTC()
		v.UpdatedAt = &t
	}
	return v
}
