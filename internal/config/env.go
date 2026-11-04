package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadEnv carrega variáveis de um arquivo .env para as variáveis de ambiente do processo.
// Se paths não for fornecido, procura por ".env" no diretório atual de execução.
// Se o arquivo não existir, retorna nil silenciosamente sem erro.
// Não sobrescreve variáveis de ambiente que já estejam previamente definidas no sistema operacional.
func LoadEnv(paths ...string) error {
	filePath := ".env"
	if len(paths) > 0 && paths[0] != "" {
		filePath = paths[0]
	}

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Ignora linhas vazias e comentários
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Suporta sintaxe 'export KEY=VAL'
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}

		val := strings.TrimSpace(parts[1])

		// Trata aspas simples ou duplas envolventes
		if len(val) >= 2 {
			if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
				(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
				val = val[1 : len(val)-1]
			}
		}

		// Apenas define se a variável ainda não foi definida no ambiente
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}

	return scanner.Err()
}
