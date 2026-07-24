package research

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// CitationSource is an immutable entry in the source catalogue given to the
// model. Citation numbers are local to one synthesis execution.
type CitationSource struct {
	Number         int
	ID, URL, Title string
	Type           EvidenceType
}

type CitationValidationError struct{ Problems []string }

func (e *CitationValidationError) Error() string {
	return "invalid citations: " + strings.Join(e.Problems, "; ")
}

var (
	citationPattern  = regexp.MustCompile(`\[(\d+)\]`)
	urlPattern       = regexp.MustCompile(`https?://[^\s)>\]]+`)
	referencePattern = regexp.MustCompile(`(?m)^\s*(?:[-*\d.]+\s*)?\[(\d+)\]\s*[:—\-]?\s*.*?(https?://[^\s)>\]"']+).*?$`)
	headingPattern   = regexp.MustCompile(`(?m)^(#{1,6})[ \t]+(.+?)[ \t]*$`)
)

// ValidateCitations verifies that every citation and URL is anchored to the
// source catalogue. It deliberately validates provenance, not factual truth.
func ValidateCitations(report string, sources []CitationSource) error {
	allowed, allowedURLs := map[int]CitationSource{}, map[string]struct{}{}
	for _, source := range sources {
		allowed[source.Number] = source
		allowedURLs[normalizeCitationURL(source.URL)] = struct{}{}
	}
	problems, cited := []string{}, map[int]struct{}{}
	for _, match := range citationPattern.FindAllStringSubmatch(reportWithoutReferences(report), -1) {
		var number int
		_, _ = fmt.Sscanf(match[1], "%d", &number)
		if _, ok := allowed[number]; !ok {
			problems = append(problems, fmt.Sprintf("citation [%d] is not in the source catalogue", number))
			continue
		}
		cited[number] = struct{}{}
	}
	referenced := map[int]struct{}{}
	for _, match := range referencePattern.FindAllStringSubmatch(referencesSection(report), -1) {
		var number int
		_, _ = fmt.Sscanf(match[1], "%d", &number)
		source, ok := allowed[number]
		if !ok {
			problems = append(problems, fmt.Sprintf("reference [%d] is not in the source catalogue", number))
			continue
		}
		if normalizeCitationURL(match[2]) != normalizeCitationURL(source.URL) {
			problems = append(problems, fmt.Sprintf("reference [%d] does not match its recovered URL", number))
		}
		if _, duplicate := referenced[number]; duplicate {
			problems = append(problems, fmt.Sprintf("reference [%d] is duplicated", number))
		}
		referenced[number] = struct{}{}
	}
	for _, rawURL := range urlPattern.FindAllString(report, -1) {
		if _, ok := allowedURLs[normalizeCitationURL(rawURL)]; !ok {
			problems = append(problems, fmt.Sprintf("URL %q was not recovered in this research", rawURL))
		}
	}
	for number := range cited {
		if _, ok := referenced[number]; !ok {
			problems = append(problems, fmt.Sprintf("citation [%d] has no reference entry", number))
		}
	}
	for number := range referenced {
		if _, ok := cited[number]; !ok {
			problems = append(problems, fmt.Sprintf("reference [%d] is not cited", number))
		}
	}
	for _, title := range uncitedFactualSectionTitles(report) {
		problems = append(problems, fmt.Sprintf("factual section %q has no citation", title))
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return &CitationValidationError{Problems: problems}
}

func referencesSection(report string) string {
	for _, section := range markdownSections(report) {
		if isReferencesHeading(section.Title) {
			return report[section.Start:]
		}
	}
	return ""
}

func reportWithoutReferences(report string) string {
	for _, section := range markdownSections(report) {
		if isReferencesHeading(section.Title) {
			return report[:section.Start]
		}
	}
	return report
}

func hasUncitedFactualSection(report string) bool {
	return len(uncitedFactualSectionTitles(report)) > 0
}

type markdownSection struct {
	Title      string
	Start      int
	ContentEnd int
	BodyStart  int
}

// markdownSections returns each heading with only its directly-owned prose.
// Content beneath a nested heading belongs to that nested section, preventing
// a citation in a child section from accidentally satisfying its parent.
func markdownSections(report string) []markdownSection {
	matches := headingPattern.FindAllStringSubmatchIndex(report, -1)
	sections := make([]markdownSection, 0, len(matches))
	for i, match := range matches {
		contentEnd := len(report)
		if i+1 < len(matches) {
			contentEnd = matches[i+1][0]
		}
		sections = append(sections, markdownSection{
			Title:      strings.TrimSpace(report[match[4]:match[5]]),
			Start:      match[0],
			BodyStart:  match[1],
			ContentEnd: contentEnd,
		})
	}
	return sections
}

func uncitedFactualSectionTitles(report string) []string {
	var titles []string
	for _, section := range markdownSections(reportWithoutReferences(report)) {
		if isReferencesHeading(section.Title) {
			continue
		}
		body := strings.TrimSpace(report[section.BodyStart:section.ContentEnd])
		// Short labels, connective fragments, and empty parent headings are not
		// factual sections. Substantive prose must contain a recovered citation.
		if len([]rune(body)) > 40 && !citationPattern.MatchString(body) {
			titles = append(titles, section.Title)
		}
	}
	return titles
}

func isReferencesHeading(title string) bool {
	title = strings.ToLower(strings.TrimSpace(title))
	return title == "referencias" || title == "references" ||
		strings.HasSuffix(title, ". referencias") ||
		strings.HasSuffix(title, ". references")
}
func normalizeCitationURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), ".,;:")
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/")
}
