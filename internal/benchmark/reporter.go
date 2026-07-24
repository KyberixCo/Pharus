package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BenchmarkReporter generates formatted outputs (JSON, Markdown) for benchmark runs.
type BenchmarkReporter struct{}

// NewBenchmarkReporter creates a new instance of BenchmarkReporter.
func NewBenchmarkReporter() *BenchmarkReporter {
	return &BenchmarkReporter{}
}

// ExportJSON writes the benchmark summary to a JSON file.
func (br *BenchmarkReporter) ExportJSON(summary *BenchmarkSummary, filePath string) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory for JSON report: %w", err)
	}

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal benchmark summary to JSON: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write JSON report to %s: %w", filePath, err)
	}
	return nil
}

// ExportMarkdown generates a Markdown summary report and writes it to disk.
func (br *BenchmarkReporter) ExportMarkdown(summary *BenchmarkSummary, filePath string) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory for Markdown report: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# 📊 Reporte de Evaluaciones y Benchmarking - Pharus\n\n")
	sb.WriteString(fmt.Sprintf("**ID de Ejecución:** `%s`  \n", summary.RunID))
	sb.WriteString(fmt.Sprintf("**Inicio:** %s  \n", summary.StartTime.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**Duración Total:** %s  \n\n", summary.TotalDuration))

	sb.WriteString("## 📈 Métricas Generales (RACE Score & Profundidad Analítica)\n\n")
	sb.WriteString("| Métrica | Valor Promedio |\n")
	sb.WriteString("| :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **Overall RACE Score** | **%.4f** |\n", summary.Overall.AvgOverallRACE))
	sb.WriteString(fmt.Sprintf("| Relevancia (Relevance) | %.4f |\n", summary.Overall.AvgRelevance))
	sb.WriteString(fmt.Sprintf("| Autenticidad (Authenticity) | %.4f |\n", summary.Overall.AvgAuthenticity))
	sb.WriteString(fmt.Sprintf("| Claridad (Clarity) | %.4f |\n", summary.Overall.AvgClarity))
	sb.WriteString(fmt.Sprintf("| Evidencia (Evidence) | %.4f |\n", summary.Overall.AvgEvidence))
	sb.WriteString(fmt.Sprintf("| **Conteo Promedio de Palabras** | **%.1f palabras** |\n", summary.Overall.AvgWordCount))
	sb.WriteString(fmt.Sprintf("| **Citas Promedio por Reporte** | **%.1f citas** |\n", summary.Overall.AvgCitationCount))
	sb.WriteString(fmt.Sprintf("| **Densidad de Citas (Citas/Sección)** | **%.2f** |\n", summary.Overall.AvgCitationsPerSection))
	sb.WriteString(fmt.Sprintf("| Tasa de Aprobación (Pass Rate) | %.2f%% |\n", summary.Overall.PassRate))
	sb.WriteString(fmt.Sprintf("| Tiempo Promedio por Ítem | %.2f ms |\n\n", summary.Overall.AvgDurationMs))

	if len(summary.BySuite) > 0 {
		sb.WriteString("## 🎯 Desglose por Benchmark Suite\n\n")
		sb.WriteString("| Suite | Ítems | RACE Score | Palabras Prom. | Citas Prom. | Citas/Sección | Pass Rate |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")
		for name, m := range summary.BySuite {
			sb.WriteString(fmt.Sprintf("| `%s` | %d | **%.4f** | %.1f | %.1f | %.2f | %.2f%% |\n",
				name, m.TotalItems, m.AvgOverallRACE, m.AvgWordCount, m.AvgCitationCount, m.AvgCitationsPerSection, m.PassRate))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## 📋 Detalle de Resultados por Ítem\n\n")
	sb.WriteString("| ID | Suite | Tópico | Estado | RACE Score | Palabras | Citas | Duración |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")
	for _, r := range summary.Results {
		status := "✅ PASS"
		if !r.Success {
			status = "❌ FAIL"
		}
		scoreStr := "N/A"
		if r.Score != nil {
			scoreStr = fmt.Sprintf("%.4f", r.Score.OverallRACE)
		}
		sb.WriteString(fmt.Sprintf("| `%s` | `%s` | %s | %s | %s | %d | %d | %d ms |\n",
			r.ItemID, r.Suite, truncate(r.Topic, 40), status, scoreStr, r.WordCount, r.CitationCount, r.DurationMs))
	}

	if err := os.WriteFile(filePath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("failed to write Markdown report to %s: %w", filePath, err)
	}
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
