package main

import (
	"context"
	"fmt"

	"github.com/KyberixCo/Pharus/internal/sysconfig"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Autoconfigura el sistema operativo macOS y las dependencias para Pharus",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(tr("🚀 Starting Pharus system auto-configuration...", "🚀 Inicializando autoconfiguración del sistema para Pharus..."))
		fmt.Println("------------------------------------------------------------")

		ctx := context.Background()
		results, err := sysconfig.AutoConfigureOS(ctx, cfg)
		if err != nil {
			fmt.Printf(tr("❌ Auto-configuration error: %v\n", "❌ Error durante la autoconfiguración: %v\n"), err)
			return
		}

		for _, res := range results {
			icon := "✅"
			if !res.Success {
				icon = "⚠️"
			}
			fmt.Printf("%s [%s]: %s\n", icon, res.Component, res.Message)
		}

		fmt.Println("------------------------------------------------------------")
		fmt.Printf(tr("✨ Configuration completed. Configuration file: %s\n", "✨ Configuración completada. Archivo de configuración: %s\n"), cfg.ConfigFile)
		fmt.Println(tr("👉 Verify the status by running: 'pharus doctor'", "👉 Puedes verificar el estado ejecutando: 'pharus doctor'"))
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
