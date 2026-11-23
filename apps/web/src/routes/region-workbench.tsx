import { AtlasBoundaryReview } from "../components/AtlasBoundaryReview";
import { AtlasProposalReview } from "../components/AtlasProposalReview";
import { useRef, useState } from "react";
import { useIsMutating, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, Lock, LogOut, Send, ShieldAlert, ShieldCheck } from "lucide-react";
import { TopBar } from "../components/TopBar";
import {
  ApiError,
  claimRegionTile,
  decideRegionCandidate,
  fetchRegionCandidateCrop,
  fetchRegionTile,
  fetchRegionTiles,
  fetchRegionNameSuggestions,
  releaseRegionTile,
  submitRegionTile,
  type RegionDecideInput,
  type RegionTileCandidate,
  type RegionTileDetailResponse,
  type RegionTileListResponse,
  type RegionTileSubmitResult,
} from "../lib/api";

/**
 * ورضا مراجعة الملصقات المكانية — the tile workbench for the region lanes
 * (plans 030-E/F). A plate's proposals are divided into bounded squares; a
 * reviewer claims a square, reads each proposal's crop from the plate's own
 * pixels, and answers through the keyboard lane: accept the shown reading,
 * correct a misreading (type what the crop actually says), reject what is not
 * a tribal label (the cartouche and the town names are proposed too, because
 * the machine cannot tell), or defer what cannot be read now. Submitting
 * closes that reviewer's pass; a second reviewer's independent pass retires
 * the tile — or returns it to the queue with the difference named.
 *
 * The page fixes nothing and decides nothing: every reading travels the
 * candidate lane's decide surface under the reviewer's own id, and nothing a
 * tile shows renders anywhere as fact. Being without the moderator role is an
 * answer, and the page says so.
 */

const LANES = [
  { key: "open", label: "المفتوحة" },
  { key: "in_progress", label: "قيد العمل" },
  { key: "submitted", label: "بانتظار مراجعة ثانية" },
  { key: "retired", label: "المتقاعدة" },
] as const;

const statusLabels: Record<string, string> = {
  suggested: "مقترح بلا قراءة",
  accepted: "مقبول بقراءة",
  corrected: "مصحَّح بقراءة",
  rejected: "مرفوض",
  deferred: "مؤجَّل",
};

export function RegionWorkbenchPage() {
  const [lane, setLane] = useState("open");
  const [openTileID, setOpenTileID] = useState<string | null>(null);
  const queue = useQuery({
    queryKey: ["region-tiles", lane],
    queryFn: () => fetchRegionTiles({ status: lane }),
  });

  if (openTileID) {
    return <TileBench taskID={openTileID} onBack={() => setOpenTileID(null)} />;
  }

  return (
    <div className="page-stack">
      <TopBar
        eyebrow="ورشة القبائل"
        title="ورشة مراجعة الملصقات"
        description="مربعات محدودة من اللوحة: يقرأها مراجع، ويتقاعد منها باتفاق مراجعين مستقلين. لا شيء هنا حقيقة قبل القراءة."
      />
      <section className="workbench-proposals-card" aria-label="قراءات المصادر المستوردة">
        <AtlasProposalReview />
      </section>
      <div className="workbench-lanes" role="tablist" aria-label="طوابير المربعات">
        {LANES.map((entry) => (
          <button
            key={entry.key}
            role="tab"
            aria-selected={lane === entry.key}
            className={`workbench-lane${lane === entry.key ? " workbench-lane-active" : ""}`}
            onClick={() => setLane(entry.key)}
          >
            {entry.label}
          </button>
        ))}
      </div>
      {queue.isPending ? <p className="workbench-note">يُجهَّز الطابور…</p> : null}
      {queue.isError ? <WorkbenchNotice error={queue.error} /> : null}
      {queue.data ? <TileQueue data={queue.data} onOpen={setOpenTileID} /> : null}
    </div>
  );
}

function TileQueue({ data, onOpen }: { data: RegionTileListResponse; onOpen: (taskID: string) => void }) {
  if (data.tiles.length === 0) {
    return <p className="workbench-note">لا مربعات في هذا الطابور الآن.</p>;
  }
  return (
    <section aria-label="طابور المربعات">
      {data.truncated ? (
        <p className="workbench-note">القائمة مقتطعة عند {data.limit} مربعاً؛ هذه حدّ القراءة الواحدة ليستقيم الطابور.</p>
      ) : null}
      <ul className="workbench-tiles">
        {data.tiles.map((tile) => (
          <li key={tile.id}>
            <button className="workbench-tile" onClick={() => onOpen(tile.id)}>
              <b>{tile.tileId}</b>
              <span>
                {tile.boxPx.w}×{tile.boxPx.h} بكسل، من (‏{tile.boxPx.x}، {tile.boxPx.y}‏).
              </span>
              <small>{tile.status === "open" ? "بانتظار تبنّي مراجع" : tile.status === "in_progress" ? "مقعد محتلّ — لا يدخله مراجع ثانٍ حتى يُترك" : tile.status === "submitted" ? "بانتظار مراجعة ثانية" : "اتفق عليها مراجعان مستقلان"}</small>
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}

function TileBench({ taskID, onBack }: { taskID: string; onBack: () => void }) {
  const queryClient = useQueryClient();
  const savingDecisions = useIsMutating({ mutationKey: ["region-tile-decision", taskID] }) > 0;
  const detail = useQuery({
    queryKey: ["region-tile", taskID],
    queryFn: ({ signal }) => fetchRegionTile(taskID, signal),
  });

  const refreshTile = () => queryClient.invalidateQueries({ queryKey: ["region-tile", taskID] });
  const invalidate = () => Promise.all([refreshTile(), queryClient.invalidateQueries({ queryKey: ["region-tiles"] })]);

  const claim = useMutation({ mutationFn: () => claimRegionTile(taskID), onSuccess: invalidate });
  const release = useMutation({
    mutationFn: (note: string) => releaseRegionTile(taskID, note),
    onSuccess: async () => {
      await invalidate();
      onBack();
    },
  });
  const submit = useMutation({
    mutationFn: (note: string) => submitRegionTile(taskID, note),
    onSuccess: async (result: RegionTileSubmitResult) => {
      await invalidate();
      if (result.action === "confirm") {
        onBack();
      }
    },
  });

  const tile = detail.data?.tile;
  const hasSeat = detail.data?.claimedByMe === true;
  const changingSeat = release.isPending || submit.isPending;
  const allAnswered = Boolean(
    detail.isSuccess && !detail.data.truncated && detail.data.candidates.length > 0 &&
    detail.data.candidates.every((candidate) => candidate.answeredByMe),
  );

  return (
    <div className="page-stack">
      <TopBar
        eyebrow="ورشة القبائل"
        title={tile ? tile.tileId : "مربع من اللوحة"}
        description="اقرأ القصاصة من اللوحة نفسها واكتب ما تراه: قبول قراءة معروضة، تصحيح ما أخطأ، رفض ما هو ليس اسماً قبيلة، تأجيل ما لا يُقرأ الآن — ثم أرسل مرتكزك."
      />
      <div className="workbench-toolbar">
        <button className="workbench-back" onClick={onBack}>
          عودة للطابور <ArrowRight size={14} />
        </button>
        {!hasSeat && (tile?.status === "open" || tile?.status === "submitted") ? (
          <button className="workbench-claim" onClick={() => claim.mutate()} disabled={claim.isPending}>
            <Lock size={14} /> {tile?.status === "submitted" ? "ابنَ المراجعة الثانية" : "تبنَّ المربع"}
          </button>
        ) : null}
        {hasSeat ? (
          <>
            <button className="workbench-release" onClick={() => release.mutate("أترك المقعد مؤقتاً")} disabled={changingSeat || savingDecisions}>
              <LogOut size={14} /> أجِّل المقعد
            </button>
            <button
              className="workbench-submit"
              onClick={() => submit.mutate("")}
              disabled={changingSeat || savingDecisions || !allAnswered}
              title={allAnswered ? "أرسل مرتكزك على المربع" : "لم تُجب كل المقترحات بعد (التأجيل إجابة)"}
            >
              <Send size={14} /> أرسل المرتكز
            </button>
          </>
        ) : null}
      </div>
      {claim.isError ? <WorkbenchNotice error={claim.error} /> : null}
      {release.isError ? <WorkbenchNotice error={release.error} /> : null}
      {submit.isError ? <WorkbenchNotice error={submit.error} /> : null}
      {submit.data?.action === "invalidate" ? <DisagreementNotice /> : null}
      {detail.isPending ? <p className="workbench-note">يُقرأ المربع…</p> : null}
      {detail.isError ? <WorkbenchNotice error={detail.error} /> : null}
      {detail.data ? <CandidateList detail={detail.data} disabled={!hasSeat || changingSeat} onDecision={refreshTile} /> : null}
    </div>
  );
}

function CandidateList({ detail, disabled, onDecision }: {
  detail: RegionTileDetailResponse;
  disabled: boolean;
  onDecision: () => Promise<void>;
}) {
  return (
    <section aria-label="مقترحات المربع">
      <ul className="workbench-candidates">
        {detail.candidates.map((candidate, index) => (
          <li key={candidate.id}>
            <CandidateCard candidate={candidate} taskID={detail.tile.id} index={index} disabled={disabled} onDecision={onDecision} />
          </li>
        ))}
      </ul>
      {detail.candidates.length === 0 ? <p className="workbench-note">لا مقترحات في هذا المربع.</p> : null}
      {detail.truncated ? <p className="workbench-note">تجاوز عدد المقترحات حد العرض. يجب تقسيم المربع قبل إرسال المراجعة.</p> : null}
    </section>
  );
}

function CandidateCard({ candidate, taskID, index, disabled, onDecision }: {
  candidate: RegionTileCandidate;
  taskID: string;
  index: number;
  disabled: boolean;
  onDecision: () => Promise<void>;
}) {
  const crop = useQuery({
    queryKey: ["region-candidate-crop", candidate.id],
    queryFn: () => fetchRegionCandidateCrop(candidate.id),
    staleTime: Infinity,
  });

  const [reading, setReading] = useState("");

  const decide = useMutation({
    mutationKey: ["region-tile-decision", taskID],
    mutationFn: (input: RegionDecideInput) => decideRegionCandidate(candidate.id, input),
    onSuccess: () => {
      setReading("");
      return onDecision();
    },
  });

  const busy = disabled || decide.isPending;
  const suggested = candidate.status === "suggested";
  const canRead = suggested || candidate.status === "accepted" || candidate.status === "corrected";
  const draft = reading.trim();
  function saveReading() {
    if (!busy && canRead && draft) decide.mutate({ action: "correct", labelTextAr: draft });
  }

  return (
    <div className="workbench-candidate">
      <div className="workbench-candidate-meta">
        <header>
          <b>مقترح {index + 1}</b>
          <small> · {candidate.occurrenceId} </small>
          <span className={`workbench-pill workbench-pill-${candidate.status}`}>
            {statusLabels[candidate.status]}
          </span>
          {!disabled && <small>{candidate.answeredByMe ? "أجبتَ عن هذا المقترح" : "بانتظار إجابتك"}</small>}
        </header>
        <NamePicker
          value={reading}
          onPick={(choice) => setReading(choice)}
          onChangeText={setReading}
          onSubmit={saveReading}
          disabled={busy || !canRead}
          placeholder="اكتب ما تقرأه في القصاصة"
        />
        {candidate.labelTextAr ? (
          <p className="workbench-candidate-reading-shown">
            القراءة الحالية: «{candidate.labelTextAr}»
          </p>
        ) : null}
        <div className="workbench-candidate-actions">
          <button disabled={busy || !canRead || !candidate.labelTextAr || Boolean(draft)}
            onClick={() => decide.mutate({ action: suggested ? "accept" : "correct", labelTextAr: candidate.labelTextAr })}>
            تأكيد القراءة المعروضة
          </button>
          <button disabled={busy || !canRead || !draft} onClick={saveReading}>حفظ القراءة</button>
          <button disabled={busy || (!suggested && candidate.status !== "rejected")}
            onClick={() => decide.mutate({ action: "reject" })}>رفض</button>
          <button disabled={busy || (!suggested && candidate.status !== "deferred")}
            onClick={() => decide.mutate({ action: "defer" })}>تأجيل</button>
          {!suggested && <button disabled={busy} onClick={() => decide.mutate({ action: "reopen" })}>إعادة فتح</button>}
        </div>
        {decide.isError ? <WorkbenchNotice error={decide.error} /> : null}
        {candidate.status === "accepted" && <AtlasBoundaryReview candidateId={candidate.id} />}
      </div>
      {crop.data ? (
        <figure className="workbench-crop" aria-label="قصاصة من اللوحة">
          <img alt="قصاصة المقترح من اللوحة" src={`data:image/png;base64,${crop.data.tightPng}`} />
        </figure>
      ) : null}
    </div>
  );
}

function WorkbenchNotice({ error }: { error: unknown }) {
  const status = error instanceof ApiError ? error.status : undefined;
  const message = error instanceof ApiError ? error.message : "";
  const copy = (reason: string) => (
    <section className="workbench-refusal">
      <ShieldAlert size={16} />
      <p>{reason}</p>
    </section>
  );
  if (status === 401) return copy("هذه الورشة بجلسة موثقة؛ سجّل الدخول ثم عد.");
  if (status === 403) return copy("الورشة بدور المشرف أو المدير؛ طلبها بلا الدور هو طلب ما لا يُرى.");
  if (status === 400) return copy(message || "الطلب مرفوض: اجتهاد ناقص أو قراءة بلا نص.");
  if (status === 409) return copy(message || "المربع اتخذ موقفه: أجل آخر سارّ قبلك، أو المقعد لغيرك الآن.");
  if (status === 404) return copy("المربع أو المرشح غير موجود في هذه اللوحة.");
  if (status === 503) return copy("لا تُنتَج القصاصة الآن: الملف المخزَّن غائب أو غير مقروء. الإصلاح إعادة رفع، لا إعادة محاولة بالشكل نفسه.");
  return copy("فشل غير متوقع؛ لا أرقام تُصنع بدل الإخفاق.");
}

function DisagreementNotice() {
  return (
    <section className="workbench-disagreement" aria-label="نتيجة مقارنة المرتكزين">
      <ShieldCheck size={16} />
      <p>المرتكزان لم يتطابقا، فعاد المربع إلى الطابور والمُختلف مسمّى في سجلّه.</p>
    </section>
  );
}

function NamePicker({
  value,
  onPick,
  onChangeText,
  onSubmit,
  placeholder,
  disabled = false,
}: {
  value: string;
  onPick: (choice: string) => void;
  onChangeText: (text: string) => void;
  onSubmit: () => void;
  placeholder: string;
  disabled?: boolean;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const suggestions = useQuery({
    queryKey: ["region-name-suggestions", value],
    queryFn: () => fetchRegionNameSuggestions(value),
    enabled: !disabled,
    staleTime: 30_000,
  });
  const chips = suggestions.data ?? [];
  return (
    <div className="workbench-name-picker">
      <input
        ref={inputRef}
        className="workbench-name-picker"
        disabled={disabled}
        value={value}
        onChange={(event) => onChangeText(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter" && !event.nativeEvent.isComposing) onSubmit();
        }}
        placeholder={placeholder}
        dir="rtl"
      />
      <div className="workbench-chip-list" role="listbox" aria-label="أسماء مقترحة">
        {(chips).map((chip) => (
          <button
            key={chip.valueAr}
            type="button"
            role="option"
            aria-selected="false"
            disabled={disabled}
            className="workbench-name-chip"
            onClick={() => {
              onPick(chip.valueAr);
              inputRef.current?.focus();
            }}
          >
            {chip.valueAr}
          </button>
        ))}
      </div>
    </div>
  );
}
