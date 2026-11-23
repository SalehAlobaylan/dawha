import { ChevronLeft, CircleHelp, LoaderCircle, ShieldQuestion, X } from "lucide-react";
import { Link } from "@tanstack/react-router";
import type { AtlasInferenceState } from "../lib/atlas-inference";
import type { GeospatialFinding } from "../types";
import { formatMapCount } from "../lib/map-vocabulary";

/**
 * The inference panel.
 *
 * Everything in this block answers "what did the platform infer?" and nothing
 * else. It renders one run — the latest this account can reach — and its rows
 * carry no authority of their own: a sealed cluster is a hypothesis about
 * co-occurrence, and a finding is a request for human review. The stamp-ring
 * grammar is the only grammar here because a request for review is never drawn
 * like a record.
 *
 * The handoff is explicit: each finding points at the review UI in the question
 * workspace. Approving, dismissing or confirming happens there, with the
 * existing review flow, never here and never without a human.
 */
export function AtlasInferenceOverlay({
  state,
  onFocus,
  onClose,
}: {
  state: AtlasInferenceState;
  onFocus: (key: string, longitude: number, latitude: number) => void;
  onClose?: () => void;
}) {
  if (!state.enabled) return null;

  const needsReview = sortFindings(state.findings).filter((finding) => finding.status === "needs_review").length;
  const clusters = state.run?.report.clusters ?? [];

  return (
    <section className="atlas-inference" aria-label="الطبقة الاستنتاجية">
      <div className="atlas-inference-head">
        <span className="atlas-inference-seal-glyph" aria-hidden="true">▣</span>
        <div className="atlas-inference-head-copy">
          <h3>الطبقة الاستنتاجية</h3>
          <p>ختم منصة، لا شهادة مصدر. لا شيء هنا يُعرض كأنه موثق.</p>
        </div>
        <div className="atlas-inference-head-actions">
          {state.run ? (
            <span className="atlas-inference-count" aria-label={`${formatMapCount(needsReview)} ملاحظة تحتاج مراجعة، ${formatMapCount(clusters.length)} تجمعاً`}>
              {formatMapCount(needsReview)} تحتاج مراجعة · {formatMapCount(clusters.length)} تجمعاً
            </span>
          ) : null}
          {onClose ? (
            <button type="button" className="atlas-inference-close" onClick={onClose} aria-label="أغلق الطبقة الاستنتاجية" title="إغلاق">
              <X size={14} />
            </button>
          ) : null}
        </div>
      </div>

      {state.forbidden ? (
        <p className="atlas-inference-empty"><CircleHelp size={16} /> طبقة الاستنتاجات تتطلب صلاحية باحث؛ سجّل الدخول لعرضها.</p>
      ) : null}
      {state.isError ? (
        <p className="atlas-inference-empty" role="alert">تعذر استعادة التحليل المحفوظ. أعد المحاولة من مساحة التحقيق.</p>
      ) : null}
      {state.isPending && !state.run ? (
        <p className="atlas-inference-empty"><LoaderCircle size={16} className="atlas-spin" /> جارٍ استعادة آخر تحليل محفوظ…</p>
      ) : null}
      {!state.isPending && !state.isError && !state.forbidden && !state.run ? (
        <div className="atlas-inference-empty-box">
          <p className="atlas-inference-empty">لم يبدأ تحليل جغرافي بهذا الحساب بعد. ابدأ تحليلاً من مساحة التحقيق، وستظهر نتائجه هنا مختومة.</p>
          <Link to="/research" className="secondary-button">
            افتح مساحة التحقيق
            <ChevronLeft size={13} />
          </Link>
        </div>
      ) : null}

      {state.run ? (
        <>
          <small className="atlas-inference-scope">النطاق: {state.run.entityName} · نصف قطر {state.run.scope.radiusKm} كم · {formatMapCount(state.run.report.placeResolution.resolvedCount)} موضعاً محلولاً</small>
          {state.findings.length > 0 ? (
            <ul className="atlas-finding-list">
              {sortFindings(state.findings).map((finding) => (
                <li key={finding.id} className="atlas-finding-row">
                  <span className="atlas-inference-seal-glyph atlas-finding-seal" aria-hidden="true">▣</span>
                  <div>
                    <strong>{finding.titleAr}</strong>
                    <p>{finding.explanationAr}</p>
                    <small>{layerLabel(finding)} · {severityLabel(finding.severity)} · {findingStatusLabel(finding.status)}{finding.sourceIds.length > 0 ? ` · ${formatMapCount(finding.sourceIds.length)} مصدر` : ""}</small>
                    {state.run?.questionId ? (
                      <Link to="/research/$questionId" params={{ questionId: state.run.questionId }} search={{ panel: "analysis" }} className="atlas-inference-review-link">
                        افتح في مساحة التحقيق
                        <ChevronLeft size={13} />
                      </Link>
                    ) : (
                      <small className="atlas-inspector-note">هذا التحليل بلا سؤال مفتوح؛ المراجعة تجري من مساحة التحقيق التي أُطلق منها.</small>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          ) : (
            <p className="atlas-inference-empty">لا توجد ملاحظات في هذا التحليل. الامتناع عن الاستنتاج نتيجة صالحة.</p>
          )}
          {clusters.length > 0 ? (
            <section aria-label="التجمعات الاستنتاجية">
              <h4 className="atlas-inference-subhead">التجمعات الاستنتاجية</h4>
              <ul className="atlas-cluster-list">
                {clusters.map((cluster) => (
                  <li key={cluster.id} className="atlas-cluster-row">
                    <button type="button" className="atlas-cluster-button" onClick={() => onFocus(`cluster-${cluster.id}`, cluster.centerLongitude, cluster.centerLatitude)} aria-label={`ركّز على تجمع ${cluster.placeNames.join("، ")}`}>
                      <span className="atlas-inference-seal-glyph" aria-hidden="true">▣</span>
                      <span>
                        <strong>{cluster.placeNames.join("، ") || "تجمع غير مسمى"}</strong>
                        <small>{formatMapCount(cluster.associationIds.length)} إشارة · {formatMapCount(cluster.sourceBackedCount)} موثقة · {formatMapCount(cluster.inferredCount)} مستنتجة · نصف قطر {Math.round(cluster.radiusKm)} كم</small>
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
          {state.run.report.limitations.length > 0 ? (
            <small className="atlas-inference-limitations">{state.run.report.limitations.join(" ")}</small>
          ) : null}
          <div className="atlas-inference-legend">
            <ShieldQuestion size={13} />
            <span>الطبقة الاستنتاجية لا تغيّر ما تراه من أدلة؛ المراجعة بشرية وموثقة في مساحة التحقيق.</span>
          </div>
        </>
      ) : null}
    </section>
  );
}

function sortFindings(findings: GeospatialFinding[]): GeospatialFinding[] {
  const order: Record<GeospatialFinding["status"], number> = {
    needs_review: 0,
    investigating: 1,
    confirmed: 2,
    dismissed: 3,
  };
  return [...findings].sort((left, right) => order[left.status] - order[right.status] || left.id.localeCompare(right.id));
}

function layerLabel(finding: GeospatialFinding): string {
  return finding.layer === "source_backed" ? "طبقة مدعومة" : "طبقة استنتاجية";
}

function severityLabel(severity: GeospatialFinding["severity"]): string {
  if (severity === "high") return "شدة عالية";
  if (severity === "medium") return "شدة متوسطة";
  return "شدة منخفضة";
}

function findingStatusLabel(status: GeospatialFinding["status"]): string {
  if (status === "needs_review") return "تحتاج مراجعة";
  if (status === "investigating") return "قيد التحقيق";
  if (status === "confirmed") return "مؤكدة";
  return "مرفوضة";
}
