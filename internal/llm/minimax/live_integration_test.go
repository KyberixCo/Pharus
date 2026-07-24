package minimax

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/llm/types"
)

func loadEnvForTest() {
	paths := []string{".env", "../../.env", "../../../.env"}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					val := strings.Trim(strings.TrimSpace(parts[1]), `"`)
					if os.Getenv(key) == "" && val != "" {
						os.Setenv(key, val)
					}
				}
			}
			break
		}
	}
}

func TestLiveMiniMax_SimpleCompletion(t *testing.T) {
	loadEnvForTest()

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.MiniMax.APIKey == "" {
		t.Skip("PHARUS_MINIMAX_API_KEY no está configurada en .env o en el entorno. Saltando prueba en vivo.")
	}

	client := NewClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	messages := []types.Message{
		{Role: "user", Content: "¿Cuál es la capital de Francia? Responde en una sola palabra."},
	}

	reply, err := client.GenerateCompletion(ctx, messages, 0.1)
	if err != nil {
		t.Fatalf("Error llamando a MiniMax Responses API: %v", err)
	}

	t.Logf("MiniMax Response: %s", reply)

	if !strings.Contains(strings.ToLower(reply), "parís") && !strings.Contains(strings.ToLower(reply), "paris") {
		t.Errorf("Se esperaba 'París' en la respuesta, pero se obtuvo: '%s'", reply)
	}
}

func TestLiveMiniMax_SystemInstructions(t *testing.T) {
	loadEnvForTest()

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.MiniMax.APIKey == "" {
		t.Skip("PHARUS_MINIMAX_API_KEY no está configurada. Saltando prueba en vivo.")
	}

	client := NewClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	messages := []types.Message{
		{Role: "system", Content: "Eres un asistente técnico especializado en el lenguaje de programación Go. Responde de forma muy concisa."},
		{Role: "user", Content: "¿Qué es una goroutine en Go?"},
	}

	reply, err := client.GenerateCompletion(ctx, messages, 0.2)
	if err != nil {
		t.Fatalf("Error llamando a MiniMax Responses API con sistema: %v", err)
	}

	t.Logf("MiniMax System Instruction Response:\n%s", reply)

	if len(reply) < 10 {
		t.Errorf("Respuesta inesperadamente corta: '%s'", reply)
	}
}
