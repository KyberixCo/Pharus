package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveMarkdownReportDefaultsToCurrentDirectory(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workingDir := t.TempDir()
	if err := os.Chdir(workingDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	path, err := saveMarkdownReport(
		"Avances en computación cuántica",
		"research_20094620499915cd897d3c25",
		"# Informe\n\nContenido.",
		"",
	)
	if err != nil {
		t.Fatalf("save report: %v", err)
	}
	reportDirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	workingDirInfo, err := os.Stat(workingDir)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(reportDirInfo, workingDirInfo) {
		t.Fatalf("expected report in current directory %s, got %s", workingDir, path)
	}
	if filepath.Base(path) != "pharus-avances-en-computación-cuántica-2009462049.md" {
		t.Fatalf("unexpected default filename: %s", filepath.Base(path))
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Informe\n\nContenido.\n" {
		t.Fatalf("unexpected report content: %q", content)
	}
}

func TestSaveMarkdownReportHonorsOutputDirectory(t *testing.T) {
	directory := t.TempDir()
	path, err := saveMarkdownReport("Tema", "research_abcdef", "# Reporte", directory)
	if err != nil {
		t.Fatalf("save report: %v", err)
	}
	if filepath.Dir(path) != directory || !strings.HasSuffix(path, ".md") {
		t.Fatalf("expected Markdown report inside requested directory, got %s", path)
	}
}

func TestSaveMarkdownReportAddsMarkdownExtension(t *testing.T) {
	path, err := saveMarkdownReport("Tema", "research_abcdef", "# Reporte", filepath.Join(t.TempDir(), "resultado"))
	if err != nil {
		t.Fatalf("save report: %v", err)
	}
	if filepath.Ext(path) != ".md" {
		t.Fatalf("expected .md extension, got %s", path)
	}
}

func TestSaveMarkdownReportRejectsEmptyReport(t *testing.T) {
	if _, err := saveMarkdownReport("Tema", "research_abcdef", "  ", ""); err == nil {
		t.Fatal("expected empty report to be rejected")
	}
}
