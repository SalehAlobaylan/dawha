package identity

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

func NormalizeArabicName(value string) string {
	normalized := norm.NFKC.String(value)
	var builder strings.Builder
	builder.Grow(len(normalized))

	for _, character := range normalized {
		if unicode.Is(unicode.Mn, character) || character == '\u0640' {
			continue
		}

		switch character {
		case 'أ', 'إ', 'آ', 'ٱ':
			character = 'ا'
		case 'ى':
			character = 'ي'
		case 'ة':
			character = 'ه'
		}

		if unicode.IsSpace(character) || unicode.IsPunct(character) {
			builder.WriteRune(' ')
			continue
		}
		builder.WriteRune(character)
	}

	return strings.Join(strings.Fields(builder.String()), " ")
}
