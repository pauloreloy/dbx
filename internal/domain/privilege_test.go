package domain

import (
	"testing"
)

func TestPrivilegeApplicability(t *testing.T) {
	tests := []struct {
		name     string
		secType  SecurableType
		priv     Privilege
		expected bool
	}{
		{"SELECT on TABLE", SecurableTable, PrivilegeSelect, true},
		{"MODIFY on TABLE", SecurableTable, PrivilegeModify, true},
		{"USE_SCHEMA on TABLE (Invalid)", SecurableTable, PrivilegeUseSchema, false},
		{"USE_CATALOG on TABLE (Invalid)", SecurableTable, PrivilegeUseCatalog, false},
		{"CREATE_TABLE on SCHEMA", SecurableSchema, PrivilegeCreateTable, true},
		{"USE_SCHEMA on SCHEMA", SecurableSchema, PrivilegeUseSchema, true},
		{"CREATE_SCHEMA on CATALOG", SecurableCatalog, PrivilegeCreateSchema, true},
		{"READ_VOLUME on VOLUME", SecurableVolume, PrivilegeReadVolume, true},
		{"WRITE_VOLUME on VOLUME", SecurableVolume, PrivilegeWriteVolume, true},
		{"SELECT on VOLUME (Invalid)", SecurableVolume, PrivilegeSelect, false},
		{"SELECT on VIEW", SecurableView, PrivilegeSelect, true},
		{"MODIFY on VIEW (Invalid)", SecurableView, PrivilegeModify, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsPrivilegeApplicable(tc.secType, tc.priv)
			if got != tc.expected {
				t.Errorf("IsPrivilegeApplicable(%s, %s) = %v; want %v", tc.secType, tc.priv, got, tc.expected)
			}
		})
	}
}

func TestValidatePrivileges(t *testing.T) {
	invalid := ValidatePrivileges(SecurableTable, []Privilege{
		PrivilegeSelect,
		PrivilegeUseSchema,
		PrivilegeModify,
		PrivilegeCreateSchema,
	})

	if len(invalid) != 2 {
		t.Fatalf("expected 2 invalid privileges, got %d: %v", len(invalid), invalid)
	}

	if invalid[0] != PrivilegeUseSchema || invalid[1] != PrivilegeCreateSchema {
		t.Errorf("unexpected invalid list: %v", invalid)
	}
}
