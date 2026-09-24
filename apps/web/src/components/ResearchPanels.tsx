import { Link } from "@tanstack/react-router";
import { ArrowUpLeft, FileText, Layers3 } from "lucide-react";
import type { Activity, DashboardData, EpistemicTone } from "../types";
import { SectionHeading } from "./SectionHeading";
import { StatusBadge } from "./StatusBadge";

const toneForKind: Record<Activity["kind"], EpistemicTone> = {
  question: "question",
  claim: "claim",
  finding: "finding",
};

export function ActivityFeed({ activities }: { activities: Activity[] }) {
  return (
    <div className="activity-feed">
      {activities.map((activity) => (
        <article className="activity-row" key={activity.id}>
          <span className={`activity-marker marker-${activity.kind}`}>
            {activity.kind === "question" ? "؟" : activity.kind === "finding" ? "⌁" : "↗"}
          </span>
          <div className="activity-copy">
            <div className="activity-meta">
              <StatusBadge tone={toneForKind[activity.kind]} compact>{activity.status}</StatusBadge>
              <span>{activity.updatedAt}</span>
            </div>
            <h3>{activity.title}</h3>
            <p>{activity.description}</p>
          </div>
          <div className="activity-source-count">
            <FileText size={14} />
            <span>{activity.sourceCount} مصادر</span>
          </div>
        </article>
      ))}
    </div>
  );
}

export function LayerStack({ data }: { data: DashboardData }) {
  return (
    <div className="layer-stack-panel">
      <div className="layer-stack-head">
        <div>
          <div className="eyebrow">منهج دَوْحة</div>
          <h2>أربع طبقات، لا حقيقة واحدة</h2>
        </div>
        <Layers3 size={20} strokeWidth={1.5} />
      </div>
      <div className="layer-stack-list">
        {data.layerLegend.map((layer, index) => (
          <div className="layer-row" key={layer.label}>
            <span className={`layer-index layer-${layer.tone}`}>٠{index + 1}</span>
            <div>
              <strong>{layer.label}</strong>
              <span>{layer.description}</span>
            </div>
            <ArrowUpLeft size={15} />
          </div>
        ))}
      </div>
      <Link className="panel-link" to="/research">
        افتح مكتب البحث <span>←</span>
      </Link>
    </div>
  );
}

export function OpenQuestionsPreview({ data }: { data: DashboardData }) {
  return (
    <div className="questions-preview">
      <SectionHeading eyebrow="ما لم يُحسم" title="أسئلة تستحق وقتاً" description="الخلاف جزء من الخريطة، لا عطلاً فيها." />
      <div className="question-preview-list">
        {data.openQuestions.map((question) => (
          <Link to="/questions" className="question-preview-row" key={question.id}>
            <span className="question-priority">{question.priority}</span>
            <span className="question-preview-copy">
              <strong>{question.title}</strong>
              <small>{question.claimCount} ادعاءات مرتبطة · {question.updatedAt}</small>
            </span>
            <span className="question-status">{question.status}</span>
          </Link>
        ))}
      </div>
    </div>
  );
}
