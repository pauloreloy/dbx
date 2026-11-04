.PHONY: venv run test coverage lint docker-build docker-up docker-down docker-logs clean

VENV := .venv
PYTHON := $(if $(wildcard $(VENV)/bin/python3),$(VENV)/bin/python3,python3)
PYTEST := $(if $(wildcard $(VENV)/bin/pytest),$(VENV)/bin/pytest,python3 -m pytest)
UVICORN := $(if $(wildcard $(VENV)/bin/uvicorn),$(VENV)/bin/uvicorn,python3 -m uvicorn)

venv:
	python3 -m venv --system-site-packages $(VENV)
	@chmod +x $(VENV)/bin/* 2>/dev/null || true

run:
	PYTHONPATH=. $(UVICORN) app.main:app --host 0.0.0.0 --port 8000 --reload

test:
	PYTHONPATH=. $(PYTEST) tests -v

coverage:
	PYTHONPATH=. $(PYTEST) tests --cov=app --cov-report=term-missing

docker-build:
	docker compose build

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f

clean:
	rm -rf __pycache__ .pytest_cache .coverage htmlcov
