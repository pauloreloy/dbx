from typing import Optional

from app.domain.audit import AuditEvent
from app.ports.stores import AuditFilter, AuditStore


class AuditService:
    """Gerencia a consulta da trilha global de auditoria (D10)."""

    def __init__(self, audit_store: Optional[AuditStore] = None) -> None:
        self._audit_store = audit_store

    def list_events(
        self,
        filter_: Optional[AuditFilter] = None,
        page_size: int = 0,
        page_token: str = "",
    ) -> tuple[list[AuditEvent], str]:
        if not self._audit_store:
            return [], ""
        return self._audit_store.list_events(filter_, page_size, page_token)

    def get_event(self, id_: str) -> AuditEvent:
        if not self._audit_store:
            from app.domain.errors import NotFoundException

            raise NotFoundException("AuditEvent", id_)
        return self._audit_store.get_event(id_)
