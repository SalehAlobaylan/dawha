import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, BookOpen, CheckCircle2, ExternalLink, FileText, GitCompare, History, MapPin, PenLine, X } from "lucide-react";
import { useState } from "react";
import { PlatePointRead } from "./PlateComparePanel";
import {
  fetchAtlasOccurrences,
  fetchAtlasSourceRelationship,
  plateCaptionRevisions,
  savePlateCaptionRevision,
  type AtlasPlateRegisterRow,
  type AtlasSourceOccurrence,
} from "../lib/api";
import type { AtlasPlateTribeLabel } from "../lib/atlas";
import type { OttomanHaritaTerritory } from "../lib/ottoman-harita-1910-territories";

const arabicScriptPattern = /[\u0600-\u06FF]/;

export function PlateCaptionPanel({ label, plate, canModerate, onClose, floating = false, hasTerritory = false, territorySpanAr }: {
  label: AtlasPlateTribeLabel;
  plate: AtlasPlateRegisterRow;
  canModerate: boolean;
  onClose: () => void;
  floating?: boolean;
  hasTerritory?: boolean;
  territorySpanAr?: string;
}) {
  const printedIsArabic = arabicScriptPattern.test(label.printedName);
  const showPrintedVariant = label.printedName.trim() !== "" && label.printedName.trim() !== label.nameAr.trim();

  return (
    <aside
      className={floating ? "plate-caption-panel plate-caption-panel-floating" : "atlas-inspector plate-caption-panel"}
      aria-label="قراءة اسم من اللوحة"
    >
      <div className="atlas-panel-head plate-caption-head">
        <div className="plate-caption-title-group">
          <div className="eyebrow">{printedIsArabic ? "اللوحة التاريخية / قراءة أصلية" : "اللوحة التاريخية / تعريب اسم"}</div>
          <div className="plate-caption-name-row">
            <h2>{label.nameAr}</h2>
            {showPrintedVariant ? (
              <span className="plate-caption-printed" lang={printedIsArabic ? "ar" : "en"}>
                <bdi>{label.printedName}</bdi>
              </span>
            ) : null}
          </div>
        </div>
        <button type="button" className="atlas-inspector-close" onClick={onClose} aria-label="إغلاق الاسم" title="إغلاق الاسم">
          <X size={15} />
        </button>
      </div>

      <div className="plate-caption-source-meta">
        <MapPin size={13} aria-hidden="true" />
        <div>
          <strong>{plate.titleAr || plate.plateKey}</strong>
          <span>
            {printedIsArabic ? "قراءة أصلية بالعربية على اللوحة التاريخية" : "تعريب لاسم مطبوع على اللوحة التاريخية"}
            {" · "}
            {label.coordinates[1].toFixed(2)}°N، {label.coordinates[0].toFixed(2)}°E
          </span>
        </div>
      </div>

      {hasTerritory ? (
        <div className="plate-caption-territory-note" role="note">
          <strong>النطاق المرقَّم على اللوحة</strong>
          <span>{territorySpanAr || "الحدود المظلَّلة على الخريطة مرقَّمة مباشرةً من الخطوط المنقّطة في هذه اللوحة."}</span>
          <small>حدود تقريبية حسب رسم اللوحة التاريخية، وليست حدوداً إدارية أو موثَّقة مستقلاً.</small>
        </div>
      ) : null}

      <section className="plate-caption-section" aria-label="قصاصة الأصل من اللوحة">
        <div className="plate-caption-section-head">
          <h3>قصاصة الأصل المطبوع</h3>
        </div>
        <PlatePointRead plate={plate} longitude={label.coordinates[0]} latitude={label.coordinates[1]} size={720} />
      </section>

      <Link
        className="plate-caption-dossier-cta"
        to="/search"
        search={label.tribeId ? { kind: "tribes", id: label.tribeId } : { kind: "tribes", q: label.nameAr }}
      >
        <BookOpen size={15} aria-hidden="true" />
        <span>{label.tribeId ? "افتح سجل القبيلة" : "ابحث عن الاسم في فهرس القبائل"}</span>
        <ArrowLeft size={14} aria-hidden="true" />
      </Link>

      <CaptionCrossSources label={label} plate={plate} />
      {canModerate && label.catalogId ? <CaptionReview key={label.catalogId} captionId={label.catalogId} currentName={label.nameAr} /> : null}
    </aside>
  );
}

function CaptionCrossSources({ label, plate }: { label: AtlasPlateTribeLabel; plate: AtlasPlateRegisterRow }) {
  const plateSourceId = plate.sourceId;
  const [compared, setCompared] = useState<AtlasSourceOccurrence | null>(null);
  const query = useQuery({
    queryKey: ["atlas-caption-cross-sources", label.nameAr, label.tribeId ?? ""],
    queryFn: ({ signal }) => fetchAtlasOccurrences({ q: label.nameAr }, signal),
    staleTime: 60000,
    retry: false,
  });
  const otherSources = (query.data?.occurrences ?? []).filter(
    (row) => row.id !== label.catalogId && row.caption !== label.id,
  );
  const relQuery = useQuery({
    queryKey: ["atlas-source-relationship", plateSourceId ?? "", compared?.sourceId ?? ""],
    queryFn: ({ signal }) => fetchAtlasSourceRelationship(plateSourceId!, compared!.sourceId, signal),
    enabled: Boolean(plateSourceId && compared && plateSourceId !== compared.sourceId),
    retry: false,
  });

  if (query.isPending) {
    return <p className="plate-caption-sources-status" role="status">تُفحص المصادر التاريخية الأخرى…</p>;
  }
  if (otherSources.length === 0) return null;

  const relationshipNote = !compared
    ? ""
    : plateSourceId === compared.sourceId
      ? "شاهدان من المصدر نفسه."
      : relQuery.isPending
        ? "تُفحص صلة المصدرين…"
        : relQuery.data?.status === "needs_review"
          ? "صلة المصدرين بانتظار المراجعة."
          : relQuery.data?.status === "confirmed"
            ? (relQuery.data.kind === "shared_origin" || relQuery.data.kind === "derived_from"
              ? "توجد صلة موثّقة في أصل المصدرين."
              : "توجد إحالة أو صلة مسجّلة بين المصدرين. استقلال الشاهدين غير محقّق.")
            : "استقلال المصدرين غير محقّق.";

  return (
    <section className="plate-caption-sources" aria-label="شواهد الاسم في مصادر أخرى">
      <div className="plate-caption-section-head">
        <h3>في مصادر أخرى</h3>
        <span className="plate-caption-count-badge">{otherSources.length}</span>
      </div>
      <ul className="plate-caption-sources-list">
        {otherSources.map((row) => {
          const period = row.periodAr || (row.depictedFrom === row.depictedTo
            ? row.depictedFrom
            : [row.depictedFrom, row.depictedTo].filter((year) => year !== undefined).join("–"));
          const isCompared = compared?.id === row.id;
          return (
            <li key={row.id} className={`plate-caption-source-item${isCompared ? " plate-caption-source-item-active" : ""}`}>
              <div className="plate-caption-source-top">
                <strong className="plate-caption-source-book">{row.titleAr}</strong>
                <span className="plate-caption-spelling-chip">
                  <bdi lang={row.language}>{row.original}</bdi>
                </span>
              </div>
              <div className="plate-caption-source-details">
                <FileText size={12} aria-hidden="true" />
                <span>
                  {row.locatorAr}
                  {period ? ` · ${period}` : ""}
                  {row.method === "catalogue" ? " · قراءة من الفهرس" : ""}
                </span>
              </div>
              {row.sourceSelector?.figure && row.sourceSelector.legend ? (
                <div className="plate-caption-selector-pill">
                  رمز الخريطة {row.sourceSelector.figure.symbol} · صف المفتاح {row.sourceSelector.legend.row}
                </div>
              ) : null}
              {row.excerptAr ? <blockquote>{row.excerptAr}</blockquote> : null}
              <div className="plate-caption-source-actions">
                <button
                  type="button"
                  className={`plate-caption-action-btn${isCompared ? " plate-caption-action-btn-active" : ""}`}
                  aria-pressed={isCompared}
                  onClick={() => setCompared(isCompared ? null : row)}
                >
                  <GitCompare size={13} aria-hidden="true" />
                  <span>{isCompared ? "إخفاء المقارنة" : "قارن مع اللوحة"}</span>
                </button>
                {row.url ? (
                  <a className="plate-caption-action-link" href={row.url} target="_blank" rel="noreferrer">
                    <ExternalLink size={13} aria-hidden="true" />
                    <span>افتح المصدر عند الشاهد</span>
                  </a>
                ) : (
                  <Link className="plate-caption-action-link" to="/sources" search={{ sourceId: row.sourceId }}>
                    <FileText size={13} aria-hidden="true" />
                    <span>سجل المصدر</span>
                  </Link>
                )}
              </div>
              {isCompared ? (
                <div className="plate-caption-compare-box" role="status">
                  <div className="plate-caption-compare-grid">
                    <div>
                      <small>اللوحة التاريخية</small>
                      <strong>{label.nameAr}</strong>
                      <bdi lang="en">{label.printedName}</bdi>
                    </div>
                    <div>
                      <small>المصدر المقارن</small>
                      <strong>{row.displayAr}</strong>
                      <bdi lang={row.language}>{row.original}</bdi>
                    </div>
                  </div>
                  <p className="atlas-source-relationship">{relationshipNote}</p>
                </div>
              ) : null}
            </li>
          );
        })}
      </ul>
    </section>
  );
}

function CaptionReview({ captionId, currentName }: { captionId: string; currentName: string }) {
  const [draft, setDraft] = useState("");
  const client = useQueryClient();
  const queryKey = ["plate-caption-revisions", captionId];
  const history = useQuery({ queryKey, queryFn: () => plateCaptionRevisions(captionId), retry: false });
  const save = useMutation({
    mutationFn: (revision: { nameAr: string } | { id: string }) => savePlateCaptionRevision(captionId, revision),
    onSuccess: async (_, revision) => {
      if ("nameAr" in revision) setDraft("");
      await client.invalidateQueries({ queryKey });
      if ("id" in revision) {
        await Promise.all([
          client.invalidateQueries({ queryKey: ["atlas-plate-register"] }),
          client.invalidateQueries({ queryKey: ["atlas-occurrences"] }),
          client.invalidateQueries({ queryKey: ["atlas-occurrence", captionId] }),
        ]);
      }
    },
  });
  const revisions = history.data ?? [];
  return (
    <details className="plate-caption-review">
      <summary>
        <PenLine size={13} aria-hidden="true" />
        <span>مراجعة التعريب</span>
        {revisions.length > 0 ? <small>{revisions.length}</small> : null}
      </summary>
      <div className="plate-caption-review-body">
        <form
          className="plate-caption-review-form"
          onSubmit={(event) => {
            event.preventDefault();
            if (draft.trim()) save.mutate({ nameAr: draft.trim() });
          }}
        >
          <label className="plate-caption-review-label">
            <span>اقترح تهجئة عربية بديلة</span>
            <input
              aria-label="التعريب المقترح"
              value={draft}
              placeholder={currentName}
              onChange={(event) => setDraft(event.target.value)}
              maxLength={200}
            />
          </label>
          <button type="submit" className="secondary-button" disabled={save.isPending || !draft.trim()}>
            احفظ اقتراحاً
          </button>
        </form>
        {save.isError && <p className="plate-caption-review-error" role="alert">تعذّر حفظ التعديل.</p>}
        {history.isPending && <p className="plate-caption-sources-status" role="status">جارٍ تحميل سجل التعريب…</p>}
        {history.isError && <p className="plate-caption-review-error" role="alert">تعذّر تحميل سجل التعريب.</p>}
        {revisions.length > 0 ? (
          <div className="plate-caption-revisions">
            <div className="plate-caption-revisions-head">
              <History size={12} aria-hidden="true" />
              <span>سجل المقترحات</span>
            </div>
            <ul>
              {revisions.map((revision) => (
                <li key={revision.id}>
                  <div>
                    <strong>{revision.nameAr}</strong>
                    <small>{revision.proposedBy}</small>
                  </div>
                  {revision.published ? (
                    <span className="plate-caption-published-pill">
                      <CheckCircle2 size={12} aria-hidden="true" />
                      منشور
                    </span>
                  ) : (
                    <button
                      type="button"
                      className="secondary-button"
                      disabled={save.isPending}
                      onClick={() => save.mutate({ id: revision.id })}
                    >
                      انشر
                    </button>
                  )}
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </div>
    </details>
  );
}

export function OttomanHaritaPanel({ territory, onClose, floating = false }: {
  territory: OttomanHaritaTerritory;
  onClose: () => void;
  floating?: boolean;
}) {
  const crossQuery = useQuery({
    queryKey: ["atlas-ottoman-cross-sources", territory.nameAr, territory.tribeId ?? ""],
    queryFn: ({ signal }) => fetchAtlasOccurrences({ q: territory.nameAr }, signal),
    staleTime: 60000,
    retry: false,
  });
  const otherSources = (crossQuery.data?.occurrences ?? []).filter(
    (row) => row.sourceId !== "b0000000-0000-0000-0000-00000000e096",
  );
  const showOttomanOriginal = territory.ottomanName.trim() !== "" && territory.ottomanName.trim() !== territory.nameAr.trim();

  return (
    <aside
      className={floating ? "plate-caption-panel plate-caption-panel-floating" : "atlas-inspector plate-caption-panel"}
      aria-label="قراءة نطاق عثماني من خريطة ٩٣٦٧٧"
    >
      <div className="atlas-panel-head plate-caption-head">
        <div className="plate-caption-title-group">
          <div className="eyebrow">خريطة ٩٣٦٧٧ العثمانية · حوالي ١٩١٠م</div>
          <div className="plate-caption-name-row">
            <h2>{territory.nameAr}</h2>
            {showOttomanOriginal ? (
              <span className="plate-caption-printed" lang="ar">
                <bdi>{territory.ottomanName}</bdi>
              </span>
            ) : null}
          </div>
        </div>
        <button type="button" className="atlas-inspector-close" onClick={onClose} aria-label="إغلاق النطاق العثماني" title="إغلاق">
          <X size={15} />
        </button>
      </div>

      <div className="plate-caption-source-meta">
        <MapPin size={13} aria-hidden="true" />
        <div>
          <strong>خريطة وجدول عشائر العراق ونجد العثمانية (Harita 93677)</strong>
          <span>
            مكتبة جامعة إسطنبول · تفريغ GIS عبر OpenGulf (NYUAD) · {territory.centroid[1].toFixed(2)}°N، {territory.centroid[0].toFixed(2)}°E
          </span>
        </div>
      </div>

      <div className="plate-caption-territory-note plate-caption-ottoman-note" role="note">
        <strong>البيانات العثمانية المرافقة (حوالي ١٩١٠م)</strong>
        <span>{territory.summaryAr}</span>
        {(territory.population || territory.tents || territory.cavalry || territory.locationAr) ? (
          <div className="ottoman-census-grid">
            {territory.population ? (
              <div className="ottoman-census-stat">
                <span>تقدير السكان</span>
                <strong>{territory.population.toLocaleString("ar-EG")}</strong>
              </div>
            ) : null}
            {territory.tents ? (
              <div className="ottoman-census-stat">
                <span>الخيام</span>
                <strong>{territory.tents.toLocaleString("ar-EG")}</strong>
              </div>
            ) : null}
            {territory.cavalry ? (
              <div className="ottoman-census-stat">
                <span>الخيالة</span>
                <strong>{territory.cavalry.toLocaleString("ar-EG")}</strong>
              </div>
            ) : null}
            {territory.locationAr ? (
              <div className="ottoman-census-stat">
                <span>الموضع/التبعية</span>
                <strong>{territory.locationAr}</strong>
              </div>
            ) : null}
          </div>
        ) : null}
        <small>نطاق تقريبي مُسقَط من خريطة عثمانية تاريخية، وليس حداً إدارياً أو موثّقاً مستقلاً.</small>
      </div>

      <Link
        className="plate-caption-dossier-cta"
        to="/search"
        search={territory.tribeId ? { kind: "tribes", id: territory.tribeId } : { kind: "tribes", q: territory.nameAr }}
      >
        <BookOpen size={15} aria-hidden="true" />
        <span>{territory.tribeId ? "افتح سجل القبيلة" : "ابحث عن الاسم في فهرس القبائل"}</span>
        <ArrowLeft size={14} aria-hidden="true" />
      </Link>

      {crossQuery.isPending ? (
        <p className="plate-caption-sources-status" role="status">تُفحص المصادر التاريخية الأخرى…</p>
      ) : otherSources.length > 0 ? (
        <section className="plate-caption-sources" aria-label="شواهد الاسم في مصادر أخرى">
          <div className="plate-caption-section-head">
            <h3>في مصادر وخرائط أخرى</h3>
            <span className="plate-caption-count-badge">{otherSources.length}</span>
          </div>
          <ul className="plate-caption-sources-list">
            {otherSources.slice(0, 6).map((row) => (
              <li key={row.id} className="plate-caption-source-item">
                <div className="plate-caption-source-top">
                  <strong className="plate-caption-source-book">{row.titleAr}</strong>
                  <span className="plate-caption-spelling-chip">
                    <bdi lang={row.language}>{row.original}</bdi>
                  </span>
                </div>
                <div className="plate-caption-source-details">
                  <FileText size={12} aria-hidden="true" />
                  <span>{row.locatorAr}{row.periodAr ? ` · ${row.periodAr}` : ""}</span>
                </div>
                {row.url ? (
                  <div className="plate-caption-source-actions">
                    <a className="plate-caption-action-link" href={row.url} target="_blank" rel="noreferrer">
                      <ExternalLink size={12} aria-hidden="true" />
                      <span>افتح المصدر</span>
                    </a>
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </aside>
  );
}
