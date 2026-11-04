# ============================================================================
# Estágio 1: Builder (Compilação estática em Go)
# ============================================================================
FROM golang:alpine AS builder

WORKDIR /src

# Habilita download automático de toolchain se o go.mod exigir versão mais recente
ENV GOTOOLCHAIN=auto

# Instala certificados e utilitários de compilação
RUN apk add --no-cache git ca-certificates tzdata

# Cache de dependências do Go Modules
COPY go.mod go.sum ./
RUN go mod download

# Copia todo o código fonte
COPY . .

# Compilação estática do binário Go
# - CGO_ENABLED=0 para portabilidade total e dependência zero de libc dinâmica
# - ldflags "-s -w" para remover símbolos de debug e reduzir o tamanho da imagem
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /bin/governance-api \
    ./cmd/api

# ============================================================================
# Estágio 2: Imagem final minimalista e segura
# ============================================================================
FROM alpine:3.21

# Instala certificados SSL e curl (para healthcheck dos probes)
RUN apk add --no-cache ca-certificates tzdata curl && \
    addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

# Copia o binário compilado do estágio builder
COPY --from=builder /bin/governance-api /app/governance-api

# Executa com usuário não-root por segurança
USER appuser

EXPOSE 8080

# Healthcheck interno via probe /healthz
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/app/governance-api"]
