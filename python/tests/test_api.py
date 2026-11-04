from fastapi.testclient import TestClient
import pytest

from app.main import create_app


@pytest.fixture
def client() -> TestClient:
    app = create_app()
    return TestClient(app)


def test_health_probes(client: TestClient) -> None:
    """Verifica os probes de liveness e readiness."""
    res_h = client.get("/healthz")
    assert res_h.status_code == 200
    assert res_h.json() == {"status": "UP"}
    assert "X-Request-Id" in res_h.headers

    res_r = client.get("/readyz")
    assert res_r.status_code == 200
    assert res_r.json() == {"status": "READY"}
    assert "X-Request-Id" in res_r.headers


def test_missing_change_ref_header_on_mutation(client: TestClient) -> None:
    """Mutation POST sem X-Change-Ref deve retornar 400 Bad Request RFC 9457."""
    res = client.post(
        "/v1/workspaces/prod-br/catalogs",
        json={"name": "test_cat"},
        headers={"Idempotency-Key": "idemp-001"},
    )
    assert res.status_code == 400
    assert "application/problem+json" in res.headers["content-type"]
    body = res.json()
    assert body["type"] == "/problems/bad-request"
    assert body["status"] == 400
    assert "X-Change-Ref" in body["errors"][0]["field"]


def test_catalog_create_and_idempotency(client: TestClient) -> None:
    """Criação de catálogo com suporte a idempotência e replay."""
    cat_body = {"name": "sales_cat", "comment": "Catálogo de vendas"}
    headers = {
        "X-Change-Ref": "TICKET-123",
        "Idempotency-Key": "idemp-sales-01",
    }

    # 1. Criação inicial
    res = client.post("/v1/workspaces/prod-br/catalogs", json=cat_body, headers=headers)
    assert res.status_code == 201
    assert res.headers.get("Location") == "/v1/workspaces/prod-br/catalogs/sales_cat"
    data = res.json()
    assert data["name"] == "sales_cat"

    # 2. Replay da requisição com a mesma chave de idempotência
    res_replay = client.post(
        "/v1/workspaces/prod-br/catalogs", json=cat_body, headers=headers
    )
    assert res_replay.status_code == 201
    data_replay = res_replay.json()
    assert data_replay["name"] == "sales_cat"


def test_privilege_not_applicable_rule(client: TestClient) -> None:
    """Tentativa de conceder USE_SCHEMA em TABLE deve ser rejeitada com 422."""
    grant_body = {
        "changes": [
            {
                "principal": "grp-analytics",
                "privileges": ["USE_SCHEMA"],
            }
        ]
    }
    headers = {"X-Change-Ref": "TICKET-999"}
    res = client.post(
        "/v1/workspaces/prod-br/catalogs/main/schemas/default/tables/orders/grants/grant",
        json=grant_body,
        headers=headers,
    )
    assert res.status_code == 422
    assert "application/problem+json" in res.headers["content-type"]
    body = res.json()
    assert body["type"] == "/problems/privilege-not-applicable"
    assert body["status"] == 422
    assert len(body["errors"]) > 0


def test_anti_lockout_protected_principal(client: TestClient) -> None:
    """Tentativa de revogar MANAGE de identidade protegida deve retornar 409 Conflict."""
    revoke_body = {
        "changes": [
            {
                "principal": "sp-governance-service",
                "privileges": ["MANAGE"],
            }
        ]
    }
    headers = {"X-Change-Ref": "SECURITY-01"}
    res = client.post(
        "/v1/workspaces/prod-br/catalogs/main/grants/revoke",
        json=revoke_body,
        headers=headers,
    )
    assert res.status_code == 409
    assert "application/problem+json" in res.headers["content-type"]
    body = res.json()
    assert body["type"] == "/problems/protected-principal"
    assert body["status"] == 409


def test_delete_without_confirmation(client: TestClient) -> None:
    """DELETE sem confirmação idêntica deve retornar 422."""
    headers = {"X-Change-Ref": "TICKET-DEL"}
    res = client.delete(
        "/v1/workspaces/prod-br/catalogs/main?confirm=wrong_name",
        headers=headers,
    )
    assert res.status_code == 422
    assert "application/problem+json" in res.headers["content-type"]
    body = res.json()
    assert body["type"] == "/problems/confirmation-mismatch"
    assert body["status"] == 422


def test_global_audit_trail_logging(client: TestClient) -> None:
    """Gravação de evento no log de auditoria e posterior consulta filtrada."""
    # 1. Executa um Grant válido
    grant_body = {
        "changes": [
            {
                "principal": "grp-analytics",
                "privileges": ["USE_CATALOG"],
            }
        ]
    }
    headers = {"X-Change-Ref": "TICKET-AUDIT-TEST"}
    res_grant = client.post(
        "/v1/workspaces/prod-br/catalogs/main/grants/grant",
        json=grant_body,
        headers=headers,
    )
    assert res_grant.status_code == 200
    grant_res = res_grant.json()
    assert grant_res["changed"] is True
    assert "audit_id" in grant_res

    # 2. Consulta a trilha de auditoria global por change_ref
    res_audit = client.get("/v1/audit/events?change_ref=TICKET-AUDIT-TEST")
    assert res_audit.status_code == 200
    audit_data = res_audit.json()
    assert len(audit_data["events"]) >= 1

    event = audit_data["events"][0]
    assert event["action"] == "GRANT"
    assert event["outcome"] == "SUCCESS"
    assert event["workspace"] == "prod-br"
    assert event["metastore_id"] == "meta-sa-east-1"
    assert event["change_ref"] == "TICKET-AUDIT-TEST"


def test_workspaces_endpoints(client: TestClient) -> None:
    """Consulta à listagem e detalhes de workspaces."""
    res_list = client.get("/v1/workspaces")
    assert res_list.status_code == 200
    workspaces = res_list.json()["workspaces"]
    assert len(workspaces) >= 2

    res_get = client.get("/v1/workspaces/prod-br")
    assert res_get.status_code == 200
    assert res_get.json()["id"] == "prod-br"
    assert res_get.json()["metastore_id"] == "meta-sa-east-1"
