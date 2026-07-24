package research

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/llm"
	"github.com/KyberixCo/Pharus/internal/vectordb"
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

func TestLiveResearch_CoSTORMAndSynthesis(t *testing.T) {
	loadEnvForTest()

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Error cargando configuración: %v", err)
	}

	if testing.Short() || (cfg.MiniMax.APIKey == "" && cfg.OpenAI.APIKey == "" && cfg.Anthropic.APIKey == "") {
		t.Skip("Saltando prueba en vivo Co-STORM (modo corto o sin API keys configuradas).")
	}

	provider, err := llm.NewProvider(cfg)
	if err != nil {
		t.Fatalf("Error creando proveedor LLM: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	topic := "Model Context Protocol (MCP) en arquitecturas de agentes autónomos"

	// 1. Probar generación de perspectivas de experto Co-STORM con MiniMax
	discourseMgr := NewDiscourseManager(provider)
	roles, err := discourseMgr.GeneratePerspectives(ctx, topic)
	if err != nil {
		t.Fatalf("Fallo GeneratePerspectives: %v", err)
	}

	t.Logf("Roles Co-STORM generados (%d):", len(roles))
	for _, r := range roles {
		t.Logf(" - Rol: %s | Perspectiva: %s", r.Name, r.Perspective)
	}

	if len(roles) == 0 {
		t.Errorf("Se esperaba al menos 1 rol generado por Co-STORM")
	}

	// 2. Probar síntesis de reporte final con MiniMax
	synthesizer := NewSynthesizer(provider)
	mockEvidence := []vectordb.SearchResult{
		{
			ID:      "doc_1",
			Content: "Model Context Protocol (MCP) es un estándar abierto desarrollado para conectar modelos de lenguaje con herramientas y recursos externos de forma segura.",
			Metadata: map[string]string{
				"url":   "https://modelcontextprotocol.io",
				"title": "Documentación Oficial MCP",
			},
		},
	}

	report, err := synthesizer.SynthesizeEnrichedReport(ctx, topic, mockEvidence, discourseMgr.ConceptMap, discourseMgr.InsightBank)
	if err != nil {
		t.Fatalf("Fallo la síntesis de reporte: %v", err)
	}

	t.Logf("Reporte sintetizado con éxito (Longitud: %d caracteres):\n%.300s...", len(report), report)

	if !strings.Contains(report, "Resumen") && !strings.Contains(report, "Investigación") && !strings.Contains(report, "MCP") {
		t.Errorf("El reporte generado no contiene las secciones esperadas")
	}
}
