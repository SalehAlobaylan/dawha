import { Link } from "@tanstack/react-router";
import { ArrowUpLeft, Check, CircleAlert, ExternalLink, FileText, GitBranch, MessageCircleQuestion } from "lucide-react";
import type { SourceRecord } from "../types";
import { StatusBadge } from "./StatusBadge";

export function SourceCard({ source }: { source: SourceRecord }) {
  return (
    <article className="source-card">
      <div className="source-card-top">
        <span className="source-type-icon"><FileText size={17} /></span>
        <div className="source-card-heading">
          <div className="source-card-kicker"><span>{source.type}</span><span>{source.date}</span></div>
          <h3>{source.title}</h3>
          <p>{source.locator}</p>
        </div>
        <StatusBadge tone={source.dependent ? "disputed" : "source"} compact>{source.status}</StatusBadge>
      </div>
      <blockquote>{source.excerpt}</blockquote>
      <div className="source-card-footer">
        <span className="source-derived-note">
          {source.dependent ? <CircleAlert size={13} /> : <Check size={13} />}
          {source.dependent ? "يحتاج فحص الاعتماد" : "سجل مستقل مبدئياً"}
        </span>
        <Link to="/sources" className="card-arrow" aria-label={`فتح ${source.title}`}><ExternalLink size={14} /></Link>
      </div>
    </article>
  );
}

export function EvidenceComparison() {
  return (
    <div className="evidence-comparison">
      <div className="comparison-head">
        <div>
          <div className="eyebrow">مقارنة الأدلة</div>
          <h2>ما تقوله المصادر، لا ما نريد استنتاجه</h2>
        </div>
        <GitBranch size={19} strokeWidth={1.5} />
      </div>
      <div className="comparison-columns">
        <div className="comparison-column comparison-column-source">
          <div className="comparison-column-head"><span className="comparison-marker" />المصدر الأول</div>
          <strong>محمد بن سعد</strong>
          <p>ذكرت العبارة صلة عبدالله بمحمد، لكنها لا تحسم صحة الرواية.</p>
          <span className="comparison-footer">الجزء الثاني · ص ١٢١</span>
        </div>
        <div className="comparison-column comparison-column-counter">
          <div className="comparison-column-head"><span className="comparison-marker" />رواية منافسة</div>
          <strong>صالح</strong>
          <p>رواية تذكر والداً آخر، مع إشارات إلى اعتماد محتمل.</p>
          <span className="comparison-footer">المجلد الأول · ص ٨٤</span>
        </div>
      </div>
      <div className="comparison-conclusion">
        <MessageCircleQuestion size={15} />
        <span>النتيجة: ادعاء متنازع عليه يستحق سؤالاً مفتوحاً، لا رفضاً تلقائياً.</span>
        <Link to="/questions"><ArrowUpLeft size={14} /></Link>
      </div>
    </div>
  );
}
