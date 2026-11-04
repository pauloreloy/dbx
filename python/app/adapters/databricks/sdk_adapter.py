from contextlib import contextmanager
from datetime import datetime, timezone
import logging
from typing import Any, Iterator, Optional

from databricks.sdk.errors import (
    AlreadyExists,
    BadRequest,
    DatabricksError,
    InvalidParameterValue,
    NotFound,
    PermissionDenied,
    RequestLimitExceeded,
    ResourceAlreadyExists,
    ResourceConflict,
    ResourceDoesNotExist,
    STATUS_CODE_MAPPING,
    TooManyRequests,
    Unauthenticated,
)
from databricks.sdk.service import catalog

from app.adapters.databricks.client_factory import WorkspaceClientFactory
from app.domain.errors import (
    NotFoundException,
    PrincipalNotFoundException,
    ProblemException,
    UpstreamError,
)
from app.domain.models import (
    Catalog,
    ChangeOutcome,
    Column,
    DataSchema,
    EffectiveGrant,
    EffectivePrivilege,
    PrincipalPrivileges,
    PrivilegeChange,
    SecurableRef,
    SecurableType,
    Table,
    View,
    Volume,
)
from app.domain.principal import Principal, PrincipalType
from app.domain.privilege import Privilege
from app.ports.databricks import (
    CatalogsAPI,
    GrantsAPI,
    IdentityAPI,
    OwnershipAPI,
    SchemasAPI,
    TablesAPI,
    TagsAPI,
    ViewsAPI,
    VolumesAPI,
)

logger = logging.getLogger("governance_service.databricks_sdk")

_REVERSE_STATUS_CODE = {cls: code for code, cls in STATUS_CODE_MAPPING.items()}


def map_databricks_error(e: Exception, workspace: Optional[str] = None) -> ProblemException:
    """Mapeia exceções da Databricks (DatabricksError, NotFound, etc.) para RFC 9457 ProblemException."""
    if isinstance(e, ProblemException):
        if workspace and not e.workspace:
            e.workspace = workspace
        return e

    msg = str(e)
    error_code = getattr(e, "error_code", None) or e.__class__.__name__
    retry_after = getattr(e, "retry_after_secs", None)

    if isinstance(e, (NotFound, ResourceDoesNotExist)):
        status = 404
        prob_type = "/problems/not-found"
        title = "Recurso não encontrado"
    elif isinstance(e, (ResourceAlreadyExists, ResourceConflict, AlreadyExists)):
        status = 409
        prob_type = "/problems/conflict"
        title = "Conflito de recurso"
    elif isinstance(e, PermissionDenied):
        status = 403
        prob_type = "/problems/forbidden"
        title = "Acesso negado no Databricks"
    elif isinstance(e, (BadRequest, InvalidParameterValue)):
        status = 400
        prob_type = "/problems/bad-request"
        title = "Requisição inválida"
    elif isinstance(e, (TooManyRequests, RequestLimitExceeded)):
        status = 429
        prob_type = "/problems/rate-limit-exceeded"
        title = "Limite de requisições excedido"
    elif isinstance(e, Unauthenticated):
        status = 401
        prob_type = "/problems/unauthenticated"
        title = "Não autenticado no Databricks"
    else:
        # Tenta resolver o status code via hierarquia de classes do SDK
        resolved_status = None
        for cls in e.__class__.mro():
            if cls in _REVERSE_STATUS_CODE:
                resolved_status = _REVERSE_STATUS_CODE[cls]
                break
        status = resolved_status or 502
        prob_type = "/problems/upstream-error"
        title = "Erro no serviço Databricks"

    upstream = UpstreamError(
        error_code=str(error_code) if error_code else None,
        message=msg,
        http_status=status,
    )

    return ProblemException(
        status=status,
        type_=prob_type,
        title=title,
        detail=msg,
        workspace=workspace,
        retry_after_seconds=retry_after,
        upstream=upstream,
    )


def _to_datetime(ts: Any) -> Optional[datetime]:
    if ts is None:
        return None
    if isinstance(ts, int):
        return datetime.fromtimestamp(ts / 1000.0, tz=timezone.utc)
    if isinstance(ts, datetime):
        return ts
    return None


def _to_domain_privilege(p: Any) -> Optional[Privilege]:
    val = p.value if hasattr(p, "value") else str(p)
    try:
        return Privilege(val)
    except ValueError:
        return None


def _sec_type_to_entity_type(sec_type: SecurableType) -> str:
    mapping = {
        SecurableType.CATALOG: "catalogs",
        SecurableType.SCHEMA: "schemas",
        SecurableType.TABLE: "tables",
        SecurableType.VIEW: "tables",
        SecurableType.VOLUME: "volumes",
    }
    return mapping.get(sec_type, f"{sec_type.value.lower()}s")


def _map_catalog(c: Any) -> Catalog:
    cat_type = (
        c.catalog_type.value
        if hasattr(c.catalog_type, "value")
        else str(c.catalog_type)
        if c.catalog_type
        else None
    )
    return Catalog(
        name=c.name or "",
        comment=c.comment,
        owner=c.owner,
        catalog_type=cat_type,
        storage_root=c.storage_root,
        properties=c.properties,
        created_at=_to_datetime(c.created_at),
        updated_at=_to_datetime(c.updated_at),
    )


def _map_schema(s: Any) -> DataSchema:
    return DataSchema(
        name=s.name or "",
        catalog_name=s.catalog_name,
        full_name=s.full_name,
        comment=s.comment,
        owner=s.owner,
        storage_root=s.storage_root,
        properties=s.properties,
        created_at=_to_datetime(s.created_at),
        updated_at=_to_datetime(s.updated_at),
    )


def _map_columns(cols: Optional[list[Any]]) -> Optional[list[Column]]:
    if not cols:
        return None
    return [
        Column(
            name=col.name or "",
            type_text=col.type_text or "",
            position=col.position if col.position is not None else idx,
            comment=col.comment,
            nullable=col.nullable if col.nullable is not None else True,
        )
        for idx, col in enumerate(cols)
    ]


def _map_table(t: Any) -> Table:
    ttype = (
        t.table_type.value
        if hasattr(t.table_type, "value")
        else str(t.table_type)
        if t.table_type
        else None
    )
    fmt = (
        t.data_source_format.value
        if hasattr(t.data_source_format, "value")
        else str(t.data_source_format)
        if t.data_source_format
        else None
    )
    return Table(
        name=t.name or "",
        catalog_name=t.catalog_name or "",
        schema_name=t.schema_name or "",
        full_name=t.full_name or f"{t.catalog_name}.{t.schema_name}.{t.name}",
        table_type=ttype,
        data_source_format=fmt,
        storage_location=t.storage_location,
        comment=t.comment,
        owner=t.owner,
        columns=_map_columns(t.columns),
        created_at=_to_datetime(t.created_at),
        updated_at=_to_datetime(t.updated_at),
    )


def _map_view(t: Any) -> View:
    return View(
        name=t.name or "",
        catalog_name=t.catalog_name or "",
        schema_name=t.schema_name or "",
        full_name=t.full_name or f"{t.catalog_name}.{t.schema_name}.{t.name}",
        view_definition=getattr(t, "view_definition", None),
        comment=t.comment,
        owner=t.owner,
        columns=_map_columns(t.columns),
        created_at=_to_datetime(t.created_at),
        updated_at=_to_datetime(t.updated_at),
    )


def _map_volume(v: Any) -> Volume:
    vtype = (
        v.volume_type.value
        if hasattr(v.volume_type, "value")
        else str(v.volume_type)
        if v.volume_type
        else "MANAGED"
    )
    return Volume(
        name=v.name or "",
        catalog_name=v.catalog_name or "",
        schema_name=v.schema_name or "",
        full_name=v.full_name or f"{v.catalog_name}.{v.schema_name}.{v.name}",
        volume_type=vtype,
        storage_location=v.storage_location,
        comment=v.comment,
        owner=v.owner,
        created_at=_to_datetime(v.created_at),
        updated_at=_to_datetime(v.updated_at),
    )


class DatabricksSDKAdapter(
    GrantsAPI,
    CatalogsAPI,
    SchemasAPI,
    TablesAPI,
    ViewsAPI,
    VolumesAPI,
    OwnershipAPI,
    TagsAPI,
    IdentityAPI,
):
    """Adaptador de portas que implementa a integração com a API real do Databricks via SDK."""

    def __init__(self, factory: Optional[WorkspaceClientFactory] = None) -> None:
        self._factory = factory or WorkspaceClientFactory.from_env()

    @contextmanager
    def _handle_errors(self, ws: str) -> Iterator[None]:
        try:
            yield
        except Exception as ex:
            raise map_databricks_error(ex, workspace=ws) from ex

    # --- GrantsAPI ---

    def get_grants(
        self, ws: str, sec: SecurableRef, principal_filter: Optional[str] = None
    ) -> list[PrincipalPrivileges]:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            securable_type = sec.type.value.lower()
            resp = w.grants.get(
                securable_type=securable_type,
                full_name=sec.name,
                principal=principal_filter,
            )
            result: list[PrincipalPrivileges] = []
            if resp and resp.privilege_assignments:
                for pa in resp.privilege_assignments:
                    if not pa.principal:
                        continue
                    privs = [
                        p_enum
                        for p in (pa.privileges or [])
                        if (p_enum := _to_domain_privilege(p)) is not None
                    ]
                    result.append(
                        PrincipalPrivileges(principal=pa.principal, privileges=privs)
                    )
            return result

    def get_effective_grants(
        self, ws: str, sec: SecurableRef, principal_filter: Optional[str] = None
    ) -> list[EffectiveGrant]:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            securable_type = sec.type.value.lower()
            resp = w.grants.get_effective(
                securable_type=securable_type,
                full_name=sec.name,
                principal=principal_filter,
            )
            result: list[EffectiveGrant] = []
            if resp and resp.privilege_assignments:
                for epa in resp.privilege_assignments:
                    if not epa.principal:
                        continue
                    eff_privs: list[EffectivePrivilege] = []
                    for ep in epa.privileges or []:
                        p_enum = _to_domain_privilege(ep.privilege)
                        if not p_enum:
                            continue
                        inherited = bool(ep.inherited_from_name or ep.inherited_from_type)
                        inherited_from = None
                        if inherited and ep.inherited_from_name:
                            inherited_type_str = (
                                ep.inherited_from_type.value
                                if hasattr(ep.inherited_from_type, "value")
                                else str(ep.inherited_from_type)
                            )
                            try:
                                inherited_type = SecurableType(inherited_type_str.upper())
                            except ValueError:
                                inherited_type = SecurableType.CATALOG
                            inherited_from = SecurableRef(
                                type=inherited_type, name=ep.inherited_from_name
                            )
                        eff_privs.append(
                            EffectivePrivilege(
                                privilege=p_enum,
                                inherited=inherited,
                                inherited_from=inherited_from,
                            )
                        )
                    result.append(
                        EffectiveGrant(principal=epa.principal, privileges=eff_privs)
                    )
            return result

    def update_grants(
        self,
        ws: str,
        sec: SecurableRef,
        changes: list[PrivilegeChange],
        is_revoke: bool,
    ) -> tuple[list[ChangeOutcome], list[PrincipalPrivileges], bool]:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            securable_type = sec.type.value.lower()
            sdk_changes: list[catalog.PermissionsChange] = []
            outcomes: list[ChangeOutcome] = []

            for ch in changes:
                sdk_privs = [catalog.Privilege(p.value) for p in ch.privileges]
                if not is_revoke:
                    sdk_changes.append(
                        catalog.PermissionsChange(principal=ch.principal, add=sdk_privs)
                    )
                else:
                    sdk_changes.append(
                        catalog.PermissionsChange(principal=ch.principal, remove=sdk_privs)
                    )
                outcomes.append(
                    ChangeOutcome(
                        principal=ch.principal,
                        applied=list(ch.privileges),
                        unchanged=[],
                    )
                )

            resp = w.grants.update(
                securable_type=securable_type,
                full_name=sec.name,
                changes=sdk_changes,
            )

            resulting_grants: list[PrincipalPrivileges] = []
            if resp and resp.privilege_assignments:
                for pa in resp.privilege_assignments:
                    if not pa.principal:
                        continue
                    privs = [
                        p_enum
                        for p in (pa.privileges or [])
                        if (p_enum := _to_domain_privilege(p)) is not None
                    ]
                    resulting_grants.append(
                        PrincipalPrivileges(principal=pa.principal, privileges=privs)
                    )
            else:
                resulting_grants = self.get_grants(ws, sec)

            overall_changed = len(changes) > 0
            return outcomes, resulting_grants, overall_changed

    # --- CatalogsAPI ---

    def list_catalogs(
        self, ws: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[Catalog], str]:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            max_results = page_size if page_size > 0 else None
            p_token = page_token if page_token else None
            catalogs_iter = w.catalogs.list(max_results=max_results, page_token=p_token)
            items: list[Catalog] = []
            for c in catalogs_iter:
                items.append(_map_catalog(c))
                if page_size > 0 and len(items) >= page_size:
                    break
            return items, ""

    def get_catalog(self, ws: str, name: str) -> Catalog:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            info = w.catalogs.get(name=name)
            return _map_catalog(info)

    def create_catalog(self, ws: str, cat: Catalog) -> Catalog:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            info = w.catalogs.create(
                name=cat.name,
                comment=cat.comment,
                properties=cat.properties,
                storage_root=cat.storage_root,
            )
            return _map_catalog(info)

    def update_catalog(
        self,
        ws: str,
        name: str,
        new_name: Optional[str] = None,
        comment: Optional[str] = None,
        props: Optional[dict[str, str]] = None,
    ) -> Catalog:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            info = w.catalogs.update(
                name=name,
                new_name=new_name,
                comment=comment,
                properties=props,
            )
            return _map_catalog(info)

    def delete_catalog(self, ws: str, name: str, force: bool = False) -> None:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            w.catalogs.delete(name=name, force=force)

    # --- SchemasAPI ---

    def list_schemas(
        self, ws: str, catalog_name: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[DataSchema], str]:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            max_results = page_size if page_size > 0 else None
            p_token = page_token if page_token else None
            schemas_iter = w.schemas.list(
                catalog_name=catalog_name, max_results=max_results, page_token=p_token
            )
            items: list[DataSchema] = []
            for s in schemas_iter:
                items.append(_map_schema(s))
                if page_size > 0 and len(items) >= page_size:
                    break
            return items, ""

    def get_schema(self, ws: str, catalog_name: str, schema: str) -> DataSchema:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            full_name = f"{catalog_name}.{schema}"
            info = w.schemas.get(full_name=full_name)
            return _map_schema(info)

    def create_schema(self, ws: str, catalog_name: str, s: DataSchema) -> DataSchema:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            info = w.schemas.create(
                name=s.name,
                catalog_name=catalog_name,
                comment=s.comment,
                properties=s.properties,
                storage_root=s.storage_root,
            )
            return _map_schema(info)

    def update_schema(
        self,
        ws: str,
        catalog_name: str,
        schema: str,
        new_name: Optional[str] = None,
        comment: Optional[str] = None,
        props: Optional[dict[str, str]] = None,
    ) -> DataSchema:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            full_name = f"{catalog_name}.{schema}"
            info = w.schemas.update(
                full_name=full_name,
                new_name=new_name,
                comment=comment,
                properties=props,
            )
            return _map_schema(info)

    def delete_schema(
        self, ws: str, catalog_name: str, schema: str, force: bool = False
    ) -> None:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            full_name = f"{catalog_name}.{schema}"
            w.schemas.delete(full_name=full_name, force=force)

    # --- TablesAPI ---

    def list_tables(
        self,
        ws: str,
        catalog_name: str,
        schema_name: str,
        page_size: int = 0,
        page_token: str = "",
    ) -> tuple[list[Table], str]:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            max_results = page_size if page_size > 0 else None
            p_token = page_token if page_token else None
            tables_iter = w.tables.list(
                catalog_name=catalog_name,
                schema_name=schema_name,
                max_results=max_results,
                page_token=p_token,
            )
            items: list[Table] = []
            for t in tables_iter:
                ttype = (
                    t.table_type.value
                    if hasattr(t.table_type, "value")
                    else str(t.table_type)
                    if t.table_type
                    else ""
                )
                if ttype in ("VIEW", "MATERIALIZED_VIEW", "METRIC_VIEW"):
                    continue
                items.append(_map_table(t))
                if page_size > 0 and len(items) >= page_size:
                    break
            return items, ""

    def get_table(
        self, ws: str, catalog_name: str, schema_name: str, table: str
    ) -> Table:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            full_name = f"{catalog_name}.{schema_name}.{table}"
            info = w.tables.get(full_name=full_name)
            ttype = (
                info.table_type.value
                if hasattr(info.table_type, "value")
                else str(info.table_type)
                if info.table_type
                else ""
            )
            if ttype in ("VIEW", "MATERIALIZED_VIEW", "METRIC_VIEW"):
                raise NotFoundException("Table", full_name)
            return _map_table(info)

    def delete_table(
        self, ws: str, catalog_name: str, schema_name: str, table: str
    ) -> None:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            full_name = f"{catalog_name}.{schema_name}.{table}"
            w.tables.delete(full_name=full_name)

    # --- ViewsAPI ---

    def list_views(
        self,
        ws: str,
        catalog_name: str,
        schema_name: str,
        page_size: int = 0,
        page_token: str = "",
    ) -> tuple[list[View], str]:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            max_results = page_size if page_size > 0 else None
            p_token = page_token if page_token else None
            tables_iter = w.tables.list(
                catalog_name=catalog_name,
                schema_name=schema_name,
                max_results=max_results,
                page_token=p_token,
            )
            items: list[View] = []
            for t in tables_iter:
                ttype = (
                    t.table_type.value
                    if hasattr(t.table_type, "value")
                    else str(t.table_type)
                    if t.table_type
                    else ""
                )
                if ttype not in ("VIEW", "MATERIALIZED_VIEW", "METRIC_VIEW"):
                    continue
                items.append(_map_view(t))
                if page_size > 0 and len(items) >= page_size:
                    break
            return items, ""

    def get_view(self, ws: str, catalog_name: str, schema_name: str, view: str) -> View:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            full_name = f"{catalog_name}.{schema_name}.{view}"
            info = w.tables.get(full_name=full_name)
            ttype = (
                info.table_type.value
                if hasattr(info.table_type, "value")
                else str(info.table_type)
                if info.table_type
                else ""
            )
            if ttype not in ("VIEW", "MATERIALIZED_VIEW", "METRIC_VIEW"):
                raise NotFoundException("View", full_name)
            return _map_view(info)

    def delete_view(
        self, ws: str, catalog_name: str, schema_name: str, view: str
    ) -> None:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            full_name = f"{catalog_name}.{schema_name}.{view}"
            w.tables.delete(full_name=full_name)

    # --- VolumesAPI ---

    def list_volumes(
        self,
        ws: str,
        catalog_name: str,
        schema_name: str,
        page_size: int = 0,
        page_token: str = "",
    ) -> tuple[list[Volume], str]:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            max_results = page_size if page_size > 0 else None
            p_token = page_token if page_token else None
            volumes_iter = w.volumes.list(
                catalog_name=catalog_name,
                schema_name=schema_name,
                max_results=max_results,
                page_token=p_token,
            )
            items: list[Volume] = []
            for v in volumes_iter:
                items.append(_map_volume(v))
                if page_size > 0 and len(items) >= page_size:
                    break
            return items, ""

    def get_volume(
        self, ws: str, catalog_name: str, schema_name: str, volume: str
    ) -> Volume:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            full_name = f"{catalog_name}.{schema_name}.{volume}"
            info = w.volumes.read(name=full_name)
            return _map_volume(info)

    def create_volume(
        self, ws: str, catalog_name: str, schema_name: str, v: Volume
    ) -> Volume:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            vol_type = (
                catalog.VolumeType(v.volume_type)
                if hasattr(catalog, "VolumeType")
                and hasattr(catalog.VolumeType, v.volume_type)
                else catalog.VolumeType.MANAGED
            )
            info = w.volumes.create(
                catalog_name=catalog_name,
                schema_name=schema_name,
                name=v.name,
                volume_type=vol_type,
                comment=v.comment,
                storage_location=v.storage_location,
            )
            return _map_volume(info)

    def update_volume(
        self,
        ws: str,
        catalog_name: str,
        schema_name: str,
        volume: str,
        new_name: Optional[str] = None,
        comment: Optional[str] = None,
    ) -> Volume:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            full_name = f"{catalog_name}.{schema_name}.{volume}"
            info = w.volumes.update(name=full_name, new_name=new_name, comment=comment)
            return _map_volume(info)

    def delete_volume(
        self, ws: str, catalog_name: str, schema_name: str, volume: str
    ) -> None:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            full_name = f"{catalog_name}.{schema_name}.{volume}"
            w.volumes.delete(name=full_name)

    # --- OwnershipAPI ---

    def get_owner(self, ws: str, sec: SecurableRef) -> str:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            if sec.type == SecurableType.CATALOG:
                info = w.catalogs.get(name=sec.name)
            elif sec.type == SecurableType.SCHEMA:
                info = w.schemas.get(full_name=sec.name)
            elif sec.type in (SecurableType.TABLE, SecurableType.VIEW):
                info = w.tables.get(full_name=sec.name)
            elif sec.type == SecurableType.VOLUME:
                info = w.volumes.read(name=sec.name)
            else:
                raise NotFoundException(str(sec.type), sec.name)

            owner = getattr(info, "owner", None)
            if not owner:
                raise NotFoundException(str(sec.type), sec.name)
            return owner

    def set_owner(self, ws: str, sec: SecurableRef, new_owner: str) -> None:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            if sec.type == SecurableType.CATALOG:
                w.catalogs.update(name=sec.name, owner=new_owner)
            elif sec.type == SecurableType.SCHEMA:
                w.schemas.update(full_name=sec.name, owner=new_owner)
            elif sec.type in (SecurableType.TABLE, SecurableType.VIEW):
                w.tables.update(full_name=sec.name, owner=new_owner)
            elif sec.type == SecurableType.VOLUME:
                w.volumes.update(name=sec.name, owner=new_owner)
            else:
                raise NotFoundException(str(sec.type), sec.name)

    # --- TagsAPI ---

    def list_tags(self, ws: str, sec: SecurableRef) -> dict[str, str]:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            entity_type = _sec_type_to_entity_type(sec.type)
            tags: dict[str, str] = {}
            for tag in w.entity_tag_assignments.list(
                entity_type=entity_type, entity_name=sec.name
            ):
                if tag.tag_key:
                    tags[tag.tag_key] = tag.tag_value or ""
            return tags

    def set_tag(self, ws: str, sec: SecurableRef, key: str, value: str) -> None:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            entity_type = _sec_type_to_entity_type(sec.type)
            assignment = catalog.EntityTagAssignment(
                entity_name=sec.name,
                entity_type=entity_type,
                tag_key=key,
                tag_value=value,
            )
            w.entity_tag_assignments.create(assignment)

    def delete_tag(self, ws: str, sec: SecurableRef, key: str) -> None:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            entity_type = _sec_type_to_entity_type(sec.type)
            w.entity_tag_assignments.delete(
                entity_type=entity_type, entity_name=sec.name, tag_key=key
            )

    # --- IdentityAPI ---

    def list_principals(
        self,
        ws: str,
        p_type: Optional[PrincipalType] = None,
        query: Optional[str] = None,
        page_size: int = 0,
        page_token: str = "",
    ) -> tuple[list[Principal], str]:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)
            results: list[Principal] = []

            # 1. Usuários
            if p_type is None or p_type == PrincipalType.USER:
                try:
                    for u in w.users.list():
                        name = u.user_name or u.display_name or ""
                        if name:
                            active = u.active if u.active is not None else True
                            results.append(
                                Principal(
                                    name=name, type=PrincipalType.USER, active=active
                                )
                            )
                except Exception as ex:
                    logger.warning(
                        "Falha ao listar usuários no workspace %s: %s", ws, ex
                    )

            # 2. Grupos
            if p_type is None or p_type == PrincipalType.GROUP:
                try:
                    for g in w.groups.list():
                        name = g.display_name or ""
                        if name:
                            results.append(
                                Principal(
                                    name=name, type=PrincipalType.GROUP, active=True
                                )
                            )
                except Exception as ex:
                    logger.warning("Falha ao listar grupos no workspace %s: %s", ws, ex)

            # 3. Service Principals
            if p_type is None or p_type == PrincipalType.SERVICE_PRINCIPAL:
                try:
                    for sp in w.service_principals.list():
                        name = sp.application_id or sp.display_name or ""
                        if name:
                            active = sp.active if sp.active is not None else True
                            results.append(
                                Principal(
                                    name=name,
                                    type=PrincipalType.SERVICE_PRINCIPAL,
                                    active=active,
                                )
                            )
                except Exception as ex:
                    logger.warning(
                        "Falha ao listar service principals no workspace %s: %s", ws, ex
                    )

            if query:
                q = query.lower()
                results = [p for p in results if q in p.name.lower()]

            if page_size > 0 and len(results) > page_size:
                results = results[:page_size]

            return results, ""

    def get_principal(self, ws: str, name: str) -> Principal:
        with self._handle_errors(ws):
            w = self._factory.get_client(ws)

            # Tenta buscar como User
            try:
                for u in w.users.list(filter=f'userName eq "{name}"'):
                    p_name = u.user_name or name
                    active = u.active if u.active is not None else True
                    return Principal(
                        name=p_name, type=PrincipalType.USER, active=active
                    )
            except Exception:
                pass

            # Tenta buscar como Service Principal (por applicationId ou displayName)
            try:
                for sp in w.service_principals.list(filter=f'applicationId eq "{name}"'):
                    p_name = sp.application_id or name
                    active = sp.active if sp.active is not None else True
                    return Principal(
                        name=p_name,
                        type=PrincipalType.SERVICE_PRINCIPAL,
                        active=active,
                    )
            except Exception:
                pass

            try:
                for sp in w.service_principals.list(filter=f'displayName eq "{name}"'):
                    p_name = sp.display_name or name
                    active = sp.active if sp.active is not None else True
                    return Principal(
                        name=p_name,
                        type=PrincipalType.SERVICE_PRINCIPAL,
                        active=active,
                    )
            except Exception:
                pass

            # Tenta buscar como Group
            try:
                for g in w.groups.list(filter=f'displayName eq "{name}"'):
                    p_name = g.display_name or name
                    return Principal(name=p_name, type=PrincipalType.GROUP, active=True)
            except Exception:
                pass

            raise PrincipalNotFoundException(name)
