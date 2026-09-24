import { useMutation, useQuery } from "@tanstack/react-query";
import { ArrowLeft, GitCompareArrows, GitFork, LoaderCircle, X } from "lucide-react";
import { FormEvent, useState } from "react";
import { fetchTreeDiff, forkTree } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import type { TreeDetail, TreeDiff, TreeDiffRelationship } from "../types";

type ForkDiffPanelProps = {
  detail: TreeDetail;
  mode: "fork" | "diff";
  onForked: (detail: TreeDetail) => void;
  onClose: () => void;
};

export function ForkDiffPanel({ detail, mode, onForked, onClose }: ForkDiffPanelProps) {
  const [name, setName] = useState(`${detail.tree.name} — نسخة مستقلة`);
  const [message, setMessage] = useState("");
  const forkMutation = useMutation({
    mutationFn: () => forkTree(detail.tree.id, { version_id: detail.selectedVersion.id, name_ar: name.trim() || undefined, visibility: "private" }),
    onSuccess: (forked) => {
      setMessage("أُنشئ التفريع كمسودة مستقلة.");
      onForked(forked);
    },
    onError: (error) => setMessage(error instanceof Error ? error.message : "تعذر إنشاء التفريع."),
  });
  const canCompare = Boolean(detail.tree.parentTreeId && detail.tree.parentVersionId);
  const diffQuery = useQuery({
    queryKey: ["tree", detail.tree.id, "diff", detail.tree.parentVersionId ?? "", detail.selectedVersion.id],
    queryFn: () => fetchTreeDiff(detail.tree.id, { fromTreeId: detail.tree.parentTreeId ?? "", fromVersionId: detail.tree.parentVersionId ?? "", toVersionId: detail.selectedVersion.id }),
    enabled: mode === "diff" && canCompare,
  });

  return (
    <section className="fork-diff-panel">
      <div className="fork-diff-head">
        <div className="fork-diff-title"><div className="fork-diff-icon">{mode === "fork" ? <GitFork size={18} /> : <GitCompareArrows size={18} />}</div><div><div className="eyebrow">{mode === "fork" ? "نسخة مستقلة" : "مقارنة دلالية"}</div><h2>{mode === "fork" ? "افتح تفسيراً آخر دون تعديل الأصل" : "ماذا تغيّر منذ النسخة الأصلية؟"}</h2><p>{mode === "fork" ? "ينسخ التفريع نسخة منشورة إلى مسودة جديدة، ويبقى المصدر كما هو." : "نعرض الأشخاص والعلاقات والتواريخ، ولا نعتبر التشابه كافياً لإثبات حقيقة."}</p></div></div>
        <button className="icon-button" type="button" aria-label="إغلاق اللوحة" onClick={onClose}><X size={16} /></button>
      </div>
      {message ? <div className="fork-diff-message" role="status">{message}</div> : null}
      {mode === "fork" ? <form className="fork-form" onSubmit={(event: FormEvent<HTMLFormElement>) => { event.preventDefault(); forkMutation.mutate(); }}><label className="composer-label">اسم النسخة المستقلة<input required maxLength={200} value={name} onChange={(event) => setName(event.target.value)} /></label><div className="fork-form-note"><StatusBadge tone="claim">مسودة خاصة</StatusBadge><span>لن يتغير المصدر أو نسخة منشورة منه.</span></div><button className="primary-button" type="submit" disabled={forkMutation.isPending || detail.selectedVersion.state !== "published"}>{forkMutation.isPending ? <LoaderCircle className="spin" size={15} /> : <GitFork size={15} />} {forkMutation.isPending ? "جارٍ التفريع…" : "أنشئ نسخة مستقلة"}</button></form> : <DiffContent detail={detail} diff={diffQuery.data} pending={diffQuery.isPending} error={diffQuery.error} canCompare={canCompare} />}
    </section>
  );
}

function DiffContent({ detail, diff, pending, error, canCompare }: { detail: TreeDetail; diff?: TreeDiff; pending: boolean; error: unknown; canCompare: boolean }) {
  if (!canCompare) {
    return <div className="fork-diff-empty"><GitCompareArrows size={19} /><p>هذه الشجرة ليست تفريعاً موثقاً بعد. افتح تفريعاً منشوراً أو اختر نسختين من الشجرة نفسها في خطوة لاحقة.</p></div>;
  }
  if (pending) {
    return <div className="fork-diff-empty"><LoaderCircle className="spin" size={19} /><p>جارٍ حساب الفروق الدلالية…</p></div>;
  }
  if (error) {
    return <div className="fork-diff-empty fork-diff-error"><p>{error instanceof Error ? error.message : "تعذر تحميل المقارنة."}</p></div>;
  }
  if (!diff) {
    return null;
  }
  return <div className="diff-content"><div className="diff-endpoints"><span>من <strong>{diff.from.treeName}</strong> · v{diff.from.versionNumber}</span><ArrowLeft size={14} /><span>إلى <strong>{diff.to.treeName}</strong> · v{diff.to.versionNumber}</span></div><div className="diff-summary"><span><strong>{diff.peopleAdded.length}</strong> مضاف</span><span><strong>{diff.peopleRemoved.length}</strong> محذوف</span><span><strong>{diff.dateChanges.length}</strong> تاريخي</span><span><strong>{diff.relationshipChanges.length + diff.relationshipsAdded.length + diff.relationshipsRemoved.length}</strong> علاقة متغيرة</span><span className={diff.affectedDescendants > 0 ? "diff-impact" : ""}><strong>{diff.affectedDescendants}</strong> أثر متصل</span></div><div className="diff-columns"><DiffPeople title="أشخاص مضافون" items={diff.peopleAdded.map((item) => item.displayName)} tone="source" /><DiffPeople title="أشخاص محذوفون" items={diff.peopleRemoved.map((item) => item.displayName)} tone="disputed" /><DiffRelationships title="علاقات مضافة" items={diff.relationshipsAdded} /><DiffRelationships title="علاقات محذوفة" items={diff.relationshipsRemoved} /></div>{diff.relationshipChanges.length > 0 ? <div className="diff-section"><div className="detail-label">تغيّرات العلاقات</div>{diff.relationshipChanges.map((item) => <div className="diff-line" key={`${item.subject}-${item.object}-${item.predicate}`}><span>{item.subject} <ArrowLeft size={11} /> {item.object}</span><small>{item.beforeStatus} ← {item.afterStatus}</small></div>)}</div> : null}{diff.dateChanges.length > 0 ? <div className="diff-section"><div className="detail-label">تغيّرات التواريخ</div>{diff.dateChanges.map((item) => <div className="diff-line" key={item.personId}><span>{item.displayName}</span><small>{item.beforeYears} ← {item.afterYears}</small></div>)}</div> : null}{detail.tree.parentVersionId ? <div className="diff-footnote">المقارنة بين تفسيرين، ولا تغيّر حالة الشخص أو المصدر الأصلية.</div> : null}</div>;
}

function DiffPeople({ title, items, tone }: { title: string; items: string[]; tone: "source" | "disputed" }) {
  return <div className="diff-section"><div className="detail-label">{title}</div>{items.length > 0 ? items.map((item) => <div className="diff-line" key={item}><StatusBadge tone={tone}>{item}</StatusBadge></div>) : <span className="diff-empty">لا يوجد</span>}</div>;
}

function DiffRelationships({ title, items }: { title: string; items: TreeDiffRelationship[] }) {
  return <div className="diff-section"><div className="detail-label">{title}</div>{items.length > 0 ? items.map((item) => <div className="diff-line" key={`${item.subject}-${item.object}-${item.predicate}`}><span>{item.subject} <ArrowLeft size={11} /> {item.object}</span><small>{predicateLabel(item.predicate)}</small></div>) : <span className="diff-empty">لا يوجد</span>}</div>;
}

function predicateLabel(predicate: string): string {
  if (predicate === "parent_of") return "أبوة";
  if (predicate === "spouse_of") return "زواج";
  return "إخوة";
}
