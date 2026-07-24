package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFileDoesNotOverrideProcessEnvironment(t *testing.T) {
	const existingKey = "PHARUS_TEST_EXISTING_ENV"
	const missingKey = "PHARUS_TEST_MISSING_ENV"
	t.Setenv(existingKey, "from-process")
	t.Setenv(missingKey, "")

	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(existingKey+"=from-file\n"+missingKey+"=loaded\n"), 0600); err != nil {
		t.Fatal(err)
	}

	loadEnvFile(path)

	if got := os.Getenv(existingKey); got != "from-process" {
		t.Fatalf("process environment was overwritten: %q", got)
	}
	if got := os.Getenv(missingKey); got != "loaded" {
		t.Fatalf("missing environment value was not loaded: %q", got)
	}
}

func TestDefaultLanguageUsesAutomaticDetection(t *testing.T) {
	if got := DefaultConfig().Language; got != "auto" {
		t.Fatalf("expected automatic language by default, got %q", got)
	}
}
