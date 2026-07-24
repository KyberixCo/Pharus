package sysconfig

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/config"
)

func TestWriteManagedSearXNGSettingsEnablesJSONAndPreservesExistingFile(t *testing.T) {
	path, err := writeManagedSearXNGSettings(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	contents := string(data)
	if !strings.Contains(contents, "- html") || !strings.Contains(contents, "- json") {
		t.Fatalf("managed settings do not enable required formats: %s", contents)
	}
	if err := os.WriteFile(path, []byte("custom settings"), 0600); err != nil {
		t.Fatal(err)
	}
	secondPath, err := writeManagedSearXNGSettings(filepath.Dir(filepath.Dir(path)))
	if err != nil {
		t.Fatal(err)
	}
	if secondPath != path {
		t.Fatalf("expected stable settings path, got %s", secondPath)
	}
	data, _ = os.ReadFile(path)
	if string(data) != "custom settings" {
		t.Fatal("existing settings were overwritten")
	}
}

func TestEnsureManagedSearXNGRequiresExplicitMigration(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.BaseDir = t.TempDir()
	cfg.Search.SearXNGImage = managedSearXNGImage
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "info" {
			return nil, nil
		}
		return []byte("searxng/searxng:old|/tmp/old.yml:/etc/searxng/settings.yml;"), nil
	}
	_, err := ensureManagedSearXNG(context.Background(), cfg, run)
	if !errors.Is(err, ErrSearXNGMigrationRequired) {
		t.Fatalf("expected migration error, got %v", err)
	}
}

func TestEnsureManagedSearXNGIsIdempotentForManagedContainer(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.BaseDir = t.TempDir()
	cfg.Search.SearXNGImage = managedSearXNGImage
	settingsPath := filepath.Join(cfg.BaseDir, "searxng", "settings.yml")
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "info":
			return nil, nil
		case "inspect":
			return []byte(managedSearXNGImage + "|" + settingsPath + ":/etc/searxng/settings.yml:ro;"), nil
		default:
			t.Fatalf("setup should not create a duplicate container: docker %s", strings.Join(args, " "))
			return nil, nil
		}
	}
	path, err := ensureManagedSearXNG(context.Background(), cfg, run)
	if err != nil {
		t.Fatal(err)
	}
	if path != settingsPath {
		t.Fatalf("expected settings path %s, got %s", settingsPath, path)
	}
}

func TestIsManagedContainer(t *testing.T) {
	path := "/tmp/pharus/searxng/settings.yml"
	if !isManagedContainer(managedSearXNGImage+"|"+path+":/etc/searxng/settings.yml:ro;", managedSearXNGImage, path) {
		t.Fatal("expected managed container to be recognized")
	}
}
