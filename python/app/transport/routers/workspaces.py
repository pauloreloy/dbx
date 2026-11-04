from typing import Any, Optional
from fastapi import APIRouter, Query, Request
from fastapi.responses import JSONResponse

from app.domain.models import Workspace
from app.services.workspace_service import WorkspaceService

router = APIRouter(prefix="/v1/workspaces", tags=["Workspaces"])


def _get_workspace_service(request: Request) -> WorkspaceService:
    return request.app.state.workspace_service


@router.get("", response_model=dict[str, Any])
async def list_workspaces(
    request: Request,
    page_size: int = Query(default=0, ge=0),
    page_token: str = Query(default=""),
) -> JSONResponse:
    """Lista todos os workspaces Databricks federados registrados."""
    service = _get_workspace_service(request)
    workspaces, next_token = service.list_workspaces(page_size=page_size, page_token=page_token)
    res: dict[str, Any] = {
        "workspaces": [w.model_dump(mode="json", exclude_none=True) for w in workspaces]
    }
    if next_token:
        res["next_page_token"] = next_token
    return JSONResponse(content=res)


@router.get("/{ws}", response_model=Workspace)
async def get_workspace(ws: str, request: Request) -> JSONResponse:
    """Obtém os detalhes de um workspace registrado pelo alias."""
    service = _get_workspace_service(request)
    workspace = service.get_workspace(ws)
    return JSONResponse(content=workspace.model_dump(mode="json", exclude_none=True))
