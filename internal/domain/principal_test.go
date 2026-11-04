package domain

import (
	"testing"
)

func TestProtectedPrincipalsRegistry(t *testing.T) {
	registry := NewProtectedRegistry(map[string][]string{
		"prod-br": {"sp-governance-service", "admin@company.com"},
	})

	if !registry.IsProtected("prod-br", "sp-governance-service") {
		t.Errorf("expected sp-governance-service to be protected in prod-br")
	}

	if !registry.IsProtected("prod-br", "SP-GOVERNANCE-SERVICE") {
		t.Errorf("expected case-insensitive matching for protected principal")
	}

	if registry.IsProtected("prod-br", "developer@company.com") {
		t.Errorf("expected developer to not be protected")
	}

	if registry.IsProtected("dev-br", "sp-governance-service") {
		t.Errorf("expected sp to not be protected in dev-br (not configured)")
	}

	// Safety check: Cannot revoke MANAGE
	if registry.ValidateRevokeSafety("prod-br", "sp-governance-service", PrivilegeManage) {
		t.Errorf("expected ValidateRevokeSafety to return false for MANAGE on protected principal")
	}

	// Safety check: Cannot revoke ALL_PRIVILEGES
	if registry.ValidateRevokeSafety("prod-br", "sp-governance-service", PrivilegeAllPrivileges) {
		t.Errorf("expected ValidateRevokeSafety to return false for ALL_PRIVILEGES on protected principal")
	}

	// Safe to revoke other privilege (or revoke on non-protected)
	if !registry.ValidateRevokeSafety("prod-br", "sp-governance-service", PrivilegeSelect) {
		t.Errorf("expected ValidateRevokeSafety to allow revoking SELECT on protected principal")
	}

	if !registry.ValidateRevokeSafety("prod-br", "developer@company.com", PrivilegeManage) {
		t.Errorf("expected ValidateRevokeSafety to allow revoking MANAGE on non-protected principal")
	}
}

func TestIdentifierValidation(t *testing.T) {
	validNames := []string{"catalog_a", "schema1", "t", "my_table_2026"}
	for _, n := range validNames {
		if !ValidateCreateIdentifier(n) {
			t.Errorf("expected '%s' to be valid for creation", n)
		}
	}

	invalidNames := []string{"1schema", "Catalog", "my-schema", "schema.name", "schema/name"}
	for _, n := range invalidNames {
		if ValidateCreateIdentifier(n) {
			t.Errorf("expected '%s' to be invalid for creation", n)
		}
	}
}
