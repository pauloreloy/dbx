from datetime import datetime, timedelta, timezone
import threading
from typing import Optional

from app.domain.audit import AuditEvent
from app.domain.errors import NotFoundException
from app.ports.stores import AuditFilter, AuditStore, IdempotencyRecord, IdempotencyStore


class MemoryAuditStore(AuditStore):
    """Armazena eventos de auditoria em memória de forma thread-safe."""

    def __init__(self) -> None:
        self._lock = threading.RLock()
        self._events: list[AuditEvent] = []
        self._by_id: dict[str, AuditEvent] = {}

    def record(self, event: AuditEvent) -> None:
        with self._lock:
            # Mais recente primeiro
            self._events.insert(0, event)
            self._by_id[event.id] = event

    def list_events(
        self, filter_: Optional[AuditFilter] = None, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[AuditEvent], str]:
        with self._lock:
            result: list[AuditEvent] = []
            for e in self._events:
                if filter_:
                    if filter_.workspace and e.workspace != filter_.workspace:
                        continue
                    if filter_.actor and e.actor != filter_.actor:
                        continue
                    if filter_.action and e.action != filter_.action:
                        continue
                    if filter_.change_ref and e.change_ref != filter_.change_ref:
                        continue
                    if filter_.outcome and e.outcome != filter_.outcome:
                        continue
                    if filter_.since and e.occurred_at < filter_.since:
                        continue
                    if filter_.until and e.occurred_at > filter_.until:
                        continue
                result.append(e)

            if page_size > 0 and len(result) > page_size:
                result = result[:page_size]

            return result, ""

    def get_event(self, id_: str) -> AuditEvent:
        with self._lock:
            event = self._by_id.get(id_)
            if not event:
                raise NotFoundException("AuditEvent", id_)
            return event


class MemoryIdempotencyStore(IdempotencyStore):
    """Armazena chaves de idempotência com TTL em memória de forma thread-safe."""

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._records: dict[str, Optional[IdempotencyRecord]] = {}
        self._expires: dict[str, datetime] = {}

    def _composite_key(self, scope: str, key: str) -> str:
        return f"{scope}:{key}"

    def acquire(
        self, scope: str, key: str, ttl: timedelta = timedelta(hours=24)
    ) -> tuple[bool, Optional[IdempotencyRecord]]:
        with self._lock:
            ck = self._composite_key(scope, key)
            now = datetime.now(timezone.utc)
            exp = self._expires.get(ck)

            if exp and now < exp:
                cached = self._records.get(ck)
                if cached is not None:
                    return False, cached
                # Em processamento por outra requisição concorrente
                return False, None

            # Nova aquisição
            self._expires[ck] = now + ttl
            self._records[ck] = None  # Marcado como em processamento
            return True, None

    def commit(self, scope: str, key: str, status_code: int, response_body: bytes) -> None:
        with self._lock:
            ck = self._composite_key(scope, key)
            self._records[ck] = IdempotencyRecord(
                status_code=status_code,
                body=response_body,
                created_at=datetime.now(timezone.utc),
            )

    def release(self, scope: str, key: str) -> None:
        with self._lock:
            ck = self._composite_key(scope, key)
            self._records.pop(ck, None)
            self._expires.pop(ck, None)
