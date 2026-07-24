package main

import (
	"fmt"
	"os"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/pkg/logger"
	"github.com/spf13/cobra"
)

func main() {
	var cfgFile string
	var cfg *config.Config

	var rootCmd = &cobra.Command{
		Use:   "pharus",
		Short: "Pharus: Motor de Deep Research Local y Demonio Resiliente para macOS",
		Long: `Pharus es un motor de investigación profunda (Deep Research) de alto rendimiento en Go.
Integra autoconfiguración de macOS con Homebrew, base de datos vectorial embebida (chromem-go),
embeddings locales y síntesis remota guiada por el LLM MiniMax (minimax-m3).`,
	}

	cobra.OnInitialize(func() {
		var err error
		cfg, err = config.LoadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
			cfg = config.DefaultConfig()
		}
		logger.InitLogger(cfg.LogLevel, cfg.LogsDir+"/pharus.log")
	})

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.pharus/config.yaml)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
