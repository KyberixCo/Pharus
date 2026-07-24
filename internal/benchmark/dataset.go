package benchmark

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Built-in sample suites supported by Pharus. They are smoke-test inputs, not
// copies of the named external benchmark datasets.
const (
	SuiteDeepResearchBench = "deep_research_bench"
	SuiteBrowseComp        = "browsecomp"
	SuiteGAIA              = "gaia"
	SuiteHLE               = "hle"
	SuiteSynthetic         = "synthetic"
)

// DatasetLoader provides methods for reading benchmark dataset items from files or built-in suites.
type DatasetLoader struct{}

// NewDatasetLoader initializes a new DatasetLoader.
func NewDatasetLoader() *DatasetLoader {
	return &DatasetLoader{}
}

// LoadFromFile reads benchmark items from a JSON or JSONL file.
func (dl *DatasetLoader) LoadFromFile(filePath string) ([]DatasetItem, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open dataset file %s: %w", filePath, err)
	}
	defer file.Close()

	if strings.HasSuffix(filePath, ".jsonl") {
		return dl.loadJSONL(file)
	}
	return dl.loadJSON(file)
}

func (dl *DatasetLoader) loadJSON(r io.Reader) ([]DatasetItem, error) {
	var items []DatasetItem
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&items); err != nil {
		return nil, fmt.Errorf("failed to decode JSON dataset array: %w", err)
	}
	return items, nil
}

func (dl *DatasetLoader) loadJSONL(r io.Reader) ([]DatasetItem, error) {
	var items []DatasetItem
	scanner := bufio.NewScanner(r)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			continue
		}

		var item DatasetItem
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("failed to parse JSONL line %d: %w", lineNum, err)
		}
		items = append(items, item)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning JSONL dataset: %w", err)
	}
	return items, nil
}

// GetBuiltInSuite returns sample items shaped after common benchmark domains
// when an external, versioned dataset file is not specified.
func (dl *DatasetLoader) GetBuiltInSuite(suiteName string) ([]DatasetItem, error) {
	switch strings.ToLower(suiteName) {
	case SuiteDeepResearchBench, "deep_research":
		return []DatasetItem{
			{
				ID:       "drb-001",
				Suite:    SuiteDeepResearchBench,
				Topic:    "Análisis comparativo de arquitecturas de bases de datos vectoriales embebidas en Go (LanceDB vs Chromem-go)",
				Category: "Computer Science",
			},
			{
				ID:       "drb-002",
				Suite:    SuiteDeepResearchBench,
				Topic:    "Mecanismos de aislamiento de memoria y control OOM en clientes MCP stateless sobre HTTP streaming",
				Category: "Systems Engineering",
			},
			{
				ID:       "drb-003",
				Suite:    SuiteDeepResearchBench,
				Topic:    "Evolución de algoritmos de navegación profunda en la web y destilación heurística de contenido HTML",
				Category: "Web Mining",
			},
		}, nil

	case SuiteBrowseComp, "browsecomp_plus":
		return []DatasetItem{
			{
				ID:       "bc-001",
				Suite:    SuiteBrowseComp,
				Topic:    "Avances en navegadores headless y protección SSRF con DNS Pinning en frameworks de scraping modernos",
				Category: "Security",
			},
			{
				ID:       "bc-002",
				Suite:    SuiteBrowseComp,
				Topic:    "Técnicas de scraping con renderizado dinámico y deduplicación multivariada en SearXNG",
				Category: "Information Retrieval",
			},
		}, nil

	case SuiteGAIA:
		return []DatasetItem{
			{
				ID:       "gaia-001",
				Suite:    SuiteGAIA,
				Topic:    "Ejecución segura de scripts en sandbox efímeros de Python/Bash bajo restricciones de SO",
				Category: "Tool Execution",
			},
		}, nil

	case SuiteHLE, "humanity_last_exam":
		return []DatasetItem{
			{
				ID:       "hle-001",
				Suite:    SuiteHLE,
				Topic:    "Modelado de gramáticas GBNF hiper-específicas para inferencia determinista en samplers de llama.cpp",
				Category: "Artificial Intelligence",
			},
		}, nil

	case SuiteSynthetic, "all":
		// Combined synthetic suite covering all areas
		items := make([]DatasetItem, 0)
		for _, name := range []string{SuiteDeepResearchBench, SuiteBrowseComp, SuiteGAIA, SuiteHLE} {
			suiteItems, _ := dl.GetBuiltInSuite(name)
			items = append(items, suiteItems...)
		}
		return items, nil

	default:
		return nil, fmt.Errorf("unknown benchmark suite: %s. Supported: deep_research_bench, browsecomp, gaia, hle, synthetic", suiteName)
	}
}
