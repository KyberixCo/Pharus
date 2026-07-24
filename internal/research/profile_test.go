package research

import (
	"context"
	"strings"
	"testing"

	"github.com/KyberixCo/Pharus/internal/i18n"
	"github.com/KyberixCo/Pharus/internal/vectordb"
)

func TestParseProfileAliasesAndBudgets(t *testing.T) {
	tests := map[string]Profile{
		"quick": ProfileQuick, "rápida": ProfileQuick,
		"medium": ProfileBalanced, "media": ProfileBalanced,
		"deep": ProfileDeep, "compleja": ProfileDeep,
	}
	for input, expected := range tests {
		got, err := ParseProfile(input)
		if err != nil || got != expected {
			t.Fatalf("ParseProfile(%q) = %q, %v; want %q", input, got, err, expected)
		}
	}
	if _, err := ParseProfile("unbounded"); err == nil {
		t.Fatal("expected an invalid profile error")
	}
	if budgetForProfile(ProfileQuick).MaxSections >= budgetForProfile(ProfileDeep).MaxSections {
		t.Fatal("quick profile must use fewer sections than deep")
	}
}

func TestBoundedEvidenceCatalogueLimitsEverySource(t *testing.T) {
	evidence := []vectordb.SearchResult{
		{Content: strings.Repeat("a", 100), Metadata: map[string]string{"url": "https://one.test"}},
		{Content: strings.Repeat("b", 100), Metadata: map[string]string{"url": "https://two.test"}},
	}
	sources := citationSources(evidence)
	catalogue := boundedEvidenceCatalogue(context.Background(), evidence, sources, 12)
	if strings.Contains(catalogue, strings.Repeat("a", 13)) || strings.Contains(catalogue, strings.Repeat("b", 13)) {
		t.Fatal("evidence excerpt exceeded its per-source character budget")
	}
	if !strings.Contains(catalogue, "[1]") || !strings.Contains(catalogue, "[2]") {
		t.Fatal("bounded catalogue must retain stable source numbers")
	}
}

func TestSelectSectionsAcrossOutlineRetainsCoverage(t *testing.T) {
	nodes := make([]*TaxonNode, 10)
	for i := range nodes {
		nodes[i] = &TaxonNode{ID: string(rune('a' + i))}
	}
	selected := selectSectionsAcrossOutline(nodes, 4)
	if len(selected) != 4 || selected[0] != nodes[0] || selected[len(selected)-1] != nodes[len(nodes)-1] {
		t.Fatalf("expected four sections spanning the complete outline, got %#v", selected)
	}
}

func TestDeepProfileKeepsLegacyCheckpointKey(t *testing.T) {
	topic := "tema"
	language := i18n.Spanish
	if got, want := profileSessionTopic(topic, language, ProfileDeep), languageSessionTopic(topic, language); got != want {
		t.Fatalf("deep checkpoint key %q does not preserve legacy key %q", got, want)
	}
	if profileSessionTopic(topic, language, ProfileBalanced) == languageSessionTopic(topic, language) {
		t.Fatal("balanced checkpoint must not reuse an exhaustive legacy session")
	}
}
