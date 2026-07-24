package research

import (
	"errors"
	"fmt"
)

// ResearchStatus is the terminal state of a research execution.
type ResearchStatus string

const (
	ResearchStatusSuccess  ResearchStatus = "success"
	ResearchStatusDegraded ResearchStatus = "degraded"
	ResearchStatusFailed   ResearchStatus = "failed"
)

// FailureCode identifies the stage that prevented Pharus from producing a
// trustworthy report. It is safe to expose through the CLI and API.
type FailureCode string

const (
	FailureCodeSearchUnavailable    FailureCode = "search_unavailable"
	FailureCodeEvidenceInsufficient FailureCode = "evidence_insufficient"
	FailureCodeEngineUnavailable    FailureCode = "engine_unavailable"
	FailureCodeVectorIndexing       FailureCode = "vector_indexing_failed"
	FailureCodeVectorRetrieval      FailureCode = "vector_retrieval_failed"
	FailureCodeSynthesis            FailureCode = "synthesis_failed"
	FailureCodeCitations            FailureCode = "citations_invalid"
)

// ResearchResult is the common terminal result used by direct execution, the
// daemon and MCP. A failed result never contains a generated report.
type ResearchResult struct {
	ResearchID      string           `json:"research_id"`
	Status          ResearchStatus   `json:"status"`
	Report          string           `json:"report,omitempty"`
	EvidenceCount   int              `json:"evidence_count"`
	Evidence        []Evidence       `json:"evidence,omitempty"`
	EvidenceMetrics EvidenceMetrics  `json:"evidence_metrics"`
	QueryPlan       []SearchQuery    `json:"query_plan,omitempty"`
	ResearchPlan    *ResearchPlan    `json:"research_plan,omitempty"`
	TaxonTree       *TaxmorphTree    `json:"taxon_tree,omitempty"`
	Warnings        []string         `json:"warnings,omitempty"`
	FailureCode     FailureCode      `json:"failure_code,omitempty"`
	PhaseDurations  map[string]int64 `json:"phase_durations_ms,omitempty"`
}

// ResearchError preserves a machine-readable failure code while retaining the
// underlying cause for logs and errors.Is/errors.As callers.
type ResearchError struct {
	Code FailureCode
	Err  error
}

func (e *ResearchError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e *ResearchError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// FailureCodeOf returns the stable failure code carried by a research error.
func FailureCodeOf(err error) FailureCode {
	var researchErr *ResearchError
	if errors.As(err, &researchErr) {
		return researchErr.Code
	}
	return ""
}

func failedResult(code FailureCode, evidenceCount int, warnings []string, err error) (*ResearchResult, error) {
	return &ResearchResult{
		Status:        ResearchStatusFailed,
		EvidenceCount: evidenceCount,
		Warnings:      warnings,
		FailureCode:   code,
	}, &ResearchError{Code: code, Err: err}
}
