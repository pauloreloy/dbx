package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pauloeloy/databricks-governance-service/internal/adapters/databricks"
	"github.com/pauloeloy/databricks-governance-service/internal/adapters/memory"
	"github.com/pauloeloy/databricks-governance-service/internal/config"
	"github.com/pauloeloy/databricks-governance-service/internal/domain"
	"github.com/pauloeloy/databricks-governance-service/internal/ports"
	"github.com/pauloeloy/databricks-governance-service/internal/service"
	transport "github.com/pauloeloy/databricks-governance-service/internal/transport/http"
)

func main() {
	// 0. Carrega variáveis de ambiente a partir do arquivo .env se presente
	if err := config.LoadEnv(); err != nil {
		log.Printf("Aviso: erro ao carregar arquivo .env: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Iniciando Databricks Governance Service v0.1.0 na porta %s...", port)

	// 1. Inicializa Registry de Workspaces (D5)
	workspaceRegistry := memory.NewMemoryWorkspaceRegistry([]domain.Workspace{
		{
			ID:                    "prod-br",
			DisplayName:           "Produção Brasil",
			DatabricksWorkspaceID: "7474652819690760",
			MetastoreID:           "meta-sa-east-1",
			Cloud:                 "AWS",
			Region:                "sa-east-1",
			Status:                "ACTIVE",
		},
		{
			ID:                    "dev-br",
			DisplayName:           "Desenvolvimento Brasil",
			DatabricksWorkspaceID: "8392019382019382",
			MetastoreID:           "meta-sa-east-1",
			Cloud:                 "AWS",
			Region:                "sa-east-1",
			Status:                "ACTIVE",
		},
	})

	// 2. Inicializa Regras Anti-Lockout (D7)
	protectedRegistry := domain.NewProtectedRegistry(map[string][]string{
		"prod-br": {"sp-governance-service", "admin@company.com"},
		"dev-br":  {"sp-governance-service"},
	})

	// 3. Inicializa Adapters de Armazenamento (D9, D10)
	auditStore := memory.NewMemoryAuditStore()
	idempotencyStore := memory.NewMemoryIdempotencyStore()

	// 4. Inicializa Adapter do Databricks
	// Se credenciais Databricks foram fornecidas no ambiente, inicializa o SDK Adapter real.
	// Caso contrário, faz fallback transparente para o cliente simulado em memória.
	var databricksClient ports.DatabricksClient
	if databricks.HasDatabricksCredentials() {
		log.Println("Credenciais Databricks encontradas no ambiente. Inicializando Databricks SDK Adapter real...")
		creds := databricks.LoadCredentialsFromEnv()
		factory := databricks.NewClientFactory(creds)
		databricksClient = databricks.NewDatabricksSDKAdapter(factory)
	} else {
		log.Println("Databricks credentials not found in env, using in-memory mock client")
		databricksClient = memory.NewMemoryDatabricksClient()
	}

	// 5. Inicializa Camada de Serviço (Casos de Uso)
	grantService := service.NewGrantService(
		databricksClient,
		databricksClient,
		protectedRegistry,
		auditStore,
		workspaceRegistry,
	)

	catalogService := service.NewCatalogService(
		databricksClient,
		databricksClient,
		databricksClient,
		databricksClient,
		databricksClient,
		databricksClient,
		databricksClient,
		databricksClient,
		auditStore,
		workspaceRegistry,
	)

	auditService := service.NewAuditService(auditStore)
	workspaceService := service.NewWorkspaceService(workspaceRegistry)

	// 6. Configura Handlers e Router HTTP
	handler := transport.NewHandler(
		grantService,
		catalogService,
		auditService,
		workspaceService,
		idempotencyStore,
	)
	router := transport.NewRouter(handler)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 7. Inicialização Assíncrona e Graceful Shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Servidor pronto para receber requisições em http://localhost:%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Erro fatal no servidor HTTP: %v", err)
		}
	}()

	<-stop
	log.Println("Sinal de encerramento recebido. Desligando graciosamente...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Erro durante o encerramento gracioso: %v", err)
	}

	log.Println("Servidor encerrado com sucesso.")
}
