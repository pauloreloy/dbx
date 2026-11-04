from fastapi import APIRouter
from fastapi.responses import JSONResponse

router = APIRouter(tags=["Health"])


@router.get("/healthz")
async def healthz() -> JSONResponse:
    """Probe operacional de liveness."""
    return JSONResponse(content={"status": "UP"})


@router.get("/readyz")
async def readyz() -> JSONResponse:
    """Probe operacional de readiness."""
    return JSONResponse(content={"status": "READY"})
