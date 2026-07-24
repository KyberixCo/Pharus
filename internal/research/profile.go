package research

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Profile string

const (
	ProfileQuick    Profile = "quick"
	ProfileBalanced Profile = "balanced"
	ProfileDeep     Profile = "deep"
)

type profileBudget struct {
	Depth                 DepthLevel
	MaxSections           int
	MaxQueries            int
	SearchResultsPerQuery int
	RetrievalResults      int
	EvidenceCharacters    int
	RecoveryEvidenceChars int
	SectionWords          string
	SectionMaxTokens      int
	SectionAttempts       int
	SectionTimeout        time.Duration
	CriticIterations      int
}

type profileContextKey struct{}

func ParseProfile(value string) (Profile, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "balanced", "standard", "medium", "media":
		return ProfileBalanced, nil
	case "quick", "fast", "rapida", "rápida":
		return ProfileQuick, nil
	case "deep", "complex", "compleja":
		return ProfileDeep, nil
	default:
		return "", fmt.Errorf("invalid research profile %q (use quick, balanced, or deep)", value)
	}
}

func WithProfile(ctx context.Context, profile Profile) context.Context {
	return context.WithValue(ctx, profileContextKey{}, profile)
}

func ProfileFromContext(ctx context.Context) Profile {
	if ctx != nil {
		if profile, ok := ctx.Value(profileContextKey{}).(Profile); ok {
			if normalized, err := ParseProfile(string(profile)); err == nil {
				return normalized
			}
		}
	}
	return ProfileBalanced
}

func budgetForProfile(profile Profile) profileBudget {
	switch profile {
	case ProfileQuick:
		return profileBudget{
			Depth: DepthOverview, MaxSections: 6, MaxQueries: 3,
			SearchResultsPerQuery: 2, RetrievalResults: 8,
			EvidenceCharacters: 1500, RecoveryEvidenceChars: 750,
			SectionWords: "180–300", SectionMaxTokens: 3072,
			SectionAttempts: 2, SectionTimeout: 2 * time.Minute,
			CriticIterations: 0,
		}
	case ProfileDeep:
		return profileBudget{
			Depth: DepthExhaustive, MaxSections: 32, MaxQueries: 6,
			SearchResultsPerQuery: 3, RetrievalResults: 24,
			EvidenceCharacters: 4000, RecoveryEvidenceChars: 1800,
			SectionWords: "400–700", SectionMaxTokens: 6144,
			SectionAttempts: 2, SectionTimeout: 5 * time.Minute,
			CriticIterations: 2,
		}
	default:
		return profileBudget{
			Depth: DepthDeepDive, MaxSections: 15, MaxQueries: 4,
			SearchResultsPerQuery: 3, RetrievalResults: 15,
			EvidenceCharacters: 2500, RecoveryEvidenceChars: 1200,
			SectionWords: "300–500", SectionMaxTokens: 4096,
			SectionAttempts: 2, SectionTimeout: 3 * time.Minute,
			CriticIterations: 1,
		}
	}
}
