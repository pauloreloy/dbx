from contextlib import asynccontextmanager
import logging
import os
from typing import AsyncGenerator, Optional
from dotenv import load_dotenv
from fastapi import FastAPI
import uvicorn

# Carrega variáveis de ambiente do .env
load_dotenv()

from app.adapters.databricks import DatabricksSDKAdapter, WorkspaceClientFactory
from app.adapters.memory.audit import MemoryAuditStore, MemoryIdempotencyStore
from app.adapters.memory.databricks import MemoryDatabricksClient
from app.adapters.memory.registry import MemoryWorkspaceRegistry
from app.domain.principal import ProtectedPrincipalsRegistry
from app.services.audit_service import AuditService
from app.services.catalog_service import CatalogService
from app.services.grant_service import GrantService
from app.services.workspace_service import WorkspaceService
from app.transport.middlewares import (
    ActorMiddleware,
    ChangeRefMiddleware,
    RequestIDMiddleware,
    register_exception_handlers,
)
from app.transport.routers import audit, catalogs, grants, health, workspaces

logger = logging.getLogger("governance_service")


def init_app_state(app: FastAPI) -> None:
    """Inicializa os adaptadores e serviços no state da aplicação."""
    workspace_registry = MemoryWorkspaceRegistry()
    protected_registry = ProtectedPrincipalsRegistry(
        {
            "prod-br": ["sp-governance-service", "admin@company.com"],
            "dev-br": ["sp-governance-service"],
        }
    )
    audit_store = MemoryAuditStore()
    idempotency_store = MemoryIdempotencyStore()

    # Verifica se as credenciais do Databricks estão configuradas
    client_factory = WorkspaceClientFactory.from_env()
    if client_factory.is_configured():
        logger.info(
            "Credenciais Databricks detectadas no ambiente. Inicializando DatabricksSDKAdapter."
        )
        databricks_client = DatabricksSDKAdapter(factory=client_factory)
    else:
        logger.info(
            "Credenciais Databricks (DATABRICKS_HOST, DATABRICKS_CLIENT_ID, DATABRICKS_CLIENT_SECRET) não configuradas. "
            "Utilizando fallback transparente para MemoryDatabricksClient."
        )
        databricks_client = MemoryDatabricksClient()

    grant_service = GrantService(
        grants_api=databricks_client,
        identity_api=databricks_client,
        protected=protected_registry,
        audit_store=audit_store,
        registry=workspace_registry,
    )

    catalog_service = CatalogService(
        catalogs_api=databricks_client,
        schemas_api=databricks_client,
        tables_api=databricks_client,
        views_api=databricks_client,
        volumes_api=databricks_client,
        ownership_api=databricks_client,
        tags_api=databricks_client,
        identity_api=databricks_client,
        audit_store=audit_store,
        registry=workspace_registry,
    )

    audit_service = AuditService(audit_store=audit_store)
    workspace_service = WorkspaceService(registry=workspace_registry)

    app.state.workspace_registry = workspace_registry
    app.state.protected_registry = protected_registry
    app.state.audit_store = audit_store
    app.state.idempotency_store = idempotency_store
    app.state.databricks_client = databricks_client
    app.state.grant_service = grant_service
    app.state.catalog_service = catalog_service
    app.state.audit_service = audit_service
    app.state.workspace_service = workspace_service


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncGenerator[None, None]:
    """Gerencia o ciclo de vida da aplicação FastAPI."""
    if not hasattr(app.state, "grant_service"):
        init_app_state(app)
    yield


def create_app() -> FastAPI:
    """Fábrica de criação da aplicação FastAPI com middlewares e rotas."""
    app = FastAPI(
        title="Databricks Governance Service (Python)",
        description="Microsserviço de Governança para Databricks Unity Catalog seguindo Arquitetura Hexagonal e RFC 9457",
        version="0.1.0",
        lifespan=lifespan,
    )

    # Inicializa estado imediatamente para testes síncronos
    init_app_state(app)

    # Registra Exception Handlers (RFC 9457)
    register_exception_handlers(app)

    # Middlewares (ordem de execução: RequestID -> Actor -> ChangeRef)
    app.add_middleware(ChangeRefMiddleware)
    app.add_middleware(ActorMiddleware)
    app.add_middleware(RequestIDMiddleware)

    # Routers
    app.include_router(health.router)
    app.include_router(workspaces.router)
    app.include_router(audit.router)
    app.include_router(catalogs.router)
    app.include_router(grants.router)

    return app


app = create_app()

if __name__ == "__main__":
    port = int(os.getenv("PORT", "8000"))
    uvicorn.run("app.main:app", host="0.0.0.0", port=port, reload=False)
