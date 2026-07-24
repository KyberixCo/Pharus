package research

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/KyberixCo/Pharus/internal/config"
	"github.com/KyberixCo/Pharus/internal/embedding"
	"github.com/KyberixCo/Pharus/internal/i18n"
	"github.com/KyberixCo/Pharus/internal/llm"
	"github.com/KyberixCo/Pharus/internal/llm/llamacpp"
	"github.com/KyberixCo/Pharus/internal/scraper"
	"github.com/KyberixCo/Pharus/internal/vectordb"
	"github.com/KyberixCo/Pharus/pkg/logger"
	"github.com/philippgille/chromem-go"
)

type Engine struct {
	cfg         *config.Config
	llm         llm.Provider
	localLLM    *llamacpp.Client
	embed       embedding.Provider
	vectorDB    vectordb.VectorStore
	searx       *scraper.SearXNGClient
	fetcher     pageFetcher
	discourse   *DiscourseManager
	planner     *QueryPlanner
	planBuilder *PlanBuilder
	taxmorph    *TaxmorphService
	dataSTORM   *DataSTORMAnalyzer
	synth       *Synthesizer
}

func NewEngine(cfg *config.Config) (*Engine, error) {
	structuredData, err := StructuredDataProviderFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to configure DataSTORM: %w", err)
	}
	return NewEngineWithStructuredDataProvider(cfg, structuredData)
}

// NewEngineWithStructuredDataProvider constructs an engine with an explicitly
// configured structured-data source. Passing nil keeps DataSTORM disabled;
// Pharus must never substitute demonstration rows in a production research.
func NewEngineWithStructuredDataProvider(cfg *config.Config, structuredData StructuredDataProvider) (*Engine, error) {
	llmProvider, err := llm.NewProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to init LLM provider: %w", err)
	}

	localLLMClient := llamacpp.NewClient(cfg)
	embedProvider := embedding.NewProvider(cfg)

	var vStore vectordb.VectorStore

	collection := cfg.Vector.Collection
	if collection == "" {
		collection = "pharus_research"
	}
	// A model change gets a distinct persistent collection. This preserves old
	// corpora while making it impossible to compare vectors from different
	// embedding spaces.
	collection = collection + "_" + embeddingModelKey(cfg.Embed.Provider, cfg.Embed.Model)
	embedFunc := func(ctx context.Context, text string) ([]float32, error) { return embedProvider.Embed(ctx, text) }
	switch strings.ToLower(strings.TrimSpace(cfg.Vector.Provider)) {
	case "", "chromem":
		vStore, err = vectordb.NewStore(cfg.VectorDir, collection, chromem.EmbeddingFunc(embedFunc))
	case "lancedb":
		vStore, err = vectordb.NewLanceDBStore(cfg.VectorDir, collection, embedFunc)
	default:
		return nil, fmt.Errorf("unsupported vector provider %q", cfg.Vector.Provider)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to init vector store: %w", err)
	}

	searxClient, err := scraper.NewSearXNGClientWithOptions(
		cfg.Search.SearXNGURL,
		time.Duration(cfg.Search.SearXNGTimeoutSeconds)*time.Second,
		cfg.Search.SearXNGMaxResponseBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to configure SearXNG client: %w", err)
	}
	secureFetcher := scraper.NewSecureFetcher()
	discourseMgr := NewDiscourseManager(llmProvider)
	queryPlanner := NewQueryPlanner(llmProvider, QueryPlannerConfig{
		MaxCharacters: cfg.Search.QueryMaxCharacters,
		MaxTerms:      cfg.Search.QueryMaxTerms,
	})
	datastormAnalyzer := NewDataSTORMAnalyzer(discourseMgr.InsightBank, structuredData, llmProvider)

	return &Engine{
		cfg:         cfg,
		llm:         llmProvider,
		localLLM:    localLLMClient,
		embed:       embedProvider,
		vectorDB:    vStore,
		searx:       searxClient,
		fetcher:     secureFetcher,
		discourse:   discourseMgr,
		planner:     queryPlanner,
		planBuilder: NewPlanBuilder(llmProvider),
		taxmorph:    NewTaxmorphService(llmProvider),
		dataSTORM:   datastormAnalyzer,
		synth:       NewSynthesizer(llmProvider),
	}, nil
}

// LocalLLM returns the local llama.cpp inference client.
func (e *Engine) LocalLLM() *llamacpp.Client {
	return e.localLLM
}

// LLMProvider exposes the configured provider for auxiliary components such as
// the benchmark evaluator. Research execution itself remains encapsulated.
func (e *Engine) LLMProvider() llm.Provider {
	return e.llm
}

// SearchEvidence exposes real semantic retrieval to transports such as MCP.
func (e *Engine) SearchEvidence(ctx context.Context, query string, topK int) ([]vectordb.SearchResult, error) {
	return e.SearchEvidenceFiltered(ctx, query, topK, nil)
}

// SearchEvidenceFiltered restricts retrieval to exact metadata values, for
// example a single research_id.
func (e *Engine) SearchEvidenceFiltered(ctx context.Context, query string, topK int, filter map[string]string) ([]vectordb.SearchResult, error) {
	if e == nil || e.vectorDB == nil {
		return nil, fmt.Errorf("vector store is not initialized")
	}
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("vector search query is required")
	}
	if topK <= 0 {
		topK = 5
	}
	return e.vectorDB.SearchSimilarFiltered(ctx, query, topK, filter)
}

// ExecuteResearch preserves the legacy interface used by benchmarks. New
// transport code should use ExecuteResearchResult to retain status and failure
// information.
func (e *Engine) ExecuteResearch(ctx context.Context, topic string) (string, error) {
	result, err := e.ExecuteResearchResult(ctx, topic)
	if err != nil {
		return "", err
	}
	return result.Report, nil
}

// ExecuteResearchResult runs the research workflow and refuses to synthesize a
// report unless web evidence was successfully collected.
func (e *Engine) ExecuteResearchResult(ctx context.Context, topic string) (*ResearchResult, error) {
	profile := ProfileFromContext(ctx)
	if _, explicitlySet := ctx.Value(profileContextKey{}).(Profile); !explicitlySet && e.cfg != nil {
		if configured, err := ParseProfile(e.cfg.Research.DefaultProfile); err == nil {
			profile = configured
		}
	}
	ctx = WithProfile(ctx, profile)
	budget := budgetForProfile(profile)
	language, hasLanguage := i18n.LanguageFromContext(ctx)
	if !hasLanguage {
		requested := "auto"
		if e.cfg != nil {
			requested = e.cfg.Language
		}
		resolved, err := i18n.Resolve(requested, nil)
		if err != nil {
			return nil, fmt.Errorf("resolve research language: %w", err)
		}
		language = resolved
		ctx = i18n.WithLanguage(ctx, language)
	}
	researchID := researchExecutionID(topic + "\x00" + string(language) + "\x00profile=" + string(profile))
	observer := newResearchObserver(logger.Get(), researchID)
	ctx = llm.WithCorrelationID(ctx, researchID)
	ctx = withResearchObserver(ctx, observer)
	if e.cfg != nil && e.cfg.Research.ResumeSessions {
		maxAgeHours := e.cfg.Research.CheckpointMaxAgeHours
		if maxAgeHours <= 0 {
			maxAgeHours = 72
		}
		sessionTopic := profileSessionTopic(topic, language, profile)
		ctx = withSynthesisCheckpoint(ctx, e.cfg.BaseDir, sessionTopic, time.Duration(maxAgeHours)*time.Hour)
	}
	log := observer.log
	log.Info("research workflow started", "profile", profile)
	warnings := make([]string, 0)

	// 1. Co-STORM: Generate expert perspectives, ResearchPlan & TAXMORPH TaxonTree
	endPlanning := observer.phase("planning")
	var researchPlan *ResearchPlan
	var taxonTree *TaxmorphTree
	planningCheckpoint, checkpointErr := loadPlanningCheckpoint(ctx)
	if checkpointErr != nil {
		log.Warn("research planning checkpoint unavailable; rebuilding plan", "phase", "planning", "error_kind", observableErrorKind(checkpointErr))
	}
	if planningCheckpoint != nil {
		researchPlan = planningCheckpoint.ResearchPlan
		taxonTree = planningCheckpoint.TaxonTree
	} else {
		planBuilder := e.planBuilder
		if planBuilder == nil {
			planBuilder = NewPlanBuilder(e.llm)
		}
		endPlanBuild := progressOperation(ctx, "planning", "research_plan")
		var researchPlanErr error
		researchPlan, researchPlanErr = planBuilder.BuildPlan(ctx, topic, &ResearchIntent{Depth: budget.Depth})
		endPlanBuild(researchPlanErr)

		taxmorph := e.taxmorph
		if taxmorph == nil {
			taxmorph = NewTaxmorphService(e.llm)
		}
		endTaxonomy := progressOperation(ctx, "planning", "taxonomy_refinement")
		var taxonomyErr error
		taxonTree, taxonomyErr = taxmorph.RefineOutline(ctx, researchPlan)
		endTaxonomy(taxonomyErr)
		if err := savePlanningCheckpoint(ctx, researchPlan, taxonTree); err != nil {
			log.Warn("research planning checkpoint could not be saved", "phase", "planning", "error_kind", observableErrorKind(err))
		}
	}
	if taxonTree != nil {
		log.Info("research taxonomy ready", "phase", "planning", "nodes", taxonTree.NodeCount(), "leaf_sections", len(taxonTree.LeafNodes()), "depth", taxonTree.Depth())
	}

	endPerspectives := progressOperation(ctx, "planning", "expert_perspectives")
	roles, err := e.discourse.GeneratePerspectives(ctx, topic)
	endPerspectives(err)
	if err != nil {
		log.Warn("research perspectives unavailable; using defaults", "error_kind", observableErrorKind(err))
		warnings = append(warnings, fmt.Sprintf("No se pudieron generar perspectivas: %v", err))
	}

	// 2. Build short search-engine queries rather than concatenating narrative
	// perspectives to the topic.
	planner := e.planner
	if planner == nil {
		// Keep manually constructed engines (notably focused tests and embedders)
		// on the same safe deterministic planning path.
		planner = NewQueryPlanner(e.llm, QueryPlannerConfig{})
	}
	endQueryPlan := progressOperation(ctx, "planning", "primary_query_plan")
	queryPlan, planErr := planner.PlanPrimary(ctx, topic, roles)
	if len(queryPlan) > budget.MaxQueries {
		queryPlan = queryPlan[:budget.MaxQueries]
	}
	endQueryPlan(planErr)
	endPlanning(planErr)
	if planErr != nil {
		result, resultErr := failedResult(FailureCodeEvidenceInsufficient, 0, warnings, fmt.Errorf("failed to create search query plan: %w", planErr))
		result.ResearchID, result.PhaseDurations, result.ResearchPlan, result.TaxonTree = researchID, observer.durations, researchPlan, taxonTree
		observer.summary(result, resultErr)
		return result, resultErr
	}
	metrics := EvidenceMetrics{Failures: map[string]int{}}
	var evidence []Evidence
	failedWithPlan := func(code FailureCode, evidenceCount int, cause error) (*ResearchResult, error) {
		result, err := failedResult(code, evidenceCount, warnings, cause)
		result.ResearchID = researchID
		result.QueryPlan = queryPlan
		result.ResearchPlan = researchPlan
		result.TaxonTree = taxonTree
		result.Evidence = evidence
		result.EvidenceMetrics = metrics
		result.PhaseDurations = observer.durations
		observer.summary(result, err)
		return result, err
	}

	// 3. Collect a traceable corpus from every planned query.
	collector := NewEvidenceCollector(e.fetcher, researchID)
	var searchErrors []error
	collectQuery := func(query SearchQuery, maxResults int) {
		endSearch := observer.phase("search")
		results, err := e.searx.Search(ctx, query.Text, maxResults)
		endSearch(err)
		if err != nil {
			log.Warn("research search request failed", "error_kind", observableErrorKind(err))
			searchErrors = append(searchErrors, err)
			return
		}
		endExtraction := observer.phase("extraction")
		_, collectedMetrics := collector.Collect(ctx, query, results)
		endExtraction(nil)
		MergeEvidenceMetrics(&metrics, collectedMetrics)
	}
	for _, query := range queryPlan {
		collectQuery(query, budget.SearchResultsPerQuery)
	}
	evidence = collector.Evidence()

	// A report without retrieved evidence is unsafe: do not run gap analysis,
	// DataSTORM, vector indexing, or the LLM synthesizer.
	if len(evidence) == 0 {
		var cause error
		code := FailureCodeEvidenceInsufficient
		if len(searchErrors) > 0 {
			code = FailureCodeSearchUnavailable
			cause = fmt.Errorf("all %d search request(s) failed: %w", len(searchErrors), errors.Join(searchErrors...))
		} else {
			cause = fmt.Errorf("search completed but did not yield usable documents")
		}
		log.Error("Research stopped before synthesis: no usable evidence", "failure_code", code, "search_failures", len(searchErrors))
		return failedWithPlan(code, 0, cause)
	}
	for _, searchErr := range searchErrors {
		warnings = append(warnings, fmt.Sprintf("Una consulta de búsqueda falló: %v", searchErr))
	}

	// 4. Review the initial evidence and issue at most two planned gap queries.
	log.Info("research evidence review started")
	evidenceSummary := ""
	for _, doc := range evidence {
		if len(evidenceSummary) < 1500 {
			evidenceSummary += doc.Content + "\n"
		}
	}

	endGapPlanning := observer.phase("planning")
	endGapAnalysis := progressOperation(ctx, "planning", "evidence_gap_analysis")
	gapQueries, err := planner.PlanGaps(ctx, topic, evidenceSummary)
	endGapAnalysis(err)
	endGapPlanning(err)
	if err == nil && len(gapQueries) > 0 {
		queryPlan = append(queryPlan, gapQueries...)
		for _, query := range gapQueries {
			log.Info("research gap query planned", "purpose", query.Purpose)
			collectQuery(query, 2)
			e.discourse.ConceptMap.ResolveGap(query.Purpose, query.Text)
		}
	}
	evidence = collector.Evidence()
	policy := DefaultEvidencePolicy()
	if e.cfg != nil {
		policy = EvidencePolicy{MinimumUsable: e.cfg.Search.EvidenceMinimumUsable, MinimumUniqueURLs: e.cfg.Search.EvidenceMinimumURLs, MinimumFullText: e.cfg.Search.EvidenceMinimumFullText}
	}
	status, sufficiencyErr := EvidenceSufficiency(evidence, policy)
	if sufficiencyErr != nil {
		return failedWithPlan(FailureCodeEvidenceInsufficient, len(evidence), sufficiencyErr)
	}
	if status == ResearchStatusDegraded {
		warnings = append(warnings, "La evidencia disponible contiene solo snippets o no alcanza el mínimo de texto completo.")
	}
	scrapedDocs := collector.Documents(evidence)
	principal := principalFromContext(ctx)
	for i := range scrapedDocs {
		if scrapedDocs[i].Metadata == nil {
			scrapedDocs[i].Metadata = map[string]string{}
		}
		scrapedDocs[i].Metadata["research_id"] = researchID
		scrapedDocs[i].Metadata["embedding_model"] = e.cfg.Embed.Model
		if principal != "" {
			scrapedDocs[i].Metadata["user_id"] = principal
		}
	}
	endEnrichment := progressOperation(ctx, "analysis", "evidence_enrichment", "items_total", len(evidence))
	for index, item := range evidence {
		e.discourse.UpdateConceptMapFromEvidence(ctx, topic, item.Content)
		_, _ = e.discourse.InsightBank.ExtractInsightsFromEvidence(ctx, e.llm, topic, item)
		if (index+1)%5 == 0 || index+1 == len(evidence) {
			log.Info("research evidence enrichment progress", "phase", "analysis", "items_completed", index+1, "items_total", len(evidence))
		}
	}
	endEnrichment(nil)

	// 3.5. DataSTORM: Exploratory Relational Data & Hypothesis Cross-Validation
	if e.dataSTORM != nil && e.dataSTORM.Enabled() {
		log.Info("research structured-data analysis started")
		hypothesis := fmt.Sprintf("Evaluación analítica cuantitativa de métricas relacionales para %s", topic)
		_, datastormInsights, err := e.dataSTORM.ExploreHypothesis(ctx, topic, hypothesis)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("El análisis DataSTORM falló: %v", err))
		} else if len(datastormInsights) > 0 {
			log.Info("research structured-data analysis completed", "insight_count", len(datastormInsights))
		}
	}

	// 4. Index into local chromem-go / LanceDB vector DB
	// Vector stores perform embeddings while adding documents. Record both
	// operational phases; their durations intentionally overlap.
	endEmbeddings := observer.phase("embeddings")
	endIndexing := observer.phase("indexing")
	if err := e.vectorDB.AddDocuments(ctx, scrapedDocs); err != nil {
		endEmbeddings(err)
		endIndexing(err)
		log.Error("research vector indexing failed", "error_kind", observableErrorKind(err))
		return failedWithPlan(FailureCodeVectorIndexing, len(scrapedDocs), fmt.Errorf("failed to index evidence: %w", err))
	}
	endEmbeddings(nil)
	endIndexing(nil)

	// 5. Retrieve top relevant vector segments
	endRetrieval := observer.phase("retrieval")
	nResults := budget.RetrievalResults
	if len(scrapedDocs) < nResults {
		nResults = len(scrapedDocs)
	}
	topEvidence, err := e.vectorDB.SearchSimilarFiltered(ctx, topic, nResults, map[string]string{"research_id": researchID})
	if err != nil {
		endRetrieval(err)
		log.Error("research vector retrieval failed", "error_kind", observableErrorKind(err))
		return failedWithPlan(FailureCodeVectorRetrieval, len(scrapedDocs), fmt.Errorf("failed to retrieve indexed evidence: %w", err))
	}
	endRetrieval(nil)
	if len(topEvidence) == 0 {
		return failedWithPlan(FailureCodeEvidenceInsufficient, len(scrapedDocs), fmt.Errorf("vector retrieval returned no evidence"))
	}

	// 6. DataSTORM & Co-STORM Enriched Synthesis with LLM Provider & TAXMORPH Hierarchical Redaction
	endSynthesis := observer.phase("synthesis")
	report, err := e.synth.SynthesizeHierarchicalReport(ctx, topic, taxonTree, topEvidence, e.discourse.ConceptMap, e.discourse.InsightBank)
	endSynthesis(err)
	if err != nil {
		endValidation := observer.phase("validation")
		endValidation(err)
		var citationErr *CitationValidationError
		if errors.As(err, &citationErr) {
			warnings = append(warnings, "El informe se rechazó porque sus citas o referencias no corresponden al corpus recuperado.")
			return failedWithPlan(FailureCodeCitations, len(scrapedDocs), fmt.Errorf("invalid report citations: %w", err))
		}
		return failedWithPlan(FailureCodeSynthesis, len(scrapedDocs), fmt.Errorf("failed to synthesize report: %w", err))
	}
	// 6.5 Phase 6: Critic Loop Governance (Loop Engineering)
	if budget.CriticIterations > 0 {
		endCritic := observer.phase("critic_loop")
		sources := citationSources(topEvidence)
		critic := NewCriticLoop(e.llm, e.embed, CriticConfig{
			MaxIterations:        budget.CriticIterations,
			ConvergenceThreshold: 0.96,
		})
		refinedReport, criticStatus, criticWarnings, criticErr := critic.RunCriticLoop(ctx, topic, report, sources)
		endCritic(criticErr)
		if criticErr == nil {
			report = refinedReport
			if criticStatus == ResearchStatusDegraded && status != ResearchStatusDegraded {
				status = ResearchStatusDegraded
			}
			warnings = append(warnings, criticWarnings...)
		} else {
			log.Warn("critic loop execution encountered error; retaining initial report", "error_kind", observableErrorKind(criticErr))
			warnings = append(warnings, fmt.Sprintf("El bucle de crítica no se pudo completar: %v", criticErr))
		}
	}

	if highProfessionalRelevance(topic) {
		warnings = append(warnings, "Este informe es material de investigación, no asesoría profesional. Verifique las fuentes primarias antes de tomar decisiones jurídicas, médicas, financieras o de seguridad.")
	}

	result := &ResearchResult{
		ResearchID:      researchID,
		Report:          report,
		Status:          status,
		EvidenceCount:   len(evidence),
		Evidence:        evidence,
		EvidenceMetrics: metrics,
		QueryPlan:       queryPlan,
		ResearchPlan:    researchPlan,
		TaxonTree:       taxonTree,
		Warnings:        warnings,
		PhaseDurations:  observer.durations,
	}
	// Citation validation is executed by the synthesizer, but is emitted as a
	// separate terminal phase so operators can distinguish it from generation.
	endValidation := observer.phase("validation")
	endValidation(nil)
	if err := clearResearchSession(ctx); err != nil {
		log.Warn("completed research session checkpoint could not be cleared", "error_kind", observableErrorKind(err))
	}
	result.PhaseDurations = observer.durations
	observer.summary(result, nil)
	return result, nil
}

func highProfessionalRelevance(topic string) bool {
	lower := strings.ToLower(topic)
	for _, word := range []string{"juríd", "legal", "ley", "sentencia", "médic", "salud", "financ", "invers", "seguridad", "security"} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

func researchExecutionID(topic string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", topic, time.Now().UnixNano())))
	return fmt.Sprintf("research_%x", sum[:12])
}

func embeddingModelKey(provider, model string) string {
	sum := sha256.Sum256([]byte(provider + "\x00" + model))
	return fmt.Sprintf("%x", sum[:6])
}
