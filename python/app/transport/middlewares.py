import uuid
from typing import Any, Callable
from fastapi import FastAPI, HTTPException, Request, Response
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware

from app.domain.errors import ProblemException


EXEMPT_PATHS = {"/healthz", "/readyz", "/docs", "/openapi.json", "/redoc"}


class RequestIDMiddleware(BaseHTTPMiddleware):
    """Assegura que toda requisição e resposta tenham um X-Request-Id."""

    async def dispatch(self, request: Request, call_next: Callable[[Request], Any]) -> Response:
        req_id = request.headers.get("x-request-id")
        if not req_id:
            req_id = f"req_{uuid.uuid4().hex}"

        request.state.request_id = req_id
        response: Response = await call_next(request)
        response.headers["X-Request-Id"] = req_id
        return response


class ChangeRefMiddleware(BaseHTTPMiddleware):
    """Exige o header X-Change-Ref em métodos mutativos de escrita (POST, PUT, PATCH, DELETE)."""

    async def dispatch(self, request: Request, call_next: Callable[[Request], Any]) -> Response:
        if request.url.path in EXEMPT_PATHS:
            return await call_next(request)

        if request.method in ("POST", "PUT", "PATCH", "DELETE"):
            change_ref = request.headers.get("x-change-ref", "").strip()
            if not change_ref:
                req_id = getattr(request.state, "request_id", None) or request.headers.get(
                    "x-request-id", f"req_{uuid.uuid4().hex}"
                )
                problem_payload = {
                    "type": "/problems/bad-request",
                    "title": "Requisição inválida",
                    "status": 400,
                    "detail": "O cabeçalho 'X-Change-Ref' é obrigatório para todas as operações de escrita (auditoria/ticket)",
                    "instance": request.url.path,
                    "request_id": req_id,
                    "errors": [
                        {
                            "field": "X-Change-Ref",
                            "message": "Cabeçalho ausente ou vazio",
                            "code": "missing-change-ref",
                        }
                    ],
                }
                response = JSONResponse(
                    status_code=400,
                    content=problem_payload,
                    media_type="application/problem+json; charset=utf-8",
                )
                response.headers["X-Request-Id"] = req_id
                return response

        return await call_next(request)


class ActorMiddleware(BaseHTTPMiddleware):
    """Extrai a identidade do chamador a partir de JWT/mTLS ou headers corporativos."""

    async def dispatch(self, request: Request, call_next: Callable[[Request], Any]) -> Response:
        actor = "anonymous"
        auth_header = request.headers.get("authorization", "")
        if auth_header.startswith("Bearer "):
            actor = "authenticated-service"
        elif consumer := request.headers.get("x-consumer-username"):
            actor = consumer.strip()

        request.state.actor = actor
        return await call_next(request)


def get_request_id(request: Request) -> str:
    """Extrai o request_id do state da requisição ou gera um fallback."""
    return getattr(request.state, "request_id", None) or request.headers.get("x-request-id", "unknown")


def get_actor(request: Request) -> str:
    """Extrai o actor do state da requisição."""
    return getattr(request.state, "actor", "anonymous")


async def problem_exception_handler(request: Request, exc: ProblemException) -> JSONResponse:
    """Serializa exceções ProblemException conforme a RFC 9457."""
    req_id = get_request_id(request)
    if not exc.instance:
        exc.instance = request.url.path
    if not exc.request_id:
        exc.request_id = req_id

    content = exc.to_dict()
    response = JSONResponse(
        status_code=exc.status,
        content=content,
        media_type="application/problem+json; charset=utf-8",
    )
    response.headers["X-Request-Id"] = req_id
    return response


async def validation_exception_handler(
    request: Request, exc: RequestValidationError
) -> JSONResponse:
    """Converte erros de validação do Pydantic/FastAPI no formato RFC 9457."""
    req_id = get_request_id(request)
    errors = []
    for err in exc.errors():
        field = ".".join(str(loc) for loc in err["loc"] if loc not in ("body", "query", "path"))
        errors.append(
            {
                "field": field or "body",
                "message": err["msg"],
                "code": err.get("type", "validation-error"),
            }
        )

    content = {
        "type": "/problems/bad-request",
        "title": "Requisição inválida",
        "status": 400,
        "detail": "Erro de validação nos parâmetros da requisição",
        "instance": request.url.path,
        "request_id": req_id,
        "errors": errors,
    }
    response = JSONResponse(
        status_code=400,
        content=content,
        media_type="application/problem+json; charset=utf-8",
    )
    response.headers["X-Request-Id"] = req_id
    return response


async def http_exception_handler(request: Request, exc: HTTPException) -> JSONResponse:
    """Converte HTTPException padrão no formato RFC 9457."""
    req_id = get_request_id(request)
    content = {
        "type": "/problems/http-error",
        "title": exc.detail if isinstance(exc.detail, str) else "Erro HTTP",
        "status": exc.status_code,
        "detail": str(exc.detail),
        "instance": request.url.path,
        "request_id": req_id,
    }
    response = JSONResponse(
        status_code=exc.status_code,
        content=content,
        media_type="application/problem+json; charset=utf-8",
    )
    response.headers["X-Request-Id"] = req_id
    return response


async def unhandled_exception_handler(request: Request, exc: Exception) -> JSONResponse:
    """Captura exceções inesperadas e retorna Problem 500 RFC 9457."""
    req_id = get_request_id(request)
    content = {
        "type": "/problems/internal-error",
        "title": "Erro interno",
        "status": 500,
        "detail": "Erro inesperado processando a requisição",
        "instance": request.url.path,
        "request_id": req_id,
    }
    response = JSONResponse(
        status_code=500,
        content=content,
        media_type="application/problem+json; charset=utf-8",
    )
    response.headers["X-Request-Id"] = req_id
    return response


def register_exception_handlers(app: FastAPI) -> None:
    """Registra todos os manipuladores de exceção na aplicação FastAPI."""
    app.add_exception_handler(ProblemException, problem_exception_handler)
    app.add_exception_handler(RequestValidationError, validation_exception_handler)
    app.add_exception_handler(HTTPException, http_exception_handler)
    app.add_exception_handler(Exception, unhandled_exception_handler)
