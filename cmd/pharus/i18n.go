package main

import (
	"fmt"

	"github.com/KyberixCo/Pharus/internal/i18n"
	"github.com/spf13/cobra"
)

var cliLanguage = i18n.English

func tr(english, spanish string) string {
	return i18n.Select(cliLanguage, english, spanish)
}

func trf(english, spanish string, args ...any) string {
	return fmt.Sprintf(tr(english, spanish), args...)
}

func effectiveLanguage(cmd *cobra.Command) (i18n.Language, error) {
	requested := languageFlag
	if cmd != nil && !cmd.Flags().Changed("language") && !cmd.Flags().Changed("lang") && cfg != nil {
		requested = cfg.Language
	}
	language, err := i18n.Resolve(requested, nil)
	if err != nil {
		return "", err
	}
	cliLanguage = language
	return language, nil
}

func localizeCommandMetadata(language i18n.Language) {
	cliLanguage = language
	rootCmd.Short = tr(
		"Pharus: local Deep Research engine and resilient daemon for macOS",
		"Pharus: motor de Deep Research local y demonio resiliente para macOS",
	)
	rootCmd.Long = tr(
		"Pharus is a high-performance Deep Research engine written in Go.\nIt integrates macOS auto-configuration, an embedded vector database,\nlocal embeddings, and LLM-guided remote synthesis.",
		"Pharus es un motor de investigación profunda (Deep Research) de alto rendimiento en Go.\nIntegra autoconfiguración de macOS, base de datos vectorial embebida,\nembeddings locales y síntesis remota guiada por LLM.",
	)
	researchCmd.Use = tr("research [topic or question]", "research [tema o pregunta]")
	researchCmd.Short = tr(
		"Run deep research on a topic",
		"Ejecuta una investigación profunda sobre un tema",
	)
	researchCmd.Long = tr(
		"Run deep research using retrieved evidence.\n\nThe result ends as success, degraded, or failed. Failed research produces no report;\nuse the research_id from HTTP or MCP responses and the JSON logs to diagnose each phase\nwithout exposing the topic or retrieved content.",
		"Ejecuta una investigación profunda con evidencia recuperada.\n\nEl resultado termina como success, degraded o failed. Las investigaciones fallidas\nno generan informe; consulta el research_id en la respuesta HTTP o MCP y los logs\nJSON para diagnosticar cada fase sin exponer el tema ni el contenido recuperado.",
	)
	configCmd.Short = tr("Interactive wizard for configuring Pharus", "Asistente interactivo para configurar Pharus")
	setupCmd.Short = tr("Auto-configure macOS and Pharus dependencies", "Autoconfigura macOS y las dependencias de Pharus")
	doctorCmd.Short = tr("Run system, service, and connectivity health diagnostics", "Ejecuta diagnósticos de salud del sistema, servicios y conectividad")
	daemonCmd.Short = tr("Manage the background daemon lifecycle", "Gestiona el ciclo de vida del servidor demonio en background")
	daemonStartCmd.Short = tr("Register and start the daemon with launchd", "Registra e inicia el demonio con launchd")
	daemonStopCmd.Short = tr("Stop and uninstall the launchd daemon", "Detiene y desinstala el demonio de launchd")
	daemonStatusCmd.Short = tr("Show daemon status over its Unix domain socket", "Muestra el estado del demonio mediante su socket Unix")
	daemonRunCmd.Short = tr("Run the daemon server in the foreground", "Ejecuta el servidor demonio en primer plano")
	benchmarkCmd.Short = tr("Run benchmarks and RACE Score evaluation", "Ejecuta benchmarks y evaluación RACE Score")
	benchmarkRunCmd.Short = tr("Evaluate a specified dataset", "Ejecuta una evaluación sobre un dataset especificado")
	benchmarkListCmd.Short = tr("List available built-in benchmark suites", "Lista las suites de benchmark predefinidas")

	setFlagUsage(rootCmd, "config", tr("configuration file (default: $HOME/.pharus/config.yaml)", "archivo de configuración (por defecto: $HOME/.pharus/config.yaml)"))
	setFlagUsage(rootCmd, "language", tr("language: auto, en, or es", "idioma: auto, en o es"))
	setFlagUsage(rootCmd, "lang", tr("alias for --language", "alias de --language"))
	setFlagUsage(researchCmd, "direct", tr("run directly without the IPC daemon", "ejecuta directamente sin el demonio IPC"))
	setFlagUsage(researchCmd, "no-resume", tr("do not reuse prior session checkpoints (requires --direct)", "no reutiliza checkpoints anteriores (requiere --direct)"))
	setFlagUsage(researchCmd, "restart", tr("delete the topic checkpoint and start over (requires --direct)", "elimina el checkpoint del tema y comienza de cero (requiere --direct)"))
	setFlagUsage(researchCmd, "output", tr("Markdown report path (default: current directory)", "ruta del reporte Markdown (por defecto: directorio actual)"))
	setFlagUsage(researchCmd, "stdout", tr("also print the complete report to stdout", "también imprime el reporte completo en stdout"))
}

func setFlagUsage(command *cobra.Command, name, usage string) {
	if flag := command.Flags().Lookup(name); flag != nil {
		flag.Usage = usage
		return
	}
	if flag := command.PersistentFlags().Lookup(name); flag != nil {
		flag.Usage = usage
	}
}

func requestedLanguageFromArgs(args []string) string {
	for index, arg := range args {
		for _, name := range []string{"--language=", "--lang="} {
			if len(arg) > len(name) && arg[:len(name)] == name {
				return arg[len(name):]
			}
		}
		if (arg == "--language" || arg == "--lang") && index+1 < len(args) {
			return args[index+1]
		}
	}
	return "auto"
}
