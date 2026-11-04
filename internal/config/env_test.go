package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pauloeloy/databricks-governance-service/internal/config"
)

func TestLoadEnv(t *testing.T) {
	// Cria arquivo temporário .env
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `
# Linha de comentário
DATABRICKS_TEST_VAR_1=simple_value
export DATABRICKS_TEST_VAR_2="quoted_double"
DATABRICKS_TEST_VAR_3='quoted_single'
DATABRICKS_TEST_VAR_4=with=equals=sign
# Outro comentário
DATABRICKS_TEST_EXISTING=new_value
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("falha ao criar arquivo de teste: %v", err)
	}

	// Pré-define uma variável para testar que não é sobrescrita
	_ = os.Setenv("DATABRICKS_TEST_EXISTING", "original_value")
	defer os.Unsetenv("DATABRICKS_TEST_EXISTING")
	defer os.Unsetenv("DATABRICKS_TEST_VAR_1")
	defer os.Unsetenv("DATABRICKS_TEST_VAR_2")
	defer os.Unsetenv("DATABRICKS_TEST_VAR_3")
	defer os.Unsetenv("DATABRICKS_TEST_VAR_4")

	if err := config.LoadEnv(envPath); err != nil {
		t.Fatalf("LoadEnv retornou erro: %v", err)
	}

	if v := os.Getenv("DATABRICKS_TEST_VAR_1"); v != "simple_value" {
		t.Errorf("esperado 'simple_value', obteve '%s'", v)
	}
	if v := os.Getenv("DATABRICKS_TEST_VAR_2"); v != "quoted_double" {
		t.Errorf("esperado 'quoted_double', obteve '%s'", v)
	}
	if v := os.Getenv("DATABRICKS_TEST_VAR_3"); v != "quoted_single" {
		t.Errorf("esperado 'quoted_single', obteve '%s'", v)
	}
	if v := os.Getenv("DATABRICKS_TEST_VAR_4"); v != "with=equals=sign" {
		t.Errorf("esperado 'with=equals=sign', obteve '%s'", v)
	}
	if v := os.Getenv("DATABRICKS_TEST_EXISTING"); v != "original_value" {
		t.Errorf("esperado 'original_value' (não sobrescrito), obteve '%s'", v)
	}

	// Testa arquivo inexistente (deve retornar nil)
	if err := config.LoadEnv(filepath.Join(tmpDir, ".env.nonexistent")); err != nil {
		t.Errorf("esperado nil para arquivo inexistente, obteve %v", err)
	}
}
