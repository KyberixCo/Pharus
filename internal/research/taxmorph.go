package research

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/KyberixCo/Pharus/internal/llm"
)

// TaxonNode represents a hierarchical node in the refined report taxonomy tree.
type TaxonNode struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	Level        int          `json:"level"` // 1 = Section (H2), 2 = Subsection (H3), 3 = Sub-subsection (H4)
	ParentID     string       `json:"parent_id,omitempty"`
	KeyQuestions []string     `json:"key_questions,omitempty"`
	SubNodes     []*TaxonNode `json:"sub_nodes,omitempty"`
}

// TaxmorphTree is the refined, multi-tiered taxonomy produced by TAXMORPH.
type TaxmorphTree struct {
	Topic string       `json:"topic"`
	Nodes []*TaxonNode `json:"nodes"`
}

// TaxmorphService executes taxonomic transformations (Renaming, Kinship Rearrangement, Bridge Generation, Merge & Split).
type TaxmorphService struct {
	llm llm.Provider
}

func NewTaxmorphService(provider llm.Provider) *TaxmorphService {
	return &TaxmorphService{llm: provider}
}

// RefineOutline transforms a flat or raw ResearchPlan into an expanded, balanced TaxmorphTree.
func (ts *TaxmorphService) RefineOutline(ctx context.Context, plan *ResearchPlan) (*TaxmorphTree, error) {
	if plan == nil || len(plan.Outline) == 0 {
		return nil, fmt.Errorf("taxmorph requires a valid non-empty research plan")
	}

	if ts.llm != nil {
		tree, err := ts.refineWithLLM(ctx, plan)
		if err == nil && tree != nil && len(tree.Nodes) > 0 {
			return tree, nil
		}
		loggerForResearchContext(ctx).Warn("research taxonomy fallback activated",
			"phase", "planning",
			"operation", "taxonomy_refinement",
			"fallback", "deterministic",
			"error_kind", observableErrorKind(err),
		)
	}

	return ts.deterministicRefine(ctx, plan), nil
}

func (ts *TaxmorphService) refineWithLLM(ctx context.Context, plan *ResearchPlan) (*TaxmorphTree, error) {
	planJSON, err := plan.ToJSON()
	if err != nil {
		return nil, err
	}

	promptTemplate := researchText(ctx, `Original Research Plan:
%s

Act as Pharus's TAXMORPH Taxonomic Refinement Module.
Refine and expand the outline by applying four key operations:
1. SEMANTIC RENAMING: Make section and subsection titles conceptually specific and unambiguous.
2. KINSHIP REARRANGEMENT: Organize nodes into a clear hierarchy of level 1 main sections and level 2 analytical subsections.
3. BRIDGE-NODE GENERATION: Insert bridge subsections between complex concepts when needed.
4. MERGE AND SPLIT: Split overloaded sections into specific child nodes.

Return ONLY a valid JSON object with this exact structure:
{
  "topic": %q,
  "nodes": [
    {
      "id": "node_1",
      "title": "1. Specific Section Title",
      "description": "Description of the analytical scope",
      "level": 1,
      "key_questions": ["Question 1?", "Question 2?"],
      "sub_nodes": [
        {
          "id": "node_1_1",
          "title": "1.1 Specific Subsection",
          "description": "Analytical detail for this subsection",
          "level": 2,
          "parent_id": "node_1",
          "key_questions": ["Specific question?"]
        }
      ]
    }
  ]
}`, `Plan de Investigación Original:
%s

Actúa como el Módulo TAXMORPH de Refinamiento Taxonómico de Pharus.
Refina y expande el esquema anterior aplicando 4 operaciones clave:
1. RENOMBRADO SEMÁNTICO: Haz que los títulos de secciones y subsecciones sean conceptualmente específicos e inconfundibles.
2. REARREGLO DE PARENTESCO: Organiza los nodos en un árbol jerárquico claro de nivel 1 (secciones principales) y nivel 2 (subsecciones analíticas).
3. GENERACIÓN DE NODOS INTERMEDIOS (Bridge Nodes): Inserta subsecciones puente entre conceptos complejos cuando sea necesario.
4. FUSIÓN Y DIVISIÓN: Divide apartados sobrecargados en subnodos específicos.

Devuelve ÚNICAMENTE un objeto JSON válido con la siguiente estructura exacta:
{
  "topic": %q,
  "nodes": [
    {
      "id": "node_1",
      "title": "1. Título Específico de Sección",
      "description": "Descripción del alcance analítico",
      "level": 1,
      "key_questions": ["Pregunta 1?", "Pregunta 2?"],
      "sub_nodes": [
        {
          "id": "node_1_1",
          "title": "1.1 Subsección Específica",
          "description": "Detalle analítico de esta subsección",
          "level": 2,
          "parent_id": "node_1",
          "key_questions": ["Pregunta específica?"]
        }
      ]
    }
  ]
}`)
	prompt := fmt.Sprintf(promptTemplate, planJSON, plan.Topic)

	messages := []llm.Message{
		{Role: "system", Content: researchText(ctx,
			"You are Pharus's TAXMORPH module. You generate perfectly structured hierarchical taxonomy trees as JSON.",
			"Eres el Módulo TAXMORPH de Pharus. Generas árboles taxonómicos jerárquicos perfectamente estructurados en JSON.",
		) + "\n\n" + reportLanguageDirective(ctx)},
		{Role: "user", Content: prompt},
	}

	resp, err := ts.llm.GenerateCompletion(ctx, messages, 0.2)
	if err != nil {
		return nil, fmt.Errorf("taxmorph llm completion: %w", err)
	}

	cleanJSON := extractJSON(resp)
	var tree TaxmorphTree
	if err := json.Unmarshal([]byte(cleanJSON), &tree); err != nil {
		return nil, fmt.Errorf("taxmorph json unmarshal: %w", err)
	}

	if len(tree.Nodes) == 0 {
		return nil, fmt.Errorf("taxmorph generated empty nodes")
	}
	if err := validateTaxmorphLanguage(&tree); err != nil {
		return nil, err
	}

	return &tree, nil
}

func validateTaxmorphLanguage(tree *TaxmorphTree) error {
	if tree == nil {
		return nil
	}
	treeJSON, err := tree.ToJSON()
	if err != nil {
		return err
	}
	return ValidateReportLanguage(treeJSON)
}

func (ts *TaxmorphService) deterministicRefine(ctx context.Context, plan *ResearchPlan) *TaxmorphTree {
	treeNodes := make([]*TaxonNode, 0, len(plan.Outline))

	for i, spec := range plan.Outline {
		mainID := fmt.Sprintf("sec_%d", i+1)
		mainNode := &TaxonNode{
			ID:           mainID,
			Title:        spec.Title,
			Description:  spec.Description,
			Level:        1,
			KeyQuestions: spec.KeyQuestions,
			SubNodes:     make([]*TaxonNode, 0, len(spec.SubTopics)),
		}

		// Split sub-topics into explicit Level 2 sub-nodes
		for j, sub := range spec.SubTopics {
			subID := fmt.Sprintf("%s_sub_%d", mainID, j+1)
			subTitle := fmt.Sprintf("%d.%d %s", i+1, j+1, sub)
			subNode := &TaxonNode{
				ID:          subID,
				Title:       subTitle,
				Description: fmt.Sprintf(researchText(ctx, "In-depth analysis of %s within %s.", "Análisis de profundidad sobre %s dentro de %s."), sub, spec.Title),
				Level:       2,
				ParentID:    mainID,
				KeyQuestions: []string{
					fmt.Sprintf(researchText(ctx, "What are the key aspects and documentary evidence concerning %s?", "¿Cuáles son los aspectos clave y evidencia documental sobre %s?"), sub),
				},
			}
			mainNode.SubNodes = append(mainNode.SubNodes, subNode)
		}

		treeNodes = append(treeNodes, mainNode)
	}

	return &TaxmorphTree{
		Topic: plan.Topic,
		Nodes: treeNodes,
	}
}

// LeafNodes extracts all bottom-level nodes (leaf nodes) from the taxonomy tree in order of appearance.
func (tree *TaxmorphTree) LeafNodes() []*TaxonNode {
	var leaves []*TaxonNode
	var traverse func(node *TaxonNode)
	traverse = func(node *TaxonNode) {
		if len(node.SubNodes) == 0 {
			leaves = append(leaves, node)
		} else {
			for _, child := range node.SubNodes {
				traverse(child)
			}
		}
	}

	for _, top := range tree.Nodes {
		traverse(top)
	}
	return leaves
}

// FlattenAllNodes returns a flat list of all nodes (top-level and sub-nodes) in depth-first order.
func (tree *TaxmorphTree) FlattenAllNodes() []*TaxonNode {
	var flat []*TaxonNode
	var traverse func(node *TaxonNode)
	traverse = func(node *TaxonNode) {
		flat = append(flat, node)
		for _, child := range node.SubNodes {
			traverse(child)
		}
	}

	for _, top := range tree.Nodes {
		traverse(top)
	}
	return flat
}

func (tree *TaxmorphTree) ToJSON() (string, error) {
	b, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Validate checks tree integrity, ensuring non-empty nodes and unique non-empty IDs.
func (tree *TaxmorphTree) Validate() error {
	if tree == nil {
		return fmt.Errorf("taxonomy tree is nil")
	}
	if len(tree.Nodes) == 0 {
		return fmt.Errorf("taxonomy tree contains no root nodes")
	}

	seenIDs := make(map[string]bool)
	allNodes := tree.FlattenAllNodes()
	for _, n := range allNodes {
		if n == nil {
			return fmt.Errorf("taxonomy tree contains nil node pointer")
		}
		if n.ID == "" {
			return fmt.Errorf("taxonomy node title %q has empty ID", n.Title)
		}
		if seenIDs[n.ID] {
			return fmt.Errorf("duplicate node ID %q in taxonomy tree", n.ID)
		}
		seenIDs[n.ID] = true
	}
	return nil
}

// Depth calculates the maximum hierarchical depth of the taxonomy tree.
func (tree *TaxmorphTree) Depth() int {
	maxDepth := 0
	var traverse func(node *TaxonNode, current int)
	traverse = func(node *TaxonNode, current int) {
		if current > maxDepth {
			maxDepth = current
		}
		for _, child := range node.SubNodes {
			traverse(child, current+1)
		}
	}
	for _, root := range tree.Nodes {
		traverse(root, 1)
	}
	return maxDepth
}

// NodeCount returns the total number of nodes in the taxonomy tree.
func (tree *TaxmorphTree) NodeCount() int {
	return len(tree.FlattenAllNodes())
}
