package databricks

import (
	"os"
	"testing"
)

func TestHasDatabricksCredentials(t *testing.T) {
	// Limpa variáveis do teste
	clearEnv := func() {
		os.Unsetenv("DATABRICKS_HOST")
		os.Unsetenv("DATABRICKS_CLIENT_ID")
		os.Unsetenv("DATABRICKS_CLIENT_SECRET")
		os.Unsetenv("DATABRICKS_WS_TEST_HOST")
		os.Unsetenv("DATABRICKS_WS_TEST_CLIENT_ID")
		os.Unsetenv("DATABRICKS_WS_TEST_CLIENT_SECRET")
	}

	clearEnv()
	defer clearEnv()

	// 1. Sem credenciais
	if HasDatabricksCredentials() {
		t.Errorf("esperava falso quando não há credenciais no ambiente")
	}

	// 2. Apenas HOST incompleto
	os.Setenv("DATABRICKS_HOST", "https://dbc-test.cloud.databricks.com")
	if HasDatabricksCredentials() {
		t.Errorf("esperava falso quando credenciais estão incompletas")
	}

	// 3. Credenciais padrão completas
	os.Setenv("DATABRICKS_CLIENT_ID", "client-id-123")
	os.Setenv("DATABRICKS_CLIENT_SECRET", "secret-456")
	if !HasDatabricksCredentials() {
		t.Errorf("esperava verdadeiro quando DATABRICKS_HOST, CLIENT_ID e CLIENT_SECRET estão definidos")
	}

	// 4. Sem padrão, mas com workspace específico
	clearEnv()
	os.Setenv("DATABRICKS_WS_TEST_HOST", "https://dbc-ws.cloud.databricks.com")
	if !HasDatabricksCredentials() {
		t.Errorf("esperava verdadeiro quando DATABRICKS_WS_*_HOST está definido")
	}
}

func TestLoadCredentialsFromEnv(t *testing.T) {
	clearEnv := func() {
		os.Unsetenv("DATABRICKS_HOST")
		os.Unsetenv("DATABRICKS_CLIENT_ID")
		os.Unsetenv("DATABRICKS_CLIENT_SECRET")
		os.Unsetenv("DATABRICKS_WS_PROD_BR_HOST")
		os.Unsetenv("DATABRICKS_WS_PROD_BR_CLIENT_ID")
		os.Unsetenv("DATABRICKS_WS_PROD_BR_CLIENT_SECRET")
		os.Unsetenv("DATABRICKS_WS_DEV_BR_HOST")
	}

	clearEnv()
	defer clearEnv()

	// Configura credenciais globais e multi-workspace
	os.Setenv("DATABRICKS_HOST", "https://dbc-default.cloud.databricks.com")
	os.Setenv("DATABRICKS_CLIENT_ID", "default-client-id")
	os.Setenv("DATABRICKS_CLIENT_SECRET", "default-secret")

	os.Setenv("DATABRICKS_WS_PROD_BR_HOST", "https://dbc-prod-br.cloud.databricks.com")
	os.Setenv("DATABRICKS_WS_PROD_BR_CLIENT_ID", "prod-client-id")
	os.Setenv("DATABRICKS_WS_PROD_BR_CLIENT_SECRET", "prod-secret")

	// dev-br sem client id/secret explícitos (deve herdar default)
	os.Setenv("DATABRICKS_WS_DEV_BR_HOST", "https://dbc-dev-br.cloud.databricks.com")

	creds := LoadCredentialsFromEnv()

	// Verifica workspace default
	defCred, ok := creds["default"]
	if !ok {
		t.Fatalf("esperava credencial 'default' presente")
	}
	if defCred.Host != "https://dbc-default.cloud.databricks.com" {
		t.Errorf("host default incorreto: %s", defCred.Host)
	}

	// Verifica workspace prod-br
	prodCred, ok := creds["prod-br"]
	if !ok {
		t.Fatalf("esperava credencial 'prod-br' presente")
	}
	if prodCred.Host != "https://dbc-prod-br.cloud.databricks.com" {
		t.Errorf("host prod-br incorreto: %s", prodCred.Host)
	}
	if prodCred.ClientID != "prod-client-id" {
		t.Errorf("clientID prod-br incorreto: %s", prodCred.ClientID)
	}
	if prodCred.ClientSecret != "prod-secret" {
		t.Errorf("clientSecret prod-br incorreto: %s", prodCred.ClientSecret)
	}

	// Verifica que também registrou com underscore
	if _, ok := creds["prod_br"]; !ok {
		t.Errorf("esperava chave 'prod_br' presente no mapa")
	}

	// Verifica que dev-br herdou credenciais default
	devCred, ok := creds["dev-br"]
	if !ok {
		t.Fatalf("esperava credencial 'dev-br' presente")
	}
	if devCred.Host != "https://dbc-dev-br.cloud.databricks.com" {
		t.Errorf("host dev-br incorreto: %s", devCred.Host)
	}
	if devCred.ClientID != "default-client-id" {
		t.Errorf("esperava que dev-br herdasse default-client-id, obteve %s", devCred.ClientID)
	}
	if devCred.ClientSecret != "default-secret" {
		t.Errorf("esperava que dev-br herdasse default-secret, obteve %s", devCred.ClientSecret)
	}
}
