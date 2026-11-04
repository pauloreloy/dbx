# Guia de Instruções para Agentes de IA (AGENT.md)

Este documento estabelece as diretrizes de engenharia, arquitetura, segurança e boas práticas para agentes de IA que forem atuar no desenvolvimento, refatoração, manutenção ou expansão deste repositório.

---

## 1. Visão Geral do Repositório

O projeto consiste em um **Microserviço de Governança de Dados para o Databricks Unity Catalog**, projetado como uma **API de domínio intermediária** entre sistemas internos e a nuvem do Databricks.

O repositório possui uma **arquitetura dual (espelhada)**:
1. **Projeto Go (Raiz):** Implementação de referência em Go 1.22+, utilizando a biblioteca padrão `net/http` e o `databricks-sdk-go`.
2. **Projeto Python (`python/`):** Implementação espelho em Python 3.12+, utilizando FastAPI, Pydantic v2 e `databricks-sdk`.

Ambos os projetos implementam **exatamente o mesmo contrato REST** e as **mesmas regras de negócio**. Qualquer alteração de domínio ou contrato em um dos lados deve ser obrigatoriamente replicada no outro para preservar a paridade.

---

## 2. Fontes de Verdade e Contratos

Antes de realizar qualquer alteração, consulte as fontes canônicas de decisão:
* [**`ADR-0001-databricks-governance-service.md`**](ADR-0001-databricks-governance-service.md): Registro de Decisões de Arquitetura (ADR). Define o modelo conceitual, forças, trade-offs e fases de entrega.
* [**`openapi.yaml`**](openapi.yaml): Especificação OpenAPI 3.1 do contrato da API (52 rotas, 68 operações).
* [**`.env.example`**](.env.example) e [**`python/.env.example`**](python/.env.example): Templates de configuração de credenciais.

---

## 3. Invariantes e Regras de Domínio Obrigatórias

Todo agente que implementar novos fluxos ou modificar código deve garantir o cumprimento das seguintes invariantes:

### 3.1. Matriz de Privilégios do Unity Catalog
* Os privilégios não podem ser repassados cegamente ao Databricks. A matriz de aplicabilidade por tipo de securable (`CATALOG`, `SCHEMA`, `TABLE`, `VIEW`, `VOLUME`) deve ser validada **antes** de qualquer chamada de rede.
* Violações da matriz devem retornar HTTP **`422 Unprocessable Entity`** com RFC 9457 (`/problems/privilege-not-applicable`).

### 3.2. Salvaguarda Anti-Lockout de Service Principals (D7)
* O Service Principal do serviço (e identidades administrativas configuradas no `ProtectedPrincipalsRegistry`) **nunca** pode perder privilégios como `MANAGE` ou `ALL_PRIVILEGES`.
* Tentativas de revogar esses privilégios devem ser bloqueadas no domínio com HTTP **`409 Conflict`** (`/problems/protected-principal`).

### 3.3. Rastreabilidade com `X-Change-Ref` Obrigatório (D9)
* Qualquer mutação (`POST`, `PUT`, `PATCH`, `DELETE`) exige o cabeçalho `X-Change-Ref` contendo a referência da mudança (ticket JIRA, change request).
* Se ausente, o serviço deve rejeitar com HTTP **`400 Bad Request`**.

### 3.4. Idempotência em DDL (D9)
* Operações de criação estrutural (`POST` de catálogos, schemas, volumes) exigem o cabeçalho `Idempotency-Key`.
* Replays com a mesma chave dentro de 24 horas devem devolver a resposta cacheada sem reexecutar a criação.

### 3.5. Proteção de Deleção com Confirmação Exata
* Requisições de `DELETE` em securables estruturais exigem o parâmetro de query `confirm=<nome_completo>`.
* Deleções em lote/cascata (`force=true`) só são permitidas em `CATALOG` e `SCHEMA`.

### 3.6. Padronização de Erros (RFC 9457)
* Todas as respostas de erro (4xx e 5xx) devem utilizar o MIME type `application/problem+json; charset=utf-8`.
* Erros originados no Databricks devem ser encapsulados no campo `upstream` (`error_code`, `message`, `http_status`), sem expor credenciais ou stack traces sensíveis.

---

## 4. Arquitetura Hexagonal (Clean Architecture)

Mantenha a estrita separação entre camadas:

```
[Transport / Web] (HTTP handlers, middlewares, serialização RFC 9457)
       │
       ▼
[Service / Use Cases] (Orquestração, auditoria before/after, idempotência)
       │
       ▼
[Domain] (Entidades puras, matriz de privilégios, anti-lockout, zero dependências de framework)
       │
   ┌───┴───────────────────────────┐
   ▼                               ▼
[Ports] (Interfaces ABC/Go)      [Ports]
   │                               │
   ▼                               ▼
[Adapters Databricks SDK]        [Adapters Postgres / Memory]
```

* **Regra Fundamental:** O pacote `domain` **nunca** deve importar o SDK do Databricks, frameworks HTTP (FastAPI, Gin, Chi) ou drivers de banco de dados. Todas as integrações com mundo externo pertencem a `adapters/` e se comunicam através de interfaces em `ports/`.

---

## 5. Instruções Específicas por Linguagem

### 5.1. Projeto Go (Raiz)
* **Compilador:** Go 1.22+ (utilizar o roteador padrão `net/http` com pattern matching nativo em path).
* **Dependências:** Gerenciadas via `go.mod`. O SDK oficial é `github.com/databricks/databricks-sdk-go`.
* **Comandos Essenciais:**
  ```bash
  make test       # Executa testes unitários e de integração com detector de race conditions (-race)
  make build      # Compila o binário estático em bin/api
  make docker-up  # Sobe a stack Go + Postgres via Docker Compose
  ```

### 5.2. Projeto Python (`python/`)
* **Ambiente Virtual Obrigatório:** **Sempre** utilize o interpretador e os utilitários do ambiente virtual localizado em [**`python/.venv`**](python/.venv).
  * ⚠️ **Nunca** execute `pip install` globalmente ou rode `pytest` fora do virtualenv.
  * Comandos de terminal devem invocar explicitamente:
    - `.venv/bin/python`
    - `.venv/bin/pytest`
    - `.venv/bin/uvicorn`
    - ou executar a partir do `python/Makefile`.
* **Comandos Essenciais:**
  ```bash
  cd python
  make test       # Executa a suíte de 65+ testes via .venv/bin/pytest
  make run        # Inicia a API FastAPI com hot-reload na porta 8000
  make docker-up  # Sobe a stack independente FastAPI + Postgres na porta 5433
  ```

---

## 6. Autenticação e Credenciais Databricks (OAuth M2M)

* **Service Principal:** Toda comunicação real com o Databricks deve utilizar credenciais OAuth M2M lidas de variáveis de ambiente (`.env`):
  * `DATABRICKS_HOST`
  * `DATABRICKS_CLIENT_ID`
  * `DATABRICKS_CLIENT_SECRET`
* **Suporte Multi-Workspace:** Para múltiplos workspaces, utilize o padrão de prefixo:
  * `DATABRICKS_WS_<ALIAS_UPPER>_HOST`
  * `DATABRICKS_WS_<ALIAS_UPPER>_CLIENT_ID`
  * `DATABRICKS_WS_<ALIAS_UPPER>_CLIENT_SECRET`
* **Fallback Automático:** Quando credenciais não estiverem configuradas, ambos os projetos alternam automaticamente para o adapter `MemoryDatabricksClient`, permitindo testes e desenvolvimento local offline.
* ⚠️ **Segurança Estrita:** Nunca comite tokens, PATs ou secrets em arquivos de código ou documentação. Utilize sempre `.env` (que já está devidamente ignorado no `.gitignore`).

---

## 7. Checklist de Verificação para Agentes

Ao concluir qualquer tarefa ou implementação neste repositório, o agente deve executar o seguinte checklist antes de finalizar a resposta:

- [ ] Os testes do projeto Go passaram com 100% de sucesso? (`go test -v -race ./...`)
- [ ] O binário do Go compila sem warnings? (`make build` ou `go vet ./...`)
- [ ] Os testes do projeto Python passaram com 100% de sucesso usando o `.venv`? (`python/.venv/bin/pytest python/tests`)
- [ ] Novas rotas ou parâmetros foram refletidos no `openapi.yaml`?
- [ ] A paridade conceitual e funcional entre Go e Python foi mantida?
- [ ] O arquivo `.gitignore` protege todos os arquivos de ambiente (`.env`, `.venv`)?
- [ ] Nenhuma credencial em texto puro foi incluída em arquivos versionados?
