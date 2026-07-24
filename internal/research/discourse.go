package research

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/KyberixCo/Pharus/internal/llm"
)

type ExpertRole struct {
	Name               string `json:"name"`
	Perspective        string `json:"perspective"`
	CognitiveBias      string `json:"cognitive_bias,omitempty"`
	MandatoryBlindspot string `json:"mandatory_blindspot,omitempty"`
	TargetDomain       string `json:"target_domain,omitempty"`
}

type ConceptNode struct {
	ID           string   `json:"id"`
	Topic        string   `json:"topic"`
	Facts        []string `json:"facts"`
	SubConcepts  []string `json:"sub_concepts"`
	Gaps         []string `json:"gaps"`
	ResolvedGaps []string `json:"resolved_gaps"`
}

type ConceptMap struct {
	mu    sync.RWMutex
	Nodes map[string]*ConceptNode
}

func NewConceptMap() *ConceptMap {
	return &ConceptMap{
		Nodes: make(map[string]*ConceptNode),
	}
}

func (cm *ConceptMap) AddOrUpdateNode(nodeID string, topic string, fact string, gap string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	node, exists := cm.Nodes[nodeID]
	if !exists {
		node = &ConceptNode{
			ID:    nodeID,
			Topic: topic,
		}
		cm.Nodes[nodeID] = node
	}

	if fact != "" {
		node.Facts = append(node.Facts, fact)
	}
	if gap != "" {
		node.Gaps = append(node.Gaps, gap)
	}
}

func (cm *ConceptMap) AddSubConcept(nodeID string, subConcept string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	node, exists := cm.Nodes[nodeID]
	if !exists {
		node = &ConceptNode{ID: nodeID, Topic: nodeID}
		cm.Nodes[nodeID] = node
	}

	if subConcept != "" {
		node.SubConcepts = append(node.SubConcepts, subConcept)
	}
}

func (cm *ConceptMap) ResolveGap(nodeID string, gap string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	node, exists := cm.Nodes[nodeID]
	if exists && gap != "" {
		node.ResolvedGaps = append(node.ResolvedGaps, gap)
	}
}

func (cm *ConceptMap) Summary() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.Nodes) == 0 {
		return "Mapa Conceptual vacío."
	}

	var sb strings.Builder
	for _, node := range cm.Nodes {
		sb.WriteString(fmt.Sprintf("• Nódulo: %s (Tema: %s)\n", node.ID, node.Topic))
		if len(node.Facts) > 0 {
			sb.WriteString(fmt.Sprintf("  - Hechos (%d): %s\n", len(node.Facts), strings.Join(node.Facts, "; ")))
		}
		if len(node.SubConcepts) > 0 {
			sb.WriteString(fmt.Sprintf("  - Sub-conceptos: %s\n", strings.Join(node.SubConcepts, ", ")))
		}
		if len(node.Gaps) > 0 {
			sb.WriteString(fmt.Sprintf("  - Lagunas Abiertas: %s\n", strings.Join(node.Gaps, "; ")))
		}
		if len(node.ResolvedGaps) > 0 {
			sb.WriteString(fmt.Sprintf("  - Lagunas Resueltas: %s\n", strings.Join(node.ResolvedGaps, "; ")))
		}
	}
	return sb.String()
}

type ProvocativeQuestion struct {
	TargetGap string
	Query     string
}

type DiscourseManager struct {
	llm                 llm.Provider
	ConceptMap          *ConceptMap
	InsightBank         *GlobalInsightBank
	ConsistencyDetector *QueryConsistencyDetector
}

func NewDiscourseManager(llm llm.Provider) *DiscourseManager {
	bank := NewGlobalInsightBank()
	return &DiscourseManager{
		llm:                 llm,
		ConceptMap:          NewConceptMap(),
		InsightBank:         bank,
		ConsistencyDetector: NewQueryConsistencyDetector(bank),
	}
}

// UpdateConceptMapFromEvidence updates the concept map nodes from evidence text.
func (d *DiscourseManager) UpdateConceptMapFromEvidence(ctx context.Context, topic string, evidence string) {
	if evidence == "" {
		return
	}

	nodeID := strings.ToLower(strings.ReplaceAll(topic, " ", "_"))
	if len(nodeID) > 30 {
		nodeID = nodeID[:30]
	}

	lines := strings.Split(evidence, "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "Título:") || strings.HasPrefix(trimmed, "Contenido:") {
			d.ConceptMap.AddOrUpdateNode(nodeID, topic, trimmed, "")
			break
		}
	}
}

func defaultExpertRoles(ctx context.Context) []ExpertRole {
	if researchText(ctx, "en", "es") == "en" {
		return []ExpertRole{
			{Name: "Architectural and Theoretical Analyst", Perspective: "Structure, design, open standards, and design-pattern evaluation", CognitiveBias: "Skeptical of proprietary solutions or weak theoretical foundations", MandatoryBlindspot: "Ignore operating costs and focus on technical elegance and maintainability", TargetDomain: "Architecture and Engineering"},
			{Name: "Operations and Performance Specialist", Perspective: "Performance, latency, resource efficiency, deployment, and ROI", CognitiveBias: "Pragmatic and oriented toward production metrics and cost-benefit", MandatoryBlindspot: "Ignore theoretical abstractions with no direct effect on latency or consumption", TargetDomain: "Operations and Business"},
			{Name: "Security and Governance Researcher", Perspective: "Vulnerabilities, risk mitigation, privacy, and regulation", CognitiveBias: "Paranoid about attack surface and data leakage", MandatoryBlindspot: "Ignore prototyping speed in favor of resilience and control", TargetDomain: "Security and Compliance"},
		}
	}
	return []ExpertRole{
		{
			Name:               "Analista Arquitectónico y Teórico",
			Perspective:        "Evaluación de estructura, diseño, estándares abiertos y patrones de diseño",
			CognitiveBias:      "Escéptico de soluciones propietarias o sin fundamentos teóricos sólidos",
			MandatoryBlindspot: "Ignorar costos operativos y enfocarse en elegancia técnica y mantenibilidad",
			TargetDomain:       "Arquitectura e Ingeniería",
		},
		{
			Name:               "Especialista Operativo y de Rendimiento",
			Perspective:        "Rendimiento, latencia, eficiencia de recursos, despliegue y ROI",
			CognitiveBias:      "Pragmático orientado a métricas de producción y costo-beneficio",
			MandatoryBlindspot: "Ignorar abstracciones teóricas sin impacto directo en latencia o consumo",
			TargetDomain:       "Operaciones y Negocio",
		},
		{
			Name:               "Investigador de Seguridad y Gobernanza",
			Perspective:        "Vulnerabilidades, mitigación de riesgos, privacidad y regulación",
			CognitiveBias:      "Paranoide ante la superficie de ataque y fugas de datos",
			MandatoryBlindspot: "Ignorar la velocidad de prototipado en favor de la resiliencia y el control",
			TargetDomain:       "Seguridad y Cumplimiento",
		},
	}
}

// GeneratePerspectives uses LLM to generate complementary expert domain roles for a topic.
func (d *DiscourseManager) GeneratePerspectives(ctx context.Context, topic string) ([]ExpertRole, error) {
	if d.llm == nil {
		return defaultExpertRoles(ctx), nil
	}

	promptTemplate := researchText(ctx, `Given this research topic: "%s"
Generate three highly contrasting expert roles with explicit biases to prevent personality collapse in Co-STORM.
Return exactly one role per line in this pipe-delimited format:
Role Name | Perspective or Focus | Cognitive Bias | Mandatory Blind Spot | Target Domain`, `Dado el siguiente tema de investigación: "%s"
Genera 3 roles de expertos altamente contrastados con sesgos explícitos para evitar el colapso de personalidad en Co-STORM.
Devuelve exactamente en este formato por línea (delimitado por '|'):
Nombre del Rol | Perspectiva o enfoque | Sesgo Cognitivo | Área Ciega Obligatoria | Dominio Objetivo`)
	prompt := fmt.Sprintf(promptTemplate, topic)

	messages := []llm.Message{
		{Role: "system", Content: researchText(ctx,
			"You are a Co-STORM research orchestrator that generates highly specialized and contrasting research profiles.",
			"Eres un orquestador de investigación Co-STORM que genera perfiles de investigación altamente especializados y contrastados.",
		) + "\n\n" + reportLanguageDirective(ctx)},
		{Role: "user", Content: prompt},
	}

	resp, err := d.llm.GenerateCompletion(ctx, messages, 0.3)
	if err != nil {
		return defaultExpertRoles(ctx), fmt.Errorf("generate expert perspectives; using deterministic defaults: %w", err)
	}

	var roles []ExpertRole
	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) >= 2 {
			role := ExpertRole{
				Name:        strings.TrimSpace(parts[0]),
				Perspective: strings.TrimSpace(parts[1]),
			}
			if len(parts) >= 3 {
				role.CognitiveBias = strings.TrimSpace(parts[2])
			}
			if len(parts) >= 4 {
				role.MandatoryBlindspot = strings.TrimSpace(parts[3])
			}
			if len(parts) >= 5 {
				role.TargetDomain = strings.TrimSpace(parts[4])
			}
			roles = append(roles, role)
		}
	}

	if len(roles) == 0 {
		roles = defaultExpertRoles(ctx)
	}
	for _, role := range roles {
		if err := ValidateReportLanguage(strings.Join([]string{role.Name, role.Perspective, role.CognitiveBias, role.MandatoryBlindspot, role.TargetDomain}, "\n")); err != nil {
			return defaultExpertRoles(ctx), fmt.Errorf("expert perspectives contained unexpected writing system; using deterministic defaults: %w", err)
		}
	}

	return roles, nil
}

// EvaluateGapsAndProvoke (Agente Moderador Provocativo Co-STORM)
// Analiza la evidencia recolectada e inyecta preguntas incisivas sobre vacíos informativos no abordados.
func (d *DiscourseManager) EvaluateGapsAndProvoke(ctx context.Context, topic string, evidenceSummary string) ([]ProvocativeQuestion, error) {
	if d.llm == nil {
		return []ProvocativeQuestion{
			{TargetGap: researchText(ctx, "Technical depth and edge cases", "Profundización técnica y casos de borde"), Query: fmt.Sprintf(researchText(ctx, "%s challenges limitations edge cases", "%s desafíos limitaciones casos borde"), topic)},
		}, nil
	}

	promptTemplate := researchText(ctx, `You are the Provocative Moderator Agent for the Co-STORM protocol.
Research topic: "%s"

Summary of the evidence collected so far:
%s

Instruction: Identify one or two critical information gaps, contradictions, or unexplored aspects in the prior evidence.
Generate refined, provocative search questions to cover these gaps.
Strict format, one per line:
Information Gap | Refined Search Query`, `Eres el Agente Moderador Provocativo del protocolo Co-STORM.
Tema de investigación: "%s"

Resumen de evidencias recopiladas hasta ahora:
%s

Instrucción: Identifica de 1 a 2 vacíos informativos críticos, contradicciones o aspectos no explorados en la evidencia previa.
Genera preguntas provocativas refinadas de búsqueda para cubrir estos vacíos.
Formato estricto por línea:
Vacío Informativo | Consulta de Búsqueda Refinada`)
	prompt := fmt.Sprintf(promptTemplate, topic, evidenceSummary)

	messages := []llm.Message{
		{Role: "system", Content: researchText(ctx,
			"You are a provocative Co-STORM moderator focused on uncovering knowledge gaps and challenging assumptions.",
			"Eres un Moderador Provocativo de Co-STORM enfocado en descubrir lagunas de conocimiento y desafiar premisas.",
		) + "\n\n" + reportLanguageDirective(ctx)},
		{Role: "user", Content: prompt},
	}

	resp, err := d.llm.GenerateCompletion(ctx, messages, 0.4)
	if err != nil {
		return []ProvocativeQuestion{
			{TargetGap: researchText(ctx, "Technical depth and edge cases", "Profundización técnica y casos de borde"), Query: fmt.Sprintf(researchText(ctx, "%s challenges limitations edge cases", "%s desafíos limitaciones casos borde"), topic)},
		}, nil
	}

	var questions []ProvocativeQuestion
	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) >= 2 {
			gap := strings.TrimSpace(parts[0])
			q := strings.TrimSpace(parts[1])
			questions = append(questions, ProvocativeQuestion{
				TargetGap: gap,
				Query:     q,
			})
			d.ConceptMap.AddOrUpdateNode("gap_node", topic, "", gap)
		}
	}

	if len(questions) == 0 {
		questions = append(questions, ProvocativeQuestion{
			TargetGap: researchText(ctx, "Risk and security analysis", "Análisis de riesgos y seguridad"),
			Query:     fmt.Sprintf(researchText(ctx, "%s security risk mitigation", "%s seguridad mitigación riesgos"), topic),
		})
	}

	return questions, nil
}
