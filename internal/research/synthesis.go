package research

import (
	"context"
	"fmt"
	"strings"

	"github.com/KyberixCo/Pharus/internal/llm"
	llmtypes "github.com/KyberixCo/Pharus/internal/llm/types"
	"github.com/KyberixCo/Pharus/internal/vectordb"
)

type SlidingContext struct {
	PreviousSectionTitle string
	LastParagraphs       string
	ActiveEntities       []string
}

type Synthesizer struct {
	llm llm.Provider
}

func NewSynthesizer(provider llm.Provider) *Synthesizer {
	return &Synthesizer{llm: provider}
}

// SynthesizeHierarchicalReport generates an extensive Deep Research report by writing section-by-section
// along the TAXMORPH taxonomy tree, carrying forward a SlidingContext to guarantee discursiveness and depth.
func (s *Synthesizer) SynthesizeHierarchicalReport(ctx context.Context, topic string, taxonTree *TaxmorphTree, evidence []vectordb.SearchResult, conceptMap *ConceptMap, insightBank *GlobalInsightBank) (string, error) {
	budget := budgetForProfile(ProfileFromContext(ctx))
	sources := citationSources(evidence)
	if len(sources) == 0 {
		return "", fmt.Errorf("cannot synthesize without a source catalogue")
	}
	if len(sources) != len(evidence) {
		return "", fmt.Errorf("cannot synthesize evidence missing a recovered URL")
	}

	if taxonTree == nil || len(taxonTree.Nodes) == 0 {
		pb := NewPlanBuilder(s.llm)
		plan, _ := pb.BuildPlan(ctx, topic, nil)
		tm := NewTaxmorphService(s.llm)
		taxonTree, _ = tm.RefineOutline(ctx, plan)
	}

	evidenceCatalogue := boundedEvidenceCatalogue(ctx, evidence, sources, budget.EvidenceCharacters)

	var conceptSummary string
	if conceptMap != nil {
		conceptSummary = conceptMap.Summary()
	} else {
		conceptSummary = researchText(ctx, "Not available.", "No disponible.")
	}

	var insightsSummary string
	if insightBank != nil {
		insights := insightBank.GetInsightsForTopic(topic)
		var insLines []string
		for _, ins := range insights {
			if !ins.HasValidProvenance() {
				continue
			}
			provenance := fmt.Sprintf(researchText(ctx, "%s; status=%s", "%s; estado=%s"), ins.Source, ins.ValidationStatus)
			if ins.EvidenceID != "" {
				provenance += fmt.Sprintf(researchText(ctx, "; evidence=%s", "; evidencia=%s"), ins.EvidenceID)
			} else {
				provenance += fmt.Sprintf(researchText(ctx, "; query=%s", "; consulta=%s"), ins.StructuredQuery)
			}
			insLines = append(insLines, fmt.Sprintf(researchText(ctx,
				"- %s: %s (Metric: %s; provenance: %s)",
				"- %s: %s (Métrica: %s; procedencia: %s)",
			), ins.Source, ins.Thesis, ins.Metric, provenance))
		}
		if len(insLines) > 0 {
			insightsSummary = strings.Join(insLines, "\n")
		} else {
			insightsSummary = researchText(ctx, "No additional findings.", "Sin hallazgos adicionales.")
		}
	} else {
		insightsSummary = researchText(ctx, "Not available.", "No disponible.")
	}

	if s.llm == nil {
		return s.fallbackHierarchicalReport(ctx, topic, taxonTree, sources)
	}

	cg := NewChronosGraph()
	chronologicalTimeline := cg.FormatChronologicalTimeline(evidence)

	var docBuf strings.Builder
	docBuf.WriteString(fmt.Sprintf(researchText(ctx, "# Deep Research Report: %s\n\n", "# Informe de Deep Research: %s\n\n"), topic))

	slidingCtx := &SlidingContext{}
	leafNodes := taxonTree.LeafNodes()
	if len(leafNodes) == 0 {
		leafNodes = taxonTree.FlattenAllNodes()
	}
	if len(leafNodes) > budget.MaxSections {
		log := loggerForResearchContext(ctx)
		log.Info("research synthesis outline bounded by profile",
			"phase", "synthesis",
			"profile", ProfileFromContext(ctx),
			"sections_planned", len(leafNodes),
			"sections_selected", budget.MaxSections,
		)
		leafNodes = selectSectionsAcrossOutline(leafNodes, budget.MaxSections)
	}
	log := loggerForResearchContext(ctx)
	log.Info("research synthesis outline ready", "phase", "synthesis", "sections_total", len(leafNodes), "sources_total", len(sources))
	checkpoint, checkpointErr := loadSynthesisCheckpoint(ctx, leafNodes, sources)
	if checkpointErr != nil {
		log.Warn("research synthesis checkpoint unavailable; continuing without prior sections",
			"phase", "synthesis",
			"error_kind", observableErrorKind(checkpointErr),
		)
		checkpoint = newSynthesisCheckpoint(leafNodes, sources)
	}

	for index, node := range leafNodes {
		sectionContent, resumed := checkpoint.reusableSection(node.ID, sources)
		var err error
		if resumed {
			log.Info("research synthesis section resumed from checkpoint",
				"phase", "synthesis",
				"section_index", index+1,
				"sections_total", len(leafNodes),
				"node_id", node.ID,
			)
		} else {
			endSection := progressOperation(ctx, "synthesis", "section_generation",
				"section_index", index+1,
				"sections_total", len(leafNodes),
				"node_id", node.ID,
			)
			sectionContent, err = s.synthesizeNodeSectionWithBudget(ctx, topic, node, slidingCtx, evidenceCatalogue, conceptSummary, insightsSummary, chronologicalTimeline, len(sources), budget)
			_, compactRecoveryAllowed := llm.TransientErrorKind(err)
			if err != nil && ctx.Err() == nil && compactRecoveryAllowed {
				loggerForResearchContext(ctx).Warn("research synthesis section compact recovery activated",
					"phase", "synthesis",
					"section_index", index+1,
					"sections_total", len(leafNodes),
					"node_id", node.ID,
					"error_kind", observableErrorKind(err),
				)
				recoveryCatalogue := boundedEvidenceCatalogue(ctx, evidence, sources, budget.RecoveryEvidenceChars)
				sectionContent, err = s.synthesizeNodeSectionWithBudget(ctx, topic, node, slidingCtx, recoveryCatalogue, conceptSummary, insightsSummary, chronologicalTimeline, len(sources), budget)
			}
			endSection(err)
			if err != nil {
				return "", fmt.Errorf("synthesize node %s (%s): %w", node.ID, node.Title, err)
			}
		}
		sectionContent = stripGeneratedReferences(sectionContent)
		if sectionErr := validateSynthesizedSection(sectionContent, sources); sectionErr != nil {
			endRepair := progressOperation(ctx, "synthesis", "section_validation_repair",
				"section_index", index+1,
				"sections_total", len(leafNodes),
				"node_id", node.ID,
			)
			sectionContent, err = s.repairNodeSection(ctx, node, sectionContent, evidenceCatalogue, sources, sectionErr)
			endRepair(err)
			if err != nil {
				return "", fmt.Errorf("repair citations in node %s (%s): %w", node.ID, node.Title, err)
			}
		}

		docBuf.WriteString(sectionContent)
		docBuf.WriteString("\n\n")

		slidingCtx.PreviousSectionTitle = node.Title
		slidingCtx.LastParagraphs = extractTailParagraphs(sectionContent, 3)
		slidingCtx.ActiveEntities = updateActiveEntities(slidingCtx.ActiveEntities, sectionContent)
		checkpoint.Sections[node.ID] = sectionContent
		if err := saveSynthesisCheckpoint(ctx, checkpoint, sources); err != nil {
			log.Warn("research synthesis checkpoint could not be saved",
				"phase", "synthesis",
				"section_index", index+1,
				"error_kind", observableErrorKind(err),
			)
		}
		log.Info("research synthesis progress",
			"phase", "synthesis",
			"sections_completed", index+1,
			"sections_total", len(leafNodes),
			"node_id", node.ID,
			"section_characters", len([]rune(sectionContent)),
		)
	}

	// Append Unified References Section
	docBuf.WriteString(researchText(ctx, "## References\n\n", "## Referencias\n\n"))
	cited := citedSourceNumbers(docBuf.String())
	for _, src := range sources {
		if _, ok := cited[src.Number]; !ok {
			continue
		}
		docBuf.WriteString(fmt.Sprintf("[%d] %s — %s\n", src.Number, src.Title, src.URL))
	}

	report := docBuf.String()
	if validationErr := validateSynthesizedReport(report, sources); validationErr == nil {
		return report, nil
	} else {
		endRepair := progressOperation(ctx, "synthesis", "final_citation_repair")
		repaired, err := s.repairCitations(ctx, report, sources, validationErr)
		endRepair(err)
		return repaired, err
	}
}

func selectSectionsAcrossOutline(nodes []*TaxonNode, limit int) []*TaxonNode {
	if limit <= 0 || len(nodes) <= limit {
		return nodes
	}
	if limit == 1 {
		return nodes[:1]
	}
	selected := make([]*TaxonNode, 0, limit)
	lastIndex := -1
	for i := 0; i < limit; i++ {
		index := i * (len(nodes) - 1) / (limit - 1)
		if index == lastIndex {
			continue
		}
		selected = append(selected, nodes[index])
		lastIndex = index
	}
	return selected
}

func (s *Synthesizer) synthesizeNodeSection(ctx context.Context, topic string, node *TaxonNode, slidingCtx *SlidingContext, evidenceStr, conceptSummary, insightsSummary, chronologicalTimeline string, sourceCount int) (string, error) {
	return s.synthesizeNodeSectionWithBudget(ctx, topic, node, slidingCtx, evidenceStr, conceptSummary, insightsSummary, chronologicalTimeline, sourceCount, budgetForProfile(ProfileFromContext(ctx)))
}

func (s *Synthesizer) synthesizeNodeSectionWithBudget(ctx context.Context, topic string, node *TaxonNode, slidingCtx *SlidingContext, evidenceStr, conceptSummary, insightsSummary, chronologicalTimeline string, sourceCount int, budget profileBudget) (string, error) {
	transitionDirective := ""
	if slidingCtx.PreviousSectionTitle != "" && slidingCtx.LastParagraphs != "" {
		transitionDirective = fmt.Sprintf(researchText(ctx,
			"\nDISCOURSE CONNECTION AND TRANSITION:\nPrevious section: %q\nLast sentences of the previous section:\n%q\nBegin this section with a fluid transition paragraph that connects logically to the end of the previous section.",
			"\nCONEXIÓN Y TRANSICIÓN DISCURSIVA:\nSección procedente: %q\nÚltimas oraciones de la sección anterior:\n%q\nDebes comenzar esta sección con un párrafo de transición fluido que conecte lógicamente con el final de la sección anterior.",
		), slidingCtx.PreviousSectionTitle, slidingCtx.LastParagraphs)
	}

	entitiesDirective := ""
	if len(slidingCtx.ActiveEntities) > 0 {
		entitiesDirective = fmt.Sprintf(researchText(ctx,
			"\nACTIVE ENTITIES AND CONCEPTS INTRODUCED IN EARLIER SECTIONS:\n- %s\nMaintain conceptual and terminological consistency and naming rigor for these entities.",
			"\nENTIDADES Y CONCEPTOS ACTIVOS INTRODUCIDOS EN SECCIONES ANTERIORES:\n- %s\nMantén la coherencia conceptual, terminológica y el rigor de denominación con estas entidades.",
		), strings.Join(slidingCtx.ActiveEntities, "\n- "))
	}

	headerPrefix := "##"
	if node.Level == 2 {
		headerPrefix = "###"
	} else if node.Level >= 3 {
		headerPrefix = "####"
	}

	promptTemplate := researchText(ctx, `Global Topic: %q
Section to Write: %s %s
Scope Description: %s
Key Questions to Answer: %s
%s
%s

STRUCTURE AND EVIDENCE:
Concept Map: %s
DataSTORM Findings: %s
Chronos Temporal Graph (Timeline):
%s

Retrieved Evidence (immutable catalogue from [1] to [%d]):
%s

STRICT WRITING AND CITATION INSTRUCTIONS:
1. Write the section beginning with the Markdown heading %s %s.
2. Develop it with the requested analytical depth (%s words for this section), without superficial summaries or bullet lists.
3. Follow the chronology in the Chronos Temporal Graph and avoid factual drift or anachronisms.
4. Numeric citations: support every key claim with [n] citations to valid sources in the range [1] to [%d].
5. Maintain grammatical flow, Reinhart cohesion and coherence, and resolve pronouns unambiguously.`, `Tema Global: %q
Sección a Redactar: %s %s
Descripción del Alcance: %s
Preguntas Clave a Responder: %s
%s
%s

ESTRUCTURA Y EVIDENCIA:
Mapa Conceptual: %s
Hallazgos DataSTORM: %s
Chronos Temporal Graph (Línea de Tiempo):
%s

Evidencia Recuperada (Catálogo inmutable del [1] a [%d]):
%s

INSTRUCCIONES ESTRICTAS DE REDACCIÓN Y CITAS:
1. Redacta la sección completando el encabezado Markdown %s %s.
2. Desarrolla la sección con la profundidad analítica solicitada (%s palabras para este apartado), sin resumir ni usar viñetas superficiales.
3. Respeta la evolución cronológica indicada por el Chronos Temporal Graph, evitando la deriva fáctica o anacronismos.
4. Citas Numéricas: DEBES respaldar cada afirmación clave usando citas en formato [n] referenciando fuentes válidas en el rango [1] a [%d].
5. Mantén la fluidez gramatical (cohesión y coherencia de Reinhart) y resuelve pronombres de forma unívoca.`)
	prompt := fmt.Sprintf(promptTemplate, topic, headerPrefix, node.Title, node.Description, strings.Join(node.KeyQuestions, "; "), transitionDirective, entitiesDirective, conceptSummary, insightsSummary, chronologicalTimeline, sourceCount, evidenceStr, headerPrefix, node.Title, budget.SectionWords, sourceCount)

	messages := []llm.Message{
		{Role: "system", Content: researchText(ctx,
			"You are Pharus, the lead technical Deep Research writer. You write extensive, rigorous sections with continuous empirical citations.",
			"Eres Pharus, redactor técnico principal de Deep Research. Redactas secciones extensas, rigurosas y con citación empírica ininterrumpida.",
		) + "\n\n" + reportLanguageDirective(ctx)},
		{Role: "user", Content: prompt},
	}

	requestCtx := llmtypes.WithRequestOptions(ctx, llmtypes.RequestOptions{
		MaxTokens: budget.SectionMaxTokens, MaxAttempts: budget.SectionAttempts, Timeout: budget.SectionTimeout,
	})
	return s.llm.GenerateCompletion(requestCtx, messages, 0.2)
}

func boundedEvidenceCatalogue(ctx context.Context, evidence []vectordb.SearchResult, sources []CitationSource, maxCharacters int) string {
	if maxCharacters <= 0 {
		maxCharacters = 2500
	}
	var catalogue strings.Builder
	for i, item := range evidence {
		if i >= len(sources) {
			break
		}
		content := []rune(strings.TrimSpace(item.Content))
		if len(content) > maxCharacters {
			content = content[:maxCharacters]
		}
		source := sources[i]
		template := researchText(ctx,
			"[%d] URL: %s\nTitle: %s\nEvidence level: %s\nRetrieved excerpt:\n%s\n\n",
			"[%d] URL: %s\nTítulo: %s\nNivel de evidencia: %s\nExtracto recuperado:\n%s\n\n",
		)
		catalogue.WriteString(fmt.Sprintf(template, source.Number, source.URL, source.Title, source.Type, string(content)))
	}
	return catalogue.String()
}

func (s *Synthesizer) repairNodeSection(ctx context.Context, node *TaxonNode, section, evidenceCatalogue string, sources []CitationSource, validationErr error) (string, error) {
	promptTemplate := researchText(ctx, `The following Markdown section failed citation validation: %v.

SECTION:
%s

IMMUTABLE EVIDENCE CATALOGUE:
%s

Rewrite ONLY this section:
- Preserve its heading and useful content.
- Support every factual claim with [n] citations that match the catalogue.
- Use only numbers from [1] to [%d] and do not write URLs.
- Translate or transliterate any unrelated writing system into correct technical English.
- Do not add a references section or explanations.`, `El apartado Markdown siguiente no pasó la validación de citas: %v.

APARTADO:
%s

CATÁLOGO DE EVIDENCIA INMUTABLE:
%s

Reescribe ÚNICAMENTE este apartado:
- Conserva el encabezado y el contenido útil.
- Respalda toda afirmación factual con citas [n] que correspondan al catálogo.
- Usa sólo números del [1] al [%d] y no escribas URLs.
- Sustituye cualquier carácter o término chino, japonés o coreano por su traducción técnica correcta en español.
- No agregues una sección de referencias ni explicaciones.`)
	prompt := fmt.Sprintf(promptTemplate, validationErr, section, evidenceCatalogue, len(sources))
	messages := []llm.Message{
		{Role: "system", Content: researchText(ctx,
			"You are Pharus. You repair one Markdown section locally using only retrieved evidence.",
			"Eres Pharus. Reparas de forma localizada un único apartado Markdown usando sólo evidencia recuperada.",
		) + "\n\n" + reportLanguageDirective(ctx)},
		{Role: "user", Content: prompt},
	}

	repaired, err := s.llm.GenerateCompletion(ctx, messages, 0.1)
	if err != nil {
		return "", fmt.Errorf("repair section %s: %w", node.Title, err)
	}
	repaired = stripGeneratedReferences(repaired)
	if err := validateSynthesizedSection(repaired, sources); err != nil {
		return "", err
	}
	return repaired, nil
}

func stripGeneratedReferences(section string) string {
	section = strings.TrimSpace(section)
	withoutReferences := strings.TrimSpace(reportWithoutReferences(section))
	if withoutReferences != "" {
		return withoutReferences
	}
	return section
}

func validateSectionCitations(section string, sources []CitationSource) error {
	allowed := make(map[int]struct{}, len(sources))
	allowedURLs := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		allowed[source.Number] = struct{}{}
		allowedURLs[normalizeCitationURL(source.URL)] = struct{}{}
	}

	var problems []string
	validCitations := 0
	for _, match := range citationPattern.FindAllStringSubmatch(section, -1) {
		var number int
		_, _ = fmt.Sscanf(match[1], "%d", &number)
		if _, ok := allowed[number]; !ok {
			problems = append(problems, fmt.Sprintf("citation [%d] is not in the source catalogue", number))
			continue
		}
		validCitations++
	}
	for _, rawURL := range urlPattern.FindAllString(section, -1) {
		if _, ok := allowedURLs[normalizeCitationURL(rawURL)]; !ok {
			problems = append(problems, fmt.Sprintf("URL %q was not recovered in this research", rawURL))
		}
	}
	for _, title := range uncitedFactualSectionTitles(section) {
		problems = append(problems, fmt.Sprintf("factual section %q has no citation", title))
	}
	if len([]rune(strings.TrimSpace(section))) > 40 && validCitations == 0 {
		problems = append(problems, "section has no valid citation")
	}
	if len(problems) == 0 {
		return nil
	}
	return &CitationValidationError{Problems: problems}
}

func validateSynthesizedSection(section string, sources []CitationSource) error {
	if err := ValidateReportLanguage(section); err != nil {
		return err
	}
	return validateSectionCitations(section, sources)
}

func validateSynthesizedReport(report string, sources []CitationSource) error {
	if err := ValidateReportLanguage(report); err != nil {
		return err
	}
	return ValidateCitations(report, sources)
}

func citedSourceNumbers(report string) map[int]struct{} {
	cited := make(map[int]struct{})
	for _, match := range citationPattern.FindAllStringSubmatch(reportWithoutReferences(report), -1) {
		var number int
		_, _ = fmt.Sscanf(match[1], "%d", &number)
		cited[number] = struct{}{}
	}
	return cited
}

func (s *Synthesizer) repairCitations(ctx context.Context, report string, sources []CitationSource, validationErr error) (string, error) {
	repairTemplate := researchText(ctx,
		"The complete report failed automatic validation: %v.\n\nPREVIOUS RESPONSE:\n%s\n\nRepair instruction:\nRewrite the report and correct ALL listed errors.\n- The immutable catalogue contains exactly %d sources ([1] to [%d]). Do not use numbers outside [1] to [%d].\n- Ensure that EVERY factual section contains numeric [n] citations.\n- Translate or transliterate unrelated writing systems into correct technical English.\n- In the '## References' section, include EXACTLY lines in the format `[n] Title — URL`.\n- Do not explain the repair.",
		"El informe completo redactado no pasó la validación automática: %v.\n\nRESPUESTA ANTERIOR:\n%s\n\nInstrucción de reparación:\nReescribe el informe corrigiendo TODOS los errores indicados.\n- El catálogo inmutable tiene exactamente %d fuentes ([1] a [%d]). NO uses números fuera de [1] a [%d].\n- Asegúrate de que CADA sección factual tenga citas numéricas [n].\n- Elimina cualquier carácter chino, japonés o coreano del cuerpo y traduce el término al español técnico correcto.\n- En la sección '## Referencias', incluye EXACTAMENTE las líneas en el formato `[n] Título — URL`.\n- No expliques la reparación.",
	)
	repair := fmt.Sprintf(repairTemplate, validationErr, report, len(sources), len(sources), len(sources))
	messages := []llm.Message{
		{Role: "system", Content: researchText(ctx,
			"You are Pharus. You repair citations, language, and references in Markdown reports while preserving their full length and structure.",
			"Eres Pharus. Reparas citas, idioma y referencias de informes Markdown manteniendo la longitud y estructura completa.",
		) + "\n\n" + reportLanguageDirective(ctx)},
		{Role: "user", Content: repair},
	}

	repaired, repairErr := s.llm.GenerateCompletion(ctx, messages, 0.1)
	if repairErr != nil {
		return "", fmt.Errorf("repair invalid citations: %w", repairErr)
	}
	if err := validateSynthesizedReport(repaired, sources); err != nil {
		return "", err
	}
	return repaired, nil
}

func (s *Synthesizer) fallbackHierarchicalReport(ctx context.Context, topic string, taxonTree *TaxmorphTree, sources []CitationSource) (string, error) {
	var docBuf strings.Builder
	docBuf.WriteString(fmt.Sprintf(researchText(ctx, "# Deep Research Report: %s\n\n", "# Informe de Deep Research: %s\n\n"), topic))

	for _, top := range taxonTree.Nodes {
		docBuf.WriteString(fmt.Sprintf("## %s\n\n", top.Title))
		docBuf.WriteString(fmt.Sprintf(researchText(ctx,
			"%s. This analysis empirically evaluates the central claims supported by the retrieved primary sources [1].\n\n",
			"%s. Este análisis evalúa empíricamente los postulados centrales respaldados por las fuentes primarias recuperadas [1].\n\n",
		), top.Description))

		for _, sub := range top.SubNodes {
			docBuf.WriteString(fmt.Sprintf("### %s\n\n", sub.Title))
			docBuf.WriteString(fmt.Sprintf(researchText(ctx,
				"%s. The indexed evidence confirms the operational and methodological parameters [1].\n\n",
				"%s. La evidencia indizada confirma los parámetros operativos y metodológicos [1].\n\n",
			), sub.Description))
		}
	}

	docBuf.WriteString(researchText(ctx, "## References\n\n", "## Referencias\n\n"))
	cited := citedSourceNumbers(docBuf.String())
	for _, src := range sources {
		if _, ok := cited[src.Number]; !ok {
			continue
		}
		docBuf.WriteString(fmt.Sprintf("[%d] %s — %s\n", src.Number, src.Title, src.URL))
	}

	report := docBuf.String()
	if err := validateSynthesizedReport(report, sources); err != nil {
		return "", err
	}
	return report, nil
}

func (s *Synthesizer) SynthesizeEnrichedReport(ctx context.Context, topic string, evidence []vectordb.SearchResult, conceptMap *ConceptMap, insightBank *GlobalInsightBank) (string, error) {
	return s.SynthesizeHierarchicalReport(ctx, topic, nil, evidence, conceptMap, insightBank)
}

func (s *Synthesizer) SynthesizeReport(ctx context.Context, topic string, evidence []vectordb.SearchResult) (string, error) {
	return s.SynthesizeEnrichedReport(ctx, topic, evidence, nil, nil)
}

func citationSources(evidence []vectordb.SearchResult) []CitationSource {
	sources := make([]CitationSource, 0, len(evidence))
	for _, item := range evidence {
		if item.Metadata == nil || strings.TrimSpace(item.Metadata["url"]) == "" {
			continue
		}
		typ := EvidenceType(item.Metadata["evidence_type"])
		if typ == "" {
			typ = EvidenceTypeFullText
		}
		sources = append(sources, CitationSource{Number: len(sources) + 1, ID: item.ID, URL: item.Metadata["url"], Title: item.Metadata["title"], Type: typ})
	}
	return sources
}

func extractTailParagraphs(text string, count int) string {
	lines := strings.Split(text, "\n")
	var tail []string
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			tail = append([]string{trimmed}, tail...)
			if len(tail) >= count {
				break
			}
		}
	}
	return strings.Join(tail, " ")
}

func updateActiveEntities(existing []string, text string) []string {
	seen := make(map[string]bool)
	for _, e := range existing {
		seen[e] = true
	}
	result := append([]string{}, existing...)

	words := strings.Fields(text)
	for _, w := range words {
		cleaned := strings.Trim(w, "(),.:;\"'[]`*#")
		if len(cleaned) >= 2 && isAcronymOrTechnicalTerm(cleaned) {
			if !seen[cleaned] {
				seen[cleaned] = true
				result = append(result, cleaned)
				if len(result) >= 15 {
					break
				}
			}
		}
	}
	return result
}

func isAcronymOrTechnicalTerm(w string) bool {
	if len(w) < 2 || len(w) > 20 {
		return false
	}
	upperCount := 0
	for _, r := range w {
		if r >= 'A' && r <= 'Z' {
			upperCount++
		}
	}
	// All caps acronym like GBNF, LLM, REST, SSRF, RAG
	if upperCount == len(w) && upperCount >= 2 {
		return true
	}
	// CamelCase / Mixed like DataSTORM, LanceDB, Co-STORM, TAXMORPH
	if upperCount >= 2 {
		return true
	}
	// TitleCase capitalized term like Pharus, SearXNG, Ollama
	if len(w) >= 4 && w[0] >= 'A' && w[0] <= 'Z' {
		return true
	}
	return false
}
