import threading
from typing import Optional

from app.domain.errors import NotFoundException
from app.domain.models import Workspace
from app.ports.stores import WorkspaceRegistry


DEFAULT_WORKSPACES: list[Workspace] = [
    Workspace(
        id="prod-br",
        display_name="Produção Brasil",
        databricks_workspace_id="7474652819690760",
        metastore_id="meta-sa-east-1",
        cloud="AWS",
        region="sa-east-1",
        status="ACTIVE",
    ),
    Workspace(
        id="dev-br",
        display_name="Desenvolvimento Brasil",
        databricks_workspace_id="8392019382019382",
        metastore_id="meta-sa-east-1",
        cloud="AWS",
        region="sa-east-1",
        status="ACTIVE",
    ),
]


class MemoryWorkspaceRegistry(WorkspaceRegistry):
    """Armazena definições de workspace em memória de forma thread-safe."""

    def __init__(self, initial: Optional[list[Workspace]] = None) -> None:
        self._lock = threading.RLock()
        self._workspaces: dict[str, Workspace] = {}
        items = initial if initial is not None else DEFAULT_WORKSPACES
        for ws in items:
            self._workspaces[ws.id] = ws

    def list_workspaces(
        self, page_size: int = 0, page_token: str = ""
    ) -> tuple[list[Workspace], str]:
        with self._lock:
            result = list(self._workspaces.values())
            if page_size > 0 and len(result) > page_size:
                result = result[:page_size]
            return result, ""

    def get_workspace(self, alias: str) -> Workspace:
        with self._lock:
            ws = self._workspaces.get(alias)
            if not ws:
                raise NotFoundException("Workspace", alias)
            return ws
