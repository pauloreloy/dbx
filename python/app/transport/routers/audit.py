from typing import Any, Optional
from fastapi import APIRouter, Query, Request
from fastapi.responses import JSONResponse

from app.domain.audit import AuditAction, AuditEvent, AuditOutcome
from app.ports.stores import AuditFilter
from app.services.audit_service import AuditService

router = APIRouter(prefix="/v1/audit/events", tags=["Audit"])


def _get_audit_service(request: Request) -> AuditService:
    return request.app.state.audit_service


@router.get("", response_model=dict[str, Any])
async def list_audit_events(
    request: Request,
    workspace: Optional[str] = Query(default=None),
    actor: Optional[str] = Query(default=None),
    change_ref: Optional[str] = Query(default=None),
    action: Optional[AuditAction] = Query(default=None),
    outcome: Optional[AuditOutcome] = Query(default=None),
    page_size: int = Query(default=0, ge=0),
    page_token: str = Query(default=""),
) -> JSONResponse:
    """Consulta os registros da trilha global imutável de auditoria."""
    service = _get_audit_service(request)
    filter_ = AuditFilter(
        workspace=workspace,
        actor=actor,
        change_ref=change_ref,
        action=action,
        outcome=outcome,
    )
    events, next_token = service.list_events(filter_=filter_, page_size=page_size, page_token=page_token)
    res: dict[str, Any] = {
        "events": [e.model_dump(mode="json", exclude_none=True) for e in events]
    }
    if next_token:
        res["next_page_token"] = next_token
    return JSONResponse(content=res)


@router.get("/{event_id}", response_model=AuditEvent)
async def get_audit_event(event_id: str, request: Request) -> JSONResponse:
    """Obtém os detalhes de um evento de auditoria pelo ID."""
    service = _get_audit_service(request)
    event = service.get_event(event_id)
    return JSONResponse(content=event.model_dump(mode="json", exclude_none=True))
