package research

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/KyberixCo/Pharus/internal/i18n"
)

const synthesisCheckpointVersion = 1

type synthesisCheckpointContextKey struct{}

type synthesisCheckpointConfig struct {
	Path   string
	Key    string
	MaxAge time.Duration
}

type synthesisCheckpoint struct {
	Version          int               `json:"version"`
	OutlineSignature string            `json:"outline_signature"`
	SourceURLs       []string          `json:"source_urls"`
	Sections         map[string]string `json:"sections"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type planningCheckpoint struct {
	Version      int           `json:"version"`
	ResearchPlan *ResearchPlan `json:"research_plan"`
	TaxonTree    *TaxmorphTree `json:"taxon_tree"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

func withSynthesisCheckpoint(ctx context.Context, baseDir, topic string, maxAge time.Duration) context.Context {
	sum := sha256.Sum256([]byte(strings.TrimSpace(topic)))
	key := hex.EncodeToString(sum[:12])
	cfg := synthesisCheckpointConfig{
		Path:   filepath.Join(baseDir, "research_sessions", key, "synthesis.json"),
		Key:    key,
		MaxAge: maxAge,
	}
	return context.WithValue(ctx, synthesisCheckpointContextKey{}, cfg)
}

// ClearSavedResearchSession discards resumable planning and synthesis state
// for one topic without touching vector data or completed reports.
func ClearSavedResearchSession(baseDir, topic string) error {
	ctx := withSynthesisCheckpoint(context.Background(), baseDir, topic, 0)
	return clearResearchSession(ctx)
}

// ClearSavedResearchSessionForLanguage clears only the checkpoint for the
// requested output language.
func ClearSavedResearchSessionForLanguage(baseDir, topic string, language i18n.Language) error {
	ctx := withSynthesisCheckpoint(context.Background(), baseDir, languageSessionTopic(topic, language), 0)
	return clearResearchSession(ctx)
}

// ClearSavedResearchSessionForProfile clears the checkpoint matching the exact
// language and effort profile used by current research runs.
func ClearSavedResearchSessionForProfile(baseDir, topic string, language i18n.Language, profile Profile) error {
	sessionTopic := profileSessionTopic(topic, language, profile)
	ctx := withSynthesisCheckpoint(context.Background(), baseDir, sessionTopic, 0)
	return clearResearchSession(ctx)
}

func languageSessionTopic(topic string, language i18n.Language) string {
	return strings.TrimSpace(topic) + "\x00language=" + string(language)
}

func profileSessionTopic(topic string, language i18n.Language, profile Profile) string {
	base := languageSessionTopic(topic, language)
	// Before profiles existed, all runs used exhaustive planning. Reusing that
	// path for deep preserves completed sections from long legacy sessions.
	if profile == ProfileDeep {
		return base
	}
	return base + "\x00profile=" + string(profile)
}

func checkpointConfigFromContext(ctx context.Context) (synthesisCheckpointConfig, bool) {
	cfg, ok := ctx.Value(synthesisCheckpointContextKey{}).(synthesisCheckpointConfig)
	return cfg, ok && cfg.Path != ""
}

func loadPlanningCheckpoint(ctx context.Context) (*planningCheckpoint, error) {
	cfg, ok := checkpointConfigFromContext(ctx)
	if !ok {
		return nil, nil
	}
	path := filepath.Join(filepath.Dir(cfg.Path), "planning.json")
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if cfg.MaxAge > 0 && time.Since(info.ModTime()) > cfg.MaxAge {
		_ = os.Remove(path)
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var checkpoint planningCheckpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, err
	}
	if checkpoint.Version != synthesisCheckpointVersion ||
		checkpoint.ResearchPlan == nil || checkpoint.ResearchPlan.Validate() != nil ||
		validateResearchPlanLanguage(checkpoint.ResearchPlan) != nil ||
		checkpoint.TaxonTree == nil || checkpoint.TaxonTree.Validate() != nil ||
		validateTaxmorphLanguage(checkpoint.TaxonTree) != nil {
		return nil, nil
	}
	loggerForResearchContext(ctx).Info("research planning checkpoint loaded",
		"phase", "planning",
		"session_key", cfg.Key,
		"nodes", checkpoint.TaxonTree.NodeCount(),
		"leaf_sections", len(checkpoint.TaxonTree.LeafNodes()),
	)
	return &checkpoint, nil
}

func savePlanningCheckpoint(ctx context.Context, plan *ResearchPlan, tree *TaxmorphTree) error {
	cfg, ok := checkpointConfigFromContext(ctx)
	if !ok || plan == nil || tree == nil {
		return nil
	}
	checkpoint := planningCheckpoint{
		Version:      synthesisCheckpointVersion,
		ResearchPlan: plan,
		TaxonTree:    tree,
		UpdatedAt:    time.Now().UTC(),
	}
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}
	return atomicWritePrivate(filepath.Join(filepath.Dir(cfg.Path), "planning.json"), data)
}

func loadSynthesisCheckpoint(ctx context.Context, nodes []*TaxonNode, sources []CitationSource) (*synthesisCheckpoint, error) {
	cfg, ok := checkpointConfigFromContext(ctx)
	if !ok {
		return newSynthesisCheckpoint(nodes, sources), nil
	}
	info, err := os.Stat(cfg.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newSynthesisCheckpoint(nodes, sources), nil
		}
		return nil, err
	}
	if cfg.MaxAge > 0 && time.Since(info.ModTime()) > cfg.MaxAge {
		_ = os.Remove(cfg.Path)
		return newSynthesisCheckpoint(nodes, sources), nil
	}

	data, err := os.ReadFile(cfg.Path)
	if err != nil {
		return nil, err
	}
	var checkpoint synthesisCheckpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, err
	}
	if checkpoint.Version != synthesisCheckpointVersion || checkpoint.OutlineSignature != outlineSignature(nodes) {
		return newSynthesisCheckpoint(nodes, sources), nil
	}
	if checkpoint.Sections == nil {
		checkpoint.Sections = make(map[string]string)
	}
	for nodeID, section := range checkpoint.Sections {
		remapped, ok := remapCheckpointCitations(section, checkpoint.SourceURLs, sources)
		if !ok || validateSynthesizedSection(remapped, sources) != nil {
			delete(checkpoint.Sections, nodeID)
			continue
		}
		checkpoint.Sections[nodeID] = remapped
	}
	checkpoint.SourceURLs = sourceURLs(sources)
	loggerForResearchContext(ctx).Info("research synthesis checkpoint loaded",
		"phase", "synthesis",
		"session_key", cfg.Key,
		"sections_available", len(checkpoint.Sections),
	)
	return &checkpoint, nil
}

func newSynthesisCheckpoint(nodes []*TaxonNode, sources []CitationSource) *synthesisCheckpoint {
	return &synthesisCheckpoint{
		Version:          synthesisCheckpointVersion,
		OutlineSignature: outlineSignature(nodes),
		SourceURLs:       sourceURLs(sources),
		Sections:         make(map[string]string),
		UpdatedAt:        time.Now().UTC(),
	}
}

func (checkpoint *synthesisCheckpoint) reusableSection(nodeID string, sources []CitationSource) (string, bool) {
	section, ok := checkpoint.Sections[nodeID]
	if !ok || strings.TrimSpace(section) == "" {
		return "", false
	}
	if validateSynthesizedSection(section, sources) != nil {
		return "", false
	}
	return section, true
}

func saveSynthesisCheckpoint(ctx context.Context, checkpoint *synthesisCheckpoint, sources []CitationSource) error {
	cfg, ok := checkpointConfigFromContext(ctx)
	if !ok || checkpoint == nil {
		return nil
	}
	checkpoint.SourceURLs = sourceURLs(sources)
	checkpoint.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}
	return atomicWritePrivate(cfg.Path, data)
}

func atomicWritePrivate(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "checkpoint-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func clearResearchSession(ctx context.Context) error {
	cfg, ok := checkpointConfigFromContext(ctx)
	if !ok {
		return nil
	}
	var errs []error
	for _, path := range []string{cfg.Path, filepath.Join(filepath.Dir(cfg.Path), "planning.json")} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if err := os.Remove(filepath.Dir(cfg.Path)); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func outlineSignature(nodes []*TaxonNode) string {
	hash := sha256.New()
	for _, node := range nodes {
		if node == nil {
			continue
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%d\x00", node.ID, node.Title, node.Level)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func sourceURLs(sources []CitationSource) []string {
	urls := make([]string, len(sources))
	for _, source := range sources {
		if source.Number > 0 && source.Number <= len(urls) {
			urls[source.Number-1] = normalizeCitationURL(source.URL)
		}
	}
	return urls
}

func remapCheckpointCitations(section string, previousURLs []string, sources []CitationSource) (string, bool) {
	currentNumbers := make(map[string]int, len(sources))
	for _, source := range sources {
		currentNumbers[normalizeCitationURL(source.URL)] = source.Number
	}
	valid := true
	remapped := citationPattern.ReplaceAllStringFunc(section, func(marker string) string {
		raw := strings.TrimSuffix(strings.TrimPrefix(marker, "["), "]")
		oldNumber, err := strconv.Atoi(raw)
		if err != nil || oldNumber <= 0 || oldNumber > len(previousURLs) {
			valid = false
			return marker
		}
		newNumber, ok := currentNumbers[previousURLs[oldNumber-1]]
		if !ok {
			valid = false
			return marker
		}
		return fmt.Sprintf("[%d]", newNumber)
	})
	return remapped, valid
}
