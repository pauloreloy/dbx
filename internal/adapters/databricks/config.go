package databricks

import (
	"os"
	"strings"
)

// HasDatabricksCredentials verifica se credenciais do Databricks estão disponíveis no ambiente.
func HasDatabricksCredentials() bool {
	// 1. Verifica credenciais globais/padrão
	if os.Getenv("DATABRICKS_HOST") != "" &&
		os.Getenv("DATABRICKS_CLIENT_ID") != "" &&
		os.Getenv("DATABRICKS_CLIENT_SECRET") != "" {
		return true
	}

	// 2. Verifica se há variáveis específicas de algum workspace
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		if strings.HasPrefix(key, "DATABRICKS_WS_") && strings.HasSuffix(key, "_HOST") && parts[1] != "" {
			return true
		}
	}

	return false
}

// LoadCredentialsFromEnv analisa as variáveis de ambiente e extrai as credenciais Databricks
// tanto para o workspace padrão (DATABRICKS_*) quanto para múltiplos workspaces (DATABRICKS_WS_*).
func LoadCredentialsFromEnv() map[string]WorkspaceCredentials {
	creds := make(map[string]WorkspaceCredentials)

	defaultHost := os.Getenv("DATABRICKS_HOST")
	defaultClientID := os.Getenv("DATABRICKS_CLIENT_ID")
	defaultClientSecret := os.Getenv("DATABRICKS_CLIENT_SECRET")

	if defaultHost != "" && defaultClientID != "" && defaultClientSecret != "" {
		creds["default"] = WorkspaceCredentials{
			Host:         defaultHost,
			ClientID:     defaultClientID,
			ClientSecret: defaultClientSecret,
		}
	}

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		if !strings.HasPrefix(key, "DATABRICKS_WS_") || !strings.HasSuffix(key, "_HOST") {
			continue
		}

		// Extrai o alias entre DATABRICKS_WS_ e _HOST
		middle := strings.TrimPrefix(key, "DATABRICKS_WS_")
		middle = strings.TrimSuffix(middle, "_HOST")
		if middle == "" {
			continue
		}

		wsAliasUpper := middle
		wsAliasHyphen := strings.ToLower(strings.ReplaceAll(wsAliasUpper, "_", "-"))
		wsAliasUnder := strings.ToLower(wsAliasUpper)

		host := parts[1]
		clientID := os.Getenv("DATABRICKS_WS_" + wsAliasUpper + "_CLIENT_ID")
		clientSecret := os.Getenv("DATABRICKS_WS_" + wsAliasUpper + "_CLIENT_SECRET")

		// Se não especificado para o workspace, herda do padrão
		if clientID == "" {
			clientID = defaultClientID
		}
		if clientSecret == "" {
			clientSecret = defaultClientSecret
		}

		wc := WorkspaceCredentials{
			Host:         host,
			ClientID:     clientID,
			ClientSecret: clientSecret,
		}
		creds[wsAliasHyphen] = wc
		creds[wsAliasUnder] = wc
	}

	return creds
}
