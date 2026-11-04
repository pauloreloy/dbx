# ============================================================================
# Estágio 1: Builder (Instalação das dependências Python e wheel cache)
# ============================================================================
FROM python:3.12-slim AS builder

WORKDIR /build

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir --upgrade pip && \
    pip wheel --no-cache-dir --wheel-dir /build/wheels -r requirements.txt

# ============================================================================
# Estágio 2: Imagem final de execução minimalista e segura
# ============================================================================
FROM python:3.12-slim

WORKDIR /app

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PORT=8000 \
    PYTHONPATH=/app

# Instala curl para probes de healthcheck e cria usuário não-root
RUN apt-get update && apt-get install -y --no-install-recommends curl && \
    rm -rf /var/lib/apt/lists/* && \
    useradd --create-home --shell /bin/bash --uid 1000 appuser

# Copia e instala as wheels construídas no estágio builder
COPY --from=builder /build/wheels /wheels
RUN pip install --no-cache-dir /wheels/* && rm -rf /wheels

# Copia o código da aplicação
COPY app ./app

# Ajusta permissões e chaveia para usuário não-root
RUN chown -R appuser:appuser /app
USER appuser

EXPOSE 8000

# Healthcheck interno via probe /healthz
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8000/healthz || exit 1

ENTRYPOINT ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
