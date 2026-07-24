package main

import (
	"context"
	"fmt"

	"github.com/KyberixCo/Pharus/internal/daemon"
	"github.com/KyberixCo/Pharus/internal/research"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Gestiona el ciclo de vida del servidor demonio en background",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Registra e inicia el demonio en background con launchd en macOS",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(tr("⚙️ Registering and starting the Pharus daemon in the background through launchd...", "⚙️ Registrando e iniciando demonio Pharus en background vía launchd..."))
		if err := daemon.InstallLaunchdService(cfg); err != nil {
			fmt.Printf(tr("❌ Error starting launchd service: %v\n", "❌ Error iniciando servicio launchd: %v\n"), err)
			return
		}
		fmt.Println(tr("✅ Daemon registered and started successfully.", "✅ Demonio registrado e iniciado correctamente."))
		fmt.Println(tr("👉 Check its status with: 'pharus daemon status'", "👉 Puedes consultar su estado con: 'pharus daemon status'"))
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Detiene el demonio en background y desinstala el servicio launchd",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(tr("🛑 Stopping the Pharus daemon service...", "🛑 Deteniendo servicio demonio Pharus..."))
		if err := daemon.UninstallLaunchdService(); err != nil {
			fmt.Printf(tr("⚠️ Details: %v\n", "⚠️ Detalle: %v\n"), err)
		}
		fmt.Println(tr("✅ Daemon service stopped.", "✅ Servicio demonio detenido."))
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Muestra el estado del demonio en background a través de Unix Domain Socket",
	Run: func(cmd *cobra.Command, args []string) {
		client := daemon.NewClient(cfg)
		ctx := context.Background()
		status, err := client.GetStatus(ctx)
		if err != nil {
			fmt.Printf(tr("❌ The daemon is not responding on UDS (%s): %v\n", "❌ El demonio no responde en UDS (%s): %v\n"), cfg.SocketPath, err)
			return
		}
		fmt.Println(tr("🟢 Pharus daemon status:", "🟢 Estado del Demonio Pharus:"))
		for k, v := range status {
			fmt.Printf("  • %-15s: %v\n", k, v)
		}
	},
}

var daemonRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Ejecuta el servidor demonio directamente en primer plano (usado por launchd/servicios)",
	Run: func(cmd *cobra.Command, args []string) {
		cfg.Language = string(cliLanguage)
		engine, err := research.NewEngine(cfg)
		if err != nil {
			fmt.Printf(tr("Fatal: could not initialize the research engine: %v\n", "Fatal: no se pudo inicializar el motor de investigación: %v\n"), err)
			return
		}

		server := daemon.NewServer(cfg, engine)
		ctx := context.Background()
		if err := server.Start(ctx); err != nil {
			fmt.Printf(tr("Daemon server error: %v\n", "Error en el servidor demonio: %v\n"), err)
		}
	},
}

func init() {
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonRunCmd)

	rootCmd.AddCommand(daemonCmd)
}
