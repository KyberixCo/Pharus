package main

import (
	"testing"

	"github.com/KyberixCo/Pharus/internal/i18n"
)

func TestRequestedLanguageFromArgsSupportsBothFlags(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--language", "es", "research", "topic"}, "es"},
		{[]string{"research", "--lang=en", "topic"}, "en"},
		{[]string{"research", "topic"}, "auto"},
	}
	for _, test := range tests {
		if got := requestedLanguageFromArgs(test.args); got != test.want {
			t.Fatalf("args %v: expected %q, got %q", test.args, test.want, got)
		}
	}
}

func TestLocalizeCommandMetadata(t *testing.T) {
	localizeCommandMetadata(i18n.Spanish)
	if rootCmd.Short != "Pharus: motor de Deep Research local y demonio resiliente para macOS" {
		t.Fatalf("unexpected Spanish root description: %q", rootCmd.Short)
	}
	localizeCommandMetadata(i18n.English)
	if researchCmd.Use != "research [topic or question]" {
		t.Fatalf("unexpected English research usage: %q", researchCmd.Use)
	}
}
