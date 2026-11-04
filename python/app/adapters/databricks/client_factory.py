from dataclasses import dataclass
import os
import re
import threading
from typing import Callable, Optional

from dotenv import load_dotenv

from app.domain.errors import NotFoundException

# Carrega variáveis de ambiente de .env se disponível
load_dotenv()


@dataclass
class WorkspaceCredentials:
    """Credenciais OAuth M2M (Service Principal) para um workspace Databricks."""

    host: str
    client_id: str
    client_secret: str


class WorkspaceClientFactory:
    """Fábrica thread-safe e pool de conexões de WorkspaceClient por workspace (D6)."""

    def __init__(
        self,
        credentials: Optional[dict[str, WorkspaceCredentials]] = None,
        default_credentials: Optional[WorkspaceCredentials] = None,
        client_builder: Optional[Callable[[WorkspaceCredentials], any]] = None,
    ) -> None:
        self._lock = threading.RLock()
        self._credentials: dict[str, WorkspaceCredentials] = dict(credentials or {})
        self._default_credentials: Optional[WorkspaceCredentials] = default_credentials
        self._clients: dict[str, any] = {}
        self._client_builder = client_builder or self._default_client_builder

    @classmethod
    def from_env(cls) -> "WorkspaceClientFactory":
        """Instancia a fábrica inspecionando variáveis de ambiente do processo."""
        load_dotenv()

        default_creds = None
        host = os.getenv("DATABRICKS_HOST", "").strip()
        client_id = os.getenv("DATABRICKS_CLIENT_ID", "").strip()
        client_secret = os.getenv("DATABRICKS_CLIENT_SECRET", "").strip()
        if host and client_id and client_secret:
            default_creds = WorkspaceCredentials(
                host=host, client_id=client_id, client_secret=client_secret
            )

        # Mapeia workspaces específicos: DATABRICKS_WS_<PREFIX>_HOST / CLIENT_ID / CLIENT_SECRET
        ws_creds: dict[str, WorkspaceCredentials] = {}
        ws_pattern = re.compile(r"^DATABRICKS_WS_([A-Z0-9_]+)_HOST$")

        for key, val in os.environ.items():
            match = ws_pattern.match(key)
            if not match:
                continue
            prefix = match.group(1)
            ws_host = val.strip()
            ws_client_id = os.getenv(f"DATABRICKS_WS_{prefix}_CLIENT_ID", "").strip()
            ws_client_secret = os.getenv(f"DATABRICKS_WS_{prefix}_CLIENT_SECRET", "").strip()
            if ws_host and ws_client_id and ws_client_secret:
                creds = WorkspaceCredentials(
                    host=ws_host,
                    client_id=ws_client_id,
                    client_secret=ws_client_secret,
                )
                # Registra variações comuns (ex: prod-br, prod_br, PROD_BR)
                alias_kebab = prefix.lower().replace("_", "-")
                alias_snake = prefix.lower()
                ws_creds[prefix] = creds
                ws_creds[alias_kebab] = creds
                ws_creds[alias_snake] = creds

        return cls(credentials=ws_creds, default_credentials=default_creds)

    def is_configured(self) -> bool:
        """Indica se há credenciais válidas configuradas (default ou específicas)."""
        if self._default_credentials is not None:
            return True
        if bool(self._credentials):
            return True
        # Verifica variáveis do ambiente diretamente
        host = os.getenv("DATABRICKS_HOST", "").strip()
        client_id = os.getenv("DATABRICKS_CLIENT_ID", "").strip()
        client_secret = os.getenv("DATABRICKS_CLIENT_SECRET", "").strip()
        return bool(host and client_id and client_secret)

    def get_credentials(self, ws: str) -> Optional[WorkspaceCredentials]:
        """Obtém as credenciais para o alias do workspace informado."""
        with self._lock:
            # 1. Busca no cache de credenciais do construtor
            if ws in self._credentials:
                return self._credentials[ws]

            ws_lower = ws.lower()
            if ws_lower in self._credentials:
                return self._credentials[ws_lower]

            ws_kebab = ws_lower.replace("_", "-")
            if ws_kebab in self._credentials:
                return self._credentials[ws_kebab]

            ws_snake = ws_lower.replace("-", "_")
            if ws_snake in self._credentials:
                return self._credentials[ws_snake]

            # 2. Busca dinâmica em variáveis de ambiente
            prefix = ws.upper().replace("-", "_")
            env_host = os.getenv(f"DATABRICKS_WS_{prefix}_HOST", "").strip()
            env_client_id = os.getenv(f"DATABRICKS_WS_{prefix}_CLIENT_ID", "").strip()
            env_client_secret = os.getenv(f"DATABRICKS_WS_{prefix}_CLIENT_SECRET", "").strip()
            if env_host and env_client_id and env_client_secret:
                creds = WorkspaceCredentials(
                    host=env_host,
                    client_id=env_client_id,
                    client_secret=env_client_secret,
                )
                self._credentials[ws] = creds
                return creds

            # 3. Fallback para credenciais padrão
            if self._default_credentials:
                return self._default_credentials

            default_host = os.getenv("DATABRICKS_HOST", "").strip()
            default_id = os.getenv("DATABRICKS_CLIENT_ID", "").strip()
            default_secret = os.getenv("DATABRICKS_CLIENT_SECRET", "").strip()
            if default_host and default_id and default_secret:
                creds = WorkspaceCredentials(
                    host=default_host,
                    client_id=default_id,
                    client_secret=default_secret,
                )
                self._default_credentials = creds
                return creds

            return None

    def get_client(self, ws: str) -> any:
        """Retorna uma instância de WorkspaceClient para o workspace informado."""
        with self._lock:
            if ws in self._clients:
                return self._clients[ws]

            creds = self.get_credentials(ws)
            if not creds:
                raise NotFoundException("Workspace credentials", ws)

            client = self._client_builder(creds)
            self._clients[ws] = client
            return client

    def register_client(self, ws: str, client: any) -> None:
        """Registra manualmente um cliente no pool (útil para testes unitários)."""
        with self._lock:
            self._clients[ws] = client

    @staticmethod
    def _default_client_builder(creds: WorkspaceCredentials) -> any:
        from databricks.sdk import WorkspaceClient

        return WorkspaceClient(
            host=creds.host,
            client_id=creds.client_id,
            client_secret=creds.client_secret,
        )
