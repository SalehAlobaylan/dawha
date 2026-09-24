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

export interface SourceMetadata {
  id: string;
  titleAr: string;
  authorAr?: string;
  sourceType: string;
  publicationDateFrom?: string;
  publicationDateTo?: string;
  editionAr?: string;
  citationAr?: string;
  locationAr?: string;
  dependencyStatus: "unknown" | "independent" | "derived" | "likely_dependent";
  visibility: "public" | "private";
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  passageCount: number;
  statementCount: number;
}

export interface SourcePassage {
  id: string;
  sourceId: string;
  sourceFileId?: string;
  sequenceNumber: number;
  pageNumber?: number;
  locatorAr?: string;
  startOffset?: number;
  endOffset?: number;
  textAr: string;
  createdAt: string;
}

export interface SourceStatement {
  id: string;
  sourceId: string;
  sourceFileId?: string;
  sourcePassageId?: string;
  statementTextAr: string;
  locatorAr?: string;
  extractionMethod: "manual" | "ocr" | "ai" | "imported";
  reviewStatus: "unreviewed" | "accepted" | "rejected" | "needs_review";
  createdBy: string;
  createdAt: string;
}

export interface SourceDetail {
  source: SourceMetadata;
  passages: SourcePassage[];
  statements: SourceStatement[];
}

export interface SourceFile {
  id: string;
  sourceId: string;
  originalFilenameAr: string;
  mimeType: string;
  byteSize: number;
  checksumSha256: string;
  processingStatus: "queued" | "running" | "succeeded" | "failed";
  processingError?: string;
  processedAt?: string;
  createdAt: string;
}

export interface SourceProcessingRun {
  id: string;
  sourceId: string;
  sourceFileId: string;
  jobId?: string;
  status: "queued" | "running" | "succeeded" | "failed";
  stage: string;
  pageCount: number;
  passageCount: number;
  candidateCount: number;
  modelVersion?: string;
  error?: string;
  startedAt?: string;
  completedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface CandidateReview {
  id: string;
  reviewerId: string;
  decision: "accepted" | "rejected";
  noteAr?: string;
  createdAt: string;
}

export interface SourceCandidate {
  id: string;
  sourceId: string;
  sourceFileId: string;
  sourcePassageId: string;
  sourceStatementId?: string;
  candidateType: "entity" | "claim";
  rawTextAr: string;
  normalizedTextAr: string;
  subjectTextAr?: string;
  predicateAr?: string;
  objectTextAr?: string;
  proposedEntityType?: string;
  proposedEntityId?: string;
  proposedEntityNameAr?: string;
  proposedMatchScore?: number;
  proposedMatchMatchedOn?: string;
  subjectEntityType?: string;
  subjectEntityId?: string;
  objectEntityType?: string;
  objectEntityId?: string;
  confidence: number;
  rationaleAr: string;
  pageNumber?: number;
  passageTextAr: string;
  locatorAr?: string;
  modelVersion?: string;
  status: "unreviewed" | "accepted" | "rejected" | "needs_review";
  reviewedBy?: string;
  reviewedAt?: string;
  reviewNoteAr?: string;
  acceptedRecordType?: string;
  acceptedRecordId?: string;
  createdAt: string;
  updatedAt: string;
  reviews: CandidateReview[];
}

export interface SourceProcessing {
  sourceId: string;
  files: SourceFile[];
  runs: SourceProcessingRun[];
  candidates: SourceCandidate[];
}

export interface ClaimEvidence {
  id: string;
  relation: "supports" | "contextualizes" | "contradicts" | "refutes";
  evidenceNoteAr?: string;
  sourceStatementId?: string;
  sourcePassageId?: string;
  sourceTitleAr?: string;
  statementTextAr?: string;
  passageTextAr?: string;
}

export interface ResearchClaim {
  id: string;
  subjectType: string;
  subjectId: string;
  predicate: string;
  objectType: string;
  objectId: string;
  timeFrom?: string;
  timeTo?: string;
  placeId?: string;
  status: string;
  notesAr?: string;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  evidence: ClaimEvidence[];
}

export type ResearchLayer = "source_statement" | "research_claim" | "tree_interpretation" | "platform_finding" | "open_question";

export interface ResearchScore {
  lexical?: number;
  vector?: number;
  combined?: number;
  rerank?: number;
}

export interface ResearchCitation {
  layer: ResearchLayer;
  type: string;
  id: string;
  sourceId?: string;
  passageId?: string;
  statementId?: string;
  claimId?: string;
  findingId?: string;
  questionId?: string;
  title: string;
  excerpt: string;
  locatorAr?: string;
  pageNumber?: number;
  reviewStatus?: string;
  status?: string;
  rank: number;
  score: ResearchScore;
}

export interface ResearchConflict {
  type: string;
  leftId: string;
  rightId: string;
  status: string;
  explanation: string;
  sourceIds?: string[];
}

export interface ResearchLayeredEvidence {
  sourceStatements: ResearchCitation[];
  researchClaims: ResearchCitation[];
  treeInterpretations: ResearchCitation[];
  platformFindings: ResearchCitation[];
  openQuestions: ResearchCitation[];
}

export type GraphOperation = "common_ancestor_path" | "evidence_connection" | "branch_claims" | "source_entities" | "geographic_path";

export interface GraphTreeScope {
  treeId?: string;
  treeVersionId?: string;
  versionNumber?: number;
  versionState?: string;
}

export interface GraphNode {
  id: string;
  type: string;
  label?: string;
  personId?: string;
  treeNodeId?: string;
  position: number;
}

export interface GraphEdge {
  id: string;
  type: string;
  fromNodeId: string;
  toNodeId: string;
  predicate?: string;
  status?: string;
  certainty?: string;
  sourceId?: string;
  claimId?: string;
  statementId?: string;
  passageId?: string;
  treeRelationshipId?: string;
  migrationEventId?: string;
  fromPlaceId?: string;
  toPlaceId?: string;
  position: number;
}

export interface GraphEvidenceRef {
  id: string;
  type: string;
  layer?: string;
  relation?: string;
  sourceId?: string;
  claimId?: string;
  statementId?: string;
  passageId?: string;
  reviewStatus?: string;
  status?: string;
  certainty?: string;
  title?: string;
  excerpt?: string;
  locatorAr?: string;
  pageNumber?: number;
}

export interface GraphPath {
  id: string;
  operation: GraphOperation;
  nodes: GraphNode[];
  edges: GraphEdge[];
  evidenceRefs: GraphEvidenceRef[];
  status: string;
  explanation: string;
  depth: number;
  truncated: boolean;
  evidenceBacked: boolean;
  structuralOnly: boolean;
  treeScope: GraphTreeScope;
  algorithmVersion: string;
}

export interface GraphStats {
  operation?: GraphOperation;
  pathCount: number;
  nodeCount: number;
  edgeCount: number;
  evidenceCount: number;
  truncated: boolean;
  pathsTruncated: boolean;
  edgesTruncated: boolean;
  maxDepth: number;
  algorithmVersion?: string;
}

export interface ResearchRetrievalStats {
  lexicalCandidates: number;
  vectorCandidates: number;
  fusedCandidates: number;
  rerankedCandidates: number;
  evidenceCount: number;
}

export type ResearchRoute = "ignore" | "cheap" | "deep";

export interface ResearchRouting {
  route: ResearchRoute;
  queryType: string;
  reasonCode: string;
  model: string;
  fallback: boolean;
  operationalScore: number;
  sourceBearing: boolean;
  potentialContradiction: boolean;
  continueInvestigation: boolean;
  synthesisAttempted: boolean;
}

export interface ResearchQueryResult {
  runId: string;
  query: string;
  normalizedQuery: string;
  queryType: string;
  answer: string;
  insufficientEvidence: boolean;
  modelVersion?: string;
  routing?: ResearchRouting;
  createdAt: string;
  citations: ResearchCitation[];
  layers: ResearchLayeredEvidence;
  conflicts: ResearchConflict[];
  retrieval: ResearchRetrievalStats;
  graphPaths: GraphPath[];
  graphStats: GraphStats;
}

export interface ResearchQueryInput {
  question: string;
  question_id?: string;
  entity_type?: "person" | "family" | "branch";
  entity_id?: string;
  tree_id?: string;
  tree_version_id?: string;
  source_id?: string;
  person_id?: string;
  place_id?: string;
  from_year?: number;
  to_year?: number;
  graph_operation?: GraphOperation;
  graph_start_type?: "person" | "family" | "branch" | "source" | "place";
  graph_start_id?: string;
  graph_end_type?: "person" | "family" | "branch" | "source" | "place";
  graph_end_id?: string;
  graph_max_depth?: number;
}

export interface ResearchWorkspaceInput {
  questionId: string;
  entityType?: "person" | "family" | "branch";
  entityId?: string;
  treeId?: string;
  treeVersionId?: string;
}

export interface ResearchWorkspaceContext {
  questionId: string;
  questionTitleAr: string;
  questionDetailAr?: string;
  questionStatus: string;
  questionPriority: string;
  entityType?: string;
  entityId?: string;
  entityNameAr?: string;
  entityAliasesAr?: string[];
  treeId?: string;
  treeVersionId?: string;
}

export interface ResearchWorkspacePermissions {
  canRunResearch: boolean;
  canCreateClaim: boolean;
  canDisputeClaim: boolean;
  canLinkEvidence: boolean;
  canCreateQuestion: boolean;
  canManageSelectedQuestion: boolean;
  canAttachFinding: boolean;
  canReviewFinding: boolean;
  canAddNote: boolean;
  canReviewIdentityCandidate: boolean;
  canMergeIdentity: boolean;
}

export interface WorkspaceEvidence {
  id: string;
  kind: "evidence" | "counter_evidence";
  relation: string;
  sourceId?: string;
  sourceTitleAr?: string;
  statementId?: string;
  statementTextAr?: string;
  passageId?: string;
  passageTextAr?: string;
  locatorAr?: string;
  pageNumber?: number;
  reviewStatus?: string;
}

export interface WorkspaceClaim {
  id: string;
  subjectType: string;
  subjectId: string;
  predicate: string;
  objectType: string;
  objectId: string;
  timeFrom?: string;
  timeTo?: string;
  placeId?: string;
  status: string;
  notesAr?: string;
  createdAt: string;
  updatedAt: string;
  evidence: WorkspaceEvidence[];
}

export interface WorkspaceSource {
  id: string;
  titleAr: string;
  authorAr?: string;
  sourceType: string;
  dependencyStatus: string;
  visibility: string;
  citationAr?: string;
  locationAr?: string;
  passageCount: number;
  statementCount: number;
}

export interface WorkspaceTreeNode {
  id: string;
  personId: string;
  displayNameAr: string;
  sortOrder: number;
}

export interface WorkspaceTreeRelationship {
  id: string;
  subjectPersonId: string;
  objectPersonId: string;
  predicate: string;
  status: string;
  sourceId?: string;
  sourceTitleAr?: string;
}

export interface WorkspaceTreeContext {
  treeId: string;
  treeNameAr: string;
  treeVersionId: string;
  versionNumber: number;
  state: string;
  nodes: WorkspaceTreeNode[];
  relationships: WorkspaceTreeRelationship[];
}

export interface WorkspaceTimelineEvent {
  id: string;
  kind: "birth" | "death" | "claim" | "presence" | "migration";
  labelAr: string;
  dateFrom?: string;
  dateTo?: string;
  approximate: boolean;
  layer: ResearchLayer;
  status: string;
  placeId?: string;
  claimId?: string;
  sourceIds?: string[];
}

export interface WorkspaceQuestion {
  id: string;
  titleAr: string;
  descriptionAr?: string;
  status: string;
  priority: string;
  createdAt: string;
  updatedAt: string;
  claimCount: number;
  sourceCount: number;
}

export interface WorkspaceDispute {
  id: string;
  titleAr: string;
  descriptionAr?: string;
  status: string;
  createdAt: string;
  updatedAt: string;
  claimCount: number;
}

export interface WorkspaceNote {
  id: string;
  noteAr: string;
  createdBy: string;
  createdAt: string;
}

export interface WorkspaceFinding {
  id: string;
  findingType: string;
  titleAr: string;
  explanationAr: string;
  status: string;
  severity: string;
  signals: Record<string, unknown>;
  claimIds: string[];
  entityIds: string[];
  evidence: WorkspaceEvidence[];
  traceability: "evidence" | "signals_only";
  algorithmVersion?: string;
}

export interface WorkspaceIdentityCandidate {
  id: string;
  runId: string;
  entityType: string;
  leftEntityId: string;
  rightEntityId: string;
  leftNameAr: string;
  rightNameAr: string;
  matchClass: string;
  score: number;
  status: string;
  createdAt: string;
}

export interface ResearchRunSummary {
  id: string;
  questionId?: string;
  query: string;
  status: string;
  insufficientEvidence: boolean;
  citationCount: number;
  graphPathCount: number;
  graphOperation?: GraphOperation;
  graphMaxDepth?: number;
  graphTruncated: boolean;
  answerAr?: string;
  modelVersion?: string;
  route?: string;
  synthesisAttempted: boolean;
  error?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ResearchRunContext {
  scopeType: string;
  scopeId: string;
  role: string;
}

export interface ResearchRunDetail extends ResearchRunSummary {
  contexts: ResearchRunContext[];
  graphPaths: GraphPath[];
}

export interface ResearchWorkspaceSnapshot {
  context: ResearchWorkspaceContext;
  permissions: ResearchWorkspacePermissions;
  claims: WorkspaceClaim[];
  sources: WorkspaceSource[];
  treeContexts: WorkspaceTreeContext[];
  mapFeatures: MapFeature[];
  timeline: WorkspaceTimelineEvent[];
  questions: WorkspaceQuestion[];
  disputes: WorkspaceDispute[];
  notes: WorkspaceNote[];
  findings: WorkspaceFinding[];
  identityCandidates: WorkspaceIdentityCandidate[];
  history: ResearchRunSummary[];
}

export type EntityResolutionEntityType = "person" | "family" | "all";
export type EntityResolutionMatchClass = "likely_different" | "possible_match" | "strong_candidate";
export type EntityResolutionReviewStatus = "pending" | "approved" | "rejected" | "deferred" | "reopened" | "merged";

export interface EntityResolutionRun {
  id: string;
  requestedBy: string;
  entityType: EntityResolutionEntityType;
  status: "running" | "succeeded" | "failed";
  algorithmVersion: string;
  normalizationVersion: string;
  modelVersion?: string;
  candidateCount: number;
  error?: string;
  createdAt: string;
  updatedAt: string;
}

export interface EntityResolutionSignal {
  kind: string;
  detail: string;
  score: number;
}

export interface EntityResolutionCandidate {
  id: string;
  runId: string;
  entityType: "person" | "family";
  leftEntityId: string;
  rightEntityId: string;
  leftNameAr: string;
  rightNameAr: string;
  matchClass: EntityResolutionMatchClass;
  score: number;
  scoreComponents: Record<string, number>;
  matchingSignals: EntityResolutionSignal[];
  conflictingSignals: EntityResolutionSignal[];
  explanationAr: string;
  reviewStatus: EntityResolutionReviewStatus;
  candidateVersion: number;
  requiresHumanReview: boolean;
  reviewedBy?: string;
  reviewedAt?: string;
  reviewNoteAr?: string;
  createdAt: string;
  updatedAt: string;
}

export interface EntityResolutionReviewInput {
  decision: "approve" | "reject" | "defer" | "reopen";
  note_ar?: string;
  expected_version?: number;
}

export interface EntityResolutionMergeInput {
  survivor_entity_id: string;
  reason_ar: string;
  expected_candidate_version: number;
  confirm: boolean;
}

export interface EntityResolutionMerge {
  id: string;
  candidateId: string;
  entityType: "person" | "family";
  survivorId: string;
  mergedId: string;
  state: "applied" | "reversed";
  requestedBy: string;
  reasonAr: string;
  appliedAt: string;
  reversedBy?: string;
  reversedAt?: string;
  reversalReasonAr?: string;
}

export interface ContradictionRun {
  id: string;
  requestedBy: string;
  jobId?: string;
  status: "queued" | "running" | "succeeded" | "failed";
  algorithmVersion: string;
  findingCount: number;
  error?: string;
  createdAt: string;
  startedAt?: string;
  completedAt?: string;
  updatedAt: string;
}

export interface ContradictionReview {
  id: string;
  reviewerId: string;
  decision: "dismiss" | "confirm" | "investigate" | "reopen";
  noteAr?: string;
  questionId?: string;
  createdAt: string;
}

export interface ContradictionFinding {
  id: string;
  runId?: string;
  findingType: string;
  titleAr: string;
  explanationAr: string;
  status: "needs_review" | "confirmed" | "dismissed" | "investigating";
  severity: "low" | "medium" | "high";
  signals: Record<string, unknown>;
  claimIds: string[];
  entityIds: string[];
  algorithmVersion: string;
  modelVersion?: string;
  createdBy?: string;
  reviewedBy?: string;
  reviewedAt?: string;
  reviewNoteAr?: string;
  questionId?: string;
  createdAt: string;
  updatedAt: string;
  reviews: ContradictionReview[];
}

export interface ContradictionReviewInput {
  decision: "dismiss" | "confirm" | "investigate" | "reopen";
  note_ar?: string;
  create_question?: boolean;
  question_title_ar?: string;
}

export interface ClaimSummary {
  id: string;
  subjectType: string;
  predicate: string;
  objectType: string;
  status: string;
  evidenceCount: number;
  createdAt: string;
}

export interface CreateSourceInput {
  title_ar: string;
  author_ar?: string;
  source_type: string;
  publication_date_from?: string;
  publication_date_to?: string;
  edition_ar?: string;
  citation_ar?: string;
  location_ar?: string;
  dependency_status?: "unknown" | "independent" | "derived" | "likely_dependent";
}

export interface CreatePassageInput {
  sequence_number?: number;
  page_number?: number;
  locator_ar?: string;
  text_ar: string;
}

export interface CreateStatementInput {
  source_passage_id?: string;
  statement_text_ar: string;
  locator_ar?: string;
  review_status?: "unreviewed" | "accepted" | "rejected" | "needs_review";
}

export interface CreateClaimInput {
  subject_type: string;
  subject_id: string;
  predicate: string;
  object_type: string;
  object_id: string;
  time_from?: string;
  time_to?: string;
  place_id?: string;
  status?: string;
  notes_ar?: string;
}

export interface AddEvidenceInput {
  source_statement_id?: string;
  source_passage_id?: string;
  evidence_note_ar?: string;
  relation: "supports" | "contextualizes" | "contradicts" | "refutes";
}

export interface OpenQuestionRecord {
  id: string;
  titleAr: string;
  descriptionAr?: string;
  status: "open" | "under_investigation" | "resolved" | "reopened" | "archived";
  priority: "low" | "normal" | "high";
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  claimCount: number;
  sourceCount: number;
  disputeCount: number;
  noteCount: number;
}

export interface QuestionClaimLink {
  claimId: string;
  role: "concerns" | "supports" | "opposes";
  subjectType: string;
  subjectId: string;
  predicate: string;
  objectType: string;
  objectId: string;
  status: string;
}

export interface QuestionSourceLink {
  sourceId: string;
  role: "context" | "supporting" | "counter_evidence" | "missing";
  titleAr: string;
}

export interface QuestionDisputeLink {
  disputeId: string;
  titleAr: string;
  status: string;
}

export interface QuestionEntityLink {
  entityType: string;
  entityId: string;
  nameAr: string;
}

export interface QuestionFindingLink {
  findingId: string;
  findingType: string;
  titleAr: string;
  status: string;
}

export interface QuestionNote {
  id: string;
  noteAr: string;
  createdBy: string;
  createdAt: string;
}

export interface QuestionActivity {
  action: string;
  entityType: string;
  entityId: string;
  createdAt: string;
  after?: unknown;
}

export interface QuestionDetail {
  question: OpenQuestionRecord;
  claims: QuestionClaimLink[];
  sources: QuestionSourceLink[];
  disputes: QuestionDisputeLink[];
  entities: QuestionEntityLink[];
  findings: QuestionFindingLink[];
  notes: QuestionNote[];
  activity: QuestionActivity[];
}

export interface DisputeClaimLink {
  claimId: string;
  position: "concerns" | "supports" | "opposes" | "mentions";
  subjectType: string;
  subjectId: string;
  predicate: string;
  objectType: string;
  objectId: string;
  status: string;
}

export interface DisputeRecord {
  id: string;
  titleAr: string;
  descriptionAr?: string;
  status: "open" | "under_review" | "resolved" | "reopened" | "archived";
  resolutionAr?: string;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  claimCount: number;
}

export interface DisputeDetail {
  dispute: DisputeRecord;
  claims: DisputeClaimLink[];
}

export interface CreateQuestionInput {
  title_ar: string;
  description_ar?: string;
  status?: OpenQuestionRecord["status"];
  priority?: OpenQuestionRecord["priority"];
}

export interface UpdateQuestionInput {
  title_ar?: string;
  description_ar?: string;
  status?: OpenQuestionRecord["status"];
  priority?: OpenQuestionRecord["priority"];
}

export interface QuestionNoteInput {
  note_ar: string;
}

export interface QuestionClaimInput {
  claim_id: string;
  role: QuestionClaimLink["role"];
}

export interface QuestionSourceInput {
  source_id: string;
  role: QuestionSourceLink["role"];
}

export interface QuestionDisputeInput {
  dispute_id: string;
}

export interface QuestionEntityInput {
  entity_type: "person" | "family" | "branch" | "tribe" | "place";
  entity_id: string;
}

export interface QuestionFindingInput {
  finding_id: string;
}

export interface CreateDisputeInput {
  title_ar: string;
  description_ar?: string;
  status?: DisputeRecord["status"];
}

export interface UpdateDisputeInput {
  status?: DisputeRecord["status"];
  resolution_ar?: string;
}

export interface DisputeClaimInput {
  claim_id: string;
  position: "concerns" | "supports" | "opposes" | "mentions";
}

export interface SuggestionReview {
  id: string;
  reviewerId: string;
  reviewerName: string;
  decision: "accepted" | "rejected" | "converted";
  noteAr?: string;
  createdAt: string;
}

export interface SuggestionRecord {
  id: string;
  treeId: string;
  versionId: string;
  versionNumber: number;
  nodeId: string;
  treeName: string;
  nodeName: string;
  textAr: string;
  status: "pending" | "accepted" | "rejected" | "converted";
  questionId?: string;
  createdAt: string;
  updatedAt: string;
  reviewCount: number;
  reviews: SuggestionReview[];
}

export interface SubmitSuggestionInput {
  tree_id: string;
  version_id: string;
  node_id: string;
  text_ar: string;
}

export interface ReviewSuggestionInput {
  decision: SuggestionReview["decision"];
  note_ar?: string;
  question_title_ar?: string;
}

export type DictionaryKind = "families" | "tribes" | "branches" | "people" | "places" | "sources" | "questions" | "disputed-claims";

export interface DictionaryIndexItem {
  id: string;
  kind: string;
  nameAr: string;
  secondaryAr?: string;
  status?: string;
  count: number;
}

export interface DictionaryIndexResponse {
  kind: DictionaryKind;
  query: string;
  items: DictionaryIndexItem[];
}

export interface DictionaryAlias {
  valueAr: string;
  type: string;
}

export interface DictionaryReference {
  id: string;
  nameAr: string;
  detailAr?: string;
  status?: string;
  sourceType?: string;
}

export interface DictionaryTreeReference {
  id: string;
  nameAr: string;
  versionId: string;
  versionNumber: number;
}

export interface DictionaryClaimReference {
  id: string;
  subjectType: string;
  subjectId: string;
  predicate: string;
  objectType: string;
  objectId: string;
  status: string;
  evidenceCount: number;
}

export interface DictionaryQuestionReference {
  id: string;
  titleAr: string;
  status: string;
  priority: string;
  noteCount: number;
}

export interface DictionaryDetail {
  kind: "family" | "tribe" | "person" | "place";
  id: string;
  nameAr: string;
  descriptionAr?: string;
  aliases: DictionaryAlias[];
  historicalNames: DictionaryReference[];
  branches: DictionaryReference[];
  places: DictionaryReference[];
  families: DictionaryReference[];
  people: DictionaryReference[];
  tribes: DictionaryReference[];
  publishedTrees: DictionaryTreeReference[];
  claims: DictionaryClaimReference[];
  sources: DictionaryReference[];
  questions: DictionaryQuestionReference[];
  migrations: DictionaryReference[];
}

export interface MapFeature {
  id: string;
  kind: "place" | "association" | "migration" | "region";
  placeId?: string;
  placeName?: string;
  entityType?: string;
  entityId?: string;
  entityName?: string;
  relationType?: string;
  status: "documented" | "interpreted" | "platform_inferred" | "disputed" | "unresolved";
  certainty?: string;
  timeFrom?: string;
  timeTo?: string;
  sourceId?: string;
  sourceTitle?: string;
  evidenceId?: string;
  evidenceText?: string;
  evidenceStatus?: string;
  longitude?: number;
  latitude?: number;
  fromLongitude?: number;
  fromLatitude?: number;
  toLongitude?: number;
  toLatitude?: number;
}

export interface SearchResult {
  id: string;
  kind: string;
  title: string;
  subtitle?: string;
  body?: string;
  status?: string;
  score: number;
  route?: string;
  sourceId?: string;
}

export interface SearchGroup {
  key: string;
  label: string;
  items: SearchResult[];
}

export interface SearchResponse {
  query: string;
  normalizedQuery: string;
  groups: SearchGroup[];
  total: number;
}

export interface MapResponse {
  fromYear?: number;
  toYear?: number;
  status?: string;
  features: MapFeature[];
}

export interface JobView {
  id: string;
  type: string;
  payload: unknown;
  status: "queued" | "running" | "succeeded" | "failed" | "dead";
  priority: number;
  attempts: number;
  maxAttempts: number;
  runAt: string;
  lockedAt?: string;
  lockedBy?: string;
  lastError?: string;
  idempotencyKey?: string;
  createdAt: string;
  updatedAt: string;
}

export interface EnqueueJobInput {
  type: string;
  payload: unknown;
  priority?: number;
  run_at?: string;
  idempotency_key?: string;
  max_attempts?: number;
}

export interface TreeSummary {
  id: string;
  name: string;
  description: string;
  visibility: "private" | "unlisted" | "public";
  ownerId: string;
  parentTreeId?: string;
  parentVersionId?: string;
  forkedBy?: string;
  forkedAt?: string;
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

export type TreePermissionLevel = "owner" | "view" | "edit" | "review";

export interface TreePermissions {
  canEdit: boolean;
  canPublish: boolean;
  canManageCollaborators: boolean;
  permissionLevel: TreePermissionLevel | "";
}

export interface TreeDetail {
  tree: TreeSummary;
  selectedVersion: TreeVersionRecord;
  permissions: TreePermissions;
  versions: TreeVersionRecord[];
  nodes: TreeNodeRecord[];
  relationships: TreeRelationshipRecord[];
}

export interface ForkTreeInput {
  version_id: string;
  name_ar?: string;
  description_ar?: string;
  visibility: "private" | "unlisted" | "public";
}

export interface TreeDiffEndpoint {
  treeId: string;
  treeName: string;
  versionId: string;
  versionNumber: number;
}

export interface TreeDiffPerson {
  personId: string;
  displayName: string;
  years: string;
}

export interface TreeDiffDateChange {
  personId: string;
  displayName: string;
  beforeYears: string;
  afterYears: string;
}

export interface TreeDiffRelationship {
  subject: string;
  object: string;
  predicate: string;
  status: RelationshipStatus;
  sourceId?: string;
}

export interface TreeDiffRelationshipChange {
  subject: string;
  object: string;
  predicate: string;
  beforeStatus: RelationshipStatus;
  afterStatus: RelationshipStatus;
  beforeSourceId?: string;
  afterSourceId?: string;
}

export interface TreeDiffSourceChange {
  sourceId: string;
  relationshipLabel: string;
}

export interface TreeDiff {
  from: TreeDiffEndpoint;
  to: TreeDiffEndpoint;
  peopleAdded: TreeDiffPerson[];
  peopleRemoved: TreeDiffPerson[];
  dateChanges: TreeDiffDateChange[];
  relationshipsAdded: TreeDiffRelationship[];
  relationshipsRemoved: TreeDiffRelationship[];
  relationshipChanges: TreeDiffRelationshipChange[];
  sourcesAdded: TreeDiffSourceChange[];
  sourcesRemoved: TreeDiffSourceChange[];
  affectedDescendants: number;
}

export interface Invitee {
  userId: string;
  displayName: string;
  email?: string;
  permissionLevel: TreePermissionLevel | "view" | "edit" | "review";
  invitedBy?: string;
  createdAt: string;
}

export interface TreeInvitation {
  id: string;
  treeId: string;
  treeName: string;
  inviteeUserId?: string;
  inviteeEmail?: string;
  permissionLevel: "view" | "edit" | "review";
  status: "pending" | "accepted" | "revoked" | "expired";
  expiresAt: string;
  acceptedAt?: string;
  createdAt: string;
}

export interface CollaboratorsResponse {
  owner: Invitee;
  collaborators: Invitee[];
  invitations: TreeInvitation[];
  canManage: boolean;
}

export interface TreeActivity {
  id: string;
  action: string;
  entityType: string;
  entityId: string;
  actorId?: string;
  actorName?: string;
  before?: unknown;
  after?: unknown;
  reasonAr?: string;
  createdAt: string;
}

export interface InvitationCreated {
  invitation: TreeInvitation;
  acceptToken: string;
  acceptPath: string;
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

export interface UpdatePermissionInput {
  permission_level: "view" | "edit" | "review";
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
