package llm

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
	paths := []string{".env", "../.env", "../../.env"}
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

func TestLiveLLM_ProviderFactory(t *testing.T) {
	loadEnvForTest()

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Error cargando configuración: %v", err)
	}

	provider, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("Error creando proveedor LLM: %v", err)
	}

	if cfg.MiniMax.APIKey == "" && cfg.OpenAI.APIKey == "" && cfg.Anthropic.APIKey == "" {
		t.Skip("Sin API keys configuradas en el entorno. Saltando prueba en vivo.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	messages := []types.Message{
		{Role: "system", Content: "Eres un validador de integración de sistemas de IA."},
		{Role: "user", Content: "Responde exactamente 'OK: LLM Provider funcionando'."},
	}

	reply, err := provider.GenerateCompletion(ctx, messages, 0.1)
	if err != nil {
		t.Fatalf("Fallo la generación de texto con el proveedor activo (%s): %v", cfg.LLM.Provider, err)
	}

	t.Logf("Proveedor activo: %s | Respuesta: %s", cfg.LLM.Provider, reply)

	if len(reply) == 0 {
		t.Errorf("Respuesta vacía devuelta por el proveedor %s", cfg.LLM.Provider)
	}
}
