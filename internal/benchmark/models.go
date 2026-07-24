package benchmark

import (
	"time"

	"github.com/KyberixCo/Pharus/internal/research"
)

// DatasetItem represents a single benchmark query or problem item.
type DatasetItem struct {
	ID           string            `json:"id"`
	Suite        string            `json:"suite"`                  // e.g., "deep_research_bench", "browsecomp", "gaia", "hle"
	Topic        string            `json:"topic"`                  // Main prompt / research question
	Category     string            `json:"category,omitempty"`     // Domain or area (e.g., "Physics", "Computer Science")
	GoldenAnswer string            `json:"golden_answer,omitempty"`// Golden reference answer if available
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// BenchmarkResult holds the execution output, performance metrics, and RACE evaluation score for a single item.
type BenchmarkResult struct {
	ItemID              string              `json:"item_id"`
	Suite               string              `json:"suite"`
	Topic               string              `json:"topic"`
	Category            string              `json:"category,omitempty"`
	GeneratedReport     string              `json:"generated_report,omitempty"`
	WordCount           int                 `json:"word_count"`
	CitationCount       int                 `json:"citation_count"`
	CitationsPerSection float64             `json:"citations_per_section"`
	Score               *research.RACEScore `json:"score,omitempty"`
	DurationMs          int64               `json:"duration_ms"`
	Success             bool                `json:"success"`
	Error               string              `json:"error,omitempty"`
	Timestamp           time.Time           `json:"timestamp"`
}

// SuiteMetrics holds aggregated performance, depth metrics, and RACE metrics for a specific benchmark suite or overall run.
type SuiteMetrics struct {
	SuiteName              string  `json:"suite_name"`
	TotalItems             int     `json:"total_items"`
	CompletedItems         int     `json:"completed_items"`
	FailedItems            int     `json:"failed_items"`
	AvgWordCount           float64 `json:"avg_word_count"`
	AvgCitationCount       float64 `json:"avg_citation_count"`
	AvgCitationsPerSection float64 `json:"avg_citations_per_section"`
	AvgRelevance           float64 `json:"avg_relevance"`
	AvgAuthenticity        float64 `json:"avg_authenticity"`
	AvgClarity             float64 `json:"avg_clarity"`
	AvgEvidence            float64 `json:"avg_evidence"`
	AvgOverallRACE         float64 `json:"avg_overall_race"`
	AvgDurationMs          float64 `json:"avg_duration_ms"`
	PassRate               float64 `json:"pass_rate"`
}

// BenchmarkSummary encapsulates the entire run summary including per-suite and overall metrics.
type BenchmarkSummary struct {
	RunID         string                  `json:"run_id"`
	StartTime     time.Time               `json:"start_time"`
	EndTime       time.Time               `json:"end_time"`
	TotalDuration string                  `json:"total_duration"`
	Overall       SuiteMetrics            `json:"overall"`
	BySuite       map[string]SuiteMetrics `json:"by_suite"`
	Results       []BenchmarkResult       `json:"results"`
}
