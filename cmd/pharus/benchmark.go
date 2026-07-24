package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/KyberixCo/Pharus/internal/benchmark"
	"github.com/KyberixCo/Pharus/internal/i18n"
	"github.com/KyberixCo/Pharus/internal/research"
	"github.com/spf13/cobra"
)

var (
	benchSuite     string
	benchFile      string
	benchOutputDir string
	benchLimit     int
	benchCutoff    float64
)

var benchmarkCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Ejecución de automatización de benchmarks y evaluación RACE Score",
	Long: `El comando benchmark ejecuta una batería de evaluación sobre un dataset suministrado
y evalúa los reportes resultantes empleando la rúbrica RACE Score (LLM-as-a-judge).
Las suites embebidas son muestras de humo; para resultados comparables use un archivo de
dataset versionado mediante --file.`,
}

var benchmarkRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Ejecuta una batería de evaluación sobre un dataset especificado",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := benchmark.NewDatasetLoader()

		var items []benchmark.DatasetItem
		var err error

		if benchFile != "" {
			fmt.Printf(tr("📂 Loading dataset from file: %s\n", "📂 Cargando dataset desde archivo: %s\n"), benchFile)
			items, err = loader.LoadFromFile(benchFile)
		} else {
			fmt.Printf(tr("📦 Loading built-in benchmark suite: %s\n", "📦 Cargando benchmark suite embebido: %s\n"), benchSuite)
			items, err = loader.GetBuiltInSuite(benchSuite)
		}

		if err != nil {
			return fmt.Errorf(tr("error loading dataset: %w", "error cargando dataset: %w"), err)
		}

		fmt.Printf(tr("🚀 Starting evaluation of %d items...\n", "🚀 Iniciando evaluación de %d ítems...\n"), len(items))

		engine, err := research.NewEngine(cfg)
		if err != nil {
			return fmt.Errorf(tr("error configuring research engine: %w", "error configurando motor de investigación: %w"), err)
		}
		evaluator := research.NewRACEEvaluator(engine.LLMProvider())

		runner := benchmark.NewBenchmarkRunner(engine, evaluator)
		opts := benchmark.RunOptions{
			Limit:      benchLimit,
			PassCutoff: benchCutoff,
		}

		ctx := i18n.WithLanguage(context.Background(), cliLanguage)
		summary, err := runner.RunSuite(ctx, items, opts)
		if err != nil {
			return fmt.Errorf(tr("benchmark execution error: %w", "error en ejecución de benchmark: %w"), err)
		}

		// Export report
		if benchOutputDir == "" {
			benchOutputDir = filepath.Join(os.Getenv("HOME"), ".pharus", "benchmarks")
		}

		jsonPath := filepath.Join(benchOutputDir, fmt.Sprintf("%s_summary.json", summary.RunID))
		mdPath := filepath.Join(benchOutputDir, fmt.Sprintf("%s_report.md", summary.RunID))

		reporter := benchmark.NewBenchmarkReporter()
		if err := reporter.ExportJSON(summary, jsonPath); err != nil {
			fmt.Printf(tr("⚠️ Error exporting JSON: %v\n", "⚠️ Error exportando JSON: %v\n"), err)
		}
		if err := reporter.ExportMarkdown(summary, mdPath); err != nil {
			fmt.Printf(tr("⚠️ Error exporting Markdown: %v\n", "⚠️ Error exportando Markdown: %v\n"), err)
		}

		fmt.Println(tr("\n✅ Benchmark completed successfully.", "\n✅ Benchmark completado con éxito."))
		fmt.Printf("📊 Overall RACE Score: %.4f\n", summary.Overall.AvgOverallRACE)
		fmt.Printf(tr("🎯 Pass rate: %.2f%%\n", "🎯 Tasa de Aprobación: %.2f%%\n"), summary.Overall.PassRate)
		fmt.Printf(tr("💾 Markdown report saved to: %s\n", "💾 Reporte Markdown guardado en: %s\n"), mdPath)
		fmt.Printf(tr("💾 JSON metrics saved to: %s\n", "💾 Métricas JSON guardadas en: %s\n"), jsonPath)

		return nil
	},
}

var benchmarkListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista las baterías de benchmark predefinidas disponibles",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(tr("📋 Available sample suites (not the official datasets):", "📋 Suites de muestra disponibles (no son los datasets oficiales):"))
		fmt.Println("  • deep_research_bench : Muestra de investigación multidisciplinar")
		fmt.Println("  • browsecomp          : Muestra de búsqueda y navegación")
		fmt.Println("  • gaia                : Muestra de herramientas y ejecución de código")
		fmt.Println("  • hle                 : Muestra de razonamiento conceptual")
		fmt.Println("  • synthetic           : Muestra combinada de las áreas anteriores")
	},
}

func init() {
	benchmarkRunCmd.Flags().StringVarP(&benchSuite, "suite", "s", "synthetic", "Nombre de la suite predefinida (deep_research_bench, browsecomp, gaia, hle, synthetic)")
	benchmarkRunCmd.Flags().StringVarP(&benchFile, "file", "f", "", "Ruta a archivo dataset custom (.json o .jsonl)")
	benchmarkRunCmd.Flags().StringVarP(&benchOutputDir, "output", "o", "", "Directorio de destino para reportes exportados")
	benchmarkRunCmd.Flags().IntVarP(&benchLimit, "limit", "l", 0, "Límite máximo de ítems a evaluar (0 = sin límite)")
	benchmarkRunCmd.Flags().Float64VarP(&benchCutoff, "cutoff", "c", 0.70, "Puntaje mínimo de RACE Score considerado aprobado")

	benchmarkCmd.AddCommand(benchmarkRunCmd)
	benchmarkCmd.AddCommand(benchmarkListCmd)
	rootCmd.AddCommand(benchmarkCmd)
}
