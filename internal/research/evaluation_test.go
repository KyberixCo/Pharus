package research

import (
	"context"
	"errors"
	"testing"

	"github.com/KyberixCo/Pharus/internal/llm"
)

type raceJudgeStub struct {
	response string
	err      error
}

func (s raceJudgeStub) GenerateCompletion(context.Context, []llm.Message, float64) (string, error) {
	return s.response, s.err
}

func TestRACEEvaluatorParsesValidatedScores(t *testing.T) {
	evaluator := NewRACEEvaluator(raceJudgeStub{response: "Relevance: 0.9\nAuthenticity: 0.8\nClarity: 0.7\nEvidence: 0.6"})
	score, err := evaluator.EvaluateReport(context.Background(), "tema", "# informe")
	if err != nil {
		t.Fatalf("EvaluateReport() error = %v", err)
	}
	if score.OverallRACE < 0.7499 || score.OverallRACE > 0.7501 {
		t.Fatalf("OverallRACE = %v, want 0.75", score.OverallRACE)
	}
}

func TestRACEEvaluatorRejectsErrorsAndMalformedResponses(t *testing.T) {
	tests := []struct {
		name  string
		judge raceJudgeStub
	}{
		{name: "provider error", judge: raceJudgeStub{err: errors.New("unavailable")}},
		{name: "missing criterion", judge: raceJudgeStub{response: "Relevance: 0.8"}},
		{name: "out of range", judge: raceJudgeStub{response: "Relevance: 1.1\nAuthenticity: 0.8\nClarity: 0.7\nEvidence: 0.6"}},
		{name: "duplicate criterion", judge: raceJudgeStub{response: "Relevance: 0.8\nRelevance: 0.7\nAuthenticity: 0.8\nClarity: 0.7\nEvidence: 0.6"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRACEEvaluator(tt.judge).EvaluateReport(context.Background(), "tema", "informe")
			if err == nil {
				t.Fatal("expected evaluation error")
			}
		})
	}
}
