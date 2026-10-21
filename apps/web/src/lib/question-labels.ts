/**
 * The Arabic words the app uses for an open question's own state.
 *
 * The API stores a status and a priority as the fixed English tokens the
 * database constrains them to (`open_questions.status`, `open_questions.priority`
 * in `db/migrations/0003_research_and_jobs.sql`). The reader meets Arabic. This
 * is the one place that translation happens, so the question index and the
 * question workspace cannot drift into showing the same status two ways.
 *
 * It is a separate file rather than two exports from the workspace component
 * because `react-refresh/only-export-components` treats a component file that
 * exports a function as a fast-refresh boundary, and the label is needed by the
 * page above the workspace as well as by the workspace itself.
 */
export function questionStatusLabel(status: string): string {
  if (status === "under_investigation") return "قيد التحقيق";
  if (status === "resolved") return "محسوم مؤقتاً";
  if (status === "reopened") return "أُعيد فتحه";
  if (status === "archived") return "مؤرشف";
  return "مفتوح";
}

export function questionPriorityLabel(priority: string): string {
  if (priority === "high") return "عالية";
  if (priority === "low") return "منخفضة";
  return "عادية";
}
