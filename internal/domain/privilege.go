package domain

import "slices"

// Privilege define os privilégios suportados pela governança.
type Privilege string

const (
	PrivilegeAllPrivileges          Privilege = "ALL_PRIVILEGES"
	PrivilegeApplyTag               Privilege = "APPLY_TAG"
	PrivilegeBrowse                 Privilege = "BROWSE"
	PrivilegeCreateConnection       Privilege = "CREATE_CONNECTION"
	PrivilegeCreateFunction         Privilege = "CREATE_FUNCTION"
	PrivilegeCreateMaterializedView Privilege = "CREATE_MATERIALIZED_VIEW"
	PrivilegeCreateModel            Privilege = "CREATE_MODEL"
	PrivilegeCreateSchema           Privilege = "CREATE_SCHEMA"
	PrivilegeCreateTable            Privilege = "CREATE_TABLE"
	PrivilegeCreateVolume           Privilege = "CREATE_VOLUME"
	PrivilegeExecute                Privilege = "EXECUTE"
	PrivilegeExternalUseSchema      Privilege = "EXTERNAL_USE_SCHEMA"
	PrivilegeManage                 Privilege = "MANAGE"
	PrivilegeModify                 Privilege = "MODIFY"
	PrivilegeReadVolume             Privilege = "READ_VOLUME"
	PrivilegeRefresh                Privilege = "REFRESH"
	PrivilegeSelect                 Privilege = "SELECT"
	PrivilegeUseCatalog             Privilege = "USE_CATALOG"
	PrivilegeUseSchema              Privilege = "USE_SCHEMA"
	PrivilegeWriteVolume            Privilege = "WRITE_VOLUME"
)

// PrivilegeMatrix mapeia quais privilégios são permitidos por SecurableType.
var PrivilegeMatrix = map[SecurableType][]Privilege{
	SecurableCatalog: {
		PrivilegeAllPrivileges, PrivilegeApplyTag, PrivilegeBrowse,
		PrivilegeCreateConnection, PrivilegeCreateFunction, PrivilegeCreateMaterializedView,
		PrivilegeCreateModel, PrivilegeCreateSchema, PrivilegeCreateTable, PrivilegeCreateVolume,
		PrivilegeExecute, PrivilegeManage, PrivilegeModify, PrivilegeReadVolume,
		PrivilegeRefresh, PrivilegeSelect, PrivilegeUseCatalog, PrivilegeUseSchema,
		PrivilegeWriteVolume,
	},
	SecurableSchema: {
		PrivilegeAllPrivileges, PrivilegeApplyTag, PrivilegeBrowse,
		PrivilegeCreateFunction, PrivilegeCreateMaterializedView, PrivilegeCreateModel,
		PrivilegeCreateTable, PrivilegeCreateVolume, PrivilegeExecute, PrivilegeManage,
		PrivilegeModify, PrivilegeReadVolume, PrivilegeRefresh, PrivilegeSelect,
		PrivilegeUseSchema, PrivilegeWriteVolume, PrivilegeExternalUseSchema,
	},
	SecurableTable: {
		PrivilegeAllPrivileges, PrivilegeApplyTag, PrivilegeModify, PrivilegeSelect,
	},
	SecurableView: {
		PrivilegeAllPrivileges, PrivilegeApplyTag, PrivilegeSelect,
	},
	SecurableVolume: {
		PrivilegeAllPrivileges, PrivilegeApplyTag, PrivilegeReadVolume, PrivilegeWriteVolume,
	},
}

// IsPrivilegeApplicable valida se o privilégio pode ser atribuído ao tipo de securable.
func IsPrivilegeApplicable(secType SecurableType, priv Privilege) bool {
	allowed, ok := PrivilegeMatrix[secType]
	if !ok {
		return false
	}
	return slices.Contains(allowed, priv)
}

// ValidatePrivileges verifica uma lista de privilégios contra a matriz e retorna os inválidos.
func ValidatePrivileges(secType SecurableType, privs []Privilege) []Privilege {
	var invalid []Privilege
	for _, p := range privs {
		if !IsPrivilegeApplicable(secType, p) {
			invalid = append(invalid, p)
		}
	}
	return invalid
}
