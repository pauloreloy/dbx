package databricks

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/databricks/databricks-sdk-go/apierr"
	"github.com/databricks/databricks-sdk-go/service/catalog"
	"github.com/pauloeloy/databricks-governance-service/internal/domain"
)

func TestMapError(t *testing.T) {
	ws := "prod-br"

	// 1. Nil error
	if err := mapError(nil, ws); err != nil {
		t.Errorf("esperava nil para erro nil, obteve: %v", err)
	}

	// 2. ProblemError existente (passthrough)
	origProb := domain.NewNotFoundError("Catalog", "gold")
	mappedProb := mapError(origProb, ws)
	if mappedProb != origProb {
		t.Errorf("esperava passthrough de ProblemError")
	}

	// 3. apierr.APIError 404
	sdkErr404 := &apierr.APIError{
		ErrorCode:  "RESOURCE_DOES_NOT_EXIST",
		Message:    "Catalog 'test' not found",
		StatusCode: 404,
	}
	err := mapError(sdkErr404, ws)
	var prob *domain.ProblemError
	if !errors.As(err, &prob) {
		t.Fatalf("esperava que erro fosse *domain.ProblemError")
	}
	if prob.Status != 404 {
		t.Errorf("esperava status 404, obteve %d", prob.Status)
	}
	if prob.Type != "/problems/not-found" {
		t.Errorf("esperava type /problems/not-found, obteve %s", prob.Type)
	}
	if prob.Workspace != ws {
		t.Errorf("esperava workspace '%s', obteve '%s'", ws, prob.Workspace)
	}
	if prob.Upstream == nil {
		t.Fatalf("esperava Upstream não nulo")
	}
	if prob.Upstream.ErrorCode != "RESOURCE_DOES_NOT_EXIST" {
		t.Errorf("esperava ErrorCode 'RESOURCE_DOES_NOT_EXIST', obteve '%s'", prob.Upstream.ErrorCode)
	}
	if prob.Upstream.Message != "Catalog 'test' not found" {
		t.Errorf("esperava Message esperada, obteve '%s'", prob.Upstream.Message)
	}
	if prob.Upstream.HTTPStatus != 404 {
		t.Errorf("esperava HTTPStatus 404, obteve %d", prob.Upstream.HTTPStatus)
	}

	// 4. apierr.APIError 409 (Conflict)
	sdkErr409 := &apierr.APIError{
		ErrorCode:  "RESOURCE_ALREADY_EXISTS",
		Message:    "Catalog 'test' already exists",
		StatusCode: 409,
	}
	err409 := mapError(sdkErr409, ws)
	var prob409 *domain.ProblemError
	if !errors.As(err409, &prob409) || prob409.Status != 409 || prob409.Type != "/problems/conflict" {
		t.Errorf("mapeamento de 409 incorreto: %v", err409)
	}

	// 5. apierr.APIError 400 (Bad Request)
	sdkErr400 := &apierr.APIError{
		ErrorCode:  "INVALID_PARAMETER_VALUE",
		Message:    "Invalid catalog name",
		StatusCode: 400,
	}
	err400 := mapError(sdkErr400, ws)
	var prob400 *domain.ProblemError
	if !errors.As(err400, &prob400) || prob400.Status != 400 || prob400.Type != "/problems/bad-request" {
		t.Errorf("mapeamento de 400 incorreto: %v", err400)
	}

	// 6. apierr.APIError 403 (Forbidden)
	sdkErr403 := &apierr.APIError{
		ErrorCode:  "PERMISSION_DENIED",
		Message:    "User does not have USE_CATALOG",
		StatusCode: 403,
	}
	err403 := mapError(sdkErr403, ws)
	var prob403 *domain.ProblemError
	if !errors.As(err403, &prob403) || prob403.Status != 403 || prob403.Type != "/problems/forbidden" {
		t.Errorf("mapeamento de 403 incorreto: %v", err403)
	}

	// 7. Generic error
	genErr := errors.New("network timeout")
	errGen := mapError(genErr, ws)
	var probGen *domain.ProblemError
	if !errors.As(errGen, &probGen) || probGen.Status != http.StatusInternalServerError {
		t.Errorf("mapeamento de erro genérico incorreto: %v", errGen)
	}
}

func TestClientFactory(t *testing.T) {
	creds := map[string]WorkspaceCredentials{
		"default": {
			Host:         "https://dbc-default.cloud.databricks.com",
			ClientID:     "default-id",
			ClientSecret: "default-secret",
		},
		"prod-br": {
			Host:         "https://dbc-prod.cloud.databricks.com",
			ClientID:     "prod-id",
			ClientSecret: "prod-secret",
		},
	}

	factory := NewClientFactory(creds)

	// Workspace inexistente sem fallback se não houvesse default
	factoryNoDefault := NewClientFactory(map[string]WorkspaceCredentials{
		"prod-br": {
			Host:         "https://dbc-prod.cloud.databricks.com",
			ClientID:     "prod-id",
			ClientSecret: "prod-secret",
		},
	})
	_, err := factoryNoDefault.GetClient(context.Background(), "unknown-ws")
	if err == nil {
		t.Errorf("esperava erro ao buscar workspace inexistente")
	}

	// Fallback para default quando o workspace não tem configuração específica
	client, err := factory.GetClient(context.Background(), "unknown-ws")
	if err != nil {
		t.Fatalf("esperava sucesso no fallback para default: %v", err)
	}
	if client == nil {
		t.Fatalf("esperava client não nulo")
	}

	// Workspace específico com hífen e maiúsculas
	prodClient, err := factory.GetClient(context.Background(), "PROD-BR")
	if err != nil {
		t.Fatalf("esperava sucesso buscando PROD-BR: %v", err)
	}
	if prodClient == nil {
		t.Fatalf("esperava prodClient não nulo")
	}

	// Segunda chamada deve vir do cache
	cachedClient, err := factory.GetClient(context.Background(), "prod-br")
	if err != nil || cachedClient != prodClient {
		t.Errorf("esperava obter o mesmo client do cache")
	}
}

func TestModelConversions(t *testing.T) {
	nowMilli := time.Now().UnixMilli()

	// 1. Catalog
	catInfo := catalog.CatalogInfo{
		Name:        "analytics",
		Owner:       "admin@company.com",
		CatalogType: catalog.CatalogTypeManagedCatalog,
		Comment:     "Catálogo analítico",
		StorageRoot: "s3://my-bucket/analytics",
		CreatedAt:   nowMilli,
		UpdatedAt:   nowMilli,
		Properties:  map[string]string{"env": "prod"},
	}
	cat := catalogFromSDK(catInfo)
	if cat.Name != "analytics" || cat.Owner != "admin@company.com" || *cat.Comment != "Catálogo analítico" || *cat.StorageRoot != "s3://my-bucket/analytics" {
		t.Errorf("conversão de CatalogInfo falhou: %+v", cat)
	}
	if cat.CreatedAt == nil || cat.UpdatedAt == nil {
		t.Errorf("esperava timestamps de catalog não nulos")
	}

	// 2. Schema
	schemaInfo := catalog.SchemaInfo{
		Name:        "gold",
		CatalogName: "analytics",
		FullName:    "analytics.gold",
		Owner:       "admin@company.com",
		Comment:     "Camada gold",
		StorageRoot: "s3://my-bucket/analytics/gold",
		CreatedAt:   nowMilli,
		UpdatedAt:   nowMilli,
	}
	sch := schemaFromSDK(schemaInfo)
	if sch.Name != "gold" || sch.CatalogName != "analytics" || sch.FullName != "analytics.gold" {
		t.Errorf("conversão de SchemaInfo falhou: %+v", sch)
	}

	// 3. Table vs View
	tableInfo := catalog.TableInfo{
		Name:             "sales",
		CatalogName:      "analytics",
		SchemaName:       "gold",
		FullName:         "analytics.gold.sales",
		TableType:        catalog.TableTypeManaged,
		DataSourceFormat: catalog.DataSourceFormatDelta,
		Owner:            "admin@company.com",
		Columns: []catalog.ColumnInfo{
			{Name: "id", TypeText: "BIGINT", Position: 0, Nullable: false},
		},
		CreatedAt: nowMilli,
	}
	if isView(tableInfo) {
		t.Errorf("tabela MANAGED não deveria ser considerada view")
	}
	tbl := tableFromSDK(tableInfo)
	if tbl.Name != "sales" || len(tbl.Columns) != 1 || tbl.Columns[0].Name != "id" {
		t.Errorf("conversão de TableInfo falhou: %+v", tbl)
	}

	viewInfo := catalog.TableInfo{
		Name:           "v_sales",
		CatalogName:    "analytics",
		SchemaName:     "gold",
		FullName:       "analytics.gold.v_sales",
		TableType:      catalog.TableTypeView,
		ViewDefinition: "SELECT * FROM sales",
		Owner:          "admin@company.com",
		CreatedAt:      nowMilli,
	}
	if !isView(viewInfo) {
		t.Errorf("tabela com TableTypeView deveria ser considerada view")
	}
	v := viewFromSDK(viewInfo)
	if v.Name != "v_sales" || v.ViewDefinition == nil || *v.ViewDefinition != "SELECT * FROM sales" {
		t.Errorf("conversão de ViewInfo falhou: %+v", v)
	}

	// 4. Volume
	volInfo := catalog.VolumeInfo{
		Name:            "raw_files",
		CatalogName:     "analytics",
		SchemaName:      "gold",
		FullName:        "analytics.gold.raw_files",
		VolumeType:      catalog.VolumeTypeManaged,
		StorageLocation: "s3://bucket/path",
		Owner:           "admin@company.com",
		CreatedAt:       nowMilli,
	}
	vol := volumeFromSDK(volInfo)
	if vol.Name != "raw_files" || vol.VolumeType != "MANAGED" || *vol.StorageLocation != "s3://bucket/path" {
		t.Errorf("conversão de VolumeInfo falhou: %+v", vol)
	}
}

func TestHelpers(t *testing.T) {
	if entityTypeForSecurable(domain.SecurableCatalog) != "catalogs" {
		t.Errorf("esperado 'catalogs'")
	}
	if entityTypeForSecurable(domain.SecurableSchema) != "schemas" {
		t.Errorf("esperado 'schemas'")
	}
	if entityTypeForSecurable(domain.SecurableTable) != "tables" {
		t.Errorf("esperado 'tables'")
	}
	if entityTypeForSecurable(domain.SecurableView) != "tables" {
		t.Errorf("esperado 'tables' para VIEW")
	}
	if entityTypeForSecurable(domain.SecurableVolume) != "volumes" {
		t.Errorf("esperado 'volumes'")
	}

	if securableTypeForSecurable(domain.SecurableCatalog) != catalog.SecurableTypeCatalog {
		t.Errorf("esperado catalog.SecurableTypeCatalog")
	}
	if securableTypeForSecurable(domain.SecurableView) != catalog.SecurableTypeTable {
		t.Errorf("esperado catalog.SecurableTypeTable para VIEW")
	}
}
