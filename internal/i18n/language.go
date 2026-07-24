package i18n

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type Language string

const (
	Auto    Language = "auto"
	English Language = "en"
	Spanish Language = "es"
)

type contextKey struct{}

// Parse accepts the supported language codes and common locale forms.
func Parse(value string) (Language, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || normalized == string(Auto) {
		return Auto, nil
	}
	normalized = strings.SplitN(normalized, ".", 2)[0]
	normalized = strings.SplitN(normalized, "@", 2)[0]
	normalized = strings.ReplaceAll(normalized, "_", "-")
	switch strings.SplitN(normalized, "-", 2)[0] {
	case "en":
		return English, nil
	case "es":
		return Spanish, nil
	default:
		return "", fmt.Errorf("unsupported language %q (supported: auto, en, es)", value)
	}
}

// Resolve turns an explicit language or "auto" into a concrete language.
func Resolve(requested string, getenv func(string) string) (Language, error) {
	language, err := Parse(requested)
	if err != nil {
		return "", err
	}
	if language != Auto {
		return language, nil
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
		raw := strings.TrimSpace(getenv(name))
		if raw == "" {
			continue
		}
		if name == "LANGUAGE" {
			raw = strings.SplitN(raw, ":", 2)[0]
		}
		if detected, parseErr := Parse(raw); parseErr == nil && detected != Auto {
			return detected, nil
		}
	}
	return English, nil
}

func WithLanguage(ctx context.Context, language Language) context.Context {
	if language != Spanish {
		language = English
	}
	return context.WithValue(ctx, contextKey{}, language)
}

func FromContext(ctx context.Context) Language {
	language, ok := LanguageFromContext(ctx)
	if !ok {
		// Preserve the original application behavior for internal callers that
		// have not yet attached an explicit language to their context.
		return Spanish
	}
	return language
}

func LanguageFromContext(ctx context.Context) (Language, bool) {
	if ctx != nil {
		if language, ok := ctx.Value(contextKey{}).(Language); ok {
			if language == Spanish {
				return Spanish, true
			}
			return English, true
		}
	}
	return English, false
}

func Select(language Language, english, spanish string) string {
	if language == Spanish {
		return spanish
	}
	return english
}

func FromContextText(ctx context.Context, english, spanish string) string {
	return Select(FromContext(ctx), english, spanish)
}
