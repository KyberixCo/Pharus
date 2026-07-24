package research

import (
	"strings"
	"testing"
	"time"

	"github.com/KyberixCo/Pharus/internal/vectordb"
)

func TestChronosExtractTemporalMetadata(t *testing.T) {
	cg := NewChronosGraph()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		content      string
		wantFormatted string
		wantYear     int
		wantMonth    int
	}{
		{
			name:         "ISO Date YYYY-MM-DD",
			content:      "El anuncio se realizó el 2025-04-15 durante la conferencia principal.",
			wantFormatted: "2025-04-15",
			wantYear:     2025,
			wantMonth:    4,
		},
		{
			name:         "Spanish Month Year",
			content:      "Publicado en marzo de 2024 con nuevos datos de mercado.",
			wantFormatted: "2024-03",
			wantYear:     2024,
			wantMonth:    3,
		},
		{
			name:         "English Quarter",
			content:      "The feature was launched in Q3 2023 across all regions.",
			wantFormatted: "Q3 2023",
			wantYear:     2023,
			wantMonth:    7,
		},
		{
			name:         "Year Only",
			content:      "Estudio preliminar de la arquitectura en 2022.",
			wantFormatted: "2022",
			wantYear:     2022,
			wantMonth:    1,
		},
		{
			name:         "Fallback to Now",
			content:      "Este texto no contiene fechas explícitas ni menciones temporales.",
			wantFormatted: "2026-07-27",
			wantYear:     2026,
			wantMonth:    7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := cg.ExtractTemporalMetadata(tt.content, now)
			if tm.FormattedDate != tt.wantFormatted {
				t.Errorf("ExtractTemporalMetadata() FormattedDate = %q, want %q", tm.FormattedDate, tt.wantFormatted)
			}
			if tm.EventDate.Year() != tt.wantYear {
				t.Errorf("ExtractTemporalMetadata() EventDate.Year() = %d, want %d", tm.EventDate.Year(), tt.wantYear)
			}
			if int(tm.EventDate.Month()) != tt.wantMonth {
				t.Errorf("ExtractTemporalMetadata() EventDate.Month() = %d, want %d", int(tm.EventDate.Month()), tt.wantMonth)
			}
		})
	}
}

func TestChronosSortEvidenceChronologically(t *testing.T) {
	cg := NewChronosGraph()

	e1 := Evidence{
		ID:           "e1",
		CanonicalURL: "https://example.com/1",
		Content:      "Publicado el 2025-06-01.",
		Temporal:     TemporalMetadata{EventDate: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), FormattedDate: "2025-06-01"},
	}
	e2 := Evidence{
		ID:           "e2",
		CanonicalURL: "https://example.com/2",
		Content:      "Lanzamiento en 2023.",
		Temporal:     TemporalMetadata{EventDate: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), FormattedDate: "2023"},
	}
	e3 := Evidence{
		ID:           "e3",
		CanonicalURL: "https://example.com/3",
		Content:      "Actualización en 2026-01-15.",
		Temporal:     TemporalMetadata{EventDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), FormattedDate: "2026-01-15"},
	}

	items := []Evidence{e1, e2, e3}
	sorted := cg.SortEvidenceChronologically(items)

	if len(sorted) != 3 {
		t.Fatalf("expected 3 items, got %d", len(sorted))
	}
	if sorted[0].ID != "e2" {
		t.Errorf("expected first item to be e2 (2023), got %s", sorted[0].ID)
	}
	if sorted[1].ID != "e1" {
		t.Errorf("expected second item to be e1 (2025), got %s", sorted[1].ID)
	}
	if sorted[2].ID != "e3" {
		t.Errorf("expected third item to be e3 (2026), got %s", sorted[2].ID)
	}
	if sorted[0].Temporal.SequenceOrder != 1 || sorted[2].Temporal.SequenceOrder != 3 {
		t.Errorf("expected sequence orders 1..3, got %d..%d", sorted[0].Temporal.SequenceOrder, sorted[2].Temporal.SequenceOrder)
	}
}

func TestChronosFormatChronologicalTimeline(t *testing.T) {
	cg := NewChronosGraph()

	results := []vectordb.SearchResult{
		{
			ID:      "s1",
			Content: "Contenido 1",
			Metadata: map[string]string{
				"title":              "Artículo 2024",
				"url":                "https://example.com/2024",
				"temporal_formatted": "2024-05-10",
			},
		},
		{
			ID:      "s2",
			Content: "Contenido 2",
			Metadata: map[string]string{
				"title":              "Artículo 2025",
				"url":                "https://example.com/2025",
				"temporal_formatted": "2025-01-20",
			},
		},
	}

	timeline := cg.FormatChronologicalTimeline(results)

	if !strings.Contains(timeline, "SECUENCIA Y CRONOLOGÍA DE EVENTOS REGISTRADOS:") {
		t.Errorf("expected timeline header, got %q", timeline)
	}
	if !strings.Contains(timeline, "[2024-05-10]") {
		t.Errorf("expected date 2024-05-10 in timeline, got %q", timeline)
	}
	if !strings.Contains(timeline, "Artículo 2025") {
		t.Errorf("expected title in timeline, got %q", timeline)
	}
}
