from datetime import datetime, timezone
from unittest.mock import MagicMock, patch
import pytest

from databricks.sdk.errors import (
    BadRequest,
    DatabricksError,
    NotFound,
    PermissionDenied,
    ResourceAlreadyExists,
    TooManyRequests,
)
from databricks.sdk.service import catalog, iam

from app.adapters.databricks import (
    DatabricksSDKAdapter,
    WorkspaceClientFactory,
    WorkspaceCredentials,
    map_databricks_error,
)
from app.domain.errors import (
    NotFoundException,
    PrincipalNotFoundException,
    ProblemException,
)
from app.domain.models import (
    Catalog,
    DataSchema,
    PrivilegeChange,
    SecurableRef,
    SecurableType,
    Table,
    View,
    Volume,
)
from app.domain.principal import PrincipalType
from app.domain.privilege import Privilege


class TestWorkspaceClientFactory:
    def test_factory_from_env_default_credentials(self, monkeypatch):
        monkeypatch.setenv("DATABRICKS_HOST", "https://dbc-main.cloud.databricks.com")
        monkeypatch.setenv("DATABRICKS_CLIENT_ID", "sp-main-client-id")
        monkeypatch.setenv("DATABRICKS_CLIENT_SECRET", "sp-main-secret")

        factory = WorkspaceClientFactory.from_env()
        assert factory.is_configured()

        creds = factory.get_credentials("any-workspace")
        assert creds is not None
        assert creds.host == "https://dbc-main.cloud.databricks.com"
        assert creds.client_id == "sp-main-client-id"
        assert creds.client_secret == "sp-main-secret"

    def test_factory_multi_workspace_credentials(self, monkeypatch):
        monkeypatch.setenv("DATABRICKS_WS_PROD_BR_HOST", "https://dbc-prod.cloud.databricks.com")
        monkeypatch.setenv("DATABRICKS_WS_PROD_BR_CLIENT_ID", "sp-prod-id")
        monkeypatch.setenv("DATABRICKS_WS_PROD_BR_CLIENT_SECRET", "sp-prod-secret")

        factory = WorkspaceClientFactory.from_env()
        assert factory.is_configured()

        # Testa alias em kebab-case
        creds_kebab = factory.get_credentials("prod-br")
        assert creds_kebab is not None
        assert creds_kebab.host == "https://dbc-prod.cloud.databricks.com"
        assert creds_kebab.client_id == "sp-prod-id"

        # Testa alias em snake_case
        creds_snake = factory.get_credentials("prod_br")
        assert creds_snake is not None
        assert creds_snake.client_id == "sp-prod-id"

    def test_factory_missing_credentials_raises_not_found(self, monkeypatch):
        monkeypatch.delenv("DATABRICKS_HOST", raising=False)
        monkeypatch.delenv("DATABRICKS_CLIENT_ID", raising=False)
        monkeypatch.delenv("DATABRICKS_CLIENT_SECRET", raising=False)

        factory = WorkspaceClientFactory(credentials={}, default_credentials=None)
        assert not factory.is_configured()

        with pytest.raises(NotFoundException) as exc_info:
            factory.get_client("unknown-ws")
        assert "Workspace credentials" in str(exc_info.value)

    def test_factory_client_caching(self):
        builder_mock = MagicMock(return_value="mock_client_instance")
        creds = WorkspaceCredentials(host="h", client_id="c", client_secret="s")
        factory = WorkspaceClientFactory(
            default_credentials=creds, client_builder=builder_mock
        )

        c1 = factory.get_client("ws1")
        c2 = factory.get_client("ws1")

        assert c1 == "mock_client_instance"
        assert c2 == "mock_client_instance"
        assert builder_mock.call_count == 1


class TestErrorMapping:
    def test_map_not_found(self):
        err = NotFound("Catalog 'test' not found", error_code="NOT_FOUND")
        prob = map_databricks_error(err, workspace="prod-br")
        assert prob.status == 404
        assert prob.type == "/problems/not-found"
        assert prob.title == "Recurso não encontrado"
        assert prob.workspace == "prod-br"
        assert prob.upstream is not None
        assert prob.upstream.error_code == "NOT_FOUND"
        assert prob.upstream.http_status == 404

    def test_map_already_exists(self):
        err = ResourceAlreadyExists("Catalog exists", error_code="RESOURCE_ALREADY_EXISTS")
        prob = map_databricks_error(err, workspace="prod-br")
        assert prob.status == 409
        assert prob.type == "/problems/conflict"
        assert prob.upstream.error_code == "RESOURCE_ALREADY_EXISTS"

    def test_map_permission_denied(self):
        err = PermissionDenied("User lacks privilege", error_code="PERMISSION_DENIED")
        prob = map_databricks_error(err, workspace="prod-br")
        assert prob.status == 403
        assert prob.type == "/problems/forbidden"

    def test_map_too_many_requests(self):
        err = TooManyRequests("Rate limited", error_code="RATE_LIMIT_EXCEEDED")
        err.retry_after_secs = 30
        prob = map_databricks_error(err, workspace="prod-br")
        assert prob.status == 429
        assert prob.retry_after_seconds == 30
        assert prob.type == "/problems/rate-limit-exceeded"

    def test_map_generic_databricks_error(self):
        err = DatabricksError("Internal backend failure", error_code="DATABRICKS_FAULT")
        prob = map_databricks_error(err, workspace="prod-br")
        assert prob.status == 502
        assert prob.type == "/problems/upstream-error"
        assert prob.upstream.error_code == "DATABRICKS_FAULT"


@pytest.fixture
def mock_ws_client():
    client = MagicMock()
    return client


@pytest.fixture
def adapter(mock_ws_client):
    factory = WorkspaceClientFactory(
        default_credentials=WorkspaceCredentials("https://test", "id", "secret"),
        client_builder=lambda creds: mock_ws_client,
    )
    return DatabricksSDKAdapter(factory=factory)


class TestSDKAdapterCatalogs:
    def test_list_catalogs(self, adapter, mock_ws_client):
        c1 = catalog.CatalogInfo(
            name="cat1",
            comment="comment 1",
            owner="admin",
            created_at=1700000000000,
            updated_at=1700000000000,
        )
        mock_ws_client.catalogs.list.return_value = [c1]

        cats, next_token = adapter.list_catalogs("prod-br")
        assert len(cats) == 1
        assert cats[0].name == "cat1"
        assert cats[0].owner == "admin"
        assert cats[0].comment == "comment 1"
        assert cats[0].created_at is not None

    def test_get_catalog(self, adapter, mock_ws_client):
        mock_ws_client.catalogs.get.return_value = catalog.CatalogInfo(
            name="cat_a", owner="sp-gov", comment="Sample"
        )
        cat = adapter.get_catalog("prod-br", "cat_a")
        assert cat.name == "cat_a"
        assert cat.owner == "sp-gov"

    def test_create_catalog(self, adapter, mock_ws_client):
        mock_ws_client.catalogs.create.return_value = catalog.CatalogInfo(
            name="new_cat", comment="New"
        )
        created = adapter.create_catalog(
            "prod-br", Catalog(name="new_cat", comment="New")
        )
        assert created.name == "new_cat"
        mock_ws_client.catalogs.create.assert_called_once_with(
            name="new_cat", comment="New", properties=None, storage_root=None
        )

    def test_update_catalog(self, adapter, mock_ws_client):
        mock_ws_client.catalogs.update.return_value = catalog.CatalogInfo(
            name="new_name", comment="Updated"
        )
        updated = adapter.update_catalog("prod-br", "old_name", new_name="new_name", comment="Updated")
        assert updated.name == "new_name"

    def test_delete_catalog(self, adapter, mock_ws_client):
        adapter.delete_catalog("prod-br", "cat_to_delete", force=True)
        mock_ws_client.catalogs.delete.assert_called_once_with(name="cat_to_delete", force=True)


class TestSDKAdapterSchemas:
    def test_list_schemas(self, adapter, mock_ws_client):
        s1 = catalog.SchemaInfo(
            name="sch1", catalog_name="cat1", full_name="cat1.sch1", owner="admin"
        )
        mock_ws_client.schemas.list.return_value = [s1]

        schemas, _ = adapter.list_schemas("prod-br", "cat1")
        assert len(schemas) == 1
        assert schemas[0].name == "sch1"
        assert schemas[0].full_name == "cat1.sch1"

    def test_get_schema(self, adapter, mock_ws_client):
        mock_ws_client.schemas.get.return_value = catalog.SchemaInfo(
            name="sch_a", catalog_name="cat1", full_name="cat1.sch_a", owner="admin"
        )
        s = adapter.get_schema("prod-br", "cat1", "sch_a")
        assert s.name == "sch_a"
        mock_ws_client.schemas.get.assert_called_once_with(full_name="cat1.sch_a")

    def test_create_schema(self, adapter, mock_ws_client):
        mock_ws_client.schemas.create.return_value = catalog.SchemaInfo(
            name="sch_new", catalog_name="cat1", full_name="cat1.sch_new"
        )
        s = adapter.create_schema("prod-br", "cat1", DataSchema(name="sch_new"))
        assert s.name == "sch_new"

    def test_delete_schema(self, adapter, mock_ws_client):
        adapter.delete_schema("prod-br", "cat1", "sch_del", force=False)
        mock_ws_client.schemas.delete.assert_called_once_with(full_name="cat1.sch_del", force=False)


class TestSDKAdapterTablesAndViews:
    def test_list_tables_excludes_views(self, adapter, mock_ws_client):
        t1 = catalog.TableInfo(
            name="tbl1",
            catalog_name="cat1",
            schema_name="sch1",
            full_name="cat1.sch1.tbl1",
            table_type=catalog.TableType.MANAGED,
        )
        v1 = catalog.TableInfo(
            name="v1",
            catalog_name="cat1",
            schema_name="sch1",
            full_name="cat1.sch1.v1",
            table_type=catalog.TableType.VIEW,
        )
        mock_ws_client.tables.list.return_value = [t1, v1]

        tables, _ = adapter.list_tables("prod-br", "cat1", "sch1")
        assert len(tables) == 1
        assert tables[0].name == "tbl1"

    def test_list_views_excludes_tables(self, adapter, mock_ws_client):
        t1 = catalog.TableInfo(
            name="tbl1",
            catalog_name="cat1",
            schema_name="sch1",
            full_name="cat1.sch1.tbl1",
            table_type=catalog.TableType.MANAGED,
        )
        v1 = catalog.TableInfo(
            name="v1",
            catalog_name="cat1",
            schema_name="sch1",
            full_name="cat1.sch1.v1",
            table_type=catalog.TableType.VIEW,
            view_definition="SELECT 1",
        )
        mock_ws_client.tables.list.return_value = [t1, v1]

        views, _ = adapter.list_views("prod-br", "cat1", "sch1")
        assert len(views) == 1
        assert views[0].name == "v1"
        assert views[0].view_definition == "SELECT 1"

    def test_get_table_when_view_raises_not_found(self, adapter, mock_ws_client):
        mock_ws_client.tables.get.return_value = catalog.TableInfo(
            name="v1",
            catalog_name="c",
            schema_name="s",
            full_name="c.s.v1",
            table_type=catalog.TableType.VIEW,
        )
        with pytest.raises(NotFoundException):
            adapter.get_table("prod-br", "c", "s", "v1")


class TestSDKAdapterVolumes:
    def test_volumes_crud(self, adapter, mock_ws_client):
        v_info = catalog.VolumeInfo(
            name="raw_vol",
            catalog_name="c",
            schema_name="s",
            full_name="c.s.raw_vol",
            volume_type=catalog.VolumeType.MANAGED,
            owner="admin",
        )
        mock_ws_client.volumes.list.return_value = [v_info]
        mock_ws_client.volumes.read.return_value = v_info
        mock_ws_client.volumes.create.return_value = v_info
        mock_ws_client.volumes.update.return_value = v_info

        vols, _ = adapter.list_volumes("prod-br", "c", "s")
        assert len(vols) == 1
        assert vols[0].name == "raw_vol"

        single = adapter.get_volume("prod-br", "c", "s", "raw_vol")
        assert single.name == "raw_vol"

        created = adapter.create_volume(
            "prod-br", "c", "s", Volume(name="raw_vol", catalog_name="c", schema_name="s", full_name="c.s.raw_vol")
        )
        assert created.name == "raw_vol"

        adapter.delete_volume("prod-br", "c", "s", "raw_vol")
        mock_ws_client.volumes.delete.assert_called_once_with(name="c.s.raw_vol")


class TestSDKAdapterGrants:
    def test_get_grants(self, adapter, mock_ws_client):
        pa = catalog.PrivilegeAssignment(
            principal="analyst@company.com",
            privileges=[catalog.Privilege.SELECT, catalog.Privilege.MODIFY],
        )
        mock_ws_client.grants.get.return_value = catalog.GetPermissionsResponse(
            privilege_assignments=[pa]
        )

        sec = SecurableRef(type=SecurableType.TABLE, name="main.default.users")
        grants = adapter.get_grants("prod-br", sec)
        assert len(grants) == 1
        assert grants[0].principal == "analyst@company.com"
        assert Privilege.SELECT in grants[0].privileges
        assert Privilege.MODIFY in grants[0].privileges

    def test_get_effective_grants(self, adapter, mock_ws_client):
        ep = catalog.EffectivePrivilege(
            privilege=catalog.Privilege.USE_CATALOG,
            inherited_from_name="main",
            inherited_from_type=catalog.SecurableType.CATALOG,
        )
        epa = catalog.EffectivePrivilegeAssignment(
            principal="analyst@company.com",
            privileges=[ep],
        )
        mock_ws_client.grants.get_effective.return_value = catalog.EffectivePermissionsList(
            privilege_assignments=[epa]
        )

        sec = SecurableRef(type=SecurableType.TABLE, name="main.default.users")
        eff = adapter.get_effective_grants("prod-br", sec)
        assert len(eff) == 1
        assert eff[0].principal == "analyst@company.com"
        assert eff[0].privileges[0].inherited is True
        assert eff[0].privileges[0].inherited_from.name == "main"

    def test_update_grants(self, adapter, mock_ws_client):
        pa = catalog.PrivilegeAssignment(
            principal="analyst@company.com",
            privileges=[catalog.Privilege.SELECT],
        )
        mock_ws_client.grants.update.return_value = catalog.UpdatePermissionsResponse(
            privilege_assignments=[pa]
        )

        sec = SecurableRef(type=SecurableType.TABLE, name="main.default.users")
        changes = [
            PrivilegeChange(
                principal="analyst@company.com",
                privileges=[Privilege.SELECT],
            )
        ]
        outcomes, resulting_grants, changed = adapter.update_grants(
            "prod-br", sec, changes, is_revoke=False
        )
        assert changed is True
        assert len(outcomes) == 1
        assert outcomes[0].applied == [Privilege.SELECT]
        assert len(resulting_grants) == 1


class TestSDKAdapterOwnershipAndTags:
    def test_ownership_catalog(self, adapter, mock_ws_client):
        mock_ws_client.catalogs.get.return_value = catalog.CatalogInfo(
            name="cat1", owner="old-owner"
        )
        sec = SecurableRef(type=SecurableType.CATALOG, name="cat1")

        owner = adapter.get_owner("prod-br", sec)
        assert owner == "old-owner"

        adapter.set_owner("prod-br", sec, "new-owner")
        mock_ws_client.catalogs.update.assert_called_once_with(
            name="cat1", owner="new-owner"
        )

    def test_tags_crud(self, adapter, mock_ws_client):
        t1 = catalog.EntityTagAssignment(
            entity_name="cat1",
            entity_type="catalogs",
            tag_key="environment",
            tag_value="production",
        )
        mock_ws_client.entity_tag_assignments.list.return_value = [t1]

        sec = SecurableRef(type=SecurableType.CATALOG, name="cat1")
        tags = adapter.list_tags("prod-br", sec)
        assert tags.get("environment") == "production"

        adapter.set_tag("prod-br", sec, "classification", "confidential")
        mock_ws_client.entity_tag_assignments.create.assert_called_once()

        adapter.delete_tag("prod-br", sec, "environment")
        mock_ws_client.entity_tag_assignments.delete.assert_called_once_with(
            entity_type="catalogs", entity_name="cat1", tag_key="environment"
        )


class TestSDKAdapterIdentity:
    def test_list_principals(self, adapter, mock_ws_client):
        u1 = iam.User(user_name="user@test.com", active=True)
        g1 = iam.Group(display_name="grp-devs")
        sp1 = iam.ServicePrincipal(application_id="sp-uuid-1", active=True)

        mock_ws_client.users.list.return_value = [u1]
        mock_ws_client.groups.list.return_value = [g1]
        mock_ws_client.service_principals.list.return_value = [sp1]

        principals, _ = adapter.list_principals("prod-br")
        names = [p.name for p in principals]
        assert "user@test.com" in names
        assert "grp-devs" in names
        assert "sp-uuid-1" in names

    def test_get_principal_found(self, adapter, mock_ws_client):
        u1 = iam.User(user_name="target@test.com", active=True)
        mock_ws_client.users.list.return_value = [u1]

        p = adapter.get_principal("prod-br", "target@test.com")
        assert p.name == "target@test.com"
        assert p.type == PrincipalType.USER

    def test_get_principal_not_found(self, adapter, mock_ws_client):
        mock_ws_client.users.list.return_value = []
        mock_ws_client.service_principals.list.return_value = []
        mock_ws_client.groups.list.return_value = []

        with pytest.raises(PrincipalNotFoundException):
            adapter.get_principal("prod-br", "missing-entity")


class TestMainAppInitialization:
    def test_init_app_state_fallback_to_memory(self, monkeypatch):
        from fastapi import FastAPI
        from app.adapters.memory.databricks import MemoryDatabricksClient
        from app.main import init_app_state

        monkeypatch.delenv("DATABRICKS_HOST", raising=False)
        monkeypatch.delenv("DATABRICKS_CLIENT_ID", raising=False)
        monkeypatch.delenv("DATABRICKS_CLIENT_SECRET", raising=False)

        app = FastAPI()
        init_app_state(app)
        assert isinstance(app.state.databricks_client, MemoryDatabricksClient)

    def test_init_app_state_with_credentials(self, monkeypatch):
        from fastapi import FastAPI
        from app.adapters.databricks import DatabricksSDKAdapter
        from app.main import init_app_state

        monkeypatch.setenv("DATABRICKS_HOST", "https://dbc-real.cloud.databricks.com")
        monkeypatch.setenv("DATABRICKS_CLIENT_ID", "sp-client-id")
        monkeypatch.setenv("DATABRICKS_CLIENT_SECRET", "sp-secret")

        app = FastAPI()
        init_app_state(app)
        assert isinstance(app.state.databricks_client, DatabricksSDKAdapter)
