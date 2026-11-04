from typing import Any, Optional
from pydantic import BaseModel, ConfigDict

from app.domain.models import SecurableType
from app.domain.privilege import Privilege


class FieldError(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    field: str
    message: str
    code: Optional[str] = None


class UpstreamError(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    error_code: Optional[str] = None
    message: Optional[str] = None
    http_status: Optional[int] = None


class ProblemException(Exception):
    """Exceção estruturada de acordo com o padrão RFC 9457 (application/problem+json)."""

    def __init__(
        self,
        status: int,
        type_: str,
        title: str,
        detail: Optional[str] = None,
        instance: Optional[str] = None,
        request_id: Optional[str] = None,
        workspace: Optional[str] = None,
        retry_after_seconds: Optional[int] = None,
        errors: Optional[list[FieldError]] = None,
        upstream: Optional[UpstreamError] = None,
    ) -> None:
        self.status = status
        self.type = type_
        self.title = title
        self.detail = detail
        self.instance = instance
        self.request_id = request_id
        self.workspace = workspace
        self.retry_after_seconds = retry_after_seconds
        self.errors = errors
        self.upstream = upstream

        msg = f"[{self.status} {self.title}] {self.type}"
        if self.detail:
            msg = f"{msg}: {self.detail}"
        super().__init__(msg)

    def to_dict(self) -> dict[str, Any]:
        result: dict[str, Any] = {
            "type": self.type,
            "title": self.title,
            "status": self.status,
        }
        if self.detail is not None:
            result["detail"] = self.detail
        if self.instance is not None:
            result["instance"] = self.instance
        if self.request_id is not None:
            result["request_id"] = self.request_id
        if self.workspace is not None:
            result["workspace"] = self.workspace
        if self.retry_after_seconds is not None:
            result["retry_after_seconds"] = self.retry_after_seconds
        if self.errors:
            result["errors"] = [e.model_dump(exclude_none=True) for e in self.errors]
        if self.upstream is not None:
            result["upstream"] = self.upstream.model_dump(exclude_none=True)
        return result


class PrivilegeNotApplicableException(ProblemException):
    def __init__(self, sec_type: SecurableType | str, invalid_privs: list[Privilege | str]) -> None:
        field_errors = [
            FieldError(
                field="privileges",
                message=f"Privilégio {p} não se aplica a {sec_type}",
                code="privilege-not-applicable",
            )
            for p in invalid_privs
        ]
        super().__init__(
            status=422,
            type_="/problems/privilege-not-applicable",
            title="Privilégio não aplicável",
            detail=f"Privilégios inválidos para o tipo de objeto {sec_type}",
            errors=field_errors,
        )


class ProtectedPrincipalException(ProblemException):
    def __init__(self, principal: str) -> None:
        super().__init__(
            status=409,
            type_="/problems/protected-principal",
            title="Principal protegido",
            detail=f"A identidade '{principal}' é protegida e não pode ter privilégios administrativos revogados",
        )


class ConfirmationMismatchException(ProblemException):
    def __init__(self, expected: str, received: str) -> None:
        super().__init__(
            status=422,
            type_="/problems/confirmation-mismatch",
            title="Confirmação incorreta",
            detail=f"O parâmetro confirm deve ser exatamente igual ao nome do objeto ('{expected}'), mas recebeu '{received}'",
            errors=[
                FieldError(
                    field="confirm",
                    message=f"Esperado '{expected}'",
                    code="confirmation-mismatch",
                )
            ],
        )


class PrincipalNotFoundException(ProblemException):
    def __init__(self, principal: str) -> None:
        super().__init__(
            status=422,
            type_="/problems/principal-not-found",
            title="Principal não encontrado",
            detail=f"A identidade '{principal}' não foi encontrada no workspace",
        )


class NotFoundException(ProblemException):
    def __init__(self, resource: str, identifier: str) -> None:
        super().__init__(
            status=404,
            type_="/problems/not-found",
            title="Recurso não encontrado",
            detail=f"{resource} '{identifier}' não foi encontrado",
        )


class ConflictException(ProblemException):
    def __init__(self, message: str) -> None:
        super().__init__(
            status=409,
            type_="/problems/conflict",
            title="Conflito de recurso",
            detail=message,
        )


class IdempotencyConflictException(ProblemException):
    def __init__(self, key: str) -> None:
        super().__init__(
            status=409,
            type_="/problems/idempotency-conflict",
            title="Conflito de idempotência",
            detail=f"A chave de idempotência '{key}' já foi utilizada com parâmetros diferentes ou está em processamento",
        )


class BadRequestException(ProblemException):
    def __init__(self, detail: str, errors: Optional[list[FieldError]] = None) -> None:
        super().__init__(
            status=400,
            type_="/problems/bad-request",
            title="Requisição inválida",
            detail=detail,
            errors=errors,
        )


class ForbiddenException(ProblemException):
    def __init__(self, detail: str) -> None:
        super().__init__(
            status=403,
            type_="/problems/forbidden",
            title="Acesso negado pela política",
            detail=detail,
        )


class InternalErrorException(ProblemException):
    def __init__(self, detail: str = "Erro interno") -> None:
        super().__init__(
            status=500,
            type_="/problems/internal-error",
            title="Erro interno",
            detail=detail,
        )
