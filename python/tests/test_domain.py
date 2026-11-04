import pytest

from app.domain.models import (
    SecurableType,
    validate_create_identifier,
    validate_path_identifier,
)
from app.domain.principal import ProtectedPrincipalsRegistry
from app.domain.privilege import (
    Privilege,
    is_privilege_applicable,
    validate_privileges,
)


class TestPrivilegeMatrix:
    @pytest.mark.parametrize(
        "sec_type,priv,expected",
        [
            (SecurableType.TABLE, Privilege.SELECT, True),
            (SecurableType.TABLE, Privilege.MODIFY, True),
            (SecurableType.TABLE, Privilege.USE_SCHEMA, False),
            (SecurableType.TABLE, Privilege.USE_CATALOG, False),
            (SecurableType.SCHEMA, Privilege.CREATE_TABLE, True),
            (SecurableType.SCHEMA, Privilege.USE_SCHEMA, True),
            (SecurableType.CATALOG, Privilege.CREATE_SCHEMA, True),
            (SecurableType.VOLUME, Privilege.READ_VOLUME, True),
            (SecurableType.VOLUME, Privilege.WRITE_VOLUME, True),
            (SecurableType.VOLUME, Privilege.SELECT, False),
            (SecurableType.VIEW, Privilege.SELECT, True),
            (SecurableType.VIEW, Privilege.MODIFY, False),
        ],
    )
    def test_privilege_applicability(
        self, sec_type: SecurableType, priv: Privilege, expected: bool
    ) -> None:
        assert is_privilege_applicable(sec_type, priv) == expected

    def test_validate_privileges(self) -> None:
        invalid = validate_privileges(
            SecurableType.TABLE,
            [
                Privilege.SELECT,
                Privilege.USE_SCHEMA,
                Privilege.MODIFY,
                Privilege.CREATE_SCHEMA,
            ],
        )
        assert len(invalid) == 2
        assert invalid[0] == Privilege.USE_SCHEMA
        assert invalid[1] == Privilege.CREATE_SCHEMA


class TestProtectedPrincipalsRegistry:
    def test_protected_principals_and_anti_lockout(self) -> None:
        registry = ProtectedPrincipalsRegistry(
            {"prod-br": ["sp-governance-service", "admin@company.com"]}
        )

        # Case-insensitive matching
        assert registry.is_protected("prod-br", "sp-governance-service") is True
        assert registry.is_protected("prod-br", "SP-GOVERNANCE-SERVICE") is True
        assert registry.is_protected("prod-br", "developer@company.com") is False
        assert registry.is_protected("dev-br", "sp-governance-service") is False

        # Safety check: Cannot revoke MANAGE on protected principal
        assert (
            registry.validate_revoke_safety(
                "prod-br", "sp-governance-service", Privilege.MANAGE
            )
            is False
        )

        # Safety check: Cannot revoke ALL_PRIVILEGES on protected principal
        assert (
            registry.validate_revoke_safety(
                "prod-br", "sp-governance-service", Privilege.ALL_PRIVILEGES
            )
            is False
        )

        # Safe to revoke other privilege on protected principal
        assert (
            registry.validate_revoke_safety(
                "prod-br", "sp-governance-service", Privilege.SELECT
            )
            is True
        )

        # Safe to revoke MANAGE on non-protected principal
        assert (
            registry.validate_revoke_safety(
                "prod-br", "developer@company.com", Privilege.MANAGE
            )
            is True
        )


class TestIdentifierValidation:
    @pytest.mark.parametrize(
        "name", ["catalog_a", "schema1", "t", "my_table_2026"]
    )
    def test_valid_identifiers(self, name: str) -> None:
        assert validate_create_identifier(name) is True

    @pytest.mark.parametrize(
        "name", ["1schema", "Catalog", "my-schema", "schema.name", "schema/name", ""]
    )
    def test_invalid_identifiers(self, name: str) -> None:
        assert validate_create_identifier(name) is False

    def test_path_identifier_validation(self) -> None:
        assert validate_path_identifier("main") is True
        assert validate_path_identifier("catalog-prod") is True
        assert validate_path_identifier("cat/sub") is False
        assert validate_path_identifier("cat.name") is False
