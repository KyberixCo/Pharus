package research

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/KyberixCo/Pharus/internal/vectordb"
)

// TemporalMetadata holds time-based attributes for retrieved evidence.
type TemporalMetadata struct {
	PublicationDate     time.Time `json:"publication_date"`
	EventDate           time.Time `json:"event_date"`
	SequenceOrder       int       `json:"sequence_order"`
	TemporalExpressions []string  `json:"temporal_expressions"`
	FormattedDate       string    `json:"formatted_date"`
}

// TemporalEvent represents a single node in the Chronos Event Evolution Graph.
type TemporalEvent struct {
	ID          string    `json:"id"`
	Date        time.Time `json:"date"`
	Description string    `json:"description"`
	SourceURL   string    `json:"source_url"`
}

// ChronosGraph manages temporal extraction, event ordering, and narrative timeline formatting.
type ChronosGraph struct {
	events []TemporalEvent
}

// NewChronosGraph initializes a ChronosGraph service.
func NewChronosGraph() *ChronosGraph {
	return &ChronosGraph{
		events: make([]TemporalEvent, 0),
	}
}

var (
	isoDateRegex    = regexp.MustCompile(`\b(19\d\d|20\d\d)[-/.](0[1-9]|1[0-2])[-/.](0[1-9]|[12]\d|3[01])\b`)
	isoMonthRegex   = regexp.MustCompile(`\b(19\d\d|20\d\d)[-/.](0[1-9]|1[0-2])\b`)
	quarterRegex    = regexp.MustCompile(`(?i)\b(Q[1-4])\s*(?:de\s+)?(19\d\d|20\d\d)\b`)
	yearOnlyRegex   = regexp.MustCompile(`\b(19\d\d|20\d\d)\b`)
	monthYearRegex  = regexp.MustCompile(`(?i)\b(enero|febrero|marzo|abril|mayo|junio|julio|agosto|septiembre|octubre|noviembre|diciembre|january|february|march|april|may|june|july|august|september|october|november|december)\s+(?:de\s+)?(19\d\d|20\d\d)\b`)
)

// ExtractTemporalMetadata inspects raw text content and returns extracted temporal metadata.
func (cg *ChronosGraph) ExtractTemporalMetadata(content string, retrievedAt time.Time) TemporalMetadata {
	tm := TemporalMetadata{
		PublicationDate:     retrievedAt,
		TemporalExpressions: make([]string, 0),
	}

	if retrievedAt.IsZero() {
		retrievedAt = time.Now().UTC()
		tm.PublicationDate = retrievedAt
	}

	// 1. Try ISO date (YYYY-MM-DD)
	if match := isoDateRegex.FindString(content); match != "" {
		parsed, err := time.Parse("2006-01-02", strings.ReplaceAll(strings.ReplaceAll(match, "/", "-"), ".", "-"))
		if err == nil {
			tm.EventDate = parsed
			tm.FormattedDate = parsed.Format("2006-01-02")
			tm.TemporalExpressions = append(tm.TemporalExpressions, match)
			return tm
		}
	}

	// 2. Try Month Year (e.g. "marzo 2025" or "March 2025")
	if match := monthYearRegex.FindStringSubmatch(content); len(match) >= 3 {
		monthName := strings.ToLower(match[1])
		yearStr := match[2]
		monthNum := parseMonthName(monthName)
		yearNum, err := strconv.Atoi(yearStr)
		if err == nil && monthNum > 0 {
			parsed := time.Date(yearNum, time.Month(monthNum), 1, 0, 0, 0, 0, time.UTC)
			tm.EventDate = parsed
			tm.FormattedDate = fmt.Sprintf("%04d-%02d", yearNum, monthNum)
			tm.TemporalExpressions = append(tm.TemporalExpressions, match[0])
			return tm
		}
	}

	// 3. Try Quarter (e.g. "Q2 2025")
	if match := quarterRegex.FindStringSubmatch(content); len(match) >= 3 {
		qStr := strings.ToUpper(match[1])
		yearStr := match[2]
		yearNum, err := strconv.Atoi(yearStr)
		if err == nil {
			monthNum := 1
			switch qStr {
			case "Q1":
				monthNum = 1
			case "Q2":
				monthNum = 4
			case "Q3":
				monthNum = 7
			case "Q4":
				monthNum = 10
			}
			parsed := time.Date(yearNum, time.Month(monthNum), 1, 0, 0, 0, 0, time.UTC)
			tm.EventDate = parsed
			tm.FormattedDate = fmt.Sprintf("%s %04d", qStr, yearNum)
			tm.TemporalExpressions = append(tm.TemporalExpressions, match[0])
			return tm
		}
	}

	// 4. Try ISO Month (YYYY-MM)
	if match := isoMonthRegex.FindString(content); match != "" {
		parts := strings.Split(strings.ReplaceAll(strings.ReplaceAll(match, "/", "-"), ".", "-"), "-")
		if len(parts) == 2 {
			y, err1 := strconv.Atoi(parts[0])
			m, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil && m >= 1 && m <= 12 {
				parsed := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
				tm.EventDate = parsed
				tm.FormattedDate = fmt.Sprintf("%04d-%02d", y, m)
				tm.TemporalExpressions = append(tm.TemporalExpressions, match)
				return tm
			}
		}
	}

	// 5. Try Year Only (YYYY)
	if match := yearOnlyRegex.FindString(content); match != "" {
		y, err := strconv.Atoi(match)
		if err == nil && y >= 1990 && y <= 2035 {
			parsed := time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC)
			tm.EventDate = parsed
			tm.FormattedDate = fmt.Sprintf("%04d", y)
			tm.TemporalExpressions = append(tm.TemporalExpressions, match)
			return tm
		}
	}

	// Fallback to retrievedAt date
	tm.EventDate = retrievedAt
	tm.FormattedDate = retrievedAt.Format("2006-01-02")
	return tm
}

func parseMonthName(name string) int {
	switch name {
	case "enero", "january", "jan":
		return 1
	case "febrero", "february", "feb":
		return 2
	case "marzo", "march", "mar":
		return 3
	case "abril", "april", "apr":
		return 4
	case "mayo", "may":
		return 5
	case "junio", "june", "jun":
		return 6
	case "julio", "july", "jul":
		return 7
	case "agosto", "august", "aug":
		return 8
	case "septiembre", "september", "sep", "sept":
		return 9
	case "octubre", "october", "oct":
		return 10
	case "noviembre", "november", "nov":
		return 11
	case "diciembre", "december", "dec":
		return 12
	default:
		return 0
	}
}

// SortEvidenceChronologically orders evidence slices chronologically by EventDate/PublicationDate.
func (cg *ChronosGraph) SortEvidenceChronologically(items []Evidence) []Evidence {
	sorted := make([]Evidence, len(items))
	copy(sorted, items)

	sort.SliceStable(sorted, func(i, j int) bool {
		dateI := sorted[i].Temporal.EventDate
		if dateI.IsZero() {
			dateI = sorted[i].RetrievedAt
		}
		dateJ := sorted[j].Temporal.EventDate
		if dateJ.IsZero() {
			dateJ = sorted[j].RetrievedAt
		}
		return dateI.Before(dateJ)
	})

	for i := range sorted {
		sorted[i].Temporal.SequenceOrder = i + 1
	}

	return sorted
}

// FormatChronologicalTimeline generates a structured text timeline summary from evidence items.
func (cg *ChronosGraph) FormatChronologicalTimeline(items []vectordb.SearchResult) string {
	if len(items) == 0 {
		return "Sin evidencia suficiente para cronología."
	}

	var sb strings.Builder
	sb.WriteString("SECUENCIA Y CRONOLOGÍA DE EVENTOS REGISTRADOS:\n")

	for i, item := range items {
		dateStr := "Fecha N/A"
		if item.Metadata != nil && item.Metadata["temporal_formatted"] != "" {
			dateStr = item.Metadata["temporal_formatted"]
		} else if item.Metadata != nil && item.Metadata["event_date"] != "" {
			dateStr = item.Metadata["event_date"]
		}

		title := "Sin título"
		if item.Metadata != nil && item.Metadata["title"] != "" {
			title = item.Metadata["title"]
		}

		urlStr := ""
		if item.Metadata != nil && item.Metadata["url"] != "" {
			urlStr = item.Metadata["url"]
		}

		sb.WriteString(fmt.Sprintf("%d. [%s] Fuente [%d] %s (%s)\n", i+1, dateStr, i+1, title, urlStr))
	}

	return sb.String()
}
