from datetime import datetime, timezone
from typing import Optional
import uuid

from app.domain.audit import AuditAction, AuditError, AuditEvent, AuditOutcome
from app.domain.errors import (
    PrincipalNotFoundException,
    PrivilegeNotApplicableException,
    ProblemException,
    ProtectedPrincipalException,
)
from app.domain.models import (
    EffectiveGrant,
    GrantChangeResult,
    PrincipalPrivileges,
    PrivilegeChange,
    SecurableRef,
)
from app.domain.principal import ProtectedPrincipalsRegistry
from app.domain.privilege import validate_privileges
from app.ports.databricks import GrantsAPI, IdentityAPI
from app.ports.stores import AuditStore, WorkspaceRegistry


class GrantService:
    """Orquestra as regras de negócio de concessão e revogação de acessos."""

    def __init__(
        self,
        grants_api: GrantsAPI,
        identity_api: Optional[IdentityAPI] = None,
        protected: Optional[ProtectedPrincipalsRegistry] = None,
        audit_store: Optional[AuditStore] = None,
        registry: Optional[WorkspaceRegistry] = None,
    ) -> None:
        self._grants_api = grants_api
        self._identity_api = identity_api
        self._protected = protected
        self._audit_store = audit_store
        self._registry = registry

    def get_grants(
        self, ws: str, sec: SecurableRef, principal_filter: Optional[str] = None
    ) -> list[PrincipalPrivileges]:
        """Retorna os privilégios diretos concedidos no objeto."""
        return self._grants_api.get_grants(ws, sec, principal_filter)

    def get_effective_grants(
        self, ws: str, sec: SecurableRef, principal_filter: Optional[str] = None
    ) -> list[EffectiveGrant]:
        """Retorna os privilégios efetivos (incluindo herança)."""
        return self._grants_api.get_effective_grants(ws, sec, principal_filter)

    def apply_changes(
        self,
        ws: str,
        sec: SecurableRef,
        changes: list[PrivilegeChange],
        is_revoke: bool,
        actor: str,
        change_ref: str,
        request_id: str,
    ) -> GrantChangeResult:
        """Executa grant ou revoke garantindo validações de domínio e auditoria."""
        action = AuditAction.REVOKE if is_revoke else AuditAction.GRANT
        event_id = f"evt_{uuid.uuid4().hex}"
        targets: list[str] = []

        # 1. Obter informações de workspace para metastore_id
        metastore_id: Optional[str] = None
        if self._registry:
            try:
                ws_info = self._registry.get_workspace(ws)
                if ws_info and ws_info.metastore_id:
                    metastore_id = ws_info.metastore_id
                    sec.metastore_id = metastore_id
            except Exception:
                pass

        # 2. Validações de Domínio
        for change in changes:
            targets.append(change.principal)

            # A. Validar aplicabilidade dos privilégios contra a matriz do Unity Catalog
            invalid_privs = validate_privileges(sec.type, change.privileges)
            if invalid_privs:
                err = PrivilegeNotApplicableException(sec.type, invalid_privs)
                self._record_audit_failure(
                    event_id, ws, metastore_id, actor, action, change_ref, request_id, sec, targets, err
                )
                raise err

            # B. Se for Revoke, validar segurança contra lockout de identities protegidas (D7)
            if is_revoke and self._protected:
                for priv in change.privileges:
                    if not self._protected.validate_revoke_safety(ws, change.principal, priv):
                        prot_err = ProtectedPrincipalException(change.principal)
                        self._record_audit_failure(
                            event_id, ws, metastore_id, actor, action, change_ref, request_id, sec, targets, prot_err
                        )
                        raise prot_err

            # C. Validar existência do principal (se identity_api estiver configurada)
            if self._identity_api:
                try:
                    p = self._identity_api.get_principal(ws, change.principal)
                except Exception:
                    p = None
                if not p:
                    nf_err = PrincipalNotFoundException(change.principal)
                    self._record_audit_failure(
                        event_id, ws, metastore_id, actor, action, change_ref, request_id, sec, targets, nf_err
                    )
                    raise nf_err

        # 3. Capturar estado 'before' para auditoria
        before_grants: list[PrincipalPrivileges] = []
        try:
            before_grants = self._grants_api.get_grants(ws, sec, None)
        except Exception:
            pass

        # 4. Executar mutação no Databricks via Port
        try:
            outcomes, after_grants, changed = self._grants_api.update_grants(
                ws, sec, changes, is_revoke
            )
        except Exception as e:
            self._record_audit_failure(
                event_id, ws, metastore_id, actor, action, change_ref, request_id, sec, targets, e
            )
            raise e

        # 5. Gravar evento de auditoria de SUCESSO (D10)
        if self._audit_store:
            self._audit_store.record(
                AuditEvent(
                    id=event_id,
                    occurred_at=datetime.now(timezone.utc),
                    workspace=ws,
                    metastore_id=metastore_id,
                    actor=actor,
                    action=action,
                    outcome=AuditOutcome.SUCCESS,
                    change_ref=change_ref,
                    request_id=request_id,
                    securable=sec,
                    targets=targets,
                    before={"grants": [g.model_dump(mode="json") for g in before_grants]},
                    after={"grants": [g.model_dump(mode="json") for g in after_grants]},
                )
            )

        return GrantChangeResult(
            securable=sec,
            outcomes=outcomes,
            grants=after_grants,
            changed=changed,
            audit_id=event_id,
        )

    def _record_audit_failure(
        self,
        event_id: str,
        ws: str,
        metastore_id: Optional[str],
        actor: str,
        action: AuditAction,
        change_ref: str,
        request_id: str,
        sec: Optional[SecurableRef],
        targets: list[str],
        err: Exception,
    ) -> None:
        if not self._audit_store:
            return

        detail = str(err)
        err_type = "/problems/error"
        if isinstance(err, ProblemException):
            detail = err.detail or err.title
            err_type = err.type

        try:
            self._audit_store.record(
                AuditEvent(
                    id=event_id,
                    occurred_at=datetime.now(timezone.utc),
                    workspace=ws,
                    metastore_id=metastore_id,
                    actor=actor,
                    action=action,
                    outcome=AuditOutcome.DENIED,
                    change_ref=change_ref,
                    request_id=request_id,
                    securable=sec,
                    targets=targets,
                    error=AuditError(type=err_type, detail=detail),
                )
            )
        except Exception:
            pass
