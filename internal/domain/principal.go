package domain

import "strings"

// PrincipalType define a categoria de identidade.
type PrincipalType string

const (
	PrincipalTypeUser             PrincipalType = "USER"
	PrincipalTypeGroup            PrincipalType = "GROUP"
	PrincipalTypeServicePrincipal PrincipalType = "SERVICE_PRINCIPAL"
)

// Principal representa uma identidade consultada no workspace.
type Principal struct {
	Name   string        `json:"name"`
	Type   PrincipalType `json:"type"`
	ID     string        `json:"id,omitempty"`
	Active bool          `json:"active"`
}

// ProtectedPrincipalsRegistry gerencia identidades críticas que não podem sofrer lockout.
type ProtectedPrincipalsRegistry struct {
	// Mapeia workspace alias -> lista de identidades protegidas (ex: Application ID do SP do microserviço)
	protectedByWorkspace map[string][]string
}

// NewProtectedRegistry cria uma nova instância de regras de proteção.
func NewProtectedRegistry(mapping map[string][]string) *ProtectedPrincipalsRegistry {
	normalized := make(map[string][]string)
	for ws, principals := range mapping {
		normList := make([]string, len(principals))
		for i, p := range principals {
			normList[i] = strings.ToLower(strings.TrimSpace(p))
		}
		normalized[strings.ToLower(strings.TrimSpace(ws))] = normList
	}
	return &ProtectedPrincipalsRegistry{
		protectedByWorkspace: normalized,
	}
}

// IsProtected verifica se o principal é protegido no workspace fornecido.
func (r *ProtectedPrincipalsRegistry) IsProtected(ws, principal string) bool {
	if r == nil || r.protectedByWorkspace == nil {
		return false
	}
	principals, ok := r.protectedByWorkspace[strings.ToLower(strings.TrimSpace(ws))]
	if !ok {
		return false
	}
	target := strings.ToLower(strings.TrimSpace(principal))
	for _, p := range principals {
		if p == target {
			return true
		}
	}
	return false
}

// ValidateRevokeSafety impede a revogação de privilégios essenciais (como MANAGE ou ALL_PRIVILEGES) de um principal protegido.
func (r *ProtectedPrincipalsRegistry) ValidateRevokeSafety(ws, principal string, priv Privilege) bool {
	if !r.IsProtected(ws, principal) {
		return true // Não protegido, revogação permitida
	}

	// Se for protegido, não pode revogar MANAGE ou ALL_PRIVILEGES
	if priv == PrivilegeManage || priv == PrivilegeAllPrivileges {
		return false
	}
	return true
}
