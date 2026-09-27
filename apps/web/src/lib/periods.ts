/**
 * The client half of the date-display contract, and it is deliberately small.
 *
 * `services/core-api/internal/dates` is where the contract lives: the API renders a
 * stored range and sends it as `period`, and this app shows that string. The only
 * thing the client renders on its own is a period a human wrote down - "قبل ١١٥٠هـ",
 * "القرن الثاني عشر" - and it renders it verbatim.
 *
 * The verbatim part is the whole point, and it is why this file exists rather than
 * a one-line `place.period` in the component. `IMPLEMENTATION_PLAN.md:947` asks for
 * "Hijri or Gregorian display support where practical", and the tempting version of
 * this function is the one that converts a recorded sentence into a year and
 * compares it with a stored one. That would assert a calendar the record never
 * claimed, and it would do it in the presentation layer, where nothing would
 * notice. The reasoning is written out in `docs/phase-status.md` under Phase 10 and
 * in `services/core-api/internal/dates`; the test below is what keeps a later
 * reader from reintroducing the conversion one helpful commit at a time.
 *
 * The matching Go-side word is `dates.Unknown`, and the two must not drift: a
 * surface that says "غير محددة" for an absent bound and "—" for an absent
 * recorded string is the disagreement this contract exists to prevent.
 */
export const unknownPeriod = "غير محددة";

/**
 * A period a person recorded, returned as they wrote it.
 *
 * An empty value is the same word the API uses for an unknown bound, so a reader
 * meets one phrase for "the record does not say" rather than two - and a dash
 * never stands in for it, because a dash reads as an empty field rather than as a
 * statement about what is unknown.
 */
export function formatRecordedPeriod(period: string | undefined): string {
  const trimmed = (period ?? "").trim();
  return trimmed === "" ? unknownPeriod : trimmed;
}
