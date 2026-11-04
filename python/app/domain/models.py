from datetime import datetime
from enum import StrEnum
import re
from typing import Optional
from pydantic import BaseModel, ConfigDict, Field

from app.domain.privilege import Privilege


class SecurableType(StrEnum):
    CATALOG = "CATALOG"
    SCHEMA = "SCHEMA"
    TABLE = "TABLE"
    VIEW = "VIEW"
    VOLUME = "VOLUME"


class SecurableRef(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    type: SecurableType
    name: str
    metastore_id: Optional[str] = None

    def __str__(self) -> str:
        return f"{self.type}:{self.name}"


IDENTIFIER_REGEX = re.compile(r"^[a-z][a-z0-9_]{0,254}$")
PATH_PARAM_REGEX = re.compile(r"^[^./\s]{1,255}$")


def validate_create_identifier(name: str) -> bool:
    """Valida se o nome atende ao padrão estrito de criação (minúsculas, dígitos, underline)."""
    return bool(IDENTIFIER_REGEX.match(name))


def validate_path_identifier(name: str) -> bool:
    """Valida se o nome em rota atende ao padrão geral (sem pontos, barras ou espaços)."""
    return bool(PATH_PARAM_REGEX.match(name))


class Workspace(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    id: str
    display_name: str
    databricks_workspace_id: Optional[str] = None
    metastore_id: Optional[str] = None
    cloud: Optional[str] = None
    region: Optional[str] = None
    status: str = "ACTIVE"


class Catalog(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    name: str
    comment: Optional[str] = None
    owner: Optional[str] = None
    catalog_type: Optional[str] = None
    storage_root: Optional[str] = None
    properties: Optional[dict[str, str]] = None
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None


class DataSchema(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    name: str
    catalog_name: Optional[str] = None
    full_name: Optional[str] = None
    comment: Optional[str] = None
    owner: Optional[str] = None
    storage_root: Optional[str] = None
    properties: Optional[dict[str, str]] = None
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None


class Column(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    name: str
    type_text: str
    position: int = 0
    comment: Optional[str] = None
    nullable: bool = True


class Table(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    name: str
    catalog_name: str
    schema_name: str
    full_name: str
    table_type: Optional[str] = None
    data_source_format: Optional[str] = None
    storage_location: Optional[str] = None
    comment: Optional[str] = None
    owner: Optional[str] = None
    columns: Optional[list[Column]] = None
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None


class View(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    name: str
    catalog_name: str
    schema_name: str
    full_name: str
    view_definition: Optional[str] = None
    comment: Optional[str] = None
    owner: Optional[str] = None
    columns: Optional[list[Column]] = None
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None


class Volume(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    name: str
    catalog_name: str
    schema_name: str
    full_name: str
    volume_type: str = "MANAGED"
    storage_location: Optional[str] = None
    comment: Optional[str] = None
    owner: Optional[str] = None
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None


class PrincipalPrivileges(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    principal: str
    privileges: list[Privilege] = Field(default_factory=list)


class EffectivePrivilege(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    privilege: Privilege
    inherited: bool = False
    inherited_from: Optional[SecurableRef] = None


class EffectiveGrant(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    principal: str
    privileges: list[EffectivePrivilege] = Field(default_factory=list)


class PrivilegeChange(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    principal: str
    privileges: list[Privilege] = Field(default_factory=list)


class ChangeOutcome(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    principal: str
    applied: list[Privilege] = Field(default_factory=list)
    unchanged: list[Privilege] = Field(default_factory=list)


class GrantChangeResult(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    securable: SecurableRef
    outcomes: list[ChangeOutcome] = Field(default_factory=list)
    grants: list[PrincipalPrivileges] = Field(default_factory=list)
    changed: bool = False
    audit_id: str
