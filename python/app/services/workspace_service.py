from typing import Optional

from app.domain.errors import NotFoundException
from app.domain.models import Workspace
from app.ports.stores import WorkspaceRegistry


class WorkspaceService:
    """Gerencia a consulta do registry de workspaces (D5)."""

    def __init__(self, registry: Optional[WorkspaceRegistry] = None) -> None:
        self._registry = registry

    def list_workspaces(
        self, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[Workspace], str]:
        if not self._registry:
            return [], ""
        return self._registry.list_workspaces(page_size, page_token)

    def get_workspace(self, alias: str) -> Workspace:
        if not self._registry:
            raise NotFoundException("Workspace", alias)
        return self._registry.get_workspace(alias)
