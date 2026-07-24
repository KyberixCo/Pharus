package gbnf

import (
	"fmt"
	"sort"
	"strings"
)

// GBNFGenerator converts JSON Schemas or field descriptions into deterministic GBNF rules for llama.cpp.
type GBNFGenerator struct{}

func NewGBNFGenerator() *GBNFGenerator {
	return &GBNFGenerator{}
}

// GenerateObjectGrammar builds a GBNF grammar definition for a JSON object with specified field types.
// Supports types: "string", "number", "integer", "boolean", "string-array", "number-array", "enum:val1,val2".
func (g *GBNFGenerator) GenerateObjectGrammar(fields map[string]string) string {
	var sb strings.Builder

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var rules []string
	for i, name := range keys {
		fType := fields[name]
		ruleName := fmt.Sprintf("field-%d", i)
		rules = append(rules, ruleName)

		valRule := g.buildValueRule(fType)
		sb.WriteString(fmt.Sprintf("%s ::= \"\\\"%s\\\": \" %s\n", ruleName, name, valRule))
	}

	sb.WriteString("root ::= \"{\" ws ")
	for i, r := range rules {
		if i > 0 {
			sb.WriteString(" \",\" ws ")
		}
		sb.WriteString(r)
	}
	sb.WriteString(" \"}\"\n")
	sb.WriteString(commonGBNFDefinitions())

	return sb.String()
}

// GenerateToolCallGrammar builds a GBNF grammar for a structured MCP Tool Call:
// {"tool": "toolName", "arguments": { ... }}
func (g *GBNFGenerator) GenerateToolCallGrammar(toolName string, args map[string]string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("tool-name ::= \"\\\"%s\\\"\"\n", toolName))

	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var argRules []string
	for i, name := range keys {
		fType := args[name]
		ruleName := fmt.Sprintf("arg-%d", i)
		argRules = append(argRules, ruleName)

		valRule := g.buildValueRule(fType)
		sb.WriteString(fmt.Sprintf("%s ::= \"\\\"%s\\\": \" %s\n", ruleName, name, valRule))
	}

	sb.WriteString("args-obj ::= \"{\" ws ")
	for i, r := range argRules {
		if i > 0 {
			sb.WriteString(" \",\" ws ")
		}
		sb.WriteString(r)
	}
	sb.WriteString(" \"}\"\n")

	sb.WriteString("root ::= \"{\" ws \"\\\"tool\\\": \" tool-name \",\" ws \"\\\"arguments\\\": \" args-obj ws \"}\"\n")
	sb.WriteString(commonGBNFDefinitions())

	return sb.String()
}

// GenerateCoSTORMQuestionsGrammar generates a GBNF grammar for Co-STORM question generation:
// {"perspective": "...", "questions": ["q1", "q2"]}
func (g *GBNFGenerator) GenerateCoSTORMQuestionsGrammar() string {
	var sb strings.Builder

	sb.WriteString("root ::= \"{\" ws \"\\\"perspective\\\": \" string-val \",\" ws \"\\\"questions\\\": \" string-array ws \"}\"\n")
	sb.WriteString("string-array ::= \"[\" ws (string-val (\",\" ws string-val)*)? ws \"]\"\n")
	sb.WriteString(commonGBNFDefinitions())

	return sb.String()
}

func (g *GBNFGenerator) buildValueRule(fType string) string {
	lowerType := strings.ToLower(fType)
	switch {
	case lowerType == "number" || lowerType == "float" || lowerType == "int" || lowerType == "integer":
		return "number-val"
	case lowerType == "boolean" || lowerType == "bool":
		return "(\"true\" | \"false\")"
	case lowerType == "string-array" || lowerType == "[]string":
		return "(\"[\" ws (string-val (\",\" ws string-val)*)? ws \"]\")"
	case lowerType == "number-array" || lowerType == "[]number":
		return "(\"[\" ws (number-val (\",\" ws number-val)*)? ws \"]\")"
	case strings.HasPrefix(lowerType, "enum:"):
		rawOpts := strings.TrimPrefix(fType, "enum:")
		opts := strings.Split(rawOpts, ",")
		var enumRules []string
		for _, o := range opts {
			enumRules = append(enumRules, fmt.Sprintf("\"\\\"%s\\\"\"", strings.TrimSpace(o)))
		}
		return fmt.Sprintf("(%s)", strings.Join(enumRules, " | "))
	default:
		return "string-val"
	}
}

func commonGBNFDefinitions() string {
	return `ws ::= [ \t\n]*
string-val ::= "\"" ([^"\\] | "\\" .)* "\""
number-val ::= "-"? ([0-9]+ ("." [0-9]+)?)
`
}

// MaskLogits deterministically zeroes out invalid tokens given the current GBNF grammar state.
func (g *GBNFGenerator) MaskLogits(logits []float32, validIndices []int) []float32 {
	masked := make([]float32, len(logits))
	for i := range masked {
		masked[i] = -1e9
	}
	for _, idx := range validIndices {
		if idx >= 0 && idx < len(masked) {
			masked[idx] = logits[idx]
		}
	}
	return masked
}


