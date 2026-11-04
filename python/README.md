# Databricks Governance Service (Python / FastAPI)

Microsserviço de Governança corporativa para o **Databricks Unity Catalog**, implementado em **Python 3.12+ / FastAPI**, seguindo rigorosamente os princípios de **Arquitetura Hexagonal (Clean Architecture)**, validação de tipos com **Pydantic v2**, tratamento canônico de erros conforme a **RFC 9457 (`application/problem+json`)** e espelhando fielmente o microserviço em Go.

---

## 🏛️ Arquitetura e Estrutura do Projeto

O projeto adota a Arquitetura Hexagonal separando o núcleo de domínio de portas, adaptadores de infraestrutura e camada de transporte HTTP:

```
python/
├── app/
│   ├── domain/                  # Núcleo de Domínio e Regras de Negócio (Zero dependências externas)
│   │   ├── models.py            # Modelos Pydantic v2 (Catalog, Schema, Table, Volume, Workspace, SecurableRef)
│   │   ├── privilege.py         # Enum Privilege e PRIVILEGE_MATRIX por SecurableType
│   │   ├── principal.py         # Principal, PrincipalType e ProtectedPrincipalsRegistry (anti-lockout)
│   │   ├── audit.py             # Eventos imutáveis de auditoria (AuditEvent, AuditAction, AuditOutcome)
│   │   └── errors.py            # Hierarquia de exceções RFC 9457 (ProblemException e subclasses)
│   ├── ports/                   # Interfaces abstratas (ABCs)
│   │   ├── databricks.py        # ABCs: GrantsAPI, CatalogsAPI, SchemasAPI, TablesAPI, ViewsAPI, etc.
│   │   └── stores.py            # ABCs: AuditStore, IdempotencyStore, WorkspaceRegistry
│   ├── services/                # Casos de Uso da Aplicação
│   │   ├── grant_service.py     # Orquestração de concessão/revogação, matriz de privilégios e auditoria
│   │   ├── catalog_service.py   # Ciclo de vida DDL de catálogos, schemas, tabelas e volumes com confirmação
│   │   ├── audit_service.py     # Consulta da trilha global imutável de auditoria
│   │   └── workspace_service.py # Resolução de workspaces Databricks federados
│   ├── adapters/
│   │   └── memory/              # Implementações in-memory thread-safe para desenvolvimento e testes
│   │       ├── audit.py         # MemoryAuditStore e MemoryIdempotencyStore (TTL de 24h e cache)
│   │       ├── registry.py      # MemoryWorkspaceRegistry (prod-br, dev-br)
│   │       └── databricks.py    # MemoryDatabricksClient simulando o Unity Catalog
│   ├── transport/               # Camada HTTP / FastAPI
│   │   ├── middlewares.py       # RequestIDMiddleware, ChangeRefMiddleware e Exception Handlers RFC 9457
│   │   └── routers/             # Routers modulares da API
│   │       ├── health.py        # GET /healthz e GET /readyz
│   │       ├── workspaces.py    # GET /v1/workspaces e GET /v1/workspaces/{ws}
│   │       ├── audit.py         # GET /v1/audit/events e GET /v1/audit/events/{event_id}
│   │       ├── catalogs.py      # CRUD de catálogos e sub-recursos (schemas, tabelas, volumes, tags, donos)
│   │       └── grants.py        # Grants diretos e efetivos (/grants, /effective-grants, grant, revoke)
│   └── main.py                  # Ponto de entrada FastAPI, lifespan e injeção de dependências
├── tests/
│   ├── test_domain.py           # Testes unitários de regras de domínio (matriz, anti-lockout, regex)
│   └── test_api.py              # Testes de integração via TestClient
├── Dockerfile                   # Multi-stage build (python:3.12-slim, non-root user, curl healthcheck)
├── docker-compose.yaml          # Serviços governance-api-python (porta 8000) e postgres-python (porta 5433)
├── Makefile                     # Alvos para run, test, docker-up, docker-down, docker-logs
├── requirements.txt             # Dependências principais do projeto
└── README.md                    # Documentação do projeto
```

---

## 🔒 Principais Regras de Negócio e Governança

1. **Matriz de Privilégios do Unity Catalog (Decisão D6)**:
   - Validação estrita dos privilégios permitidos por tipo de objeto securable (`CATALOG`, `SCHEMA`, `TABLE`, `VIEW`, `VOLUME`).
   - Privilégios incompatíveis (como tentar conceder `USE_SCHEMA` diretamente a uma `TABLE`) são rejeitados com status HTTP `422 Unprocessable Entity` detalhando o erro RFC 9457.
2. **Anti-Lockout de Identidades Críticas (Decisão D7)**:
   - Identidades administrativas e Service Principals do próprio serviço são protegidas contra remoção inadvertida de privilégios essenciais (`MANAGE` ou `ALL_PRIVILEGES`).
   - Tentativas de revogação retornam `409 Conflict`.
3. **Cabeçalho de Rastreabilidade Obrigatório (`X-Change-Ref`)**:
   - Todas as mutações (`POST`, `PUT`, `PATCH`, `DELETE`) exigem a presença do cabeçalho `X-Change-Ref` (referência a chamado Jira, ticket ServiceNow ou PR Git). Requisições sem o cabeçalho retornam `400 Bad Request`.
4. **Idempotência em Operações DDL (Decisão D9)**:
   - Operações DDL suportam o cabeçalho `Idempotency-Key`. Replays da mesma requisição com a mesma chave retornam a resposta cacheada (`201 Created`).
5. **Prevenção de Deleção Acidental (Decisão D8)**:
   - Deleções de catálogos, schemas, tabelas e volumes exigem o parâmetro de query `confirm=<nome_exato>`. Divergências retornam `422 Unprocessable Entity`.
6. **Trilha Imutável de Auditoria (Decisão D10)**:
   - Toda operação (sucesso ou falha) registra um evento estruturado de auditoria com estado anterior (`before`) e posterior (`after`), ator, metastore_id, request_id e timestamp UTC.

---

## 🚀 Como Executar

### Pré-requisitos
- Python 3.12+
- Docker e Docker Compose (opcional)

### 1. Usando Ambiente Virtual Python (.venv)

```bash
cd python

# Cria o virtualenv e instala as dependências
python3 -m venv --system-site-packages .venv
.venv/bin/pip install -r requirements.txt

# Executa a suíte de testes com pytest
.venv/bin/pytest tests -v

# Ou via Makefile
make test

# Inicia a API localmente na porta 8000
make run
```

### 2. Usando Docker Compose

```bash
cd python

# Sobe os containers (API na porta 8000 e PostgreSQL na porta 5433)
docker compose up -d --build

# Verifica os logs
docker compose logs -f

# Derruba os containers
docker compose down
```

A documentação interativa Swagger/OpenAPI fica acessível em:
- **Swagger UI**: `http://localhost:8000/docs`
- **ReDoc**: `http://localhost:8000/redoc`
- **OpenAPI JSON**: `http://localhost:8000/openapi.json`

---

## 📡 Exemplos de Uso com cURL

### 1. Healthcheck e Liveness
```bash
# Liveness probe
curl -i -X GET http://localhost:8000/healthz

# Readiness probe
curl -i -X GET http://localhost:8000/readyz
```

### 2. Workspaces Federados
```bash
# Listar workspaces cadastrados
curl -i -X GET http://localhost:8000/v1/workspaces

# Consultar detalhes do workspace prod-br
curl -i -X GET http://localhost:8000/v1/workspaces/prod-br
```

### 3. Catálogos (DDL) com Idempotência
```bash
# Criar catálogo (exige X-Change-Ref e Idempotency-Key)
curl -i -X POST http://localhost:8000/v1/workspaces/prod-br/catalogs \
  -H "Content-Type: application/json" \
  -H "X-Change-Ref: JIRA-DATABRICKS-101" \
  -H "Idempotency-Key: idemp-cat-finance-001" \
  -d '{
    "name": "finance_lake",
    "comment": "Catálogo corporativo de dados financeiros"
  }'

# Replay da mesma requisição (retorna 201 via cache de idempotência)
curl -i -X POST http://localhost:8000/v1/workspaces/prod-br/catalogs \
  -H "Content-Type: application/json" \
  -H "X-Change-Ref: JIRA-DATABRICKS-101" \
  -H "Idempotency-Key: idemp-cat-finance-001" \
  -d '{
    "name": "finance_lake",
    "comment": "Catálogo corporativo de dados financeiros"
  }'

# Consultar o catálogo criado
curl -i -X GET http://localhost:8000/v1/workspaces/prod-br/catalogs/finance_lake
```

### 4. Gestão de Permissões (Grants)

```bash
# Conceder USE_CATALOG no catálogo para um grupo
curl -i -X POST http://localhost:8000/v1/workspaces/prod-br/catalogs/main/grants/grant \
  -H "Content-Type: application/json" \
  -H "X-Change-Ref: JIRA-SEC-450" \
  -d '{
    "changes": [
      {
        "principal": "grp-analytics",
        "privileges": ["USE_CATALOG"]
      }
    ]
  }'

# Consultar concessões diretas do catálogo
curl -i -X GET http://localhost:8000/v1/workspaces/prod-br/catalogs/main/grants

# Consultar concessões efetivas (com herança)
curl -i -X GET http://localhost:8000/v1/workspaces/prod-br/catalogs/main/effective-grants
```

### 5. Consulta da Trilha Imutável de Auditoria
```bash
# Consultar eventos filtrando por ticket de mudança
curl -i -X GET "http://localhost:8000/v1/audit/events?change_ref=JIRA-SEC-450"

# Consultar evento específico por ID
curl -i -X GET "http://localhost:8000/v1/audit/events/evt_e9c3e9818816409e86c06bdf255d65fe"
```

---

## 🛡️ Tratamento de Erros RFC 9457 (`application/problem+json`)

### Cenário A: Falta do cabeçalho `X-Change-Ref` (HTTP 400)
```bash
curl -i -X POST http://localhost:8000/v1/workspaces/prod-br/catalogs \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: idemp-01" \
  -d '{"name": "sales"}'
```
**Resposta (400 Bad Request)**:
```json
{
  "type": "/problems/bad-request",
  "title": "Requisição inválida",
  "status": 400,
  "detail": "O cabeçalho 'X-Change-Ref' é obrigatório para todas as operações de escrita (auditoria/ticket)",
  "instance": "/v1/workspaces/prod-br/catalogs",
  "request_id": "req_80b5fe68307446b19a3bfa993e54b68e",
  "errors": [
    {
      "field": "X-Change-Ref",
      "message": "Cabeçalho ausente ou vazio",
      "code": "missing-change-ref"
    }
  ]
}
```

### Cenário B: Violação da Matriz de Privilégios (HTTP 422)
```bash
curl -i -X POST http://localhost:8000/v1/workspaces/prod-br/catalogs/main/schemas/default/tables/orders/grants/grant \
  -H "Content-Type: application/json" \
  -H "X-Change-Ref: JIRA-SEC-999" \
  -d '{
    "changes": [
      {
        "principal": "grp-analytics",
        "privileges": ["USE_SCHEMA"]
      }
    ]
  }'
```
**Resposta (422 Unprocessable Entity)**:
```json
{
  "type": "/problems/privilege-not-applicable",
  "title": "Privilégio não aplicável",
  "status": 422,
  "detail": "Privilégios inválidos para o tipo de objeto TABLE",
  "instance": "/v1/workspaces/prod-br/catalogs/main/schemas/default/tables/orders/grants/grant",
  "request_id": "req_163be604588e4e94bca4e6260f8fb004",
  "errors": [
    {
      "field": "privileges",
      "message": "Privilégio USE_SCHEMA não se aplica a TABLE",
      "code": "privilege-not-applicable"
    }
  ]
}
```

### Cenário C: Proteção Anti-Lockout de Service Principal (HTTP 409)
```bash
curl -i -X POST http://localhost:8000/v1/workspaces/prod-br/catalogs/main/grants/revoke \
  -H "Content-Type: application/json" \
  -H "X-Change-Ref: JIRA-SEC-LOCKOUT" \
  -d '{
    "changes": [
      {
        "principal": "sp-governance-service",
        "privileges": ["MANAGE"]
      }
    ]
  }'
```
**Resposta (409 Conflict)**:
```json
{
  "type": "/problems/protected-principal",
  "title": "Principal protegido",
  "status": 409,
  "detail": "A identidade 'sp-governance-service' é protegida e não pode ter privilégios administrativos revogados",
  "instance": "/v1/workspaces/prod-br/catalogs/main/grants/revoke",
  "request_id": "req_c83d65b128544c6dbf6b86fe6a575b6e"
}
```

### Cenário D: Deleção sem Confirmação Exata (HTTP 422)
```bash
curl -i -X DELETE "http://localhost:8000/v1/workspaces/prod-br/catalogs/main?confirm=other_catalog" \
  -H "X-Change-Ref: JIRA-DEL-01"
```
**Resposta (422 Unprocessable Entity)**:
```json
{
  "type": "/problems/confirmation-mismatch",
  "title": "Confirmação incorreta",
  "status": 422,
  "detail": "O parâmetro confirm deve ser exatamente igual ao nome do objeto ('main'), mas recebeu 'other_catalog'",
  "instance": "/v1/workspaces/prod-br/catalogs/main",
  "request_id": "req_36c116d8084a4413b19a008c2eb8c156",
  "errors": [
    {
      "field": "confirm",
      "message": "Esperado 'main'",
      "code": "confirmation-mismatch"
    }
  ]
}
```
