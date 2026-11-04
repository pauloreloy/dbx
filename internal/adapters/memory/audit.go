package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pauloeloy/databricks-governance-service/internal/domain"
	"github.com/pauloeloy/databricks-governance-service/internal/ports"
)

// MemoryAuditStore armazena eventos de auditoria em memória para testes e desenvolvimento.
type MemoryAuditStore struct {
	mu     sync.RWMutex
	events []domain.AuditEvent
	byID   map[string]domain.AuditEvent
}

func NewMemoryAuditStore() *MemoryAuditStore {
	return &MemoryAuditStore{
		events: make([]domain.AuditEvent, 0),
		byID:   make(map[string]domain.AuditEvent),
	}
}

func (s *MemoryAuditStore) Record(ctx context.Context, event domain.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append([]domain.AuditEvent{event}, s.events...) // mais recente primeiro
	s.byID[event.ID] = event
	return nil
}

func (s *MemoryAuditStore) ListEvents(ctx context.Context, filter ports.AuditFilter, pageSize int, pageToken string) ([]domain.AuditEvent, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []domain.AuditEvent
	for _, e := range s.events {
		if filter.Workspace != nil && e.Workspace != *filter.Workspace {
			continue
		}
		if filter.Actor != nil && e.Actor != *filter.Actor {
			continue
		}
		if filter.Action != nil && e.Action != *filter.Action {
			continue
		}
		if filter.ChangeRef != nil && e.ChangeRef != *filter.ChangeRef {
			continue
		}
		result = append(result, e)
	}

	if pageSize > 0 && len(result) > pageSize {
		result = result[:pageSize]
	}

	return result, "", nil
}

func (s *MemoryAuditStore) GetEvent(ctx context.Context, id string) (*domain.AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	event, ok := s.byID[id]
	if !ok {
		return nil, domain.NewNotFoundError("AuditEvent", id)
	}
	return &event, nil
}

// MemoryIdempotencyStore armazena chaves de idempotência com TTL.
type MemoryIdempotencyStore struct {
	mu      sync.Mutex
	records map[string]*ports.IdempotencyRecord
	expires map[string]time.Time
}

func NewMemoryIdempotencyStore() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{
		records: make(map[string]*ports.IdempotencyRecord),
		expires: make(map[string]time.Time),
	}
}

func (s *MemoryIdempotencyStore) key(scope, k string) string {
	return fmt.Sprintf("%s:%s", scope, k)
}

func (s *MemoryIdempotencyStore) Acquire(ctx context.Context, scope, k string, ttl time.Duration) (bool, *ports.IdempotencyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	composite := s.key(scope, k)
	exp, exists := s.expires[composite]
	if exists && time.Now().Before(exp) {
		rec, hasRec := s.records[composite]
		if hasRec && rec != nil {
			return false, rec, nil
		}
		// Em processamento por outra requisição concorrente
		return false, nil, nil
	}

	// Novo registro
	s.expires[composite] = time.Now().Add(ttl)
	s.records[composite] = nil // Em processamento
	return true, nil, nil
}

func (s *MemoryIdempotencyStore) Commit(ctx context.Context, scope, k string, statusCode int, responseBody []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	composite := s.key(scope, k)
	s.records[composite] = &ports.IdempotencyRecord{
		StatusCode: statusCode,
		Body:       responseBody,
		CreatedAt:  time.Now(),
	}
	return nil
}

func (s *MemoryIdempotencyStore) Release(ctx context.Context, scope, k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	composite := s.key(scope, k)
	delete(s.records, composite)
	delete(s.expires, composite)
	return nil
}
