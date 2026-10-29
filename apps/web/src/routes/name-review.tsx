import { useChartWorkspace } from "../lib/use-chart-workspace";
import { useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { useState } from "react";
import { readSourceTreeSuggestions } from "../lib/api";
import { SourceSuggestionReview } from "../components/SourceSuggestionReview";
import { NameReadingRunStatus } from "../components/SourceProcessingPanel";
import { ChartWorkspaceHeader, ChartWorkspaceUnavailable } from "../components/ChartWorkspace";

export function NameReviewPage() {
  const { fileId } = useParams({ from: "/tree/charts/$fileId/names" });
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { query, item } = useChartWorkspace(fileId);
  const { queue = "pending" } = useSearch({ from: "/tree/charts/$fileId/names" });
  const [preparing, setPreparing] = useState(false);
  const [error, setError] = useState("");
  const close = () => { void queryClient.invalidateQueries({ queryKey: ["chart-library"] }); void navigate({ to: "/tree/charts/$fileId", params: { fileId }, replace: true }); };
  const startReading = async () => {
    setPreparing(true); setError("");
    try { await readSourceTreeSuggestions(fileId); await queryClient.invalidateQueries({ queryKey: ["name-reading-run", fileId] }); await query.refetch(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "تعذر إعداد الأسماء. حاول مرة أخرى."); }
    finally { setPreparing(false); }
  };
  if (!item) return <ChartWorkspaceUnavailable fileId={fileId} query={query} />;
  return <div className="page-stack name-review-route chart-workspace-page">
    <ChartWorkspaceHeader item={item} current="names" />
    <p>راجع الأسماء في مواضعها من الأصل. إغلاق المراجعة يعيدك إلى نظرة عامة على هذا المشجر.</p>
    {error ? <p role="alert">{error}</p> : null}
    <NameReadingRunStatus fileId={fileId} />
    {item.totalNames > 0 ? <SourceSuggestionReview key={`${fileId}:${queue}`} fileId={fileId} sourceTitle={item.titleAr} initialQueueMode={queue} onClose={close} onContinue={() => void navigate({ to: "/import", search: { sourceId: item.sourceId, fileId, draftId: item.draftId } })} /> : <section className="chart-review-preparation"><h2>إعداد اقتراحات الأسماء</h2><p>الملف محفوظ. ابدأ قراءة الأسماء، ثم راجع كل اقتراح على الصورة الأصلية.</p><button className="primary-button" type="button" disabled={preparing || item.readingStatus === "queued" || item.readingStatus === "running"} onClick={() => void startReading()}>{preparing ? "جارٍ بدء القراءة…" : item.readingStatus === "queued" || item.readingStatus === "running" ? "جارٍ إعداد الأسماء…" : "إعداد اقتراحات الأسماء"}</button></section>}
    <Link className="inline-link" to="/tree" search={{ view: "review", queue: queue === "deferred" ? "deferred" : undefined }}>اختيار مشجر آخر</Link>
  </div>;
}
