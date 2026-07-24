package research

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/KyberixCo/Pharus/internal/i18n"
)

const (
	englishReportDirective = `REQUIRED LANGUAGE:
Write all natural-language content exclusively in professional English. Do not mix unrelated writing systems into English sentences. Translate or transliterate foreign terms to the Latin alphabet when appropriate. Keep required JSON keys, identifiers, URLs, formats, citations, and established technical acronyms unchanged.`
	spanishReportDirective = `IDIOMA OBLIGATORIO:
Redacta todo contenido en lenguaje natural exclusivamente en español profesional. No insertes caracteres chinos, japoneses o coreanos, ni mezcles palabras en esos sistemas de escritura dentro de frases españolas. Traduce o translitera al alfabeto latino cualquier término extranjero. Conserva sin traducir las claves JSON, identificadores, URLs, formatos exigidos y siglas técnicas establecidas.`
)

func reportLanguageDirective(ctx context.Context) string {
	return i18n.FromContextText(ctx, englishReportDirective, spanishReportDirective)
}

func researchText(ctx context.Context, english, spanish string) string {
	return i18n.FromContextText(ctx, english, spanish)
}

type LanguageValidationError struct {
	UnexpectedCharacters string
}

func (e *LanguageValidationError) Error() string {
	return fmt.Sprintf("unexpected CJK characters in report prose: %q", e.UnexpectedCharacters)
}

// ValidateReportLanguage rejects CJK script in report prose. References are
// excluded because a recovered source title may legitimately use its original
// writing system.
func ValidateReportLanguage(report string) error {
	body := reportWithoutReferences(report)
	var unexpected strings.Builder
	seen := make(map[rune]struct{})
	for _, r := range body {
		if !isCJKScript(r) {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		unexpected.WriteRune(r)
		if unexpected.Len() >= 24 {
			break
		}
	}
	if unexpected.Len() == 0 {
		return nil
	}
	return &LanguageValidationError{UnexpectedCharacters: unexpected.String()}
}

func isCJKScript(r rune) bool {
	return unicode.In(r,
		unicode.Han,
		unicode.Hiragana,
		unicode.Katakana,
		unicode.Hangul,
		unicode.Bopomofo,
	)
}
