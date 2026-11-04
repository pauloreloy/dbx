from datetime import timedelta
import json
from typing import Any, Optional
from fastapi import APIRouter, Header, Query, Request, Response
from fastapi.responses import JSONResponse
from pydantic import BaseModel

from app.domain.errors import (
    BadRequestException,
    IdempotencyConflictException,
)
from app.domain.models import (
    Catalog,
    DataSchema,
    SecurableRef,
    SecurableType,
    Table,
    View,
    Volume,
)
from app.services.catalog_service import CatalogService
from app.transport.middlewares import get_actor, get_request_id

router = APIRouter(prefix="/v1/workspaces/{ws}/catalogs", tags=["Catalogs"])


def _get_catalog_service(request: Request) -> CatalogService:
    return request.app.state.catalog_service


# Request Models
class CatalogCreateRequest(BaseModel):
    name: str
    comment: Optional[str] = None
    storage_root: Optional[str] = None
    properties: Optional[dict[str, str]] = None


class CatalogUpdateRequest(BaseModel):
    new_name: Optional[str] = None
    comment: Optional[str] = None
    properties: Optional[dict[str, str]] = None


class SchemaCreateRequest(BaseModel):
    name: str
    comment: Optional[str] = None
    storage_root: Optional[str] = None
    properties: Optional[dict[str, str]] = None


class SchemaUpdateRequest(BaseModel):
    new_name: Optional[str] = None
    comment: Optional[str] = None
    properties: Optional[dict[str, str]] = None


class VolumeCreateRequest(BaseModel):
    name: str
    volume_type: str = "MANAGED"
    storage_location: Optional[str] = None
    comment: Optional[str] = None


class VolumeUpdateRequest(BaseModel):
    new_name: Optional[str] = None
    comment: Optional[str] = None


class OwnerUpdateRequest(BaseModel):
    owner: str


class TagSetRequest(BaseModel):
    key: str
    value: str


# --- Catalogs Endpoints ---

@router.get("", response_model=dict[str, Any])
async def list_catalogs(
    ws: str,
    request: Request,
    page_size: int = Query(default=0, ge=0),
    page_token: str = Query(default=""),
) -> JSONResponse:
    """Lista catálogos disponíveis no workspace."""
    service = _get_catalog_service(request)
    catalogs, next_token = service.list_catalogs(ws=ws, page_size=page_size, page_token=page_token)
    res: dict[str, Any] = {
        "catalogs": [c.model_dump(mode="json", exclude_none=True) for c in catalogs]
    }
    if next_token:
        res["next_page_token"] = next_token
    return JSONResponse(content=res)


@router.post("", status_code=201)
async def create_catalog(
    ws: str,
    req: CatalogCreateRequest,
    request: Request,
    idempotency_key: Optional[str] = Header(None, alias="Idempotency-Key"),
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> Response:
    """Cria um novo catálogo no Unity Catalog com suporte a idempotência."""
    if not idempotency_key:
        raise BadRequestException("O cabeçalho 'Idempotency-Key' é obrigatório para operações DDL")

    idemp_store = getattr(request.app.state, "idempotency_store", None)
    if idemp_store:
        acquired, cached = idemp_store.acquire(ws, idempotency_key, timedelta(hours=24))
        if not acquired and cached:
            return Response(
                content=cached.body,
                status_code=cached.status_code,
                media_type="application/json; charset=utf-8",
            )
        if not acquired and not cached:
            raise IdempotencyConflictException(idempotency_key)

    service = _get_catalog_service(request)
    cat = Catalog(
        name=req.name,
        comment=req.comment,
        storage_root=req.storage_root,
        properties=req.properties,
    )

    created = service.create_catalog(
        ws=ws,
        cat=cat,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )

    location = f"/v1/workspaces/{ws}/catalogs/{created.name}"
    resp_data = created.model_dump(mode="json", exclude_none=True)
    resp_bytes = json.dumps(resp_data).encode("utf-8")

    if idemp_store:
        idemp_store.commit(ws, idempotency_key, 201, resp_bytes)

    return Response(
        content=resp_bytes,
        status_code=201,
        media_type="application/json; charset=utf-8",
        headers={"Location": location},
    )


@router.get("/{catalog}")
async def get_catalog(ws: str, catalog: str, request: Request) -> JSONResponse:
    """Obtém detalhes de um catálogo específico."""
    service = _get_catalog_service(request)
    cat = service.get_catalog(ws=ws, name=catalog)
    return JSONResponse(content=cat.model_dump(mode="json", exclude_none=True))


@router.patch("/{catalog}")
async def update_catalog(
    ws: str,
    catalog: str,
    req: CatalogUpdateRequest,
    request: Request,
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> JSONResponse:
    """Atualiza propriedades de um catálogo."""
    service = _get_catalog_service(request)
    updated = service.update_catalog(
        ws=ws,
        name=catalog,
        new_name=req.new_name,
        comment=req.comment,
        props=req.properties,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )
    return JSONResponse(content=updated.model_dump(mode="json", exclude_none=True))


@router.delete("/{catalog}", status_code=204)
async def delete_catalog(
    ws: str,
    catalog: str,
    request: Request,
    confirm: str = Query(default=""),
    force: bool = Query(default=False),
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> Response:
    """Remove um catálogo exigindo confirmação explícita."""
    service = _get_catalog_service(request)
    service.delete_catalog(
        ws=ws,
        name=catalog,
        confirm=confirm,
        force=force,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )
    return Response(status_code=204)


# --- Catalog Ownership ---

@router.get("/{catalog}/owner")
async def get_catalog_owner(ws: str, catalog: str, request: Request) -> JSONResponse:
    service = _get_catalog_service(request)
    owner = service.get_owner(ws, SecurableRef(type=SecurableType.CATALOG, name=catalog))
    return JSONResponse(content={"owner": owner})


@router.put("/{catalog}/owner")
async def set_catalog_owner(
    ws: str,
    catalog: str,
    req: OwnerUpdateRequest,
    request: Request,
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> JSONResponse:
    service = _get_catalog_service(request)
    service.set_owner(
        ws=ws,
        sec=SecurableRef(type=SecurableType.CATALOG, name=catalog),
        new_owner=req.owner,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )
    return JSONResponse(content={"owner": req.owner})


# --- Catalog Tags ---

@router.get("/{catalog}/tags")
async def list_catalog_tags(ws: str, catalog: str, request: Request) -> JSONResponse:
    service = _get_catalog_service(request)
    tags = service.list_tags(ws, SecurableRef(type=SecurableType.CATALOG, name=catalog))
    return JSONResponse(content={"tags": tags})


@router.put("/{catalog}/tags")
async def set_catalog_tag(
    ws: str,
    catalog: str,
    req: TagSetRequest,
    request: Request,
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> JSONResponse:
    service = _get_catalog_service(request)
    service.set_tag(
        ws=ws,
        sec=SecurableRef(type=SecurableType.CATALOG, name=catalog),
        key=req.key,
        value=req.value,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )
    return JSONResponse(content={"key": req.key, "value": req.value})


@router.delete("/{catalog}/tags/{tag_key}", status_code=204)
async def delete_catalog_tag(
    ws: str,
    catalog: str,
    tag_key: str,
    request: Request,
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> Response:
    service = _get_catalog_service(request)
    service.delete_tag(
        ws=ws,
        sec=SecurableRef(type=SecurableType.CATALOG, name=catalog),
        key=tag_key,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )
    return Response(status_code=204)


# --- Schemas Endpoints ---

@router.get("/{catalog}/schemas", response_model=dict[str, Any])
async def list_schemas(
    ws: str,
    catalog: str,
    request: Request,
    page_size: int = Query(default=0, ge=0),
    page_token: str = Query(default=""),
) -> JSONResponse:
    service = _get_catalog_service(request)
    schemas, next_token = service.list_schemas(ws=ws, catalog=catalog, page_size=page_size, page_token=page_token)
    res: dict[str, Any] = {
        "schemas": [s.model_dump(mode="json", exclude_none=True) for s in schemas]
    }
    if next_token:
        res["next_page_token"] = next_token
    return JSONResponse(content=res)


@router.post("/{catalog}/schemas", status_code=201)
async def create_schema(
    ws: str,
    catalog: str,
    req: SchemaCreateRequest,
    request: Request,
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> JSONResponse:
    service = _get_catalog_service(request)
    sch = DataSchema(
        name=req.name,
        catalog_name=catalog,
        comment=req.comment,
        storage_root=req.storage_root,
        properties=req.properties,
    )
    created = service.create_schema(
        ws=ws,
        catalog=catalog,
        sch=sch,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )
    return JSONResponse(
        status_code=201,
        content=created.model_dump(mode="json", exclude_none=True),
        headers={"Location": f"/v1/workspaces/{ws}/catalogs/{catalog}/schemas/{created.name}"},
    )


@router.get("/{catalog}/schemas/{schema}")
async def get_schema(ws: str, catalog: str, schema: str, request: Request) -> JSONResponse:
    service = _get_catalog_service(request)
    sch = service.get_schema(ws=ws, catalog=catalog, schema=schema)
    return JSONResponse(content=sch.model_dump(mode="json", exclude_none=True))


@router.patch("/{catalog}/schemas/{schema}")
async def update_schema(
    ws: str,
    catalog: str,
    schema: str,
    req: SchemaUpdateRequest,
    request: Request,
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> JSONResponse:
    service = _get_catalog_service(request)
    updated = service.update_schema(
        ws=ws,
        catalog=catalog,
        schema=schema,
        new_name=req.new_name,
        comment=req.comment,
        props=req.properties,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )
    return JSONResponse(content=updated.model_dump(mode="json", exclude_none=True))


@router.delete("/{catalog}/schemas/{schema}", status_code=204)
async def delete_schema(
    ws: str,
    catalog: str,
    schema: str,
    request: Request,
    confirm: str = Query(default=""),
    force: bool = Query(default=False),
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> Response:
    service = _get_catalog_service(request)
    service.delete_schema(
        ws=ws,
        catalog=catalog,
        schema=schema,
        confirm=confirm,
        force=force,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )
    return Response(status_code=204)


# --- Tables Endpoints ---

@router.get("/{catalog}/schemas/{schema}/tables", response_model=dict[str, Any])
async def list_tables(
    ws: str,
    catalog: str,
    schema: str,
    request: Request,
    page_size: int = Query(default=0, ge=0),
    page_token: str = Query(default=""),
) -> JSONResponse:
    service = _get_catalog_service(request)
    tables, next_token = service.list_tables(
        ws=ws, catalog=catalog, schema=schema, page_size=page_size, page_token=page_token
    )
    res: dict[str, Any] = {
        "tables": [t.model_dump(mode="json", exclude_none=True) for t in tables]
    }
    if next_token:
        res["next_page_token"] = next_token
    return JSONResponse(content=res)


@router.get("/{catalog}/schemas/{schema}/tables/{table}")
async def get_table(ws: str, catalog: str, schema: str, table: str, request: Request) -> JSONResponse:
    service = _get_catalog_service(request)
    t = service.get_table(ws=ws, catalog=catalog, schema=schema, table=table)
    return JSONResponse(content=t.model_dump(mode="json", exclude_none=True))


@router.delete("/{catalog}/schemas/{schema}/tables/{table}", status_code=204)
async def delete_table(
    ws: str,
    catalog: str,
    schema: str,
    table: str,
    request: Request,
    confirm: str = Query(default=""),
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> Response:
    service = _get_catalog_service(request)
    service.delete_table(
        ws=ws,
        catalog=catalog,
        schema=schema,
        table=table,
        confirm=confirm,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )
    return Response(status_code=204)


# --- Views Endpoints ---

@router.get("/{catalog}/schemas/{schema}/views", response_model=dict[str, Any])
async def list_views(
    ws: str,
    catalog: str,
    schema: str,
    request: Request,
    page_size: int = Query(default=0, ge=0),
    page_token: str = Query(default=""),
) -> JSONResponse:
    service = _get_catalog_service(request)
    views, next_token = service.list_views(
        ws=ws, catalog=catalog, schema=schema, page_size=page_size, page_token=page_token
    )
    res: dict[str, Any] = {
        "views": [v.model_dump(mode="json", exclude_none=True) for v in views]
    }
    if next_token:
        res["next_page_token"] = next_token
    return JSONResponse(content=res)


@router.get("/{catalog}/schemas/{schema}/views/{view}")
async def get_view(ws: str, catalog: str, schema: str, view: str, request: Request) -> JSONResponse:
    service = _get_catalog_service(request)
    v = service.get_view(ws=ws, catalog=catalog, schema=schema, view=view)
    return JSONResponse(content=v.model_dump(mode="json", exclude_none=True))


@router.delete("/{catalog}/schemas/{schema}/views/{view}", status_code=204)
async def delete_view(
    ws: str,
    catalog: str,
    schema: str,
    view: str,
    request: Request,
    confirm: str = Query(default=""),
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> Response:
    service = _get_catalog_service(request)
    service.delete_view(
        ws=ws,
        catalog=catalog,
        schema=schema,
        view=view,
        confirm=confirm,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )
    return Response(status_code=204)


# --- Volumes Endpoints ---

@router.get("/{catalog}/schemas/{schema}/volumes", response_model=dict[str, Any])
async def list_volumes(
    ws: str,
    catalog: str,
    schema: str,
    request: Request,
    page_size: int = Query(default=0, ge=0),
    page_token: str = Query(default=""),
) -> JSONResponse:
    service = _get_catalog_service(request)
    volumes, next_token = service.list_volumes(
        ws=ws, catalog=catalog, schema=schema, page_size=page_size, page_token=page_token
    )
    res: dict[str, Any] = {
        "volumes": [vol.model_dump(mode="json", exclude_none=True) for vol in volumes]
    }
    if next_token:
        res["next_page_token"] = next_token
    return JSONResponse(content=res)


@router.post("/{catalog}/schemas/{schema}/volumes", status_code=201)
async def create_volume(
    ws: str,
    catalog: str,
    schema: str,
    req: VolumeCreateRequest,
    request: Request,
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> JSONResponse:
    service = _get_catalog_service(request)
    vol = Volume(
        name=req.name,
        catalog_name=catalog,
        schema_name=schema,
        full_name=f"{catalog}.{schema}.{req.name}",
        volume_type=req.volume_type,
        storage_location=req.storage_location,
        comment=req.comment,
    )
    created = service.create_volume(
        ws=ws,
        catalog=catalog,
        schema=schema,
        vol=vol,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )
    return JSONResponse(
        status_code=201,
        content=created.model_dump(mode="json", exclude_none=True),
        headers={"Location": f"/v1/workspaces/{ws}/catalogs/{catalog}/schemas/{schema}/volumes/{created.name}"},
    )


@router.get("/{catalog}/schemas/{schema}/volumes/{volume}")
async def get_volume(
    ws: str, catalog: str, schema: str, volume: str, request: Request
) -> JSONResponse:
    service = _get_catalog_service(request)
    vol = service.get_volume(ws=ws, catalog=catalog, schema=schema, volume=volume)
    return JSONResponse(content=vol.model_dump(mode="json", exclude_none=True))


@router.patch("/{catalog}/schemas/{schema}/volumes/{volume}")
async def update_volume(
    ws: str,
    catalog: str,
    schema: str,
    volume: str,
    req: VolumeUpdateRequest,
    request: Request,
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> JSONResponse:
    service = _get_catalog_service(request)
    updated = service.update_volume(
        ws=ws,
        catalog=catalog,
        schema=schema,
        volume=volume,
        new_name=req.new_name,
        comment=req.comment,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )
    return JSONResponse(content=updated.model_dump(mode="json", exclude_none=True))


@router.delete("/{catalog}/schemas/{schema}/volumes/{volume}", status_code=204)
async def delete_volume(
    ws: str,
    catalog: str,
    schema: str,
    volume: str,
    request: Request,
    confirm: str = Query(default=""),
    x_change_ref: Optional[str] = Header(None, alias="X-Change-Ref"),
) -> Response:
    service = _get_catalog_service(request)
    service.delete_volume(
        ws=ws,
        catalog=catalog,
        schema=schema,
        volume=volume,
        confirm=confirm,
        actor=get_actor(request),
        change_ref=x_change_ref or "",
        request_id=get_request_id(request),
    )
    return Response(status_code=204)
