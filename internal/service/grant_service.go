package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/pauloeloy/databricks-governance-service/internal/domain"
	"github.com/pauloeloy/databricks-governance-service/internal/ports"
)

// GrantService orquestra as regras de negócio de concessão e revogação de acessos.
type GrantService struct {
	grantsAPI  ports.GrantsAPI
	identityAPI ports.IdentityAPI
	protected  *domain.ProtectedPrincipalsRegistry
	auditStore ports.AuditStore
	registry   ports.WorkspaceRegistry
}

// NewGrantService cria uma nova instância do serviço de governança de permissões.
func NewGrantService(
	grantsAPI ports.GrantsAPI,
	identityAPI ports.IdentityAPI,
	protected *domain.ProtectedPrincipalsRegistry,
	auditStore ports.AuditStore,
	registry ports.WorkspaceRegistry,
) *GrantService {
	return &GrantService{
		grantsAPI:   grantsAPI,
		identityAPI: identityAPI,
		protected:   protected,
		auditStore:  auditStore,
		registry:    registry,
	}
}

// GetGrants retorna os privilégios diretos concedidos no objeto.
func (s *GrantService) GetGrants(ctx context.Context, ws string, sec domain.SecurableRef, principalFilter *string) ([]domain.PrincipalPrivileges, error) {
	return s.grantsAPI.GetGrants(ctx, ws, sec, principalFilter)
}

// GetEffectiveGrants retorna os privilégios efetivos (incluindo herança).
func (s *GrantService) GetEffectiveGrants(ctx context.Context, ws string, sec domain.SecurableRef, principalFilter *string) ([]domain.EffectiveGrant, error) {
	return s.grantsAPI.GetEffectiveGrants(ctx, ws, sec, principalFilter)
}

// ApplyChanges executa grant ou revoke garantindo validações de domínio e auditoria.
func (s *GrantService) ApplyChanges(
	ctx context.Context,
	ws string,
	sec domain.SecurableRef,
	changes []domain.PrivilegeChange,
	isRevoke bool,
	actor, changeRef, requestID string,
) (*domain.GrantChangeResult, error) {
	action := domain.ActionGrant
	if isRevoke {
		action = domain.ActionRevoke
	}

	eventID := "evt_" + uuid.New().String()
	targets := make([]string, 0, len(changes))

	// 1. Obter informações de workspace para metastore_id
	var metastoreID *string
	if s.registry != nil {
		if wsInfo, err := s.registry.GetWorkspace(ctx, ws); err == nil && wsInfo != nil && wsInfo.MetastoreID != "" {
			mID := wsInfo.MetastoreID
			metastoreID = &mID
			sec.MetastoreID = &mID
		}
	}

	// 2. Validações de Domínio
	for _, change := range changes {
		targets = append(targets, change.Principal)

		// A. Validar aplicabilidade dos privilégios contra a matriz do Unity Catalog
		invalidPrivs := domain.ValidatePrivileges(sec.Type, change.Privileges)
		if len(invalidPrivs) > 0 {
			err := domain.NewPrivilegeNotApplicableError(sec.Type, invalidPrivs)
			s.recordAuditFailure(ctx, eventID, ws, metastoreID, actor, action, changeRef, requestID, &sec, targets, err)
			return nil, err
		}

		// B. Se for Revoke, validar segurança contra lockout de identities protegidas (D7)
		if isRevoke && s.protected != nil {
			for _, priv := range change.Privileges {
				if !s.protected.ValidateRevokeSafety(ws, change.Principal, priv) {
					err := domain.NewProtectedPrincipalError(change.Principal)
					s.recordAuditFailure(ctx, eventID, ws, metastoreID, actor, action, changeRef, requestID, &sec, targets, err)
					return nil, err
				}
			}
		}

		// C. Validar existência do principal (se identityAPI estiver configurada)
		if s.identityAPI != nil {
			p, err := s.identityAPI.GetPrincipal(ctx, ws, change.Principal)
			if err != nil || p == nil {
				notFoundErr := domain.NewPrincipalNotFoundError(change.Principal)
				s.recordAuditFailure(ctx, eventID, ws, metastoreID, actor, action, changeRef, requestID, &sec, targets, notFoundErr)
				return nil, notFoundErr
			}
		}
	}

	// 3. Capturar estado 'before' para auditoria
	var beforeGrants []domain.PrincipalPrivileges
	if current, err := s.grantsAPI.GetGrants(ctx, ws, sec, nil); err == nil {
		beforeGrants = current
	}

	// 4. Executar mutação no Databricks via Port
	outcomes, afterGrants, changed, err := s.grantsAPI.UpdateGrants(ctx, ws, sec, changes, isRevoke)
	if err != nil {
		s.recordAuditFailure(ctx, eventID, ws, metastoreID, actor, action, changeRef, requestID, &sec, targets, err)
		return nil, err
	}

	// 5. Gravar evento de auditoria de SUCESSO (D10)
	if s.auditStore != nil {
		_ = s.auditStore.Record(ctx, domain.AuditEvent{
			ID:          eventID,
			OccurredAt:  time.Now().UTC(),
			Workspace:   ws,
			MetastoreID: metastoreID,
			Actor:       actor,
			Action:      action,
			Outcome:     domain.OutcomeSuccess,
			ChangeRef:   changeRef,
			RequestID:   requestID,
			Securable:   &sec,
			Targets:     targets,
			Before: map[string]interface{}{
				"grants": beforeGrants,
			},
			After: map[string]interface{}{
				"grants": afterGrants,
			},
		})
	}

	return &domain.GrantChangeResult{
		Securable: sec,
		Outcomes:  outcomes,
		Grants:    afterGrants,
		Changed:   changed,
		AuditID:   eventID,
	}, nil
}

func (s *GrantService) recordAuditFailure(
	ctx context.Context,
	eventID, ws string,
	metastoreID *string,
	actor string,
	action domain.AuditAction,
	changeRef, requestID string,
	sec *domain.SecurableRef,
	targets []string,
	err error,
) {
	if s.auditStore == nil {
		return
	}
	detail := err.Error()
	errType := "/problems/error"
	if pErr, ok := err.(*domain.ProblemError); ok {
		detail = pErr.Detail
		errType = pErr.Type
	}

	_ = s.auditStore.Record(ctx, domain.AuditEvent{
		ID:          eventID,
		OccurredAt:  time.Now().UTC(),
		Workspace:   ws,
		MetastoreID: metastoreID,
		Actor:       actor,
		Action:      action,
		Outcome:     domain.OutcomeDenied,
		ChangeRef:   changeRef,
		RequestID:   requestID,
		Securable:   sec,
		Targets:     targets,
		Error: &domain.AuditError{
			Type:   errType,
			Detail: detail,
		},
	})
}
