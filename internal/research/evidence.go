package research

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/KyberixCo/Pharus/internal/scraper"
	"github.com/KyberixCo/Pharus/internal/vectordb"
)

// EvidenceType records whether the evidence is the retrieved page or only the
// search-engine excerpt. Snippets must never be presented as page content.
type EvidenceType string

const (
	EvidenceTypeFullText EvidenceType = "full_text"
	EvidenceTypeSnippet  EvidenceType = "snippet"
)

type ExtractionStatus string

const (
	ExtractionStatusSucceeded     ExtractionStatus = "succeeded"
	ExtractionStatusFetchFailed   ExtractionStatus = "fetch_failed"
	ExtractionStatusDistillFailed ExtractionStatus = "distill_failed"
)

// Evidence is the durable, traceable unit passed to later research stages.
type Evidence struct {
	ID               string           `json:"id"`
	CanonicalURL     string           `json:"canonical_url"`
	Domain           string           `json:"domain"`
	Title            string           `json:"title"`
	Content          string           `json:"content"`
	SourceQueries    []string         `json:"source_queries"`
	RetrievedAt      time.Time        `json:"retrieved_at"`
	Type             EvidenceType     `json:"type"`
	ExtractionStatus ExtractionStatus `json:"extraction_status"`
	ContentHash      string           `json:"content_hash"`
	Temporal         TemporalMetadata `json:"temporal"`
}

type EvidenceMetrics struct {
	ResultsFound    int            `json:"results_found"`
	PagesDownloaded int            `json:"pages_downloaded"`
	PagesDistilled  int            `json:"pages_distilled"`
	SnippetsUsed    int            `json:"snippets_used"`
	Duplicates      int            `json:"duplicates"`
	Failures        map[string]int `json:"failures,omitempty"`
}

type EvidencePolicy struct {
	MinimumUsable     int
	MinimumUniqueURLs int
	MinimumFullText   int
}

func DefaultEvidencePolicy() EvidencePolicy {
	return EvidencePolicy{MinimumUsable: 3, MinimumUniqueURLs: 2, MinimumFullText: 1}
}

func (p EvidencePolicy) normalized() EvidencePolicy {
	d := DefaultEvidencePolicy()
	if p.MinimumUsable <= 0 {
		p.MinimumUsable = d.MinimumUsable
	}
	if p.MinimumUniqueURLs <= 0 {
		p.MinimumUniqueURLs = d.MinimumUniqueURLs
	}
	if p.MinimumFullText <= 0 {
		p.MinimumFullText = d.MinimumFullText
	}
	return p
}

// EvidenceSufficiency tells the engine whether the corpus can be synthesized.
// Snippet-only corpora are usable but deliberately never reach success.
func EvidenceSufficiency(items []Evidence, policy EvidencePolicy) (ResearchStatus, error) {
	policy = policy.normalized()
	uniqueURLs, fullText := map[string]struct{}{}, 0
	for _, item := range items {
		if item.CanonicalURL != "" {
			uniqueURLs[item.CanonicalURL] = struct{}{}
		}
		if item.Type == EvidenceTypeFullText {
			fullText++
		}
	}
	if len(items) < policy.MinimumUsable || len(uniqueURLs) < policy.MinimumUniqueURLs {
		return ResearchStatusFailed, fmt.Errorf("usable evidence below policy minimums: evidence=%d/%d urls=%d/%d", len(items), policy.MinimumUsable, len(uniqueURLs), policy.MinimumUniqueURLs)
	}
	if fullText < policy.MinimumFullText {
		return ResearchStatusDegraded, nil
	}
	return ResearchStatusSuccess, nil
}

type pageFetcher interface {
	Fetch(context.Context, string) ([]byte, error)
}

// EvidenceCollector canonicalizes, deduplicates and classifies web results.
type EvidenceCollector struct {
	fetcher    pageFetcher
	researchID string
	now        func() time.Time
	byURL      map[string]int
	byHash     map[string]int
	items      []Evidence
}

func NewEvidenceCollector(fetcher pageFetcher, researchID string) *EvidenceCollector {
	return &EvidenceCollector{fetcher: fetcher, researchID: researchID, now: time.Now, byURL: map[string]int{}, byHash: map[string]int{}}
}

func (c *EvidenceCollector) Collect(ctx context.Context, query SearchQuery, results []scraper.SearchResult) ([]Evidence, EvidenceMetrics) {
	metrics := EvidenceMetrics{ResultsFound: len(results), Failures: map[string]int{}}
	items := make([]Evidence, 0, len(results))
	for _, result := range results {
		canonical, domain, err := canonicalURL(result.URL)
		if err != nil {
			metrics.Failures["invalid_url"]++
			continue
		}
		if index, ok := c.byURL[canonical]; ok {
			c.items[index].SourceQueries = appendQuery(c.items[index].SourceQueries, query.Text)
			metrics.Duplicates++
			continue
		}
		item := Evidence{CanonicalURL: canonical, Domain: domain, Title: strings.TrimSpace(result.Title), SourceQueries: []string{query.Text}, RetrievedAt: c.now().UTC()}
		var html []byte
		var fetchErr error
		if c.fetcher == nil {
			fetchErr = errors.New("page fetcher is not configured")
		} else {
			html, fetchErr = c.fetcher.Fetch(ctx, canonical)
		}
		if fetchErr != nil {
			metrics.Failures[string(ExtractionStatusFetchFailed)]++
			item.Type, item.ExtractionStatus = EvidenceTypeSnippet, ExtractionStatusFetchFailed
			item.Content = strings.TrimSpace(strings.TrimSpace(result.Title) + "\n" + strings.TrimSpace(result.Content))
			if item.Content == "" {
				metrics.Failures["empty_snippet"]++
				continue
			}
			metrics.SnippetsUsed++
		} else {
			metrics.PagesDownloaded++
			page, distillErr := scraper.DistillHTML(html, canonical)
			if distillErr != nil || len(strings.TrimSpace(page.TextContent)) < 50 {
				metrics.Failures[string(ExtractionStatusDistillFailed)]++
				item.Type, item.ExtractionStatus = EvidenceTypeSnippet, ExtractionStatusDistillFailed
				item.Content = strings.TrimSpace(strings.TrimSpace(result.Title) + "\n" + strings.TrimSpace(result.Content))
				if item.Content == "" {
					metrics.Failures["empty_snippet"]++
					continue
				}
				metrics.SnippetsUsed++
			} else {
				item.Type, item.ExtractionStatus = EvidenceTypeFullText, ExtractionStatusSucceeded
				item.Title, item.Content = firstNonEmpty(page.Title, item.Title), page.TextContent
				metrics.PagesDistilled++
			}
		}
		item.ContentHash = contentHash(item.Content)
		chronos := NewChronosGraph()
		item.Temporal = chronos.ExtractTemporalMetadata(item.Content, item.RetrievedAt)
		if index, ok := c.byHash[item.ContentHash]; ok {
			c.items[index].SourceQueries = appendQuery(c.items[index].SourceQueries, query.Text)
			metrics.Duplicates++
			continue
		}
		item.ID = evidenceID(c.researchID, item.CanonicalURL, item.ContentHash)
		c.byURL[canonical], c.byHash[item.ContentHash] = len(c.items), len(c.items)
		c.items = append(c.items, item)
		items = append(items, item)
	}
	return items, metrics
}

func (c *EvidenceCollector) Evidence() []Evidence {
	items := append([]Evidence(nil), c.items...)
	sortEvidenceQueries(items)
	return items
}

func (c *EvidenceCollector) Documents(items []Evidence) []vectordb.Document {
	docs := make([]vectordb.Document, 0, len(items))
	for _, item := range items {
		content := item.Content
		runes := []rune(content)
		if len(runes) > 16_000 {
			content = string(runes[:16_000])
		}
		eventDateStr := ""
		if !item.Temporal.EventDate.IsZero() {
			eventDateStr = item.Temporal.EventDate.Format(time.RFC3339)
		}
		docs = append(docs, vectordb.Document{ID: item.ID, Content: content, Metadata: map[string]string{
			"url":                item.CanonicalURL,
			"title":              item.Title,
			"domain":             item.Domain,
			"evidence_type":      string(item.Type),
			"extraction_status":  string(item.ExtractionStatus),
			"content_hash":       item.ContentHash,
			"queries":            strings.Join(item.SourceQueries, " | "),
			"temporal_formatted": item.Temporal.FormattedDate,
			"event_date":         eventDateStr,
		}})
	}
	return docs
}

func canonicalURL(raw string) (string, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("invalid URL %q", raw)
	}
	u.Fragment = ""
	u.Host = strings.ToLower(u.Host)
	u.Scheme = strings.ToLower(u.Scheme)
	if (u.Scheme == "http" && u.Port() == "80") || (u.Scheme == "https" && u.Port() == "443") {
		u.Host = u.Hostname()
	}
	q := u.Query()
	u.RawQuery = q.Encode()
	return u.String(), u.Hostname(), nil
}
func contentHash(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return hex.EncodeToString(sum[:])
}
func evidenceID(researchID, rawURL, hash string) string {
	sum := sha256.Sum256([]byte(researchID + "\x00" + rawURL + "\x00" + hash))
	return "evi_" + hex.EncodeToString(sum[:12])
}
func appendQuery(queries []string, query string) []string {
	for _, q := range queries {
		if q == query {
			return queries
		}
	}
	return append(queries, query)
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// MergeEvidenceMetrics aggregates collection events from separate query batches.
func MergeEvidenceMetrics(dst *EvidenceMetrics, src EvidenceMetrics) {
	dst.ResultsFound += src.ResultsFound
	dst.PagesDownloaded += src.PagesDownloaded
	dst.PagesDistilled += src.PagesDistilled
	dst.SnippetsUsed += src.SnippetsUsed
	dst.Duplicates += src.Duplicates
	if dst.Failures == nil {
		dst.Failures = map[string]int{}
	}
	for k, v := range src.Failures {
		dst.Failures[k] += v
	}
}

// Keep deterministic query provenance in API responses.
func sortEvidenceQueries(items []Evidence) {
	for i := range items {
		sort.Strings(items[i].SourceQueries)
	}
}
