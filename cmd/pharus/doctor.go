package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/KyberixCo/Pharus/internal/sysconfig"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Ejecuta diagnósticos de salud del sistema, servicios y conectividad",
	Run: func(cmd *cobra.Command, args []string) {
		fix, _ := cmd.Flags().GetBool("fix")
		ctx := context.Background()
		embedProvider := strings.ToLower(cfg.Embed.Provider)
		if fix && embedProvider == "omlx" {
			fmt.Println(tr("🔧 Repairing the local OMLX embedding server...", "🔧 Reparando el servidor local de embeddings OMLX..."))
			if err := sysconfig.EnsureOMLX(ctx, cfg); err != nil {
				fmt.Printf(tr("⚠️ OMLX could not be repaired automatically: %v\n", "⚠️ No se pudo reparar OMLX automáticamente: %v\n"), err)
			} else {
				fmt.Println(tr("✅ OMLX is installed and running as a user service.", "✅ OMLX está instalado y ejecutándose como servicio de usuario."))
			}
		}

		fmt.Println(tr("🏥 Running Pharus health diagnostics...", "🏥 Ejecutando diagnósticos de salud de Pharus..."))
		fmt.Println("------------------------------------------------------------")

		checks := sysconfig.RunDiagnostics(ctx, cfg)

		allPassed := true
		for _, check := range checks {
			icon := "✅"
			if !check.Passed {
				icon = "❌"
				allPassed = false
			}
			fmt.Printf("%s %-32s : %s\n", icon, check.Name, check.Details)
		}

		fmt.Println("------------------------------------------------------------")
		if allPassed {
			fmt.Println(tr("🎉 All components are working and the system is ready!", "🎉 ¡Todos los componentes funcionan correctamente y el sistema está listo!"))
		} else {
			fmt.Println(tr("⚠️ Some warnings were detected. Review the details above.", "⚠️ Se detectaron algunas advertencias. Revisa los detalles arriba para solucionar."))
		}
	},
}

func init() {
	doctorCmd.Flags().Bool("fix", false, "instala e inicia automáticamente el servidor local OMLX cuando sea posible")
	rootCmd.AddCommand(doctorCmd)
}
