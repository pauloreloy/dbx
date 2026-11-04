package domain

import (
	"fmt"
	"net/http"
)

// FieldError detalha erros de validação por campo.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// UpstreamError carrega o erro original vindo do Databricks.
type UpstreamError struct {
	ErrorCode  string `json:"error_code,omitempty"`
	Message    string `json:"message,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

// ProblemError representa um erro estruturado de acordo com a RFC 9457.
type ProblemError struct {
	Type              string        `json:"type"`
	Title             string        `json:"title"`
	Status            int           `json:"status"`
	Detail            string        `json:"detail,omitempty"`
	Instance          string        `json:"instance,omitempty"`
	RequestID         string        `json:"request_id,omitempty"`
	Workspace         string        `json:"workspace,omitempty"`
	RetryAfterSeconds *int          `json:"retry_after_seconds,omitempty"`
	Errors            []FieldError  `json:"errors,omitempty"`
	Upstream          *UpstreamError `json:"upstream,omitempty"`
}

func (p *ProblemError) Error() string {
	if p.Detail != "" {
		return fmt.Sprintf("[%d %s] %s: %s", p.Status, p.Title, p.Type, p.Detail)
	}
	return fmt.Sprintf("[%d %s] %s", p.Status, p.Title, p.Type)
}

// Fábricas de erros canônicos de domínio

func NewPrivilegeNotApplicableError(secType SecurableType, invalidPrivs []Privilege) *ProblemError {
	var fieldErrors []FieldError
	for _, p := range invalidPrivs {
		fieldErrors = append(fieldErrors, FieldError{
			Field:   "privileges",
			Message: fmt.Sprintf("Privilégio %s não se aplica a %s", p, secType),
			Code:    "privilege-not-applicable",
		})
	}
	return &ProblemError{
		Type:    "/problems/privilege-not-applicable",
		Title:   "Privilégio não aplicável",
		Status:  http.StatusUnprocessableEntity,
		Detail:  fmt.Sprintf("Privilégios inválidos para o tipo de objeto %s", secType),
		Errors:  fieldErrors,
	}
}

func NewProtectedPrincipalError(principal string) *ProblemError {
	return &ProblemError{
		Type:   "/problems/protected-principal",
		Title:  "Principal protegido",
		Status: http.StatusConflict,
		Detail: fmt.Sprintf("A identidade '%s' é protegida e não pode ter privilégios administrativos revogados", principal),
	}
}

func NewConfirmationMismatchError(expected, received string) *ProblemError {
	return &ProblemError{
		Type:   "/problems/confirmation-mismatch",
		Title:  "Confirmação incorreta",
		Status: http.StatusUnprocessableEntity,
		Detail: fmt.Sprintf("O parâmetro confirm deve ser exatamente igual ao nome do objeto ('%s'), mas recebeu '%s'", expected, received),
		Errors: []FieldError{
			{
				Field:   "confirm",
				Message: fmt.Sprintf("Esperado '%s'", expected),
				Code:    "confirmation-mismatch",
			},
		},
	}
}

func NewPrincipalNotFoundError(principal string) *ProblemError {
	return &ProblemError{
		Type:   "/problems/principal-not-found",
		Title:  "Principal não encontrado",
		Status: http.StatusUnprocessableEntity,
		Detail: fmt.Sprintf("A identidade '%s' não foi encontrada no workspace", principal),
	}
}

func NewNotFoundError(resource, identifier string) *ProblemError {
	return &ProblemError{
		Type:   "/problems/not-found",
		Title:  "Recurso não encontrado",
		Status: http.StatusNotFound,
		Detail: fmt.Sprintf("%s '%s' não foi encontrado", resource, identifier),
	}
}

func NewConflictError(message string) *ProblemError {
	return &ProblemError{
		Type:   "/problems/conflict",
		Title:  "Conflito de recurso",
		Status: http.StatusConflict,
		Detail: message,
	}
}

func NewIdempotencyConflictError(key string) *ProblemError {
	return &ProblemError{
		Type:   "/problems/idempotency-conflict",
		Title:  "Conflito de idempotência",
		Status: http.StatusConflict,
		Detail: fmt.Sprintf("A chave de idempotência '%s' já foi utilizada com parâmetros diferentes ou está em processamento", key),
	}
}

func NewBadRequestError(detail string, errors ...FieldError) *ProblemError {
	return &ProblemError{
		Type:   "/problems/bad-request",
		Title:  "Requisição inválida",
		Status: http.StatusBadRequest,
		Detail: detail,
		Errors: errors,
	}
}

func NewForbiddenError(detail string) *ProblemError {
	return &ProblemError{
		Type:   "/problems/forbidden",
		Title:  "Acesso negado pela política",
		Status: http.StatusForbidden,
		Detail: detail,
	}
}

func NewInternalError(detail string) *ProblemError {
	return &ProblemError{
		Type:   "/problems/internal-error",
		Title:  "Erro interno",
		Status: http.StatusInternalServerError,
		Detail: detail,
	}
}
