package sysconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type HFFileItem struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

// DownloadHFModelNative descarga de forma 100% nativa en Go cualquier modelo desde Hugging Face
// sin requerir Python, pip ni bibliotecas externas.
func DownloadHFModelNative(ctx context.Context, repoID string) error {
	fmt.Printf("\n🌐 Descargando nativamente desde Hugging Face (sin Python): %s...\n", repoID)

	client := &http.Client{Timeout: 0} // Sin timeout global para archivos grandes

	// 1. Obtener lista recursiva de archivos desde la API pública de HuggingFace
	treeURL := fmt.Sprintf("https://huggingface.co/api/models/%s/tree/main?recursive=true", repoID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, treeURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Pharus-Go-HFDownloader/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to query Hugging Face API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Hugging Face API returned status %d for repo %s", resp.StatusCode, repoID)
	}

	var files []HFFileItem
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return fmt.Errorf("failed to parse Hugging Face file tree: %w", err)
	}

	// 2. Definir directorio de destino en cache estándar de Hugging Face
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	repoPathSanitized := strings.ReplaceAll(repoID, "/", "--")
	targetDir := filepath.Join(home, ".cache", "huggingface", "hub", "models--"+repoPathSanitized, "snapshots", "main")

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory %s: %w", targetDir, err)
	}

	fmt.Printf("📁 Directorio de almacenamiento local: %s\n", targetDir)

	// 3. Descargar cada archivo del repositorio
	var downloadedCount int
	for _, f := range files {
		if f.Type != "file" {
			continue
		}

		localFilePath := filepath.Join(targetDir, f.Path)
		if err := os.MkdirAll(filepath.Dir(localFilePath), 0755); err != nil {
			continue
		}

		// Verificar si el archivo ya existe y tiene el tamaño correcto
		if info, err := os.Stat(localFilePath); err == nil && info.Size() > 0 && (f.Size == 0 || info.Size() == f.Size) {
			fmt.Printf("  • %-40s [Ya existe - Omitido]\n", f.Path)
			downloadedCount++
			continue
		}

		downloadURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repoID, f.Path)
		fmt.Printf("  ⬇️ Descargando %s (%s)... ", f.Path, formatBytes(f.Size))

		if err := downloadFileWithProgress(ctx, client, downloadURL, localFilePath); err != nil {
			fmt.Printf("❌ Error: %v\n", err)
		} else {
			fmt.Printf("✅ Completado\n")
			downloadedCount++
		}
	}

	fmt.Printf("🎉 ¡Descarga finalizada! %d archivos verificados/descargados en %s\n", downloadedCount, targetDir)
	return nil
}

func downloadFileWithProgress(ctx context.Context, client *http.Client, url, targetPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Pharus-Go-HFDownloader/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	out, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func formatBytes(b int64) string {
	if b <= 0 {
		return "N/A"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
