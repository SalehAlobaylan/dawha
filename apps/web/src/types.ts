export type EpistemicTone = "source" | "claim" | "interpretation" | "finding" | "question" | "disputed";

export interface Metric {
  label: string;
  value: string;
  detail: string;
  tone: EpistemicTone;
}

export interface Activity {
  id: string;
  title: string;
  description: string;
  kind: "question" | "claim" | "finding";
  status: string;
  sourceCount: number;
  updatedAt: string;
}

export interface Question {
  id: string;
  title: string;
  status: string;
  priority: string;
  claimCount: number;
  updatedAt: string;
}

export interface TreePreview {
  title: string;
  version: string;
  state: string;
  people: number;
  relationships: number;
  unresolved: number;
}

export interface Layer {
  label: string;
  description: string;
  tone: EpistemicTone;
}

export interface DashboardData {
  mode: "demo" | "api";
  workspaceName: string;
  metrics: Metric[];
  activity: Activity[];
  openQuestions: Question[];
  treePreview: TreePreview;
  layerLegend: Layer[];
}

export interface TreeNode {
  id: string;
  name: string;
  role: string;
  years: string;
  x: number;
  y: number;
  tone: EpistemicTone;
  sourceCount: number;
  note: string;
}

export interface SourceRecord {
  id: string;
  title: string;
  type: string;
  locator: string;
  excerpt: string;
  status: "مفهرس" | "قيد المراجعة" | "مصدر أولي";
  date: string;
  dependent?: boolean;
}

export interface PlaceRecord {
  id: string;
  name: string;
  type: string;
  period: string;
  x: number;
  y: number;
  longitude: number;
  latitude: number;
  evidence: number;
  status: "موثق" | "مقترح" | "متنازع عليه";
}
