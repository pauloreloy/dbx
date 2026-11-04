# Databricks Governance Service

Microserviço interno em **Go** para governança de acessos e catálogo sobre o **Databricks Unity Catalog**, implementado com base na [ADR-0001](ADR-0001-databricks-governance-service.md) e no contrato OpenAPI 3.1 ([`openapi.yaml`](openapi.yaml)).

Construído sob os princípios de **Clean Code**, **Clean Architecture** e **Arquitetura Hexagonal (Ports & Adapters)**.

---

## 1. Visão Geral da Arquitetura

O serviço expõe uma API de domínio que desacopla os consumidores internos da API do Databricks, impondo regras corporativas essenciais:

```
[Consumidores HTTP]
        │
        ▼
[transport/http] (Mux Go 1.22+, RFC 9457, Middlewares: X-Request-Id, X-Change-Ref, Idempotency)
        │
        ▼
 [service] (Use Cases: GrantService, CatalogService, AuditService, WorkspaceService)
        │
        ▼
 [domain] (100% puro: Matriz de Privilégios, Anti-Lockout, Regras Invariantes, RFC 9457 Errors)
        │
   ┌────┴───────────────────────────┐
   ▼                                ▼
[ports/databricks]           [ports/stores]
   │                                │
   ▼                                ▼
[adapters/databricks]       [adapters/postgres ou memory]
(databricks-sdk-go / M2M)   (Auditoria Imutável e Idempotência)
```

---

## 2. Destaques das Regras de Domínio

1. **Matriz de Privilégios do Unity Catalog:** Antes de acionar qualquer chamada externa, o serviço valida se o privilégio solicitado é aplicável ao tipo de securable (ex.: `USE_SCHEMA` em uma `TABLE` é rejeitado na hora com status `422 Unprocessable Entity` e RFC 9457 `type: /problems/privilege-not-applicable`).
2. **Salvaguarda Anti-Lockout de Service Principals (D7):** O Service Principal do serviço (e identidades administrativas configuradas por workspace) nunca pode ter seus privilégios `MANAGE` ou `ALL_PRIVILEGES` revogados, evitando o clássico travamento operacional do Terraform/Pulumi.
3. **Multi-Workspace com Registry e Metastore Nuancy (D5):** Rotas sob `/v1/workspaces/{ws}` usam aliases internos (ex.: `prod-br`). Como o Unity Catalog é compartilhado por metastore, toda mutação registra o `metastore_id`, e a trilha de auditoria `/v1/audit/events` é global.
4. **Idempotência Estrita (D9):** Operações DDL exigem `Idempotency-Key` obrigatório com retenção de 24 horas, evitando criações duplicadas. Replays retornam a resposta cacheada.
5. **Rastreabilidade e Motivo Obrigatórios:** Mutações exigem o header `X-Change-Ref` (ticket JIRA / change request). Sem ele, a requisição é rejeitada com `400 Bad Request`.
6. **Proteção em Deleção:** Deleções exigem o parâmetro de query `confirm=<nome_do_objeto>`. Mismatches retornam `422 Unprocessable Entity`.

---

## 3. Estrutura do Projeto

```text
.
├── cmd/
│   └── api/
│       └── main.go                  # Entrypoint, injeção de dependências e graceful shutdown
├── internal/
│   ├── domain/                      # Núcleo de domínio puro (sem dependências externas)
│   │   ├── securable.go             # Entidades SecurableRef e regex de identificadores
│   │   ├── privilege.go             # Matriz de privilégios do Unity Catalog
│   │   ├── principal.go             # Identidades e regras anti-lockout de Service Principal
│   │   ├── audit.go                 # Modelos imutáveis de eventos de auditoria
│   │   ├── catalog_models.go        # Modelos de Catalog, Schema, Table, Volume
│   │   └── errors.go                # Erros canônicos e formatador RFC 9457 (Problem Details)
│   ├── ports/                       # Interfaces (Inversão de Dependência)
│   │   ├── databricks.go            # GrantsAPI, CatalogsAPI, SchemasAPI, TablesAPI, etc.
│   │   └── stores.go                # AuditStore, IdempotencyStore, WorkspaceRegistry
│   ├── service/                     # Casos de uso e orquestração
│   │   ├── grant_service.go         # Orquestração de concessão/revogação com auditoria
│   │   ├── catalog_service.go       # Ciclo de vida de catálogos, schemas, volumes, tags e owner
│   │   ├── audit_service.go         # Consulta da trilha global de auditoria
│   │   └── workspace_service.go     # Resolução de workspaces
│   ├── adapters/
│   │   ├── databricks/              # Integração real com databricks-sdk-go (OAuth M2M)
│   │   │   └── client_factory.go    # Pool e fábrica de WorkspaceClient com credenciais
│   │   └── memory/                  # Implementações em memória para testes e dev local
│   │       ├── audit.go             # MemoryAuditStore e MemoryIdempotencyStore com TTL
│   │       ├── registry.go          # MemoryWorkspaceRegistry
│   │       └── databricks.go        # Simulação in-memory autônoma do Databricks
│   └── transport/
│       └── http/                    # Camada HTTP
│           ├── problem.go           # Serializador RFC 9457 (application/problem+json)
│           ├── middlewares.go       # RequestID, ChangeRef obrigatório, Actor e Recovery
│           ├── handlers.go          # Handlers REST
│           └── router.go            # Mux nativo Go 1.22+ e registro de rotas
├── tests/
│   └── integration/
│       └── api_test.go              # Testes de integração end-to-end contra o contrato HTTP
├── ADR-0001-databricks-governance-service.md
├── openapi.yaml                     # Contrato OpenAPI 3.1
├── Makefile                         # Comandos de compilação, teste e execução
└── go.mod
```

---

## 4. Como Executar

### Pré-requisitos
- Go 1.22+ instalado.

### Execução via Docker Compose (Recomendado)
Para subir o microserviço junto com o PostgreSQL local em containers:
```bash
# Constrói a imagem e sobe a stack em background
make docker-up
# ou: docker compose up -d --build

# Acompanhar os logs
make docker-logs

# Parar os containers
make docker-down
```

### Compilação e Execução Local (Nativa)
```bash
# Executar todos os testes unitários e de integração
make test

# Verificar cobertura de testes
make coverage

# Compilar o binário
make build

# Iniciar o microserviço
make run
```

O servidor iniciará por padrão em `http://localhost:8080`.

---

## 5. Exemplos de Uso (cURL)

### 1. Health Checks
```bash
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

### 2. Listar Workspaces Registrados
```bash
curl -i http://localhost:8080/v1/workspaces
```

### 3. Criar Catálogo com Idempotência e Ticket
```bash
curl -i -X POST http://localhost:8080/v1/workspaces/prod-br/catalogs \
  -H "Content-Type: application/json" \
  -H "X-Change-Ref: JIRA-GOV-100" \
  -H "Idempotency-Key: idemp-catalog-analytics-01" \
  -d '{
    "name": "analytics",
    "comment": "Catálogo corporativo de Analytics"
  }'
```
*Retorna `201 Created` com header `Location: /v1/workspaces/prod-br/catalogs/analytics`.*

### 4. Conceder Permissões (`grant`)
```bash
curl -i -X POST http://localhost:8080/v1/workspaces/prod-br/catalogs/analytics/grants/grant \
  -H "Content-Type: application/json" \
  -H "X-Change-Ref: JIRA-GOV-101" \
  -d '{
    "changes": [
      {
        "principal": "grp-analytics",
        "privileges": ["USE_CATALOG", "SELECT"]
      }
    ]
  }'
```

### 5. Testar Anti-Lockout (Rejeição com 409 Conflict)
```bash
curl -i -X POST http://localhost:8080/v1/workspaces/prod-br/catalogs/analytics/grants/revoke \
  -H "Content-Type: application/json" \
  -H "X-Change-Ref: JIRA-GOV-102" \
  -d '{
    "changes": [
      {
        "principal": "sp-governance-service",
        "privileges": ["MANAGE"]
      }
    ]
  }'
```
*Retorna `409 Conflict` com Problem RFC 9457 (`/problems/protected-principal`).*

### 6. Consultar Trilha Global de Auditoria
```bash
curl -i "http://localhost:8080/v1/audit/events?workspace=prod-br"
```
