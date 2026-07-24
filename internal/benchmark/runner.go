package benchmark

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/KyberixCo/Pharus/internal/research"
	"github.com/KyberixCo/Pharus/pkg/logger"
)

// EngineInterface defines the research engine execution capability needed for benchmarking.
type EngineInterface interface {
	ExecuteResearch(ctx context.Context, topic string) (string, error)
}

// BenchmarkRunner orchestrates dataset execution and RACE scoring.
type BenchmarkRunner struct {
	engine    EngineInterface
	evaluator *research.RACEEvaluator
}

// NewBenchmarkRunner creates a new instance of BenchmarkRunner.
func NewBenchmarkRunner(engine EngineInterface, evaluator *research.RACEEvaluator) *BenchmarkRunner {
	return &BenchmarkRunner{
		engine:    engine,
		evaluator: evaluator,
	}
}

// RunOptions configures benchmarking execution parameters.
type RunOptions struct {
	Limit      int
	PassCutoff float64 // Minimum overall RACE score considered passing (default 0.70)
}

// RunSuite executes a slice of DatasetItems and calculates aggregate metrics.
func (br *BenchmarkRunner) RunSuite(ctx context.Context, items []DatasetItem, opts RunOptions) (*BenchmarkSummary, error) {
	log := logger.Get()
	if br == nil || br.engine == nil {
		return nil, fmt.Errorf("benchmark runner requires a research engine")
	}
	if br.evaluator == nil {
		return nil, fmt.Errorf("benchmark runner requires a RACE evaluator")
	}
	if opts.PassCutoff < 0 || opts.PassCutoff > 1 {
		return nil, fmt.Errorf("pass cutoff must be between 0 and 1")
	}
	if opts.PassCutoff == 0 {
		opts.PassCutoff = 0.70
	}

	if opts.Limit > 0 && opts.Limit < len(items) {
		items = items[:opts.Limit]
	}

	runID := fmt.Sprintf("bench_%d", time.Now().Unix())
	startTime := time.Now()

	summary := &BenchmarkSummary{
		RunID:     runID,
		StartTime: startTime,
		BySuite:   make(map[string]SuiteMetrics),
		Results:   make([]BenchmarkResult, 0, len(items)),
	}

	suiteResultsMap := make(map[string][]BenchmarkResult)

	for _, item := range items {
		log.Info("[Benchmark] Running item", "id", item.ID, "suite", item.Suite, "topic", item.Topic)
		itemStart := time.Now()

		res := BenchmarkResult{
			ItemID:    item.ID,
			Suite:     item.Suite,
			Topic:     item.Topic,
			Category:  item.Category,
			Timestamp: itemStart,
		}

		reportContent, err := br.engine.ExecuteResearch(ctx, item.Topic)
		if err != nil {
			res.Success = false
			res.Error = fmt.Sprintf("research execution failed: %v", err)
			res.DurationMs = time.Since(itemStart).Milliseconds()
			summary.Results = append(summary.Results, res)
			suiteResultsMap[item.Suite] = append(suiteResultsMap[item.Suite], res)
			continue
		}
		res.GeneratedReport = reportContent
		words, citationCount, citationsPerSec := analyzeDepthMetrics(reportContent)
		res.WordCount = words
		res.CitationCount = citationCount
		res.CitationsPerSection = citationsPerSec

		// Evaluate report with RACE Evaluator
		score, err := br.evaluator.EvaluateReport(ctx, item.Topic, reportContent)
		if err != nil {
			log.Warn("[Benchmark] RACE evaluation failed for item", "id", item.ID, "error", err)
			res.Error = fmt.Sprintf("RACE evaluation failed: %v", err)
			res.DurationMs = time.Since(itemStart).Milliseconds()
			summary.Results = append(summary.Results, res)
			suiteResultsMap[item.Suite] = append(suiteResultsMap[item.Suite], res)
			continue
		}
		res.Score = score

		res.DurationMs = time.Since(itemStart).Milliseconds()
		res.Success = res.Score != nil && res.Score.OverallRACE >= opts.PassCutoff
		summary.Results = append(summary.Results, res)
		suiteResultsMap[item.Suite] = append(suiteResultsMap[item.Suite], res)
	}

	endTime := time.Now()
	summary.EndTime = endTime
	summary.TotalDuration = endTime.Sub(startTime).String()

	// Calculate metrics per suite and overall
	summary.Overall = br.calculateMetrics("Overall", summary.Results, opts.PassCutoff)
	for suiteName, resList := range suiteResultsMap {
		summary.BySuite[suiteName] = br.calculateMetrics(suiteName, resList, opts.PassCutoff)
	}

	return summary, nil
}

func (br *BenchmarkRunner) calculateMetrics(name string, results []BenchmarkResult, cutoff float64) SuiteMetrics {
	sm := SuiteMetrics{
		SuiteName:  name,
		TotalItems: len(results),
	}

	if len(results) == 0 {
		return sm
	}

	var sumRel, sumAuth, sumClar, sumEvid, sumOverall, sumDur float64
	var sumWords, sumCitations, sumCitationsPerSec float64
	var passed int

	for _, r := range results {
		if r.Success {
			sm.CompletedItems++
			passed++
		} else if r.Error != "" {
			sm.FailedItems++
		} else {
			sm.CompletedItems++
		}

		sumDur += float64(r.DurationMs)
		sumWords += float64(r.WordCount)
		sumCitations += float64(r.CitationCount)
		sumCitationsPerSec += r.CitationsPerSection

		if r.Score != nil {
			sumRel += r.Score.Relevance
			sumAuth += r.Score.Authenticity
			sumClar += r.Score.Clarity
			sumEvid += r.Score.Evidence
			sumOverall += r.Score.OverallRACE
		}
	}

	total := float64(len(results))
	sm.AvgWordCount = round(sumWords / total)
	sm.AvgCitationCount = round(sumCitations / total)
	sm.AvgCitationsPerSection = round(sumCitationsPerSec / total)
	sm.AvgRelevance = round(sumRel / total)
	sm.AvgAuthenticity = round(sumAuth / total)
	sm.AvgClarity = round(sumClar / total)
	sm.AvgEvidence = round(sumEvid / total)
	sm.AvgOverallRACE = round(sumOverall / total)
	sm.AvgDurationMs = round(sumDur / total)
	sm.PassRate = round((float64(passed) / total) * 100.0)

	return sm
}

var citationRegex = regexp.MustCompile(`\[\d+\]`)

func analyzeDepthMetrics(report string) (wordCount int, citationCount int, citationsPerSection float64) {
	words := len(strings.Fields(report))
	matches := citationRegex.FindAllString(report, -1)
	citationCount = len(matches)

	sections := 0
	lines := strings.Split(report, "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "#") {
			sections++
		}
	}
	if sections == 0 {
		sections = 1
	}
	citationsPerSection = math.Round((float64(citationCount)/float64(sections))*100) / 100
	return words, citationCount, citationsPerSection
}

func round(val float64) float64 {
	return math.Round(val*10000) / 10000
}
