package domain

import (
	"fmt"
	"regexp"
)

// SecurableType define os tipos de objetos suportados na Fase 1.
type SecurableType string

const (
	SecurableCatalog SecurableType = "CATALOG"
	SecurableSchema  SecurableType = "SCHEMA"
	SecurableTable   SecurableType = "TABLE"
	SecurableView    SecurableType = "VIEW"
	SecurableVolume  SecurableType = "VOLUME"
)

// SecurableRef representa a referência canônica a um securable.
type SecurableRef struct {
	Type        SecurableType `json:"type"`
	Name        string        `json:"name"`
	MetastoreID *string       `json:"metastore_id,omitempty"`
}

// FullName retorna o identificador completo do objeto.
func (s SecurableRef) String() string {
	return fmt.Sprintf("%s:%s", s.Type, s.Name)
}

var identifierRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{0,254}$`)
var pathParamRegex = regexp.MustCompile(`^[^./\s]{1,255}$`)

// ValidateCreateIdentifier valida se o nome atende ao padrão estrito de criação (minúsculas, dígitos, underline).
func ValidateCreateIdentifier(name string) bool {
	return identifierRegex.MatchString(name)
}

// ValidatePathIdentifier valida se o nome em rota atende ao padrão geral (sem pontos, barras ou espaços).
func ValidatePathIdentifier(name string) bool {
	return pathParamRegex.MatchString(name)
}
