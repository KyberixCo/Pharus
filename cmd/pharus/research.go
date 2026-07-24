package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/KyberixCo/Pharus/internal/daemon"
	"github.com/KyberixCo/Pharus/internal/i18n"
	"github.com/KyberixCo/Pharus/internal/research"
	"github.com/spf13/cobra"
)

var (
	directFlag   bool
	noResumeFlag bool
	restartFlag  bool
	outputFlag   string
	stdoutFlag   bool
	profileFlag  string
)

var researchCmd = &cobra.Command{
	Use:   "research [tema o pregunta]",
	Short: "Ejecuta una investigación profunda (Deep Research) sobre un tema",
	Long: `Ejecuta una investigación profunda con evidencia recuperada.

El resultado termina como success, degraded o failed. Las investigaciones fallidas
no generan informe; consulta el research_id en la respuesta HTTP o MCP y los logs
JSON para diagnosticar cada fase sin exponer el tema ni el contenido recuperado.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		language, err := effectiveLanguage(cmd)
		if err != nil {
			return err
		}
		topic := strings.Join(args, " ")
		profileValue := profileFlag
		if strings.TrimSpace(profileValue) == "" {
			profileValue = cfg.Research.DefaultProfile
		}
		profile, err := research.ParseProfile(profileValue)
		if err != nil {
			return err
		}
		fmt.Printf(tr("🔍 Starting Deep Research on: %q\n\n", "🔍 Iniciando Deep Research sobre: %q\n\n"), topic)

		ctx := research.WithProfile(i18n.WithLanguage(context.Background(), language), profile)
		start := time.Now()

		if directFlag {
			// Embedded direct execution without daemon IPC
			fmt.Println(tr("⚡ Direct execution mode (in-process)...", "⚡ Modo de ejecución directa (in-process)..."))
			if restartFlag {
				if err := research.ClearSavedResearchSessionForProfile(cfg.BaseDir, topic, language, profile); err != nil {
					return fmt.Errorf(tr("could not clear the previous session: %w", "no se pudo limpiar la sesión anterior: %w"), err)
				}
				fmt.Println(tr("🧹 Previous checkpoint discarded; research will start from scratch.", "🧹 Checkpoint anterior descartado; la investigación comenzará desde cero."))
			}
			if noResumeFlag {
				cfg.Research.ResumeSessions = false
			}
			fmt.Printf(tr("🛡️ LLM resilience: up to %d attempts; session resume: %t\n", "🛡️ Resiliencia LLM: hasta %d intentos; reanudación de sesión: %t\n"),
				cfg.LLM.RetryMaxAttempts,
				cfg.Research.ResumeSessions,
			)
			fmt.Printf(tr("🎚️ Research profile: %s\n", "🎚️ Perfil de investigación: %s\n"), profile)
			engine, err := research.NewEngine(cfg)
			if err != nil {
				return fmt.Errorf(tr("error initializing local engine: %w", "error inicializando motor local: %w"), err)
			}
			result, err := engine.ExecuteResearchResult(ctx, topic)
			if err != nil {
				return fmt.Errorf(tr("research failed (%s): %w", "investigación fallida (%s): %w"), research.FailureCodeOf(err), err)
			}
			return deliverResearchReport(topic, result.ResearchID, result.Report, time.Since(start))
		}
		if noResumeFlag || restartFlag {
			return fmt.Errorf("%s", tr("--no-resume and --restart are only available with --direct", "--no-resume y --restart sólo están disponibles junto con --direct"))
		}

		// Client-Daemon IPC execution
		fmt.Println(tr("📡 Sending task to the Pharus daemon over a Unix Domain Socket...", "📡 Enviando tarea al demonio Pharus mediante Unix Domain Socket..."))
		client := daemon.NewClient(cfg)
		res, err := client.SubmitResearchWithProfile(ctx, topic, string(profile))
		if err != nil {
			fmt.Printf(tr("⚠️ The daemon is not responding (%v). Trying direct in-process mode...\n", "⚠️ El demonio no está respondiendo (%v). Intentando modo directo in-process...\n"), err)
			engine, err := research.NewEngine(cfg)
			if err != nil {
				return fmt.Errorf(tr("error initializing local engine: %w", "error inicializando motor local: %w"), err)
			}
			result, err := engine.ExecuteResearchResult(ctx, topic)
			if err != nil {
				return fmt.Errorf(tr("research failed (%s): %w", "investigación fallida (%s): %w"), research.FailureCodeOf(err), err)
			}
			return deliverResearchReport(topic, result.ResearchID, result.Report, time.Since(start))
		}

		if res.Status == string(research.ResearchStatusFailed) {
			return fmt.Errorf(tr("daemon research failed (%s): %s", "investigación fallida en demonio (%s): %s"), res.FailureCode, res.Error)
		}

		elapsed, _ := time.ParseDuration(res.Elapsed)
		return deliverResearchReport(topic, res.ResearchID, res.Report, elapsed)
	},
}

func deliverResearchReport(topic, researchID, report string, elapsed time.Duration) error {
	path, err := saveMarkdownReport(topic, researchID, report, outputFlag)
	if err != nil {
		return fmt.Errorf(tr("research completed, but the report could not be saved: %w", "la investigación terminó, pero no se pudo guardar el reporte: %w"), err)
	}
	fmt.Printf(tr("\n✅ Markdown report saved to: %s\n", "\n✅ Reporte Markdown guardado en: %s\n"), path)
	if elapsed > 0 {
		fmt.Printf(tr("⏱️ Elapsed time: %s\n", "⏱️ Tiempo transcurrido: %s\n"), elapsed)
	}
	if stdoutFlag {
		fmt.Println(tr("\n================ DEEP RESEARCH REPORT ================", "\n================ REPORTE DEEP RESEARCH ================"))
		fmt.Println(report)
		fmt.Println("=======================================================")
	}
	return nil
}

func saveMarkdownReport(topic, researchID, report, requestedPath string) (string, error) {
	if strings.TrimSpace(report) == "" {
		return "", fmt.Errorf("%s", tr("the report is empty", "el reporte está vacío"))
	}
	path := strings.TrimSpace(requestedPath)
	if path == "" {
		workingDir, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("obtener directorio actual: %w", err)
		}
		path = filepath.Join(workingDir, defaultReportFilename(topic, researchID))
	} else {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolver ruta de salida: %w", err)
		}
		path = absolute
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			path = filepath.Join(path, defaultReportFilename(topic, researchID))
		}
	}
	if !strings.EqualFold(filepath.Ext(path), ".md") {
		path += ".md"
	}

	parent := filepath.Dir(path)
	temp, err := os.CreateTemp(parent, ".pharus-report-*.tmp")
	if err != nil {
		return "", fmt.Errorf("crear archivo temporal en %s: %w", parent, err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0644); err != nil {
		_ = temp.Close()
		return "", err
	}
	content := report
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if _, err := temp.WriteString(content); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return "", fmt.Errorf("publicar reporte en %s: %w", path, err)
	}
	return path, nil
}

func defaultReportFilename(topic, researchID string) string {
	slug := reportSlug(topic)
	suffix := strings.TrimPrefix(researchID, "research_")
	if len(suffix) > 10 {
		suffix = suffix[:10]
	}
	if suffix == "" {
		sum := sha256.Sum256([]byte(topic + time.Now().UTC().Format(time.RFC3339Nano)))
		suffix = hex.EncodeToString(sum[:5])
	}
	return fmt.Sprintf("pharus-%s-%s.md", slug, suffix)
}

func reportSlug(topic string) string {
	var result strings.Builder
	previousDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(topic)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result.WriteRune(r)
			previousDash = false
		} else if !previousDash && result.Len() > 0 {
			result.WriteByte('-')
			previousDash = true
		}
		if result.Len() >= 64 {
			break
		}
	}
	slug := strings.Trim(result.String(), "-")
	if slug == "" {
		return "investigacion"
	}
	return slug
}

func init() {
	researchCmd.Flags().BoolVarP(&directFlag, "direct", "d", false, "Ejecuta directamente sin pasar por el demonio IPC")
	researchCmd.Flags().BoolVar(&noResumeFlag, "no-resume", false, "No reutiliza checkpoints de una sesión anterior (requiere --direct)")
	researchCmd.Flags().BoolVar(&restartFlag, "restart", false, "Elimina el checkpoint del tema y comienza desde cero (requiere --direct)")
	researchCmd.Flags().StringVarP(&outputFlag, "output", "o", "", "Ruta del reporte Markdown (por defecto, el directorio actual)")
	researchCmd.Flags().BoolVar(&stdoutFlag, "stdout", false, "También imprime el reporte completo en stdout")
	researchCmd.Flags().StringVar(&profileFlag, "profile", "", "Perfil de esfuerzo: quick, balanced o deep (por defecto: configuración o balanced)")
	rootCmd.AddCommand(researchCmd)
}
