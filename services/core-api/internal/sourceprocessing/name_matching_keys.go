package sourceprocessing

import (
	"golang.org/x/text/unicode/norm"
	"strings"
	"unicode"
)

// Matching equivalence is never an identity assertion. These are the published
// v1 evidence keys; the candidate ranking remains only in TypeScript.
func nameCanonical(value string) string {
	value = norm.NFKC.String(value)
	var builder strings.Builder
	for _, character := range value {
		switch {
		case character == 0x200B || character == 0x200C || character == 0x200D ||
			character == 0x200E || character == 0x200F || character == 0x202A ||
			character == 0x202B || character == 0x202C || character == 0x202D ||
			character == 0x202E || character == 0x2060 || character == 0xFEFF:
			continue
		case character == 0x0640:
			continue
		case character >= 0x0610 && character <= 0x061A:
			continue
		case character >= 0x064B && character <= 0x065F:
			continue
		case character == 0x0670:
			continue
		case character >= 0x06D6 && character <= 0x06ED:
			continue
		case character == 0x0623 || character == 0x0625 || character == 0x0622 || character == 0x0671:
			builder.WriteRune('ا')
		case character == 0x0649:
			builder.WriteRune('ي')
		case character == 0x0629:
			builder.WriteRune('ه')
		case character >= 0x0660 && character <= 0x0669:
			builder.WriteRune('0' + (character - 0x0660))
		case character >= 0x06F0 && character <= 0x06F9:
			builder.WriteRune('0' + (character - 0x06F0))
		case unicode.IsSpace(character):
			builder.WriteRune(' ')
		default:
			builder.WriteRune(character)
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func nameCompact(canonical string) string {
	var builder strings.Builder
	for _, character := range canonical {
		switch {
		case unicode.IsSpace(character):
			continue
		case character == '-' || character == '‐' || character == '‑' || character == '‒' ||
			character == '–' || character == '—':
			continue
		default:
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

func nameParticle(canonical string) string {
	tokens := strings.Fields(canonical)
	particles := map[string]bool{"ال": true, "بن": true, "بنت": true, "ابو": true}
	output := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if particles[token] && len(output) < 2 {
			continue
		}
		output = append(output, token)
	}
	if len(output) == 0 {
		return canonical
	}
	if len([]rune(output[0])) > 2 && strings.HasPrefix(output[0], "ال") {
		output[0] = strings.TrimPrefix(output[0], "ال")
	}
	return strings.Join(output, " ")
}
