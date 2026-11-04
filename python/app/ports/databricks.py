from abc import ABC, abstractmethod
from typing import Optional

from app.domain.models import (
    Catalog,
    ChangeOutcome,
    DataSchema,
    EffectiveGrant,
    PrincipalPrivileges,
    PrivilegeChange,
    SecurableRef,
    Table,
    View,
    Volume,
)
from app.domain.principal import Principal, PrincipalType


class GrantsAPI(ABC):
    @abstractmethod
    def get_grants(
        self, ws: str, sec: SecurableRef, principal_filter: Optional[str] = None
    ) -> list[PrincipalPrivileges]:
        pass

    @abstractmethod
    def get_effective_grants(
        self, ws: str, sec: SecurableRef, principal_filter: Optional[str] = None
    ) -> list[EffectiveGrant]:
        pass

    @abstractmethod
    def update_grants(
        self,
        ws: str,
        sec: SecurableRef,
        changes: list[PrivilegeChange],
        is_revoke: bool,
    ) -> tuple[list[ChangeOutcome], list[PrincipalPrivileges], bool]:
        pass


class CatalogsAPI(ABC):
    @abstractmethod
    def list_catalogs(
        self, ws: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[Catalog], str]:
        pass

    @abstractmethod
    def get_catalog(self, ws: str, name: str) -> Catalog:
        pass

    @abstractmethod
    def create_catalog(self, ws: str, cat: Catalog) -> Catalog:
        pass

    @abstractmethod
    def update_catalog(
        self,
        ws: str,
        name: str,
        new_name: Optional[str] = None,
        comment: Optional[str] = None,
        props: Optional[dict[str, str]] = None,
    ) -> Catalog:
        pass

    @abstractmethod
    def delete_catalog(self, ws: str, name: str, force: bool = False) -> None:
        pass


class SchemasAPI(ABC):
    @abstractmethod
    def list_schemas(
        self, ws: str, catalog: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[DataSchema], str]:
        pass

    @abstractmethod
    def get_schema(self, ws: str, catalog: str, schema: str) -> DataSchema:
        pass

    @abstractmethod
    def create_schema(self, ws: str, catalog: str, s: DataSchema) -> DataSchema:
        pass

    @abstractmethod
    def update_schema(
        self,
        ws: str,
        catalog: str,
        schema: str,
        new_name: Optional[str] = None,
        comment: Optional[str] = None,
        props: Optional[dict[str, str]] = None,
    ) -> DataSchema:
        pass

    @abstractmethod
    def delete_schema(self, ws: str, catalog: str, schema: str, force: bool = False) -> None:
        pass


class TablesAPI(ABC):
    @abstractmethod
    def list_tables(
        self, ws: str, catalog: str, schema: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[Table], str]:
        pass

    @abstractmethod
    def get_table(self, ws: str, catalog: str, schema: str, table: str) -> Table:
        pass

    @abstractmethod
    def delete_table(self, ws: str, catalog: str, schema: str, table: str) -> None:
        pass


class ViewsAPI(ABC):
    @abstractmethod
    def list_views(
        self, ws: str, catalog: str, schema: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[View], str]:
        pass

    @abstractmethod
    def get_view(self, ws: str, catalog: str, schema: str, view: str) -> View:
        pass

    @abstractmethod
    def delete_view(self, ws: str, catalog: str, schema: str, view: str) -> None:
        pass


class VolumesAPI(ABC):
    @abstractmethod
    def list_volumes(
        self, ws: str, catalog: str, schema: str, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[Volume], str]:
        pass

    @abstractmethod
    def get_volume(self, ws: str, catalog: str, schema: str, volume: str) -> Volume:
        pass

    @abstractmethod
    def create_volume(self, ws: str, catalog: str, schema: str, v: Volume) -> Volume:
        pass

    @abstractmethod
    def update_volume(
        self,
        ws: str,
        catalog: str,
        schema: str,
        volume: str,
        new_name: Optional[str] = None,
        comment: Optional[str] = None,
    ) -> Volume:
        pass

    @abstractmethod
    def delete_volume(self, ws: str, catalog: str, schema: str, volume: str) -> None:
        pass


class OwnershipAPI(ABC):
    @abstractmethod
    def get_owner(self, ws: str, sec: SecurableRef) -> str:
        pass

    @abstractmethod
    def set_owner(self, ws: str, sec: SecurableRef, new_owner: str) -> None:
        pass


class TagsAPI(ABC):
    @abstractmethod
    def list_tags(self, ws: str, sec: SecurableRef) -> dict[str, str]:
        pass

    @abstractmethod
    def set_tag(self, ws: str, sec: SecurableRef, key: str, value: str) -> None:
        pass

    @abstractmethod
    def delete_tag(self, ws: str, sec: SecurableRef, key: str) -> None:
        pass


class IdentityAPI(ABC):
    @abstractmethod
    def list_principals(
        self,
        ws: str,
        p_type: Optional[PrincipalType] = None,
        query: Optional[str] = None,
        page_size: int = 0,
        page_token: str = "",
    ) -> tuple[list[Principal], str]:
        pass

    @abstractmethod
    def get_principal(self, ws: str, name: str) -> Principal:
        pass
