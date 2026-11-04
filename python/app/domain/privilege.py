from enum import StrEnum
from typing import Any, Sequence


class Privilege(StrEnum):
    ALL_PRIVILEGES = "ALL_PRIVILEGES"
    APPLY_TAG = "APPLY_TAG"
    BROWSE = "BROWSE"
    CREATE_CONNECTION = "CREATE_CONNECTION"
    CREATE_FUNCTION = "CREATE_FUNCTION"
    CREATE_MATERIALIZED_VIEW = "CREATE_MATERIALIZED_VIEW"
    CREATE_MODEL = "CREATE_MODEL"
    CREATE_SCHEMA = "CREATE_SCHEMA"
    CREATE_TABLE = "CREATE_TABLE"
    CREATE_VOLUME = "CREATE_VOLUME"
    EXECUTE = "EXECUTE"
    EXTERNAL_USE_SCHEMA = "EXTERNAL_USE_SCHEMA"
    MANAGE = "MANAGE"
    MODIFY = "MODIFY"
    READ_VOLUME = "READ_VOLUME"
    REFRESH = "REFRESH"
    SELECT = "SELECT"
    USE_CATALOG = "USE_CATALOG"
    USE_SCHEMA = "USE_SCHEMA"
    WRITE_VOLUME = "WRITE_VOLUME"


PRIVILEGE_MATRIX: dict[str, list[Privilege]] = {
    "CATALOG": [
        Privilege.ALL_PRIVILEGES,
        Privilege.APPLY_TAG,
        Privilege.BROWSE,
        Privilege.CREATE_CONNECTION,
        Privilege.CREATE_FUNCTION,
        Privilege.CREATE_MATERIALIZED_VIEW,
        Privilege.CREATE_MODEL,
        Privilege.CREATE_SCHEMA,
        Privilege.CREATE_TABLE,
        Privilege.CREATE_VOLUME,
        Privilege.EXECUTE,
        Privilege.MANAGE,
        Privilege.MODIFY,
        Privilege.READ_VOLUME,
        Privilege.REFRESH,
        Privilege.SELECT,
        Privilege.USE_CATALOG,
        Privilege.USE_SCHEMA,
        Privilege.WRITE_VOLUME,
    ],
    "SCHEMA": [
        Privilege.ALL_PRIVILEGES,
        Privilege.APPLY_TAG,
        Privilege.BROWSE,
        Privilege.CREATE_FUNCTION,
        Privilege.CREATE_MATERIALIZED_VIEW,
        Privilege.CREATE_MODEL,
        Privilege.CREATE_TABLE,
        Privilege.CREATE_VOLUME,
        Privilege.EXECUTE,
        Privilege.MANAGE,
        Privilege.MODIFY,
        Privilege.READ_VOLUME,
        Privilege.REFRESH,
        Privilege.SELECT,
        Privilege.USE_SCHEMA,
        Privilege.WRITE_VOLUME,
        Privilege.EXTERNAL_USE_SCHEMA,
    ],
    "TABLE": [
        Privilege.ALL_PRIVILEGES,
        Privilege.APPLY_TAG,
        Privilege.MODIFY,
        Privilege.SELECT,
    ],
    "VIEW": [
        Privilege.ALL_PRIVILEGES,
        Privilege.APPLY_TAG,
        Privilege.SELECT,
    ],
    "VOLUME": [
        Privilege.ALL_PRIVILEGES,
        Privilege.APPLY_TAG,
        Privilege.READ_VOLUME,
        Privilege.WRITE_VOLUME,
    ],
}


def is_privilege_applicable(sec_type: Any, priv: Privilege | str) -> bool:
    """Valida se o privilégio pode ser atribuído ao tipo de securable."""
    type_str = str(sec_type).upper()
    allowed = PRIVILEGE_MATRIX.get(type_str)
    if allowed is None:
        return False
    try:
        priv_enum = Privilege(priv) if isinstance(priv, str) else priv
        return priv_enum in allowed
    except ValueError:
        return False


def validate_privileges(sec_type: Any, privs: Sequence[Privilege | str]) -> list[Privilege]:
    """Verifica uma lista de privilégios contra a matriz e retorna os inválidos."""
    invalid: list[Privilege] = []
    for p in privs:
        if not is_privilege_applicable(sec_type, p):
            try:
                invalid.append(Privilege(p))
            except ValueError:
                # Se não for um valor do enum Privilege, mantém representação
                invalid.append(p)  # type: ignore[arg-type]
    return invalid
