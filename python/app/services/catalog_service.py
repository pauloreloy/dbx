from datetime import datetime, timezone
from typing import Any, Optional
import uuid

from app.domain.audit import AuditAction, AuditEvent, AuditOutcome
from app.domain.errors import (
    BadRequestException,
    ConfirmationMismatchException,
    PrincipalNotFoundException,
)
from app.domain.models import (
    Catalog,
    DataSchema,
    SecurableRef,
    SecurableType,
    Table,
    View,
    Volume,
    validate_create_identifier,
)
from app.ports.databricks import (
    CatalogsAPI,
    IdentityAPI,
    OwnershipAPI,
    SchemasAPI,
    TablesAPI,
    TagsAPI,
    ViewsAPI,
    VolumesAPI,
)
from app.ports.stores import AuditStore, WorkspaceRegistry


class CatalogService:
    """Orquestra o ciclo de vida dos objetos e seus metadados."""

    def __init__(
        self,
        catalogs_api: CatalogsAPI,
        schemas_api: SchemasAPI,
        tables_api: TablesAPI,
        views_api: ViewsAPI,
        volumes_api: VolumesAPI,
        ownership_api: OwnershipAPI,
        tags_api: TagsAPI,
        identity_api: Optional[IdentityAPI] = None,
        audit_store: Optional[AuditStore] = None,
        registry: Optional[WorkspaceRegistry] = None,
    ) -> None:
        self._catalogs_api = catalogs_api
        self._schemas_api = schemas_api
        self._tables_api = tables_api
        self._views_api = views_api
        self._volumes_api = volumes_api
        self._ownership_api = ownership_api
        self._tags_api = tags_api
        self._identity_api = identity_api
        self._audit_store = audit_store
        self._registry = registry

    # --- Catalogs ---

    def list_catalogs(
        self, ws: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[Catalog], str]:
        return self._catalogs_api.list_catalogs(ws, page_size, page_token)

    def get_catalog(self, ws: str, name: str) -> Catalog:
        return self._catalogs_api.get_catalog(ws, name)

    def create_catalog(
        self, ws: str, cat: Catalog, actor: str, change_ref: str, request_id: str
    ) -> Catalog:
        if not validate_create_identifier(cat.name):
            raise BadRequestException(
                f"Nome de catálogo inválido '{cat.name}'. Deve iniciar com letra e conter apenas letras minúsculas, dígitos e sublinhados."
            )

        created = self._catalogs_api.create_catalog(ws, cat)
        sec = SecurableRef(type=SecurableType.CATALOG, name=cat.name)
        self._record_audit(
            ws,
            actor,
            AuditAction.CATALOG_CREATE,
            change_ref,
            request_id,
            sec,
            after={"catalog": created.model_dump(mode="json")},
        )
        return created

    def update_catalog(
        self,
        ws: str,
        name: str,
        new_name: Optional[str] = None,
        comment: Optional[str] = None,
        props: Optional[dict[str, str]] = None,
        actor: str = "",
        change_ref: str = "",
        request_id: str = "",
    ) -> Catalog:
        if new_name is not None and not validate_create_identifier(new_name):
            raise BadRequestException(f"Novo nome de catálogo inválido '{new_name}'")

        before: Optional[Catalog] = None
        try:
            before = self._catalogs_api.get_catalog(ws, name)
        except Exception:
            pass

        updated = self._catalogs_api.update_catalog(ws, name, new_name, comment, props)
        sec = SecurableRef(type=SecurableType.CATALOG, name=name)
        self._record_audit(
            ws,
            actor,
            AuditAction.CATALOG_UPDATE,
            change_ref,
            request_id,
            sec,
            before={"catalog": before.model_dump(mode="json")} if before else None,
            after={"catalog": updated.model_dump(mode="json")},
        )
        return updated

    def delete_catalog(
        self,
        ws: str,
        name: str,
        confirm: str,
        force: bool,
        actor: str,
        change_ref: str,
        request_id: str,
    ) -> None:
        if confirm != name:
            raise ConfirmationMismatchException(expected=name, received=confirm)

        before: Optional[Catalog] = None
        try:
            before = self._catalogs_api.get_catalog(ws, name)
        except Exception:
            pass

        self._catalogs_api.delete_catalog(ws, name, force)
        sec = SecurableRef(type=SecurableType.CATALOG, name=name)
        self._record_audit(
            ws,
            actor,
            AuditAction.CATALOG_DELETE,
            change_ref,
            request_id,
            sec,
            before={"catalog": before.model_dump(mode="json")} if before else None,
        )

    # --- Schemas ---

    def list_schemas(
        self, ws: str, catalog: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[DataSchema], str]:
        return self._schemas_api.list_schemas(ws, catalog, page_size, page_token)

    def get_schema(self, ws: str, catalog: str, schema: str) -> DataSchema:
        return self._schemas_api.get_schema(ws, catalog, schema)

    def create_schema(
        self, ws: str, catalog: str, sch: DataSchema, actor: str, change_ref: str, request_id: str
    ) -> DataSchema:
        if not validate_create_identifier(sch.name):
            raise BadRequestException(f"Nome de schema inválido '{sch.name}'")

        created = self._schemas_api.create_schema(ws, catalog, sch)
        full_name = f"{catalog}.{sch.name}"
        sec = SecurableRef(type=SecurableType.SCHEMA, name=full_name)
        self._record_audit(
            ws,
            actor,
            AuditAction.SCHEMA_CREATE,
            change_ref,
            request_id,
            sec,
            after={"schema": created.model_dump(mode="json")},
        )
        return created

    def update_schema(
        self,
        ws: str,
        catalog: str,
        schema: str,
        new_name: Optional[str] = None,
        comment: Optional[str] = None,
        props: Optional[dict[str, str]] = None,
        actor: str = "",
        change_ref: str = "",
        request_id: str = "",
    ) -> DataSchema:
        if new_name is not None and not validate_create_identifier(new_name):
            raise BadRequestException(f"Novo nome de schema inválido '{new_name}'")

        full_name = f"{catalog}.{schema}"
        before: Optional[DataSchema] = None
        try:
            before = self._schemas_api.get_schema(ws, catalog, schema)
        except Exception:
            pass

        updated = self._schemas_api.update_schema(ws, catalog, schema, new_name, comment, props)
        sec = SecurableRef(type=SecurableType.SCHEMA, name=full_name)
        self._record_audit(
            ws,
            actor,
            AuditAction.SCHEMA_UPDATE,
            change_ref,
            request_id,
            sec,
            before={"schema": before.model_dump(mode="json")} if before else None,
            after={"schema": updated.model_dump(mode="json")},
        )
        return updated

    def delete_schema(
        self,
        ws: str,
        catalog: str,
        schema: str,
        confirm: str,
        force: bool,
        actor: str,
        change_ref: str,
        request_id: str,
    ) -> None:
        full_name = f"{catalog}.{schema}"
        if confirm != full_name:
            raise ConfirmationMismatchException(expected=full_name, received=confirm)

        before: Optional[DataSchema] = None
        try:
            before = self._schemas_api.get_schema(ws, catalog, schema)
        except Exception:
            pass

        self._schemas_api.delete_schema(ws, catalog, schema, force)
        sec = SecurableRef(type=SecurableType.SCHEMA, name=full_name)
        self._record_audit(
            ws,
            actor,
            AuditAction.SCHEMA_DELETE,
            change_ref,
            request_id,
            sec,
            before={"schema": before.model_dump(mode="json")} if before else None,
        )

    # --- Tables ---

    def list_tables(
        self, ws: str, catalog: str, schema: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[Table], str]:
        return self._tables_api.list_tables(ws, catalog, schema, page_size, page_token)

    def get_table(self, ws: str, catalog: str, schema: str, table: str) -> Table:
        return self._tables_api.get_table(ws, catalog, schema, table)

    def delete_table(
        self,
        ws: str,
        catalog: str,
        schema: str,
        table: str,
        confirm: str,
        actor: str,
        change_ref: str,
        request_id: str,
    ) -> None:
        full_name = f"{catalog}.{schema}.{table}"
        if confirm != full_name:
            raise ConfirmationMismatchException(expected=full_name, received=confirm)

        before: Optional[Table] = None
        try:
            before = self._tables_api.get_table(ws, catalog, schema, table)
        except Exception:
            pass

        self._tables_api.delete_table(ws, catalog, schema, table)
        sec = SecurableRef(type=SecurableType.TABLE, name=full_name)
        self._record_audit(
            ws,
            actor,
            AuditAction.TABLE_DELETE,
            change_ref,
            request_id,
            sec,
            before={"table": before.model_dump(mode="json")} if before else None,
        )

    # --- Views ---

    def list_views(
        self, ws: str, catalog: str, schema: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[View], str]:
        return self._views_api.list_views(ws, catalog, schema, page_size, page_token)

    def get_view(self, ws: str, catalog: str, schema: str, view: str) -> View:
        return self._views_api.get_view(ws, catalog, schema, view)

    def delete_view(
        self,
        ws: str,
        catalog: str,
        schema: str,
        view: str,
        confirm: str,
        actor: str,
        change_ref: str,
        request_id: str,
    ) -> None:
        full_name = f"{catalog}.{schema}.{view}"
        if confirm != full_name:
            raise ConfirmationMismatchException(expected=full_name, received=confirm)

        before: Optional[View] = None
        try:
            before = self._views_api.get_view(ws, catalog, schema, view)
        except Exception:
            pass

        self._views_api.delete_view(ws, catalog, schema, view)
        sec = SecurableRef(type=SecurableType.VIEW, name=full_name)
        self._record_audit(
            ws,
            actor,
            AuditAction.VIEW_DELETE,
            change_ref,
            request_id,
            sec,
            before={"view": before.model_dump(mode="json")} if before else None,
        )

    # --- Volumes ---

    def list_volumes(
        self, ws: str, catalog: str, schema: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[Volume], str]:
        return self._volumes_api.list_volumes(ws, catalog, schema, page_size, page_token)

    def get_volume(self, ws: str, catalog: str, schema: str, volume: str) -> Volume:
        return self._volumes_api.get_volume(ws, catalog, schema, volume)

    def create_volume(
        self,
        ws: str,
        catalog: str,
        schema: str,
        vol: Volume,
        actor: str,
        change_ref: str,
        request_id: str,
    ) -> Volume:
        if not validate_create_identifier(vol.name):
            raise BadRequestException(f"Nome de volume inválido '{vol.name}'")

        created = self._volumes_api.create_volume(ws, catalog, schema, vol)
        full_name = f"{catalog}.{schema}.{vol.name}"
        sec = SecurableRef(type=SecurableType.VOLUME, name=full_name)
        self._record_audit(
            ws,
            actor,
            AuditAction.VOLUME_CREATE,
            change_ref,
            request_id,
            sec,
            after={"volume": created.model_dump(mode="json")},
        )
        return created

    def update_volume(
        self,
        ws: str,
        catalog: str,
        schema: str,
        volume: str,
        new_name: Optional[str] = None,
        comment: Optional[str] = None,
        actor: str = "",
        change_ref: str = "",
        request_id: str = "",
    ) -> Volume:
        if new_name is not None and not validate_create_identifier(new_name):
            raise BadRequestException(f"Novo nome de volume inválido '{new_name}'")

        full_name = f"{catalog}.{schema}.{volume}"
        before: Optional[Volume] = None
        try:
            before = self._volumes_api.get_volume(ws, catalog, schema, volume)
        except Exception:
            pass

        updated = self._volumes_api.update_volume(ws, catalog, schema, volume, new_name, comment)
        sec = SecurableRef(type=SecurableType.VOLUME, name=full_name)
        self._record_audit(
            ws,
            actor,
            AuditAction.VOLUME_UPDATE,
            change_ref,
            request_id,
            sec,
            before={"volume": before.model_dump(mode="json")} if before else None,
            after={"volume": updated.model_dump(mode="json")},
        )
        return updated

    def delete_volume(
        self,
        ws: str,
        catalog: str,
        schema: str,
        volume: str,
        confirm: str,
        actor: str,
        change_ref: str,
        request_id: str,
    ) -> None:
        full_name = f"{catalog}.{schema}.{volume}"
        if confirm != full_name:
            raise ConfirmationMismatchException(expected=full_name, received=confirm)

        before: Optional[Volume] = None
        try:
            before = self._volumes_api.get_volume(ws, catalog, schema, volume)
        except Exception:
            pass

        self._volumes_api.delete_volume(ws, catalog, schema, volume)
        sec = SecurableRef(type=SecurableType.VOLUME, name=full_name)
        self._record_audit(
            ws,
            actor,
            AuditAction.VOLUME_DELETE,
            change_ref,
            request_id,
            sec,
            before={"volume": before.model_dump(mode="json")} if before else None,
        )

    # --- Ownership ---

    def get_owner(self, ws: str, sec: SecurableRef) -> str:
        return self._ownership_api.get_owner(ws, sec)

    def set_owner(
        self,
        ws: str,
        sec: SecurableRef,
        new_owner: str,
        actor: str,
        change_ref: str,
        request_id: str,
    ) -> None:
        if self._identity_api:
            try:
                p = self._identity_api.get_principal(ws, new_owner)
            except Exception:
                p = None
            if not p:
                raise PrincipalNotFoundException(new_owner)

        old_owner = ""
        try:
            old_owner = self._ownership_api.get_owner(ws, sec)
        except Exception:
            pass

        self._ownership_api.set_owner(ws, sec, new_owner)
        self._record_audit(
            ws,
            actor,
            AuditAction.OWNER_CHANGE,
            change_ref,
            request_id,
            sec,
            targets=[new_owner],
            before={"owner": old_owner},
            after={"owner": new_owner},
        )

    # --- Tags ---

    def list_tags(self, ws: str, sec: SecurableRef) -> dict[str, str]:
        return self._tags_api.list_tags(ws, sec)

    def set_tag(
        self,
        ws: str,
        sec: SecurableRef,
        key: str,
        value: str,
        actor: str,
        change_ref: str,
        request_id: str,
    ) -> None:
        self._tags_api.set_tag(ws, sec, key, value)
        self._record_audit(
            ws,
            actor,
            AuditAction.TAG_SET,
            change_ref,
            request_id,
            sec,
            after={key: value},
        )

    def delete_tag(
        self,
        ws: str,
        sec: SecurableRef,
        key: str,
        actor: str,
        change_ref: str,
        request_id: str,
    ) -> None:
        self._tags_api.delete_tag(ws, sec, key)
        self._record_audit(
            ws,
            actor,
            AuditAction.TAG_DELETE,
            change_ref,
            request_id,
            sec,
            before={"deleted_tag": key},
        )

    # --- Auditoria Interna ---

    def _record_audit(
        self,
        ws: str,
        actor: str,
        action: AuditAction,
        change_ref: str,
        request_id: str,
        sec: Optional[SecurableRef] = None,
        targets: Optional[list[str]] = None,
        before: Optional[dict[str, Any]] = None,
        after: Optional[dict[str, Any]] = None,
    ) -> None:
        if not self._audit_store:
            return

        metastore_id: Optional[str] = None
        if self._registry:
            try:
                ws_info = self._registry.get_workspace(ws)
                if ws_info and ws_info.metastore_id:
                    metastore_id = ws_info.metastore_id
                    if sec:
                        sec.metastore_id = metastore_id
            except Exception:
                pass

        try:
            self._audit_store.record(
                AuditEvent(
                    id=f"evt_{uuid.uuid4().hex}",
                    occurred_at=datetime.now(timezone.utc),
                    workspace=ws,
                    metastore_id=metastore_id,
                    actor=actor,
                    action=action,
                    outcome=AuditOutcome.SUCCESS,
                    change_ref=change_ref,
                    request_id=request_id,
                    securable=sec,
                    targets=targets,
                    before=before,
                    after=after,
                )
            )
        except Exception:
            pass
