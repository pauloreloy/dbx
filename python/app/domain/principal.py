from enum import StrEnum
from typing import Optional
from pydantic import BaseModel, ConfigDict

from app.domain.privilege import Privilege


class PrincipalType(StrEnum):
    USER = "USER"
    GROUP = "GROUP"
    SERVICE_PRINCIPAL = "SERVICE_PRINCIPAL"


class Principal(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    name: str
    type: PrincipalType
    id: Optional[str] = None
    active: bool = True


class ProtectedPrincipalsRegistry:
    """Gerencia identidades críticas que não podem sofrer lockout."""

    def __init__(self, mapping: Optional[dict[str, list[str]]] = None) -> None:
        self._protected_by_workspace: dict[str, list[str]] = {}
        if mapping:
            for ws, principals in mapping.items():
                norm_ws = ws.strip().lower()
                norm_list = [p.strip().lower() for p in principals]
                self._protected_by_workspace[norm_ws] = norm_list

    def is_protected(self, ws: str, principal: str) -> bool:
        """Verifica se o principal é protegido no workspace fornecido."""
        norm_ws = ws.strip().lower()
        principals = self._protected_by_workspace.get(norm_ws)
        if not principals:
            return False
        return principal.strip().lower() in principals

    def validate_revoke_safety(self, ws: str, principal: str, priv: Privilege | str) -> bool:
        """
        Impede a revogação de privilégios essenciais (como MANAGE ou ALL_PRIVILEGES)
        de um principal protegido.
        Retorna False se violar a regra de proteção (inseguro), ou True se seguro.
        """
        if not self.is_protected(ws, principal):
            return True

        priv_str = str(priv)
        if priv_str in (Privilege.MANAGE.value, Privilege.ALL_PRIVILEGES.value):
            return False
        return True
