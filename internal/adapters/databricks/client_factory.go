package databricks

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/databricks/databricks-sdk-go"
	"github.com/pauloeloy/databricks-governance-service/internal/domain"
)

// WorkspaceCredentials define as credenciais OAuth M2M para um workspace.
type WorkspaceCredentials struct {
	Host         string
	ClientID     string
	ClientSecret string
}

// ClientFactory gerencia e armazena em cache clientes Databricks por workspace (D6).
type ClientFactory struct {
	mu          sync.RWMutex
	credentials map[string]WorkspaceCredentials
	clients     map[string]*databricks.WorkspaceClient
}

// NewClientFactory cria uma nova fábrica de clientes Databricks.
func NewClientFactory(creds map[string]WorkspaceCredentials) *ClientFactory {
	return &ClientFactory{
		credentials: creds,
		clients:     make(map[string]*databricks.WorkspaceClient),
	}
}

// SetClient permite injetar um WorkspaceClient para testes ou inicialização prévia.
func (f *ClientFactory) SetClient(ws string, client *databricks.WorkspaceClient) {
	f.mu.Lock()
	defer f.mu.Unlock()
	normWS := strings.ToLower(strings.TrimSpace(ws))
	f.clients[normWS] = client
}

// GetClient retorna ou inicializa o WorkspaceClient para o alias de workspace indicado.
func (f *ClientFactory) GetClient(ctx context.Context, ws string) (*databricks.WorkspaceClient, error) {
	normWS := strings.ToLower(strings.TrimSpace(ws))

	f.mu.RLock()
	client, exists := f.clients[normWS]
	f.mu.RUnlock()
	if exists {
		return client, nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Double-check após lock de escrita
	if client, exists = f.clients[normWS]; exists {
		return client, nil
	}

	cred, ok := f.credentials[normWS]
	if !ok {
		cred, ok = f.credentials[strings.ReplaceAll(normWS, "-", "_")]
	}
	if !ok {
		cred, ok = f.credentials[strings.ReplaceAll(normWS, "_", "-")]
	}
	if !ok {
		cred, ok = f.credentials["default"]
	}
	if !ok {
		return nil, domain.NewNotFoundError("Workspace credentials", ws)
	}

	cfg := &databricks.Config{
		Host:         cred.Host,
		ClientID:     cred.ClientID,
		ClientSecret: cred.ClientSecret,
	}

	newClient, err := databricks.NewWorkspaceClient(cfg)
	if err != nil {
		return nil, domain.NewInternalError(fmt.Sprintf("falha ao inicializar client Databricks para workspace '%s': %v", ws, err))
	}

	f.clients[normWS] = newClient
	return newClient, nil
}
