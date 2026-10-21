export interface BookQuoteSuggestion {
  start: number;
  end: number;
  text: string;
}

// These phrases only mark passages for a human to inspect. The result contains
// exact quote spans and deliberately carries no inferred people or relationship.
const lineageMarkers = [
  "ابن", "ابنة", "أبناء", "أبنائه", "أبيه", "أبوه", "والده", "والد", "والدة",
  "بنت", "بن", "ولد", "أمه", "أمها", "أم", "أخوه", "أخته", "أخ", "شقيق",
  "زوجته", "زوجة", "تزوج", "ذرية", "عقب", "نسل",
];
const sentenceBreak = /[\n\r,.،.!?؟؛]/u;
const wordCharacter = /[\p{L}\p{N}]/u;

export function suggestBookQuoteSpans(text: string, limit = 30): BookQuoteSuggestion[] {
  if (!text || limit <= 0) return [];
  const characters = Array.from(text);
  const suggestions: BookQuoteSuggestion[] = [];
  let segmentStart = 0;

  const consider = (segmentEnd: number) => {
    let start = segmentStart;
    let end = segmentEnd;
    while (start < end && /^\s$/u.test(characters[start])) start += 1;
    while (end > start && /^\s$/u.test(characters[end - 1])) end -= 1;
    if (start === end || end - start > 800) return;
    const segment = characters.slice(start, end).join("");
    if (!hasLineageMarker(segment)) return;
    const quote = { start, end, text: segment };
    if (!suggestions.some((item) => item.start === quote.start && item.end === quote.end)) {
      suggestions.push(quote);
    }
  };

  for (let index = 0; index <= characters.length; index += 1) {
    if (index === characters.length || sentenceBreak.test(characters[index])) {
      consider(index);
      segmentStart = index + 1;
      if (suggestions.length >= limit) break;
    }
  }
  return suggestions;
}

function hasLineageMarker(text: string): boolean {
  for (const marker of lineageMarkers) {
    let offset = text.indexOf(marker);
    while (offset >= 0) {
      const before = offset > 0 ? text[offset - 1] : undefined;
      const after = text[offset + marker.length];
      if ((!before || !wordCharacter.test(before)) && (!after || !wordCharacter.test(after))) return true;
      offset = text.indexOf(marker, offset + marker.length);
    }
  }
  return false;
}
