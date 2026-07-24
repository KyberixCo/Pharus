package sysconfig

import (
	"context"
	"testing"
	"time"
)

func TestDownloadHFModelNative(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Probar consulta de árbol en un modelo micro en Hugging Face
	err := DownloadHFModelNative(ctx, "BAAI/bge-small-en-v1.5")
	if err != nil {
		t.Fatalf("Fallo la descarga nativa en Go desde Hugging Face: %v", err)
	}
}
