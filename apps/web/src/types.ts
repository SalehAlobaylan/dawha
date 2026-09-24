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
  personId: string;
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

export interface TreeSummary {
  id: string;
  name: string;
  description: string;
  visibility: "private" | "unlisted" | "public";
  ownerId: string;
  updatedAt: string;
  latestVersionId: string;
  latestVersionNumber: number;
  latestState: "draft" | "published" | "archived";
  people: number;
  relationships: number;
  unresolved: number;
}

export interface TreeVersionRecord {
  id: string;
  number: number;
  state: "draft" | "published" | "archived";
  publicationNote: string;
  publishedAt: string | null;
  createdAt: string;
}

export interface TreeNodeRecord {
  id: string;
  personId: string;
  displayName: string;
  sortOrder: number;
  years: string;
  role: string;
  tone: EpistemicTone;
  sourceCount: number;
  note: string;
}

export type RelationshipStatus = "interpreted" | "disputed" | "unresolved";

export interface TreeRelationshipRecord {
  id: string;
  subjectNodeId: string;
  objectNodeId: string;
  predicate: string;
  status: RelationshipStatus;
}

export interface TreePermissions {
  canEdit: boolean;
  canPublish: boolean;
}

export interface TreeDetail {
  tree: TreeSummary;
  selectedVersion: TreeVersionRecord;
  permissions: TreePermissions;
  versions: TreeVersionRecord[];
  nodes: TreeNodeRecord[];
  relationships: TreeRelationshipRecord[];
}

export interface CreateTreeInput {
  name_ar: string;
  description_ar?: string;
  visibility: "private" | "unlisted" | "public";
  people: Array<{
    canonical_name_ar: string;
    gender?: "male" | "female" | "unknown";
    birth_date_from?: string;
    birth_date_to?: string;
    death_date_from?: string;
    death_date_to?: string;
  }>;
}

export interface AddPersonInput {
  canonical_name_ar: string;
  gender?: "male" | "female" | "unknown";
  birth_date_from?: string;
  birth_date_to?: string;
  death_date_from?: string;
  death_date_to?: string;
}

export interface AddRelationshipInput {
  subject_node_id: string;
  object_node_id: string;
  predicate: "parent_of" | "spouse_of" | "sibling_of";
  status: RelationshipStatus;
}

export interface UpdateRelationshipInput {
  status: RelationshipStatus;
  expected_version_id: string;
  reason_ar: string;
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
