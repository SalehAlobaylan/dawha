import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { fetchSearch, fetchSource, fetchSources } from "../lib/api";

export type PickedRecord = { id: string; name: string; type: "person" | "family" | "branch" };
export function ResearchRecordPicker({ label, value, onChange }: { label: string; value: PickedRecord | null; onChange: (value: PickedRecord | null) => void }) {
  const [query, setQuery] = useState("");
  const result = useQuery({ queryKey: ["research-record-picker", query], queryFn: ({ signal }) => fetchSearch({ q: query, kind: "names", limit: 10, signal }), enabled: query.trim().length >= 2 && !value });
  const items = result.data?.groups.flatMap((group) => group.items).filter((item) => item.kind === "person" || item.kind === "family" || item.kind === "branch") ?? [];
  return <div className="research-record-picker"><span>{label}</span>{value ? <div className="research-picked-record"><strong>{value.name}</strong><button className="text-button" type="button" onClick={() => { onChange(null); setQuery(""); }}>تغيير</button></div> : <><input aria-label={label} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="ابحث بالاسم لاختيار سجل" />{query.trim().length >= 2 ? result.isPending ? <small role="status">جارٍ البحث…</small> : result.error ? <small role="alert">تعذر البحث في الأسماء.</small> : <div className="research-picker-results">{items.map((item) => <button type="button" key={item.id} onClick={() => onChange({ id: item.id, name: item.title, type: item.kind as PickedRecord["type"] })}>{item.title}<small>{item.subtitle}</small></button>)}{!items.length ? <small>لا توجد سجلات مطابقة. جرّب اسماً آخر.</small> : null}</div> : null}</>}</div>;
}

export function SourceStatementPicker({ value, onChange }: { value: string; onChange: (id: string) => void }) {
  const [sourceId, setSourceId] = useState("");
  const sources = useQuery({ queryKey: ["sources"], queryFn: fetchSources });
  const source = useQuery({ queryKey: ["source", sourceId], queryFn: () => fetchSource(sourceId), enabled: Boolean(sourceId) });
  return <div className="research-source-picker"><label>المصدر<select aria-label="مصدر الدليل" value={sourceId} onChange={(event) => { setSourceId(event.target.value); onChange(""); }}><option value="">اختر المصدر</option>{sources.data?.map((item) => <option key={item.id} value={item.id}>{item.titleAr}</option>)}</select></label>{sourceId ? <label>عبارة الدليل<select aria-label="عبارة الدليل" required value={value} onChange={(event) => onChange(event.target.value)}><option value="">اختر عبارة محفوظة</option>{source.data?.statements.map((statement) => <option key={statement.id} value={statement.id}>{statement.statementTextAr.slice(0, 140)}</option>)}</select>{source.isSuccess && !source.data.statements.length ? <small>لا توجد عبارات محفوظة. أضف اقتباساً من صفحة المصدر أولاً.</small> : null}</label> : null}{sources.error || source.error ? <p role="alert">تعذر تحميل عبارات المصدر.</p> : null}</div>;
}
