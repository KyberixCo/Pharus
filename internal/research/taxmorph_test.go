package research

import (
	"context"
	"encoding/json"
	"testing"
)

func TestTaxmorphDeterministicRefine(t *testing.T) {
	pb := NewPlanBuilder(nil)
	plan, err := pb.BuildPlan(context.Background(), "Model Context Protocol Architecture", nil)
	if err != nil {
		t.Fatalf("unexpected error building plan: %v", err)
	}

	ts := NewTaxmorphService(nil)
	tree, err := ts.RefineOutline(context.Background(), plan)
	if err != nil {
		t.Fatalf("unexpected error refining outline: %v", err)
	}

	if tree.Topic != "Model Context Protocol Architecture" {
		t.Errorf("expected topic 'Model Context Protocol Architecture', got: %q", tree.Topic)
	}

	if len(tree.Nodes) == 0 {
		t.Fatalf("expected non-empty taxonomy tree nodes")
	}

	leaves := tree.LeafNodes()
	if len(leaves) == 0 {
		t.Errorf("expected leaf nodes in taxonomy tree")
	}

	allNodes := tree.FlattenAllNodes()
	if len(allNodes) <= len(tree.Nodes) {
		t.Errorf("expected flattened nodes to include sub-nodes (got %d all vs %d top)", len(allNodes), len(tree.Nodes))
	}
}

func TestTaxmorphLLMRefine(t *testing.T) {
	pb := NewPlanBuilder(nil)
	plan, _ := pb.BuildPlan(context.Background(), "Go Performance", nil)

	mockResponse := `{
		"topic": "Go Performance",
		"nodes": [
			{
				"id": "sec_1",
				"title": "1. Resumen Ejecutivo de Rendimiento",
				"description": "Síntesis de métricas",
				"level": 1,
				"key_questions": ["¿Cuáles son los cuellos de botella?"],
				"sub_nodes": [
					{
						"id": "sec_1_1",
						"title": "1.1 Perfilado de Memoria",
						"description": "Uso de pprof y gc",
						"level": 2,
						"parent_id": "sec_1",
						"key_questions": ["¿Cómo medir la asignación de heap?"]
					}
				]
			}
		]
	}`

	mockProvider := &mockLLMProvider{response: mockResponse}
	ts := NewTaxmorphService(mockProvider)

	tree, err := ts.RefineOutline(context.Background(), plan)
	if err != nil {
		t.Fatalf("unexpected error refining outline with LLM: %v", err)
	}

	if len(tree.Nodes) != 1 {
		t.Fatalf("expected 1 top node, got: %d", len(tree.Nodes))
	}
	if len(tree.Nodes[0].SubNodes) != 1 {
		t.Fatalf("expected 1 sub node, got: %d", len(tree.Nodes[0].SubNodes))
	}

	leaves := tree.LeafNodes()
	if len(leaves) != 1 || leaves[0].Title != "1.1 Perfilado de Memoria" {
		t.Errorf("unexpected leaf node: %#v", leaves)
	}
}

func TestTaxmorphNilPlan(t *testing.T) {
	ts := NewTaxmorphService(nil)
	_, err := ts.RefineOutline(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil plan, got nil")
	}
}

func TestTaxmorphTreeToJSON(t *testing.T) {
	pb := NewPlanBuilder(nil)
	plan, _ := pb.BuildPlan(context.Background(), "JSON Test", nil)
	ts := NewTaxmorphService(nil)
	tree, _ := ts.RefineOutline(context.Background(), plan)

	jsonStr, err := tree.ToJSON()
	if err != nil {
		t.Fatalf("unexpected error converting tree to JSON: %v", err)
	}

	var unmarshaled TaxmorphTree
	if err := json.Unmarshal([]byte(jsonStr), &unmarshaled); err != nil {
		t.Fatalf("unmarshaled invalid JSON: %v", err)
	}
}

func TestTaxmorphTreeValidationAndMetrics(t *testing.T) {
	tree := &TaxmorphTree{
		Topic: "Metrics Test",
		Nodes: []*TaxonNode{
			{
				ID:    "node_1",
				Title: "Section 1",
				Level: 1,
				SubNodes: []*TaxonNode{
					{
						ID:       "node_1_1",
						Title:    "Subsection 1.1",
						Level:    2,
						ParentID: "node_1",
					},
				},
			},
		},
	}

	if err := tree.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if tree.Depth() != 2 {
		t.Errorf("expected tree depth 2, got %d", tree.Depth())
	}
	if tree.NodeCount() != 2 {
		t.Errorf("expected node count 2, got %d", tree.NodeCount())
	}

	// Duplicate ID test
	duplicateTree := &TaxmorphTree{
		Topic: "Duplicate Test",
		Nodes: []*TaxonNode{
			{ID: "dup_1", Title: "Sec 1", Level: 1},
			{ID: "dup_1", Title: "Sec 2", Level: 1},
		},
	}
	if err := duplicateTree.Validate(); err == nil {
		t.Error("expected validation error for duplicate IDs, got nil")
	}
}
