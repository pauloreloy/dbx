package memory

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/pauloeloy/databricks-governance-service/internal/domain"
)

// MemoryWorkspaceRegistry armazena definições de workspace.
type MemoryWorkspaceRegistry struct {
	mu         sync.RWMutex
	workspaces map[string]domain.Workspace
}

func NewMemoryWorkspaceRegistry(initial []domain.Workspace) *MemoryWorkspaceRegistry {
	r := &MemoryWorkspaceRegistry{
		workspaces: make(map[string]domain.Workspace),
	}
	for _, ws := range initial {
		r.workspaces[ws.ID] = ws
	}
	return r
}

func (r *MemoryWorkspaceRegistry) ListWorkspaces(ctx context.Context, pageSize int, pageToken string) ([]domain.Workspace, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []domain.Workspace
	for _, ws := range r.workspaces {
		list = append(list, ws)
	}
	return list, "", nil
}

func (r *MemoryWorkspaceRegistry) GetWorkspace(ctx context.Context, alias string) (*domain.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, ok := r.workspaces[alias]
	if !ok {
		return nil, domain.NewNotFoundError("Workspace", alias)
	}
	return &ws, nil
}

// MemoryDatabricksClient simula a API do Databricks em memória para testes e execução autônoma.
type MemoryDatabricksClient struct {
	mu         sync.RWMutex
	catalogs   map[string]domain.Catalog
	schemas    map[string]domain.DataSchema
	tables     map[string]domain.Table
	views      map[string]domain.View
	volumes    map[string]domain.Volume
	grants     map[string]map[string][]domain.Privilege // securableKey -> principal -> []Privilege
	owners     map[string]string                        // securableKey -> owner
	tags       map[string]map[string]string             // securableKey -> key -> value
	principals map[string]domain.Principal              // principalName -> Principal
}

func NewMemoryDatabricksClient() *MemoryDatabricksClient {
	now := time.Now().UTC()
	c := &MemoryDatabricksClient{
		catalogs:   make(map[string]domain.Catalog),
		schemas:    make(map[string]domain.DataSchema),
		tables:     make(map[string]domain.Table),
		views:      make(map[string]domain.View),
		volumes:    make(map[string]domain.Volume),
		grants:     make(map[string]map[string][]domain.Privilege),
		owners:     make(map[string]string),
		tags:       make(map[string]map[string]string),
		principals: make(map[string]domain.Principal),
	}

	// Adiciona dados iniciais de exemplo
	c.principals["admin@company.com"] = domain.Principal{Name: "admin@company.com", Type: domain.PrincipalTypeUser, Active: true}
	c.principals["analyst@company.com"] = domain.Principal{Name: "analyst@company.com", Type: domain.PrincipalTypeUser, Active: true}
	c.principals["grp-analytics"] = domain.Principal{Name: "grp-analytics", Type: domain.PrincipalTypeGroup, Active: true}
	c.principals["sp-governance-service"] = domain.Principal{Name: "sp-governance-service", Type: domain.PrincipalTypeServicePrincipal, Active: true}

	catName := "main"
	c.catalogs[catName] = domain.Catalog{
		Name:        catName,
		Owner:       "sp-governance-service",
		CatalogType: "MANAGED",
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}
	c.owners["CATALOG:main"] = "sp-governance-service"

	return c
}

func (m *MemoryDatabricksClient) secKey(sec domain.SecurableRef) string {
	return sec.String()
}

// Grants
func (m *MemoryDatabricksClient) GetGrants(ctx context.Context, ws string, sec domain.SecurableRef, principalFilter *string) ([]domain.PrincipalPrivileges, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.secKey(sec)
	objGrants, ok := m.grants[key]
	if !ok {
		return []domain.PrincipalPrivileges{}, nil
	}

	var res []domain.PrincipalPrivileges
	for princ, privs := range objGrants {
		if principalFilter != nil && princ != *principalFilter {
			continue
		}
		res = append(res, domain.PrincipalPrivileges{
			Principal:  princ,
			Privileges: privs,
		})
	}
	return res, nil
}

func (m *MemoryDatabricksClient) GetEffectiveGrants(ctx context.Context, ws string, sec domain.SecurableRef, principalFilter *string) ([]domain.EffectiveGrant, error) {
	direct, err := m.GetGrants(ctx, ws, sec, principalFilter)
	if err != nil {
		return nil, err
	}

	var res []domain.EffectiveGrant
	for _, d := range direct {
		effPrivs := make([]domain.EffectivePrivilege, len(d.Privileges))
		for i, p := range d.Privileges {
			effPrivs[i] = domain.EffectivePrivilege{
				Privilege: p,
				Inherited: false,
			}
		}
		res = append(res, domain.EffectiveGrant{
			Principal:  d.Principal,
			Privileges: effPrivs,
		})
	}
	return res, nil
}

func (m *MemoryDatabricksClient) UpdateGrants(ctx context.Context, ws string, sec domain.SecurableRef, changes []domain.PrivilegeChange, isRevoke bool) ([]domain.ChangeOutcome, []domain.PrincipalPrivileges, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.secKey(sec)
	if _, ok := m.grants[key]; !ok {
		m.grants[key] = make(map[string][]domain.Privilege)
	}

	var outcomes []domain.ChangeOutcome
	overallChanged := false

	for _, ch := range changes {
		currentPrivs := m.grants[key][ch.Principal]
		var applied []domain.Privilege
		var unchanged []domain.Privilege

		if !isRevoke {
			// Grant
			for _, p := range ch.Privileges {
				if slices.Contains(currentPrivs, p) {
					unchanged = append(unchanged, p)
				} else {
					applied = append(applied, p)
					currentPrivs = append(currentPrivs, p)
					overallChanged = true
				}
			}
		} else {
			// Revoke
			var remaining []domain.Privilege
			for _, cp := range currentPrivs {
				if slices.Contains(ch.Privileges, cp) {
					applied = append(applied, cp)
					overallChanged = true
				} else {
					remaining = append(remaining, cp)
				}
			}
			for _, p := range ch.Privileges {
				if !slices.Contains(currentPrivs, p) {
					unchanged = append(unchanged, p)
				}
			}
			currentPrivs = remaining
		}

		m.grants[key][ch.Principal] = currentPrivs
		outcomes = append(outcomes, domain.ChangeOutcome{
			Principal: ch.Principal,
			Applied:   applied,
			Unchanged: unchanged,
		})
	}

	var resultingGrants []domain.PrincipalPrivileges
	for princ, privs := range m.grants[key] {
		if len(privs) > 0 {
			resultingGrants = append(resultingGrants, domain.PrincipalPrivileges{
				Principal:  princ,
				Privileges: privs,
			})
		}
	}

	return outcomes, resultingGrants, overallChanged, nil
}

// Catalogs
func (m *MemoryDatabricksClient) ListCatalogs(ctx context.Context, ws string, pageSize int, pageToken string) ([]domain.Catalog, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []domain.Catalog
	for _, c := range m.catalogs {
		list = append(list, c)
	}
	return list, "", nil
}

func (m *MemoryDatabricksClient) GetCatalog(ctx context.Context, ws, name string) (*domain.Catalog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c, ok := m.catalogs[name]
	if !ok {
		return nil, domain.NewNotFoundError("Catalog", name)
	}
	return &c, nil
}

func (m *MemoryDatabricksClient) CreateCatalog(ctx context.Context, ws string, cat domain.Catalog) (*domain.Catalog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.catalogs[cat.Name]; exists {
		return nil, domain.NewConflictError(fmt.Sprintf("Catálogo '%s' já existe", cat.Name))
	}

	now := time.Now().UTC()
	cat.CreatedAt = &now
	cat.UpdatedAt = &now
	if cat.Owner == "" {
		cat.Owner = "sp-governance-service"
	}
	m.catalogs[cat.Name] = cat
	m.owners[fmt.Sprintf("CATALOG:%s", cat.Name)] = cat.Owner

	return &cat, nil
}

func (m *MemoryDatabricksClient) UpdateCatalog(ctx context.Context, ws, name string, newName, comment *string, props map[string]string) (*domain.Catalog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.catalogs[name]
	if !ok {
		return nil, domain.NewNotFoundError("Catalog", name)
	}

	if newName != nil && *newName != name {
		delete(m.catalogs, name)
		c.Name = *newName
	}
	if comment != nil {
		c.Comment = comment
	}
	if props != nil {
		c.Properties = props
	}
	now := time.Now().UTC()
	c.UpdatedAt = &now
	m.catalogs[c.Name] = c

	return &c, nil
}

func (m *MemoryDatabricksClient) DeleteCatalog(ctx context.Context, ws, name string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.catalogs[name]; !ok {
		return domain.NewNotFoundError("Catalog", name)
	}
	delete(m.catalogs, name)
	delete(m.owners, fmt.Sprintf("CATALOG:%s", name))
	return nil
}

// Schemas
func (m *MemoryDatabricksClient) ListSchemas(ctx context.Context, ws, catalog string, pageSize int, pageToken string) ([]domain.DataSchema, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []domain.DataSchema
	for _, s := range m.schemas {
		if s.CatalogName == catalog {
			list = append(list, s)
		}
	}
	return list, "", nil
}

func (m *MemoryDatabricksClient) GetSchema(ctx context.Context, ws, catalog, schema string) (*domain.DataSchema, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s.%s", catalog, schema)
	s, ok := m.schemas[key]
	if !ok {
		return nil, domain.NewNotFoundError("Schema", key)
	}
	return &s, nil
}

func (m *MemoryDatabricksClient) CreateSchema(ctx context.Context, ws, catalog string, s domain.DataSchema) (*domain.DataSchema, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s.%s", catalog, s.Name)
	if _, exists := m.schemas[key]; exists {
		return nil, domain.NewConflictError(fmt.Sprintf("Schema '%s' já existe", key))
	}

	now := time.Now().UTC()
	s.CatalogName = catalog
	s.FullName = key
	s.CreatedAt = &now
	s.UpdatedAt = &now
	if s.Owner == "" {
		s.Owner = "sp-governance-service"
	}
	m.schemas[key] = s
	m.owners[fmt.Sprintf("SCHEMA:%s", key)] = s.Owner

	return &s, nil
}

func (m *MemoryDatabricksClient) UpdateSchema(ctx context.Context, ws, catalog, schema string, newName, comment *string, props map[string]string) (*domain.DataSchema, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s.%s", catalog, schema)
	s, ok := m.schemas[key]
	if !ok {
		return nil, domain.NewNotFoundError("Schema", key)
	}

	if newName != nil && *newName != schema {
		delete(m.schemas, key)
		s.Name = *newName
		key = fmt.Sprintf("%s.%s", catalog, *newName)
		s.FullName = key
	}
	if comment != nil {
		s.Comment = comment
	}
	if props != nil {
		s.Properties = props
	}
	now := time.Now().UTC()
	s.UpdatedAt = &now
	m.schemas[key] = s

	return &s, nil
}

func (m *MemoryDatabricksClient) DeleteSchema(ctx context.Context, ws, catalog, schema string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s.%s", catalog, schema)
	if _, ok := m.schemas[key]; !ok {
		return domain.NewNotFoundError("Schema", key)
	}
	delete(m.schemas, key)
	delete(m.owners, fmt.Sprintf("SCHEMA:%s", key))
	return nil
}

// Tables
func (m *MemoryDatabricksClient) ListTables(ctx context.Context, ws, catalog, schema string, pageSize int, pageToken string) ([]domain.Table, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []domain.Table
	for _, t := range m.tables {
		if t.CatalogName == catalog && t.SchemaName == schema {
			list = append(list, t)
		}
	}
	return list, "", nil
}

func (m *MemoryDatabricksClient) GetTable(ctx context.Context, ws, catalog, schema, table string) (*domain.Table, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s.%s.%s", catalog, schema, table)
	t, ok := m.tables[key]
	if !ok {
		return nil, domain.NewNotFoundError("Table", key)
	}
	return &t, nil
}

func (m *MemoryDatabricksClient) DeleteTable(ctx context.Context, ws, catalog, schema, table string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s.%s.%s", catalog, schema, table)
	if _, ok := m.tables[key]; !ok {
		return domain.NewNotFoundError("Table", key)
	}
	delete(m.tables, key)
	return nil
}

// Views
func (m *MemoryDatabricksClient) ListViews(ctx context.Context, ws, catalog, schema string, pageSize int, pageToken string) ([]domain.View, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []domain.View
	for _, v := range m.views {
		if v.CatalogName == catalog && v.SchemaName == schema {
			list = append(list, v)
		}
	}
	return list, "", nil
}

func (m *MemoryDatabricksClient) GetView(ctx context.Context, ws, catalog, schema, view string) (*domain.View, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s.%s.%s", catalog, schema, view)
	v, ok := m.views[key]
	if !ok {
		return nil, domain.NewNotFoundError("View", key)
	}
	return &v, nil
}

func (m *MemoryDatabricksClient) DeleteView(ctx context.Context, ws, catalog, schema, view string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s.%s.%s", catalog, schema, view)
	if _, ok := m.views[key]; !ok {
		return domain.NewNotFoundError("View", key)
	}
	delete(m.views, key)
	return nil
}

// Volumes
func (m *MemoryDatabricksClient) ListVolumes(ctx context.Context, ws, catalog, schema string, pageSize int, pageToken string) ([]domain.Volume, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []domain.Volume
	for _, v := range m.volumes {
		if v.CatalogName == catalog && v.SchemaName == schema {
			list = append(list, v)
		}
	}
	return list, "", nil
}

func (m *MemoryDatabricksClient) GetVolume(ctx context.Context, ws, catalog, schema, volume string) (*domain.Volume, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s.%s.%s", catalog, schema, volume)
	v, ok := m.volumes[key]
	if !ok {
		return nil, domain.NewNotFoundError("Volume", key)
	}
	return &v, nil
}

func (m *MemoryDatabricksClient) CreateVolume(ctx context.Context, ws, catalog, schema string, v domain.Volume) (*domain.Volume, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s.%s.%s", catalog, schema, v.Name)
	if _, exists := m.volumes[key]; exists {
		return nil, domain.NewConflictError(fmt.Sprintf("Volume '%s' já existe", key))
	}

	now := time.Now().UTC()
	v.CatalogName = catalog
	v.SchemaName = schema
	v.FullName = key
	v.CreatedAt = &now
	v.UpdatedAt = &now
	if v.Owner == "" {
		v.Owner = "sp-governance-service"
	}
	m.volumes[key] = v
	m.owners[fmt.Sprintf("VOLUME:%s", key)] = v.Owner

	return &v, nil
}

func (m *MemoryDatabricksClient) UpdateVolume(ctx context.Context, ws, catalog, schema, volume string, newName, comment *string) (*domain.Volume, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s.%s.%s", catalog, schema, volume)
	v, ok := m.volumes[key]
	if !ok {
		return nil, domain.NewNotFoundError("Volume", key)
	}

	if newName != nil && *newName != volume {
		delete(m.volumes, key)
		v.Name = *newName
		key = fmt.Sprintf("%s.%s.%s", catalog, schema, *newName)
		v.FullName = key
	}
	if comment != nil {
		v.Comment = comment
	}
	now := time.Now().UTC()
	v.UpdatedAt = &now
	m.volumes[key] = v

	return &v, nil
}

func (m *MemoryDatabricksClient) DeleteVolume(ctx context.Context, ws, catalog, schema, volume string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s.%s.%s", catalog, schema, volume)
	if _, ok := m.volumes[key]; !ok {
		return domain.NewNotFoundError("Volume", key)
	}
	delete(m.volumes, key)
	return nil
}

// Ownership
func (m *MemoryDatabricksClient) GetOwner(ctx context.Context, ws string, sec domain.SecurableRef) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.secKey(sec)
	owner, ok := m.owners[key]
	if !ok {
		return "", domain.NewNotFoundError(string(sec.Type), sec.Name)
	}
	return owner, nil
}

func (m *MemoryDatabricksClient) SetOwner(ctx context.Context, ws string, sec domain.SecurableRef, newOwner string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.secKey(sec)
	m.owners[key] = newOwner
	return nil
}

// Tags
func (m *MemoryDatabricksClient) ListTags(ctx context.Context, ws string, sec domain.SecurableRef) (map[string]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.secKey(sec)
	tags, ok := m.tags[key]
	if !ok {
		return make(map[string]string), nil
	}
	copy := make(map[string]string)
	for k, v := range tags {
		copy[k] = v
	}
	return copy, nil
}

func (m *MemoryDatabricksClient) SetTag(ctx context.Context, ws string, sec domain.SecurableRef, k, v string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.secKey(sec)
	if _, ok := m.tags[key]; !ok {
		m.tags[key] = make(map[string]string)
	}
	m.tags[key][k] = v
	return nil
}

func (m *MemoryDatabricksClient) DeleteTag(ctx context.Context, ws string, sec domain.SecurableRef, k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.secKey(sec)
	if tags, ok := m.tags[key]; ok {
		delete(tags, k)
	}
	return nil
}

// Principals
func (m *MemoryDatabricksClient) ListPrincipals(ctx context.Context, ws string, pType *domain.PrincipalType, query *string, pageSize int, pageToken string) ([]domain.Principal, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []domain.Principal
	for _, p := range m.principals {
		if pType != nil && p.Type != *pType {
			continue
		}
		list = append(list, p)
	}
	return list, "", nil
}

func (m *MemoryDatabricksClient) GetPrincipal(ctx context.Context, ws, name string) (*domain.Principal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, ok := m.principals[name]
	if !ok {
		return nil, domain.NewPrincipalNotFoundError(name)
	}
	return &p, nil
}
