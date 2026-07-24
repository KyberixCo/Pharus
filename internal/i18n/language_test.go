package i18n

import (
	"context"
	"testing"
)

func TestResolveExplicitLanguageOverridesLocale(t *testing.T) {
	got, err := Resolve("es", func(string) string { return "en_US.UTF-8" })
	if err != nil || got != Spanish {
		t.Fatalf("expected Spanish, got %q (%v)", got, err)
	}
}

func TestResolveAutoUsesLocale(t *testing.T) {
	values := map[string]string{"LC_ALL": "", "LC_MESSAGES": "es_CO.UTF-8", "LANG": "en_US.UTF-8"}
	got, err := Resolve("auto", func(name string) string { return values[name] })
	if err != nil || got != Spanish {
		t.Fatalf("expected Spanish, got %q (%v)", got, err)
	}
}

func TestResolveAutoFallsBackToEnglish(t *testing.T) {
	got, err := Resolve("", func(string) string { return "C" })
	if err != nil || got != English {
		t.Fatalf("expected English fallback, got %q (%v)", got, err)
	}
}

func TestContextLanguage(t *testing.T) {
	ctx := WithLanguage(context.Background(), Spanish)
	if got := FromContextText(ctx, "Report", "Informe"); got != "Informe" {
		t.Fatalf("unexpected localized text: %q", got)
	}
}
