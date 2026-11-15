import { Link } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, FileText, Layers3, MapPinned, Search, X } from "lucide-react";
import { useState } from "react";
import type { MapFeature, PairingCandidate } from "../types";
import { featureLabel, markerProvenance, type AtlasMarker } from "../lib/atlas";
import { fetchPlaceEvidence, ApiError, fetchPlaceExternalIDs, fetchPairingCandidates, decidePairingCandidate, fetchPairingHistory } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import {
  formatMapCount,
  mapCertaintyLabel,
  mapEvidenceStatusLabel,
  mapKindLabel,
  mapRelationLabel,
  mapStatusCopy,
  mapUndatedRegionCopy,
} from "../lib/map-vocabulary";

interface AtlasInspectorProps {
  marker?: AtlasMarker;
  feature?: MapFeature;
  onClose: () => void;
  /**
   * The year window the map fetch carries, when one is active. The era rows on
   * a region's readout count only against a window: with none, they say why
   * they are silent rather than showing numbers that would read as "in the
   * era" when no era was asked for.
   */
  window?: { from: number; to: number } | null;
  /**
   * Whether the reader holds a moderation role. It only decides whether the
   * dossier fetch *asks* for the pending lane; the server re-checks the role
   * and answers 403, so this prop grants nothing on its own.
   */
  canModerate?: boolean;
}

/**
 * The provenance chamber.
 *
 * The order is the argument: what is claimed, by which source, with which
 * evidence, in which review state, and which claim the feature serves. A map
 * feature that has no evidence says so; nothing here is filled in from the
 * component's imagination.
 */
export function AtlasInspector({ marker, feature, onClose, window: eraWindow, canModerate = false }: AtlasInspectorProps) {
  const members = marker ? marker.members : feature ? [feature] : [];
  // The dossier of the selected place. The hook runs before any early return
  // (rules of hooks); with no selection the query idles on `enabled`. Regions
  // and migrations carry no single place — the section below says so instead
  // of querying a UUID that would 400.
  const dossierPlaceID = members.find((member) => member.placeId)?.placeId;
  const dossier = useQuery({
    queryKey: ["place-dossier", dossierPlaceID ?? null, canModerate],
    enabled: Boolean(dossierPlaceID),
    retry: false,
    staleTime: 30000,
    refetchOnWindowFocus: false,
    queryFn: ({ signal }) => fetchPlaceEvidence(dossierPlaceID!, { includeNeedsReview: canModerate, signal }),
  });
  // The accepted crosswalk («معرّفات خارجية»), read only. Only reviewer-
  // decided links land here, and only through sources the reader may open.
  const externalIds = useQuery({
    queryKey: ["place-external-ids", dossierPlaceID ?? null],
    enabled: Boolean(dossierPlaceID),
    retry: false,
    staleTime: 30000,
    refetchOnWindowFocus: false,
    queryFn: ({ signal }) => fetchPlaceExternalIDs(dossierPlaceID!, signal),
  });
  // Machine proposals for this place, when the reader may adjudicate them.
  // The list is a place-scoped read of the pairing lane: no new page, no new
  // queue — the decision happens where the fact would land.
  const pairingCandidates = useQuery({
    queryKey: ["place-pairings", dossierPlaceID ?? null],
    enabled: Boolean(dossierPlaceID && canModerate),
    retry: false,
    staleTime: 30000,
    refetchOnWindowFocus: false,
    queryFn: () => fetchPairingCandidates({ status: "suggested", placeId: dossierPlaceID }),
  });
  const queryClient = useQueryClient();
  const [pairingBusyID, setPairingBusyID] = useState<string | null>(null);
  const [pairingError, setPairingError] = useState("");
  const decidePairing = async (candidateID: string, effect: "alias" | "link" | "reject" | "defer", note: string) => {
    setPairingBusyID(candidateID);
    setPairingError("");
    try {
      await decidePairingCandidate(candidateID, effect, note);
      await queryClient.invalidateQueries({ queryKey: ["place-pairings"] });
      await queryClient.invalidateQueries({ queryKey: ["place-external-ids"] });
      await queryClient.invalidateQueries({ queryKey: ["place-dossier"] });
    } catch (error) {
      setPairingError(error instanceof ApiError && error.status === 409 ? "حُسمت هذا المقترح قبل قليل؛ أعد القراءة." : "تعذر حفظ القرار. أعد المحاولة.");
    } finally {
      setPairingBusyID(null);
    }
  };
  if (members.length === 0) {
    return (
      <aside className="atlas-inspector" aria-label="فاحصة الإشارة">
        <div className="atlas-panel-head">
          <div>
            <div className="eyebrow">الفاحصة</div>
            <h2>اختر إشارة</h2>
          </div>
        </div>
        <div className="atlas-inspector-empty">
          <Layers3 size={20} />
          <strong>اضغط نقطة أو خطاً أو منطقة</strong>
          <span>ستقرأ هنا ما تقوله الإشارة، ومن قاله، وأي ادعاء تخدمه. لا نعرض شيئاً لم يُسجَّل.</span>
        </div>
      </aside>
    );
  }

  const primary = members[0];
  const title = marker?.label ?? featureLabel(primary);
  const provenance = markerProvenance(members);
  const evidence = members.filter((member) => Boolean(member.evidenceText));
  const status = mapStatusCopy(primary.status);
  const placeID = marker?.placeId ?? primary.placeId;
  const placeName = marker?.placeName ?? primary.placeName;

  return (
    <aside className="atlas-inspector" aria-label="فاحصة الإشارة">
      <div className="atlas-panel-head">
        <div>
          <div className="eyebrow">الفاحصة / {mapKindLabel(primary.kind)}</div>
          <h2>{title}</h2>
        </div>
        <button type="button" className="atlas-inspector-close" onClick={onClose} aria-label="أغلق الفاحصة">
          <X size={15} />
        </button>
      </div>

      <div className="atlas-inspector-status">
        <StatusBadge tone={status.tone}>{status.label}</StatusBadge>
        {members.length > 1 ? <span className="atlas-inspector-count">{formatMapCount(members.length)} إشارات مجمّعة</span> : null}
      </div>

      <section className="atlas-inspector-section" aria-label="ما تقوله الإشارة">
        <h3>ما تقوله الإشارة</h3>
        <div className="atlas-facts">
          <div><span>العلاقة</span><strong>{mapRelationLabel(primary.relationType)}</strong></div>
          <div><span>الفترة</span><strong>{primary.period || "غير محددة"}</strong></div>
          <div><span>اليقين</span><strong>{mapCertaintyLabel(primary.certainty)}</strong></div>
          <div><span>حالة الدليل</span><strong>{mapEvidenceStatusLabel(primary.evidenceStatus)}</strong></div>
        </div>
        {primary.entityName ? <div className="atlas-entity"><span>الكيان</span><strong>{primary.entityName}</strong></div> : null}
        {primary.kind === "migration" ? <MigrationReadout feature={primary} /> : null}
        {primary.kind === "region" ? (
          <div className="atlas-region-readout">
            <span>نطاق تقريبي</span>
            <strong>{primary.ring ? `${formatMapCount(Math.max(0, primary.ring.length - 1))} نقطة حد` : "دون حلقة مرسومة"}</strong>
            <small>المضلع وصف تقريبي، وليس حداً تاريخياً.</small>
          </div>
        ) : null}
        {primary.kind === "region" ? (
          <>
            <div className="atlas-region-readout">
              <span>مواقع موثقة داخل المنطقة</span>
              <strong>{primary.placeCount ? `${formatMapCount(primary.placeCount)} موقعاً` : "لا مواقع يقرؤها هذا القارئ"}</strong>
              <small>عدد مشتق من سجلات المواقع التي تقرؤها، لا من الشكل المرسوم.</small>
            </div>
            {eraWindow ? (
              <>
                <div className="atlas-region-readout">
                  <span>شواهد مؤرَّخة في العصر ({eraWindow.from}—{eraWindow.to})</span>
                  <strong>{primary.associationCount ? `${formatMapCount(primary.associationCount)} شواهد` : "لا وثيقة مؤرَّخة في هذا العصر"}</strong>
                  <small>اتصالات موثَّقة أو متنازع عليها عند مواضع تقرؤها، تصادف نافذة العهد تواريخاً. الغياب يُقرأ كما هو.</small>
                </div>
                <div className="atlas-region-readout">
                  <span>أعيان موثَّقون في العصر</span>
                  <strong>{primary.personCount ? `${formatMapCount(primary.personCount)} أعيان` : "لا أعيان موثَّقين في هذا العصر"}</strong>
                  <small>عدّ الأعيان لا أعداد توثيقهم: أربعة شواهد لعين واحد تُقرأ عيناً واحدة.</small>
                </div>
                {primary.datedInWindow === false ? (
                  <p className="atlas-region-era-note" role="note">{mapUndatedRegionCopy.noteInsideWindow}</p>
                ) : null}
              </>
            ) : (
              <p className="atlas-region-era-note">عدّادات العهد تُقرأ مع نافذة سنة؛ الصفّ العلوي يبقى بلا عهد دائماً.</p>
            )}
          </>
        ) : null}
      </section>

      <section className="atlas-inspector-section" aria-label="الأدلة">
        <h3>الأدلة</h3>
        {evidence.length > 0 ? (
          <div className="atlas-evidence-list">
            {evidence.map((member) => (
              <blockquote key={member.evidenceId ?? member.id}>
                <FileText size={14} />
                <p>{member.evidenceText}</p>
                <small>{mapEvidenceStatusLabel(member.evidenceStatus)}</small>
              </blockquote>
            ))}
          </div>
        ) : (
          <p className="atlas-inspector-note">لا يوجد نص دليل مرتبط بهذه الإشارة بعد. غياب النص لا يعني نفي الواقعة، ولا إثباتها.</p>
        )}
      </section>

      <section className="atlas-inspector-section" aria-label="سجل الأدلة للموضع">
        <h3>سجل الأدلة للموضع</h3>
        {!dossierPlaceID ? (
          <p className="atlas-inspector-note">هذه الإشارة منطقة أو مسار هجرة — لا تحمل ملف أدلة واحداً. افتح موضعاً للقراءة عن موضعه.</p>
        ) : dossier.isPending || dossier.isFetching ? (
          <p className="atlas-inspector-note">يُقرأ سجل الأدلة…</p>
        ) : dossier.isError ? (
          <p className="atlas-inspector-note">
            {dossier.error instanceof ApiError && dossier.error.status === 403
              ? "قناة الانتظار تتطلب دور مراجعة؛ السجل المقبول متاح كما هو."
              : "تعذّر قراءة سجل الأدلة لهذا الموضع."}
          </p>
        ) : dossier.data && dossier.data.evidence.length > 0 ? (
          <div className="atlas-dossier-list">
            {dossier.data.evidence.map((row) => {
              const pending = row.status === "needs_review";
              const citation = [row.sourceTitle, row.locator || (row.pageNumber ? `ص ${row.pageNumber}` : "")].filter(Boolean).join(" · ");
              return (
                <div key={row.id} className={pending ? "atlas-dossier-row atlas-dossier-row-pending" : "atlas-dossier-row"}>
                  {pending ? (
                    <strong className="atlas-dossier-band">بانتظار المراجعة — يُعرض هنا لحساب التدقيق، ولا يدخل السجل المقبول</strong>
                  ) : null}
                  <blockquote>
                    <FileText size={13} />
                    <p>{row.passageText}</p>
                  </blockquote>
                  <div className="atlas-dossier-meta">
                    <span>{citation || "بلا حفاف"}</span>
                    {row.sourceId ? (
                      <Link to="/sources" search={{ sourceId: row.sourceId, view: "processing" }} className="atlas-dossier-link">
                        بيئة العمل <ArrowLeft size={11} />
                      </Link>
                    ) : null}
                  </div>
                </div>
              );
            })}
            {dossier.data.truncated ? (
              <p className="atlas-dossier-cut">السجل أطول من الحد المسموع؛ عُرض المقتطع الأول فقط ({dossier.data.evidence.length} من {dossier.data.limit ?? "—"}).</p>
            ) : null}
          </div>
        ) : (
          <p className="atlas-inspector-note">لا سجل أدلة لهذا الموضع بعد. غياب السجل لا يعني نفي الواقعة، ولا إثباتها.</p>
        )}
      </section>

      {members.length > 1 ? (
        <section className="atlas-inspector-section" aria-label="الإشارات المجمعة">
          <h3>ما جُمع في هذا الموضع</h3>
          <ul className="atlas-member-list">
            {members.slice(0, 8).map((member) => (
              <li key={member.id}>
                <span className="atlas-member-glyph" aria-hidden="true">{mapStatusCopy(member.status).glyph}</span>
                <span>
                  <strong>{member.entityName || mapRelationLabel(member.relationType)}</strong>
                  <small>{mapRelationLabel(member.relationType)} · {member.period || "غير محددة"}</small>
                </span>
              </li>
            ))}
          </ul>
          {members.length > 8 ? <p className="atlas-inspector-note">عُرضت ٨ من {formatMapCount(members.length)}. النقر على النقطة يعرض المجموع في العدّاد.</p> : null}
        </section>
      ) : null}

      <section className="atlas-inspector-section" aria-label="المصادر">
        <h3>المصدر</h3>
        {provenance.sources.size > 0 ? (
          <ul className="atlas-source-list">
            {[...provenance.sources.entries()].map(([sourceID, sourceTitle]) => (
              <li key={sourceID}>
                <FileText size={13} />
                <Link to="/sources" search={{ sourceId: sourceID }}>المصدر: {sourceTitle}</Link>
              </li>
            ))}
          </ul>
        ) : (
          <p className="atlas-inspector-note">لم يُربط مصدر بهذه الإشارة. السجل قائم، وسنده غائب.</p>
        )}
      </section>

      <section className="atlas-inspector-section" aria-label="معرّفات خارجية">
        <h3>معرّفات خارجية</h3>
        {!dossierPlaceID ? (
          <p className="atlas-inspector-note">هذه الإشارة لا تحمل موضعاً، فلا معرّفات لها.</p>
        ) : externalIds.isPending ? (
          <p className="atlas-inspector-note">يُقرأ المعرّفات…</p>
        ) : externalIds.isError ? (
          <p className="atlas-inspector-note">تعذّرت قراءة المعرّفات الخارجية لهذا الموضع.</p>
        ) : externalIds.data && externalIds.data.externalIds.length > 0 ? (
          <ul className="atlas-source-list">
            {externalIds.data.externalIds.map((item) => (
              <li key={`${item.sourceId}:${item.externalKey}`}>
                <FileText size={13} />
                <span>{item.externalKey} · {item.sourceTitle || item.sourceId}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="atlas-inspector-note">لا معرّفات خارجية مقبولة بعد. المعرّف يظهر هنا بعد قرار مراجع، لا بقرار الماكينة.</p>
        )}
        {canModerate && dossierPlaceID ? (
          <div className="pairing-inspector-block">
            <h4>مقترحات ربط بانتظار القرار</h4>
            {pairingError ? <p className="pairing-error" role="alert">{pairingError}</p> : null}
            {pairingCandidates.isPending ? (
              <p className="atlas-inspector-note">يُقرأ المقترحات…</p>
            ) : pairingCandidates.isError ? (
              <p className="atlas-inspector-note">تعذّرت قراءة مقترحات الربط لهذا الموضع.</p>
            ) : pairingCandidates.data && pairingCandidates.data.candidates.length > 0 ? (
              <div className="pairing-inspector-list">
                {pairingCandidates.data.candidates.map((candidate) => (
                  <PairingProposalCard
                    key={candidate.id}
                    candidate={candidate}
                    busy={pairingBusyID === candidate.id}
                    onDecide={decidePairing}
                  />
                ))}
              </div>
            ) : (
              <p className="atlas-inspector-note">لا مقترحات ربط معلّقة لهذا الموضع.</p>
            )}
          </div>
        ) : null}
      </section>

      <section className="atlas-inspector-section atlas-inspector-exits" aria-label="مخارج البحث">
        <h3>إلى أين من هنا؟</h3>
        {provenance.claims.length > 0 ? (
          provenance.claims.map((claimID) => (
            <Link key={claimID} to="/search" search={{ claimId: claimID }} className="atlas-exit-link">
              <Search size={14} />
              افتح الادعاء الذي تخدمه الإشارة
              <ArrowLeft size={14} />
            </Link>
          ))
        ) : (
          <p className="atlas-inspector-note">هذه إشارة جغرافية، لا تخدم ادعاءً محدداً. لا نصنع لها وجهة.</p>
        )}
        {placeID && placeName ? (
          <Link to="/search" search={{ q: placeName, kind: "places" }} className="atlas-exit-link atlas-exit-secondary">
            <MapPinned size={14} />
            ابحث عن الموضع في فهرس البحث
            <ArrowLeft size={14} />
          </Link>
        ) : null}
      </section>
    </aside>
  );
}

function PairingProposalCard({ candidate, busy, onDecide }: { candidate: PairingCandidate; busy: boolean; onDecide: (id: string, effect: "alias" | "link" | "reject" | "defer", note: string) => void }) {
  const [note, setNote] = useState("");
  const [showHistory, setShowHistory] = useState(false);
  const history = useQuery({
    queryKey: ["pairing-history", candidate.id],
    queryFn: ({ signal }) => fetchPairingHistory(candidate.id, signal),
    enabled: showHistory,
    retry: false,
  });
  return (
    <article className="pairing-card pairing-card-compact">
      <div className="pairing-card-head">
        <div>
          <strong>{candidate.externalKey} · {candidate.normalizedCandidates}</strong>
          <span>{candidate.sourceTitle || candidate.sourceId}{candidate.sourceLicense ? ` · ${candidate.sourceLicense}` : ""}</span>
        </div>
        <span className="pairing-rank">#{candidate.rank ?? "—"}</span>
      </div>
      <div className="pairing-components">
        <span>تشابه الاسم: {candidate.nameSimilarity.toFixed(2)}</span>
        <span>المسافة: {candidate.distanceKm !== undefined ? `${candidate.distanceKm.toFixed(1)} كم` : "—"}</span>
        <span>فرق الإحداثيات: {candidate.coordinateDeltaKm !== undefined ? `${candidate.coordinateDeltaKm.toFixed(1)} كم` : "—"}</span>
      </div>
      <label className="pairing-note-field">
        <span>ملاحظة القرار (اختياري)</span>
        <textarea
          aria-label={`ملاحظة القرار للمقترح ${candidate.externalKey}`}
          value={note}
          maxLength={1000}
          rows={2}
          onChange={(event) => setNote(event.target.value)}
          placeholder="ماذا حسمت ولماذا؟"
        />
      </label>
      <div className="pairing-actions">
        <button type="button" className="primary-button" disabled={busy} onClick={() => onDecide(candidate.id, "alias", note)}>اسم بديل فقط</button>
        <button type="button" className="secondary-button" disabled={busy} onClick={() => onDecide(candidate.id, "link", note)}>ربط</button>
        <button type="button" className="secondary-button" disabled={busy} onClick={() => onDecide(candidate.id, "reject", note)}>رفض</button>
        <button type="button" className="secondary-button" disabled={busy} onClick={() => onDecide(candidate.id, "defer", note)}>تأجيل</button>
      </div>
      <div className="pairing-history">
        <button type="button" className="text-button" aria-expanded={showHistory} onClick={() => setShowHistory((value) => !value)}>
          سجل القرارات
        </button>
        {showHistory ? (
          history.isPending ? <p className="pairing-note">يُقرأ السجل…</p> : history.isError ? <p className="pairing-error">تعذر قراءة السجل.</p> : history.data && history.data.history.length > 0 ? (
            <ol className="pairing-history-list">
              {history.data.history.map((event) => (
                <li key={event.id}>
                  <strong>{event.action}</strong>
                  <span>{event.fromStatus} ← {event.toStatus}{event.reviewerName ? ` · ${event.reviewerName}` : ""}{event.createdAt ? ` · ${event.createdAt}` : ""}</span>
                  {event.noteAr ? <span>{event.noteAr}</span> : null}
                </li>
              ))}
            </ol>
          ) : <p className="pairing-note">لا سجل لهذا المقترح بعد.</p>
        ) : null}
      </div>
    </article>
  );
}

function MigrationReadout({ feature }: { feature: MapFeature }) {
  if (feature.fromLongitude === undefined || feature.fromLatitude === undefined || feature.toLongitude === undefined || feature.toLatitude === undefined) {
    return null;
  }
  return (
    <div className="atlas-route-readout">
      <div><span>من</span><strong>{coordinate(feature.fromLongitude, feature.fromLatitude)}</strong></div>
      <div className="atlas-route-arrow" aria-hidden="true">←</div>
      <div><span>إلى</span><strong>{coordinate(feature.toLongitude, feature.toLatitude)}</strong></div>
    </div>
  );
}

function coordinate(longitude: number, latitude: number): string {
  return `${longitude.toFixed(2)}، ${latitude.toFixed(2)}`;
}
