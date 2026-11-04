from datetime import datetime
from enum import StrEnum
from typing import Any, Optional
from pydantic import BaseModel, ConfigDict, Field

from app.domain.models import SecurableRef


class AuditAction(StrEnum):
    GRANT = "GRANT"
    REVOKE = "REVOKE"
    CATALOG_CREATE = "CATALOG_CREATE"
    CATALOG_UPDATE = "CATALOG_UPDATE"
    CATALOG_DELETE = "CATALOG_DELETE"
    SCHEMA_CREATE = "SCHEMA_CREATE"
    SCHEMA_UPDATE = "SCHEMA_UPDATE"
    SCHEMA_DELETE = "SCHEMA_DELETE"
    TABLE_DELETE = "TABLE_DELETE"
    VIEW_DELETE = "VIEW_DELETE"
    VOLUME_CREATE = "VOLUME_CREATE"
    VOLUME_UPDATE = "VOLUME_UPDATE"
    VOLUME_DELETE = "VOLUME_DELETE"
    OWNER_CHANGE = "OWNER_CHANGE"
    TAG_SET = "TAG_SET"
    TAG_DELETE = "TAG_DELETE"


class AuditOutcome(StrEnum):
    SUCCESS = "SUCCESS"
    DENIED = "DENIED"
    FAILED = "FAILED"


class AuditError(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    type: str
    detail: str


class AuditEvent(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    id: str
    occurred_at: datetime
    workspace: str
    metastore_id: Optional[str] = None
    actor: str
    action: AuditAction
    outcome: AuditOutcome
    change_ref: str
    request_id: str
    securable: Optional[SecurableRef] = None
    targets: Optional[list[str]] = None
    before: Optional[dict[str, Any]] = None
    after: Optional[dict[str, Any]] = None
    error: Optional[AuditError] = None
