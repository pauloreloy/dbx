from typing import Any, Optional
from fastapi import APIRouter, Header, Query, Request
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field

from app.domain.errors import BadRequestException
from app.domain.models import (
    GrantChangeResult,
    PrivilegeChange,
    SecurableRef,
    SecurableType,
)
from app.services.grant_service import GrantService
from app.transport.middlewares import get_actor, get_request_id

router = APIRouter(prefix="/v1/workspaces/{ws}/catalogs", tags=["Grants"])


def _get_grant_service(request: Request) -> GrantService:
    return request.app.state.grant_service


class GrantChangeBody(BaseModel):
    changes: list[PrivilegeChange] = Field(default_factory=list)


def _handle_grants(
    request: Request, ws: str, sec: SecurableRef, principal: Optional[str]
) -> JSONResponse:
    service = _get_grant_service(request)
    grants = service.get_grants(ws=ws, sec=sec, principal_filter=principal)
    return JSONResponse(
        content={
            "securable": sec.model_dump(mode="json", exclude_none=True),
            "grants": [g.model_dump(mode="json", exclude_none=True) for g in grants],
        }
    )


def _handle_effective_grants(
    request: Request, ws: str, sec: SecurableRef, principal: Optional[str]
) -> JSONResponse:
    service = _get_grant_service(request)
    eff_grants = service.get_effective_grants(ws=ws, sec=sec, principal_filter=principal)
    return JSONResponse(
        content={
            "securable": sec.model_dump(mode="json", exclude_none=True),
            "grants": [g.model_dump(mode="json", exclude_none=True) for g in eff_grants],
        }
    )


def _handle_grant_mutation(
    request: Request,
    ws: str,
    sec: SecurableRef,
    req: GrantChangeBody,
    is_revoke: bool,
    x_change_ref: Optional[str],
) -> JSONResponse:
    if not req.changes:
        raise BadRequestException("O array 'changes' deve conter ao menos 1 item")

    service = _get_grant_service(request)
    result = service.apply_changes(
        ws=ws,
        sec=sec,
        changes=req.changes,
        is_revoke=is_revoke,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )
    return JSONResponse(content=result.model_dump(mode="json", exclude_none=True))


# ============================================================================
# Catalog Grants
# ============================================================================

@router.get("/{catalog}/grants")
async def get_catalog_grants(
    ws: str,
    catalog: str,
    request: Request,
    principal: Optional[str] = Query(default=None),
) -> JSONResponse:
    sec = SecurableRef(type=SecurableType.CATALOG, name=catalog)
    return _handle_grants(request, ws, sec, principal)


@router.get("/{catalog}/effective-grants")
async def get_catalog_effective_grants(
    ws: str,
    catalog: str,
    request: Request,
    principal: Optional[str] = Query(default=None),
) -> JSONResponse:
    sec = SecurableRef(type=SecurableType.CATALOG, name=catalog)
    return _handle_effective_grants(request, ws, sec, principal)


@router.post("/{catalog}/grants/grant")
async def grant_catalog(
    ws: str,
    catalog: str,
    req: GrantChangeBody,
    request: Request,
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> JSONResponse:
    sec = SecurableRef(type=SecurableType.CATALOG, name=catalog)
    return _handle_grant_mutation(request, ws, sec, req, False, x_change_ref)


@router.post("/{catalog}/grants/revoke")
async def revoke_catalog(
    ws: str,
    catalog: str,
    req: GrantChangeBody,
    request: Request,
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> JSONResponse:
    sec = SecurableRef(type=SecurableType.CATALOG, name=catalog)
    return _handle_grant_mutation(request, ws, sec, req, True, x_change_ref)


# ============================================================================
# Schema Grants
# ============================================================================

@router.get("/{catalog}/schemas/{schema}/grants")
async def get_schema_grants(
    ws: str,
    catalog: str,
    schema: str,
    request: Request,
    principal: Optional[str] = Query(default=None),
) -> JSONResponse:
    sec = SecurableRef(type=SecurableType.SCHEMA, name=f"{catalog}.{schema}")
    return _handle_grants(request, ws, sec, principal)


@router.get("/{catalog}/schemas/{schema}/effective-grants")
async def get_schema_effective_grants(
    ws: str,
    catalog: str,
    schema: str,
    request: Request,
    principal: Optional[str] = Query(default=None),
) -> JSONResponse:
    sec = SecurableRef(type=SecurableType.SCHEMA, name=f"{catalog}.{schema}")
    return _handle_effective_grants(request, ws, sec, principal)


@router.post("/{catalog}/schemas/{schema}/grants/grant")
async def grant_schema(
    ws: str,
    catalog: str,
    schema: str,
    req: GrantChangeBody,
    request: Request,
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> JSONResponse:
    sec = SecurableRef(type=SecurableType.SCHEMA, name=f"{catalog}.{schema}")
    return _handle_grant_mutation(request, ws, sec, req, False, x_change_ref)


@router.post("/{catalog}/schemas/{schema}/grants/revoke")
async def revoke_schema(
    ws: str,
    catalog: str,
    schema: str,
    req: GrantChangeBody,
    request: Request,
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> JSONResponse:
    sec = SecurableRef(type=SecurableType.SCHEMA, name=f"{catalog}.{schema}")
    return _handle_grant_mutation(request, ws, sec, req, True, x_change_ref)


# ============================================================================
# Table Grants
# ============================================================================

@router.get("/{catalog}/schemas/{schema}/tables/{table}/grants")
async def get_table_grants(
    ws: str,
    catalog: str,
    schema: str,
    table: str,
    request: Request,
    principal: Optional[str] = Query(default=None),
) -> JSONResponse:
    sec = SecurableRef(type=SecurableType.TABLE, name=f"{catalog}.{schema}.{table}")
    return _handle_grants(request, ws, sec, principal)


@router.get("/{catalog}/schemas/{schema}/tables/{table}/effective-grants")
async def get_table_effective_grants(
    ws: str,
    catalog: str,
    schema: str,
    table: str,
    request: Request,
    principal: Optional[str] = Query(default=None),
) -> JSONResponse:
    sec = SecurableRef(type=SecurableType.TABLE, name=f"{catalog}.{schema}.{table}")
    return _handle_effective_grants(request, ws, sec, principal)


@router.post("/{catalog}/schemas/{schema}/tables/{table}/grants/grant")
async def grant_table(
    ws: str,
    catalog: str,
    schema: str,
    table: str,
    req: GrantChangeBody,
    request: Request,
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> JSONResponse:
    sec = SecurableRef(type=SecurableType.TABLE, name=f"{catalog}.{schema}.{table}")
    return _handle_grant_mutation(request, ws, sec, req, False, x_change_ref)


@router.post("/{catalog}/schemas/{schema}/tables/{table}/grants/revoke")
async def revoke_table(
    ws: str,
    catalog: str,
    schema: str,
    table: str,
    req: GrantChangeBody,
    request: Request,
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> JSONResponse:
    sec = SecurableRef(type=SecurableType.TABLE, name=f"{catalog}.{schema}.{table}")
    return _handle_grant_mutation(request, ws, sec, req, True, x_change_ref)
