from abc import ABC, abstractmethod
from datetime import datetime, timedelta
from typing import Optional
from pydantic import BaseModel, ConfigDict

from app.domain.audit import AuditAction, AuditEvent, AuditOutcome
from app.domain.models import Workspace


class AuditFilter(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    workspace: Optional[str] = None
    actor: Optional[str] = None
    securable: Optional[str] = None
    target: Optional[str] = None
    change_ref: Optional[str] = None
    action: Optional[AuditAction] = None
    outcome: Optional[AuditOutcome] = None
    since: Optional[datetime] = None
    until: Optional[datetime] = None


class AuditStore(ABC):
    @abstractmethod
    def record(self, event: AuditEvent) -> None:
        pass

    @abstractmethod
    def list_events(
        self, filter_: Optional[AuditFilter] = None, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[AuditEvent], str]:
        pass

    @abstractmethod
    def get_event(self, id_: str) -> AuditEvent:
        pass


class IdempotencyRecord(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    status_code: int
    body: bytes
    created_at: datetime


class IdempotencyStore(ABC):
    @abstractmethod
    def acquire(
        self, scope: str, key: str, ttl: timedelta = timedelta(hours=24)
    ) -> tuple[bool, Optional[IdempotencyRecord]]:
        """
        Tenta adquirir a chave de idempotência.
        Retorna (True, None) se adquirida (primeira execução).
        Retorna (False, cached_record) se já executada e resposta em cache.
        Retorna (False, None) se em processamento por outra requisição concorrente.
        """
        pass

    @abstractmethod
    def commit(self, scope: str, key: str, status_code: int, response_body: bytes) -> None:
        pass

    @abstractmethod
    def release(self, scope: str, key: str) -> None:
        pass


class WorkspaceRegistry(ABC):
    @abstractmethod
    def list_workspaces(
        self, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[Workspace], str]:
        pass

    @abstractmethod
    def get_workspace(self, alias: str) -> Workspace:
        pass
