from datetime import datetime, timezone
import threading
from typing import Optional

from app.domain.errors import ConflictException, NotFoundException, PrincipalNotFoundException
from app.domain.models import (
    Catalog,
    ChangeOutcome,
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


class MemoryDatabricksClient(
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
    """Simula a API do Databricks Unity Catalog em memória de forma thread-safe."""

    def __init__(self) -> None:
        self._lock = threading.RLock()
        now = datetime.now(timezone.utc)

        self._catalogs: dict[str, Catalog] = {}
        self._schemas: dict[str, DataSchema] = {}
        self._tables: dict[str, Table] = {}
        self._views: dict[str, View] = {}
        self._volumes: dict[str, Volume] = {}
        self._grants: dict[str, dict[str, list[Privilege]]] = {}
        self._owners: dict[str, str] = {}
        self._tags: dict[str, dict[str, str]] = {}
        self._principals: dict[str, Principal] = {}

        # Identidades de exemplo padrão
        self._principals["admin@company.com"] = Principal(
            name="admin@company.com", type=PrincipalType.USER, active=True
        )
        self._principals["analyst@company.com"] = Principal(
            name="analyst@company.com", type=PrincipalType.USER, active=True
        )
        self._principals["grp-analytics"] = Principal(
            name="grp-analytics", type=PrincipalType.GROUP, active=True
        )
        self._principals["sp-governance-service"] = Principal(
            name="sp-governance-service", type=PrincipalType.SERVICE_PRINCIPAL, active=True
        )

        # Catálogo 'main' padrão
        cat_name = "main"
        self._catalogs[cat_name] = Catalog(
            name=cat_name,
            owner="sp-governance-service",
            catalog_type="MANAGED",
            created_at=now,
            updated_at=now,
        )
        self._owners[f"CATALOG:{cat_name}"] = "sp-governance-service"

    def _sec_key(self, sec: SecurableRef) -> str:
        return str(sec)

    # --- Grants API ---

    def get_grants(
        self, ws: str, sec: SecurableRef, principal_filter: Optional[str] = None
    ) -> list[PrincipalPrivileges]:
        with self._lock:
            key = self._sec_key(sec)
            obj_grants = self._grants.get(key, {})
            result: list[PrincipalPrivileges] = []
            for princ, privs in obj_grants.items():
                if principal_filter and princ != principal_filter:
                    continue
                result.append(PrincipalPrivileges(principal=princ, privileges=list(privs)))
            return result

    def get_effective_grants(
        self, ws: str, sec: SecurableRef, principal_filter: Optional[str] = None
    ) -> list[EffectiveGrant]:
        direct = self.get_grants(ws, sec, principal_filter)
        result: list[EffectiveGrant] = []
        for d in direct:
            eff_privs = [
                EffectivePrivilege(privilege=p, inherited=False) for p in d.privileges
            ]
            result.append(EffectiveGrant(principal=d.principal, privileges=eff_privs))
        return result

    def update_grants(
        self,
        ws: str,
        sec: SecurableRef,
        changes: list[PrivilegeChange],
        is_revoke: bool,
    ) -> tuple[list[ChangeOutcome], list[PrincipalPrivileges], bool]:
        with self._lock:
            key = self._sec_key(sec)
            if key not in self._grants:
                self._grants[key] = {}

            outcomes: list[ChangeOutcome] = []
            overall_changed = False

            for ch in changes:
                current_privs = list(self._grants[key].get(ch.principal, []))
                applied: list[Privilege] = []
                unchanged: list[Privilege] = []

                if not is_revoke:
                    # Grant
                    for p in ch.privileges:
                        p_enum = Privilege(p)
                        if p_enum in current_privs:
                            unchanged.append(p_enum)
                        else:
                            applied.append(p_enum)
                            current_privs.append(p_enum)
                            overall_changed = True
                else:
                    # Revoke
                    revoke_enums = [Privilege(p) for p in ch.privileges]
                    remaining: list[Privilege] = []
                    for cp in current_privs:
                        if cp in revoke_enums:
                            applied.append(cp)
                            overall_changed = True
                        else:
                            remaining.append(cp)
                    for p in ch.privileges:
                        p_enum = Privilege(p)
                        if p_enum not in current_privs:
                            unchanged.append(p_enum)
                    current_privs = remaining

                self._grants[key][ch.principal] = current_privs
                outcomes.append(
                    ChangeOutcome(
                        principal=ch.principal,
                        applied=applied,
                        unchanged=unchanged,
                    )
                )

            resulting_grants: list[PrincipalPrivileges] = []
            for princ, privs in self._grants[key].items():
                if privs:
                    resulting_grants.append(
                        PrincipalPrivileges(principal=princ, privileges=list(privs))
                    )

            return outcomes, resulting_grants, overall_changed

    # --- Catalogs API ---

    def list_catalogs(
        self, ws: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[Catalog], str]:
        with self._lock:
            items = list(self._catalogs.values())
            if page_size > 0 and len(items) > page_size:
                items = items[:page_size]
            return items, ""

    def get_catalog(self, ws: str, name: str) -> Catalog:
        with self._lock:
            cat = self._catalogs.get(name)
            if not cat:
                raise NotFoundException("Catalog", name)
            return cat

    def create_catalog(self, ws: str, cat: Catalog) -> Catalog:
        with self._lock:
            if cat.name in self._catalogs:
                raise ConflictException(f"Catálogo '{cat.name}' já existe")

            now = datetime.now(timezone.utc)
            cat.created_at = now
            cat.updated_at = now
            if not cat.owner:
                cat.owner = "sp-governance-service"

            self._catalogs[cat.name] = cat
            self._owners[f"CATALOG:{cat.name}"] = cat.owner
            return cat

    def update_catalog(
        self,
        ws: str,
        name: str,
        new_name: Optional[str] = None,
        comment: Optional[str] = None,
        props: Optional[dict[str, str]] = None,
    ) -> Catalog:
        with self._lock:
            cat = self._catalogs.get(name)
            if not cat:
                raise NotFoundException("Catalog", name)

            if new_name and new_name != name:
                del self._catalogs[name]
                cat.name = new_name
            if comment is not None:
                cat.comment = comment
            if props is not None:
                cat.properties = props

            cat.updated_at = datetime.now(timezone.utc)
            self._catalogs[cat.name] = cat
            return cat

    def delete_catalog(self, ws: str, name: str, force: bool = False) -> None:
        with self._lock:
            if name not in self._catalogs:
                raise NotFoundException("Catalog", name)
            del self._catalogs[name]
            self._owners.pop(f"CATALOG:{name}", None)

    # --- Schemas API ---

    def list_schemas(
        self, ws: str, catalog: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[DataSchema], str]:
        with self._lock:
            items = [s for s in self._schemas.values() if s.catalog_name == catalog]
            if page_size > 0 and len(items) > page_size:
                items = items[:page_size]
            return items, ""

    def get_schema(self, ws: str, catalog: str, schema: str) -> DataSchema:
        with self._lock:
            key = f"{catalog}.{schema}"
            s = self._schemas.get(key)
            if not s:
                raise NotFoundException("Schema", key)
            return s

    def create_schema(self, ws: str, catalog: str, s: DataSchema) -> DataSchema:
        with self._lock:
            key = f"{catalog}.{s.name}"
            if key in self._schemas:
                raise ConflictException(f"Schema '{key}' já existe")

            now = datetime.now(timezone.utc)
            s.catalog_name = catalog
            s.full_name = key
            s.created_at = now
            s.updated_at = now
            if not s.owner:
                s.owner = "sp-governance-service"

            self._schemas[key] = s
            self._owners[f"SCHEMA:{key}"] = s.owner
            return s

    def update_schema(
        self,
        ws: str,
        catalog: str,
        schema: str,
        new_name: Optional[str] = None,
        comment: Optional[str] = None,
        props: Optional[dict[str, str]] = None,
    ) -> DataSchema:
        with self._lock:
            key = f"{catalog}.{schema}"
            s = self._schemas.get(key)
            if not s:
                raise NotFoundException("Schema", key)

            if new_name and new_name != schema:
                del self._schemas[key]
                s.name = new_name
                key = f"{catalog}.{new_name}"
                s.full_name = key
            if comment is not None:
                s.comment = comment
            if props is not None:
                s.properties = props

            s.updated_at = datetime.now(timezone.utc)
            self._schemas[key] = s
            return s

    def delete_schema(self, ws: str, catalog: str, schema: str, force: bool = False) -> None:
        with self._lock:
            key = f"{catalog}.{schema}"
            if key not in self._schemas:
                raise NotFoundException("Schema", key)
            del self._schemas[key]
            self._owners.pop(f"SCHEMA:{key}", None)

    # --- Tables API ---

    def list_tables(
        self, ws: str, catalog: str, schema: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[Table], str]:
        with self._lock:
            items = [
                t
                for t in self._tables.values()
                if t.catalog_name == catalog and t.schema_name == schema
            ]
            if page_size > 0 and len(items) > page_size:
                items = items[:page_size]
            return items, ""

    def get_table(self, ws: str, catalog: str, schema: str, table: str) -> Table:
        with self._lock:
            key = f"{catalog}.{schema}.{table}"
            t = self._tables.get(key)
            if not t:
                raise NotFoundException("Table", key)
            return t

    def delete_table(self, ws: str, catalog: str, schema: str, table: str) -> None:
        with self._lock:
            key = f"{catalog}.{schema}.{table}"
            if key not in self._tables:
                raise NotFoundException("Table", key)
            del self._tables[key]

    # --- Views API ---

    def list_views(
        self, ws: str, catalog: str, schema: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[View], str]:
        with self._lock:
            items = [
                v
                for v in self._views.values()
                if v.catalog_name == catalog and v.schema_name == schema
            ]
            if page_size > 0 and len(items) > page_size:
                items = items[:page_size]
            return items, ""

    def get_view(self, ws: str, catalog: str, schema: str, view: str) -> View:
        with self._lock:
            key = f"{catalog}.{schema}.{view}"
            v = self._views.get(key)
            if not v:
                raise NotFoundException("View", key)
            return v

    def delete_view(self, ws: str, catalog: str, schema: str, view: str) -> None:
        with self._lock:
            key = f"{catalog}.{schema}.{view}"
            if key not in self._views:
                raise NotFoundException("View", key)
            del self._views[key]

    # --- Volumes API ---

    def list_volumes(
        self, ws: str, catalog: str, schema: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[Volume], str]:
        with self._lock:
            items = [
                v
                for v in self._volumes.values()
                if v.catalog_name == catalog and v.schema_name == schema
            ]
            if page_size > 0 and len(items) > page_size:
                items = items[:page_size]
            return items, ""

    def get_volume(self, ws: str, catalog: str, schema: str, volume: str) -> Volume:
        with self._lock:
            key = f"{catalog}.{schema}.{volume}"
            v = self._volumes.get(key)
            if not v:
                raise NotFoundException("Volume", key)
            return v

    def create_volume(self, ws: str, catalog: str, schema: str, v: Volume) -> Volume:
        with self._lock:
            key = f"{catalog}.{schema}.{v.name}"
            if key in self._volumes:
                raise ConflictException(f"Volume '{key}' já existe")

            now = datetime.now(timezone.utc)
            v.catalog_name = catalog
            v.schema_name = schema
            v.full_name = key
            v.created_at = now
            v.updated_at = now
            if not v.owner:
                v.owner = "sp-governance-service"

            self._volumes[key] = v
            self._owners[f"VOLUME:{key}"] = v.owner
            return v

    def update_volume(
        self,
        ws: str,
        catalog: str,
        schema: str,
        volume: str,
        new_name: Optional[str] = None,
        comment: Optional[str] = None,
    ) -> Volume:
        with self._lock:
            key = f"{catalog}.{schema}.{volume}"
            v = self._volumes.get(key)
            if not v:
                raise NotFoundException("Volume", key)

            if new_name and new_name != volume:
                del self._volumes[key]
                v.name = new_name
                key = f"{catalog}.{schema}.{new_name}"
                v.full_name = key
            if comment is not None:
                v.comment = comment

            v.updated_at = datetime.now(timezone.utc)
            self._volumes[key] = v
            return v

    def delete_volume(self, ws: str, catalog: str, schema: str, volume: str) -> None:
        with self._lock:
            key = f"{catalog}.{schema}.{volume}"
            if key not in self._volumes:
                raise NotFoundException("Volume", key)
            del self._volumes[key]

    # --- Ownership API ---

    def get_owner(self, ws: str, sec: SecurableRef) -> str:
        with self._lock:
            key = self._sec_key(sec)
            owner = self._owners.get(key)
            if not owner:
                raise NotFoundException(str(sec.type), sec.name)
            return owner

    def set_owner(self, ws: str, sec: SecurableRef, new_owner: str) -> None:
        with self._lock:
            key = self._sec_key(sec)
            self._owners[key] = new_owner

    # --- Tags API ---

    def list_tags(self, ws: str, sec: SecurableRef) -> dict[str, str]:
        with self._lock:
            key = self._sec_key(sec)
            tags = self._tags.get(key, {})
            return dict(tags)

    def set_tag(self, ws: str, sec: SecurableRef, key: str, value: str) -> None:
        with self._lock:
            s_key = self._sec_key(sec)
            if s_key not in self._tags:
                self._tags[s_key] = {}
            self._tags[s_key][key] = value

    def delete_tag(self, ws: str, sec: SecurableRef, key: str) -> None:
        with self._lock:
            s_key = self._sec_key(sec)
            if s_key in self._tags:
                self._tags[s_key].pop(key, None)

    # --- Identity API ---

    def list_principals(
        self,
        ws: str,
        p_type: Optional[PrincipalType] = None,
        query: Optional[str] = None,
        page_size: int = 0,
        page_token: str = "",
    ) -> tuple[list[Principal], str]:
        with self._lock:
            items = list(self._principals.values())
            if p_type:
                items = [p for p in items if p.type == p_type]
            if query:
                q = query.lower()
                items = [p for p in items if q in p.name.lower()]
            if page_size > 0 and len(items) > page_size:
                items = items[:page_size]
            return items, ""

    def get_principal(self, ws: str, name: str) -> Principal:
        with self._lock:
            p = self._principals.get(name)
            if not p:
                raise PrincipalNotFoundException(name)
            return p
