package main

import (
	"fmt"
	"os"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/i18n"
	"github.com/KyberixCo/Pharus/pkg/logger"
	"github.com/spf13/cobra"
)

var (
	cfgFile      string
	cfg          *config.Config
	languageFlag string
)

var rootCmd = &cobra.Command{
	Use:   "pharus",
	Short: "Pharus: Motor de Deep Research Local y Demonio Resiliente para macOS",
	Long: `Pharus es un motor de investigación profunda (Deep Research) de alto rendimiento en Go.
Integra autoconfiguración de macOS con Homebrew, base de datos vectorial embebida (chromem-go),
embeddings locales y síntesis remota guiada por el LLM MiniMax (minimax-m3).`,
}

func Execute() {
	language, err := i18n.Resolve(requestedLanguageFromArgs(os.Args[1:]), nil)
	if err != nil {
		language, _ = i18n.Resolve("auto", nil)
	}
	localizeCommandMetadata(language)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.pharus/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&languageFlag, "language", "auto", "language: auto, en, or es")
	rootCmd.PersistentFlags().StringVar(&languageFlag, "lang", "auto", "alias for --language")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		language, err := effectiveLanguage(cmd)
		if err != nil {
			return err
		}
		localizeCommandMetadata(language)
		return nil
	}
}

func initConfig() {
	var err error
	cfg, err = config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, tr("Warning: failed to load config: %v\n", "Advertencia: no se pudo cargar la configuración: %v\n"), err)
		cfg = config.DefaultConfig()
	}
	logger.InitLogger(cfg.LogLevel, cfg.LogsDir+"/pharus.log")
}
