# ARCHITECTURE.md

# Arabic Lineage Intelligence Platform

## 1. Purpose

This document defines the technical architecture for the Arabic Lineage Intelligence Platform described in `PRODUCT.md`.

The system is designed to support:

- public family-tree browsing
- collaborative tree editing
- tree forking and versioning
- family, tribe, branch, person, place, and source indexes
- claim and evidence tracking
- disputed and unresolved lineage research
- historical mapping and migration analysis
- hybrid lexical and semantic retrieval
- AI-assisted Arabic source processing
- entity resolution
- contradiction detection
- statistical and temporal analysis
- GraphRAG and research-agent workflows
- low-cost semantic routing using a System One model such as Jev

The architecture deliberately keeps the application layer simple and concentrates complexity in the research, graph, source-processing, and AI systems.

---

## 2. Architectural Principles

### 2.1 Start simple

The first version should not introduce separate databases or infrastructure merely because they may become useful later.

Initial data capabilities should be consolidated around PostgreSQL where practical.

### 2.2 Domain complexity over framework complexity

The hard problem is not HTTP routing.

The hard problems are:

- lineage modeling
- provenance
- uncertainty
- conflicting claims
- tree versioning
- source interpretation
- identity resolution
- historical geography
- AI-assisted research

The core backend should therefore remain framework-light.

### 2.3 Preserve epistemic separation

The architecture must keep separate:

- source records
- source statements
- research claims
- accepted tree interpretations
- platform findings
- open questions

No service may silently promote AI output into accepted historical knowledge.

### 2.4 AI assists, humans govern publication

AI may:

- extract
- classify
- retrieve
- compare
- score
- detect anomalies
- generate hypotheses
- recommend investigation

AI must not independently publish genealogical conclusions as established truth.

### 2.5 Postgres first

Use PostgreSQL as the initial main data platform.

Extensions:

- PostGIS for geospatial data
- pgvector for embeddings and semantic retrieval

Use PostgreSQL full-text search where sufficient.

### 2.6 Add specialized systems only when justified

Introduce systems such as:

- Neo4j
- OpenSearch
- durable workflow engines
- specialized vector stores

only when workload characteristics clearly justify them.

---

# 3. High-Level Architecture

```text
                         Public Web Application
                         React + Vite + TanStack
                                  |
                                  v
                         ---------------------
                         |     Core API      |
                         |    Go net/http    |
                         ---------------------
                          |       |        |
                          |       |        |
                          v       v        v
                    PostgreSQL        Object Storage
                    + PostGIS             S3 / R2
                    + pgvector
                    + jobs
                          |
                          |
                          v
                -----------------------
                | AI / Research API   |
                | Python + FastAPI    |
                -----------------------
                  |       |       |
                  |       |       |
                  v       v       v
               Models   Jev    Workers
                  |              |
                  |              v
                  |       Source Processing
                  |
                  v
          RAG / GraphRAG / Analysis

Future:
PostgreSQL knowledge model
        |
        v
Neo4j for graph-heavy workloads when justified
```

---

# 4. Technology Stack

## 4.1 Frontend

Recommended:

- React
- Vite
- TanStack Router
- TanStack Query
- Zustand only where local/global client state requires it
- Tailwind CSS
- component library such as shadcn/ui or equivalent
- MapLibre GL for maps

Responsibilities:

- public tree browsing
- tree editing
- dictionary
- indexes
- map explorer
- research workspace
- source viewer
- claims and disputes UI
- collaboration
- AI research interfaces

The frontend must not contain research-authority logic.

---

## 4.2 Core Backend

Language:

- Go

HTTP layer:

- `net/http`
- standard `ServeMux`

Supporting libraries:

- `pgx`
- `sqlc`
- standard `slog`
- OpenTelemetry
- validation library only where useful

The backend should avoid a heavy Go web framework initially.

### Why `net/http`

Modern Go routing already supports:

```text
GET /trees/{treeID}
POST /trees/{treeID}/claims
GET /people/{personID}
```

The application benefits more from clear domain boundaries than framework abstractions.

If routing ergonomics later become painful, Chi can be introduced without changing the core application architecture.

---

## 4.3 AI / Research Backend

Language:

- Python

Framework:

- FastAPI

Responsibilities:

- Arabic NLP
- embeddings
- reranking
- entity extraction
- claim extraction
- entity resolution
- contradiction analysis
- temporal reasoning
- statistical analysis
- GraphRAG
- research-agent workflows
- geospatial inference
- source dependency analysis
- semantic decision models such as Jev

This service should not own core product truth.

It produces:

- suggestions
- findings
- scores
- hypotheses
- classifications
- evidence packages

The Go backend remains the authority for persisted product state and publication workflows.

---

# 5. Core Backend Structure

Recommended modular monolith initially.

```text
/core
  /cmd
    /api

  /internal
    /auth
    /users
    /trees
    /people
    /families
    /tribes
    /branches
    /places
    /sources
    /claims
    /questions
    /collaboration
    /forks
    /versions
    /suggestions
    /search
    /research
    /moderation
    /notifications

  /platform
    /db
    /jobs
    /storage
    /telemetry
```

Each domain module should expose application-level operations.

Avoid:

```text
handler -> SQL directly
```

Prefer:

```text
HTTP handler
    ->
application service
    ->
domain logic
    ->
repository
    ->
database
```

---

# 6. Core Domain Boundaries

## 6.1 Identity Domain

Handles:

- Person
- Family
- Tribe
- Branch
- Place

Responsibilities:

- entity lifecycle
- aliases
- normalization
- identity relationships
- person merges
- branch structure

---

## 6.2 Tree Domain

Handles:

- Tree
- TreeNode
- TreeRelationship
- TreeVersion
- Fork
- Merge

A tree is a published interpretation.

It is not the same object as the global research graph.

---

## 6.3 Evidence Domain

Handles:

- Source
- SourcePassage
- SourceStatement
- Claim
- EvidenceLink
- CounterEvidence

This is the foundation for provenance.

---

## 6.4 Research Domain

Handles:

- Interpretation
- PlatformFinding
- OpenQuestion
- Investigation
- ResearchNote

This domain must preserve unresolved states.

---

## 6.5 Collaboration Domain

Handles:

- TreeCollaborator
- Role
- Permission
- Suggestion
- Review
- Invitation

---

## 6.6 Geography Domain

Handles:

- Place
- HistoricalPlaceName
- GeographicAssociation
- MigrationEvent
- HistoricalRegion
- SpatialEvidence

---

# 7. Persistence

## 7.1 PostgreSQL

PostgreSQL is the system of record.

Primary workloads:

- users
- permissions
- trees
- tree versions
- people
- families
- branches
- claims
- sources
- questions
- collaboration
- moderation
- platform findings metadata

---

## 7.2 PostGIS

Use for:

- coordinates
- place polygons
- approximate historical regions
- migration routes
- spatial containment
- distance queries
- spatial clustering inputs
- geographic filtering

Example relationships:

```text
family -> documented_in -> place
branch -> migrated_from -> place
branch -> migrated_to -> place
person -> lived_in -> place
source -> refers_to -> place
```

Historical geographic data should include:

- time range
- source
- status
- certainty type

---

## 7.3 pgvector

Use for:

- source passage embeddings
- claim embeddings
- person-description embeddings
- family-description embeddings
- research-question embeddings
- semantic similarity
- hybrid retrieval

Do not introduce a separate vector database initially.

---

# 8. Search Architecture

Initial search:

```text
User Query
    |
    +--> PostgreSQL Full-Text Search
    |
    +--> pgvector Semantic Search
    |
    +--> Structured Filters
           |
           +-- person
           +-- family
           +-- tribe
           +-- place
           +-- date
           +-- source
           +-- claim status
```

Results are merged and reranked.

Later:

```text
Lexical retrieval
+
Vector retrieval
+
Graph retrieval
+
Temporal filtering
+
Geospatial filtering
=
Hybrid lineage retrieval
```

OpenSearch should only be added if:

- Arabic full-text requirements exceed PostgreSQL capabilities
- indexing volume becomes large
- ranking complexity requires it
- operational benefit justifies another system

---

# 9. Knowledge Graph Strategy

## 9.1 V1

Represent graph relationships in PostgreSQL.

Typical tables:

```text
entities
entity_aliases
relationships
claims
claim_evidence
source_statements
places
geographic_associations
open_questions
question_links
platform_findings
```

Relationships should be typed.

Examples:

```text
PERSON --father_of--> PERSON
PERSON --member_of--> FAMILY
FAMILY --branch_of--> TRIBE
FAMILY --documented_in--> PLACE
BRANCH --migrated_to--> PLACE
CLAIM --supported_by--> SOURCE_STATEMENT
CLAIM --disputed_by--> CLAIM
QUESTION --concerns--> CLAIM
```

---

## 9.2 Why not Neo4j immediately

The first product stage benefits from:

- transactions
- relational constraints
- versioning
- permissions
- auditability
- spatial queries
- vector queries

PostgreSQL already handles all of those.

Adding Neo4j immediately creates:

- data duplication
- synchronization problems
- deployment complexity
- consistency concerns

without yet proving graph workload requirements.

---

## 9.3 Neo4j introduction point

Introduce Neo4j when we have substantial need for:

- deep multi-hop traversal
- graph algorithms
- community detection
- large relationship networks
- graph-native research exploration
- source dependency networks
- large-scale GraphRAG

At that point:

```text
PostgreSQL
    |
    | authoritative product data
    |
    +--> Graph projection / sync
            |
            v
          Neo4j
```

PostgreSQL should remain authoritative initially.

Neo4j should be treated as a derived research projection unless a future architecture decision changes that.

---

# 10. AI Architecture

```text
                          Source / User Input
                                  |
                                  v
                      Deterministic Preprocessing
                                  |
                                  v
                         Semantic Control Layer
                              Jev / rules
                                  |
                  +---------------+---------------+
                  |               |               |
                  v               v               v
              Ignore          Cheap Path       Deep Path
                                  |               |
                                  v               v
                          Specialized Model    Reasoning LLM
                                  |               |
                                  +-------+-------+
                                          |
                                          v
                                  Platform Finding
                                          |
                                          v
                                      Human Review
```

---

# 11. Semantic Control Layer

A System One model such as Jev may be used for cheap semantic decisions.

Examples:

```text
relevant / irrelevant
same person candidate / different
potential contradiction / normal
simple query / research query
vector search / graph search / hybrid
needs review / routine
continue investigation / stop
high-impact change / low-impact change
```

Jev must not determine genealogical truth.

Jev scores are operational model scores, not historical confidence values.

---

# 12. AI Source Processing Pipeline

```text
Uploaded Source
      |
      v
Object Storage
      |
      v
Document Processing Worker
      |
      +--> OCR / text extraction
      |
      +--> page segmentation
      |
      +--> metadata extraction
      |
      v
Arabic Information Extraction
      |
      +--> persons
      +--> families
      +--> tribes
      +--> places
      +--> dates
      +--> relations
      +--> source references
      |
      v
Claim Extraction
      |
      v
Entity Resolution
      |
      v
Embedding
      |
      v
Search Index
      |
      v
Human Review Queue
```

AI-extracted facts are never automatically accepted as published research claims.

---

# 13. RAG Architecture

## 13.1 Hybrid RAG

```text
Question
   |
   v
Query Classification
   |
   +--> lexical retrieval
   +--> vector retrieval
   +--> structured query
   +--> graph retrieval
   +--> temporal filtering
   +--> geographic filtering
   |
   v
Candidate Evidence
   |
   v
Re-ranking
   |
   v
Evidence Package
   |
   v
Reasoning Model
   |
   v
Source-grounded Research Response
```

---

## 13.2 RAG response contract

Every research answer should distinguish:

- documented source statements
- accepted tree interpretation
- competing claims
- platform findings
- unresolved questions

The model must be able to answer:

> The available sources disagree.

without forcing resolution.

---

# 14. GraphRAG

GraphRAG becomes more important once relationships become dense.

Example:

```text
Question:
"What evidence connects Family X to Branch Y?"

Retrieval:
- relevant source passages
- family nodes
- branch nodes
- ancestor paths
- geographic overlap
- conflicting claims
- source dependencies
```

The graph supplements textual retrieval.

It does not replace source evidence.

---

# 15. Entity Resolution

Entity resolution must be treated as a dedicated subsystem.

Signals:

- normalized Arabic name
- father name
- grandfather name
- branch
- family
- place
- birth/death range
- siblings
- children
- source context
- titles
- kunyah
- nisbah

Pipeline:

```text
candidate generation
    ->
cheap similarity
    ->
semantic classification
    ->
deep comparison
    ->
human merge review
```

No automatic destructive merging.

Merges must be:

- reviewable
- auditable
- reversible

---

# 16. Contradiction Detection

Contradiction analysis should combine:

### Deterministic checks

Examples:

- impossible dates
- duplicated parent relationships
- impossible event ordering

### Statistical checks

Examples:

- unusual generation intervals
- unusual migration sequence
- outlier age patterns

### Semantic checks

Examples:

- conflicting statements
- probable duplicate identities
- contradictory source descriptions

Output:

```text
PlatformFinding
type = possible_contradiction
status = needs_review
```

not:

```text
Claim = false
```

---

# 17. Mapping Architecture

Frontend:

- MapLibre GL

Backend:

- Go API
- PostGIS

Data model:

```text
Place
HistoricalPlaceName
GeographicAssociation
MigrationEvent
HistoricalRegion
SpatialEvidence
```

Map layers:

- documented presence
- interpreted presence
- platform-inferred presence
- disputed presence
- migrations
- source locations
- open questions

All geographic claims should support:

- time range
- provenance
- claim status
- interpretation status

---

# 18. Tree Versioning

Trees should use explicit versions.

```text
Tree
  |
  +-- Draft Version
  |
  +-- Published Version 1
  |
  +-- Published Version 2
```

Each version should preserve:

- changed nodes
- changed relationships
- changed sources
- editor
- timestamp
- reason
- publication note

Tree versions should be immutable once published.

New changes create new versions.

---

# 19. Forking Architecture

```text
Tree A v3
   |
   +--> Fork Tree B
             |
             +--> independent edits
             +--> own publication history
             +--> comparison against upstream
```

Fork metadata:

```text
parent_tree_id
parent_version_id
forked_by
forked_at
```

Tree diffing should compare semantic domain changes rather than raw database rows.

---

# 20. Suggestions

Public suggestion workflow:

```text
Public User
    |
    v
Arabic Plain-Text Suggestion
    |
    v
Semantic Classification
    |
    v
Structured Suggestion Candidate
    |
    v
Collaborator Review
    |
    +--> accept
    +--> reject
    +--> request clarification
    +--> create open question
```

The original text must always be preserved.

---

# 21. Async Jobs

Use async processing for:

- source ingestion
- OCR
- embeddings
- entity extraction
- claim extraction
- indexing
- contradiction scans
- map enrichment
- large tree diffing
- research-agent investigations

Initial approach:

- PostgreSQL-backed job queue
- worker polling with `FOR UPDATE SKIP LOCKED`
- PostgreSQL advisory locks where coordination is required

Suggested job model:

```text
jobs
- id
- type
- payload
- status
- attempts
- run_at
- locked_at
- locked_by
- last_error
- created_at
- updated_at
```

Workers should claim small batches atomically and use retry/backoff policies.

Do not introduce Redis, Kafka, or another queueing system in V1 unless workload evidence justifies it.

---

# 22. Durable Workflows

If future research pipelines become:

- long-running
- multi-step
- failure-sensitive
- externally dependent
- retry-heavy
- human-in-the-loop

then consider a durable workflow engine such as Temporal.

Example future workflow:

```text
source upload
  ->
OCR
  ->
extract entities
  ->
extract claims
  ->
resolve places
  ->
resolve identities
  ->
generate embeddings
  ->
detect conflicts
  ->
await human review
```

A PostgreSQL-backed queue is sufficient until this complexity appears.

If queue throughput or low-latency ephemeral workloads later become substantial, Redis may be introduced as an optimization. If workflows become long-running, multi-step, failure-sensitive, or human-in-the-loop, prefer a durable workflow engine such as Temporal rather than stretching a simple queue beyond its purpose.

---

# 23. Object Storage

Use S3-compatible object storage.

Candidates:

- Cloudflare R2
- AWS S3
- equivalent compatible provider

Store:

- PDFs
- manuscripts
- scans
- OCR artifacts
- extracted text
- thumbnails
- source images
- generated reports

Metadata belongs in PostgreSQL.

Binary files belong in object storage.

---

# 24. Caching and Ephemeral State

Redis is not required in V1.

Initial approach:

- HTTP/browser caching where appropriate
- CDN caching for public static and cacheable responses
- small in-process caches for safe, non-authoritative data
- signed secure cookies or PostgreSQL-backed sessions if server-side sessions are required
- in-process rate limiting for a single API instance, with a PostgreSQL-backed strategy if coordination becomes necessary
- PostgreSQL advisory locks for limited cross-process coordination

Do not cache authoritative research state in a way that risks stale publication decisions.

Introduce Redis later only if there is a concrete need for:

- distributed rate limiting across many API instances
- very high-throughput short-lived jobs
- low-latency ephemeral state
- pub/sub or realtime fan-out
- aggressive shared caching
- queue workloads that have outgrown PostgreSQL

---

# 25. API Style

Use REST initially.

Example resources:

```text
GET    /api/v1/trees/{treeID}
POST   /api/v1/trees
POST   /api/v1/trees/{treeID}/people
POST   /api/v1/trees/{treeID}/relationships
PATCH  /api/v1/trees/{treeID}/relationships/{relationshipID}
POST   /api/v1/trees/{treeID}/publish
POST   /api/v1/trees/{treeID}/fork
GET    /api/v1/trees/{treeID}/versions
GET    /api/v1/trees/{treeID}/versions/{versionID}

GET    /api/v1/people/{personID}
GET    /api/v1/families/{familyID}
GET    /api/v1/tribes/{tribeID}

GET    /api/v1/claims/{claimID}
POST   /api/v1/claims

GET    /api/v1/questions/{questionID}
POST   /api/v1/questions

POST   /api/v1/suggestions
POST   /api/v1/sources
```

Avoid premature GraphQL unless product query patterns justify it later.

---

# 26. Internal Service Communication

Initial deployment should minimize internal service count.

Recommended:

```text
Web
 |
 v
Go API
 |
 +--> PostgreSQL
 |      +--> product data
 |      +--> PostGIS
 |      +--> pgvector
 |      +--> job queue
 +--> Object Storage
 +--> Python AI API
```

Use HTTP/JSON between Go and Python initially.

Introduce gRPC only if:

- throughput requirements justify it
- schema sharing becomes valuable
- streaming requirements become substantial

---

# 27. Authentication and Authorization

Recommended:

- standard OIDC-compatible identity provider or application-managed auth
- signed secure sessions or JWT where appropriate

Authorization must support:

```text
public visitor
registered user
tree owner
collaborator
researcher
moderator
admin
```

Permissions should be resource-scoped.

Example:

```text
tree:read
tree:edit
tree:publish
tree:invite
claim:create
claim:review
source:manage
question:resolve
moderation:review
```

---

# 28. Privacy Architecture

Living-person information requires special handling.

Possible controls:

- public
- collaborators only
- tree owner only
- hidden

Sensitive fields may require stricter visibility than the tree itself.

The API must enforce these rules server-side.

---

# 29. Audit Log

High-impact operations must generate immutable audit events.

Examples:

- person merge
- relationship change
- claim acceptance
- claim rejection
- source removal
- tree publication
- collaborator permission change
- open-question resolution

Audit record:

```text
actor
action
entity
before
after
timestamp
reason
request_id
```

---

# 30. Observability

Use:

- structured logging with `slog`
- OpenTelemetry
- traces
- metrics
- error tracking

Key metrics:

```text
API latency
DB latency
search latency
AI inference latency
AI cost
queue depth
source processing failures
embedding failures
entity-resolution review rate
contradiction precision
RAG retrieval quality
```

Trace IDs should flow from frontend request through:

```text
Go API
 ->
Python AI
 ->
model provider
 ->
database
```

where practical.

---

# 31. AI Cost Control

Use multiple execution tiers.

```text
Tier 0
Deterministic code

Tier 1
Jev / lightweight classifiers

Tier 2
Embeddings / rerankers / specialized models

Tier 3
General LLM

Tier 4
Research agent / deep reasoning
```

The system should escalate only when needed.

This is especially important for:

- public search
- suggestion processing
- source ingestion
- duplicate detection
- research-agent workflows

---

# 32. Security

Requirements:

- strict authorization
- input validation
- parameterized SQL
- object-storage access control
- malware scanning for uploaded documents
- rate limiting
- abuse protection
- audit logging
- secrets management
- encrypted transport
- encrypted storage where available

AI-specific:

- treat uploaded sources as untrusted content
- protect research agents from prompt injection
- separate source content from system instructions
- limit tools available to agents
- validate structured model outputs
- never execute generated code from source material

---

# 33. Deployment

Recommended initial deployment shape:

```text
Frontend
  -> Vercel / Cloudflare Pages / equivalent

Go API
  -> container platform

Python AI API
  -> container platform

Workers
  -> container platform

PostgreSQL
  -> managed PostgreSQL

Object Storage
  -> R2 / S3

CDN
  -> provider edge network
```

Do not require Kubernetes initially.

Containers are sufficient.

---

# 34. Repository Strategy

Recommended monorepo:

```text
/
├── apps/
│   └── web/
│
├── services/
│   ├── core-api/
│   └── ai-research/
│
├── workers/
│   └── source-processing/
│
├── packages/
│   ├── contracts/
│   ├── ui/
│   └── config/
│
├── db/
│   ├── migrations/
│   └── seeds/
│
├── docs/
│   ├── PRODUCT.md
│   └── ARCHITECTURE.md
│
└── infra/
```

The Go and Python services should remain independently buildable.

---

# 35. Initial Data Flow

## Public tree browsing

```text
Browser
  ->
Go API
  ->
PostgreSQL
  ->
JSON
  ->
React tree visualization
```

---

## Source ingestion

```text
Researcher
  ->
Go API
  ->
Object Storage
  ->
Job Queue
  ->
Python Worker
  ->
OCR / extraction
  ->
PostgreSQL
  ->
Review Queue
```

---

## Research question

```text
Browser
  ->
Go API
  ->
AI Research API
  ->
Query classification
  ->
Hybrid retrieval
  ->
Graph / temporal / geographic analysis
  ->
Reasoning model
  ->
Evidence-grounded response
  ->
Go API
  ->
Browser
```

---

# 36. V1 Architecture

V1 should contain:

```text
React + Vite
TanStack Router
TanStack Query

Go
net/http
pgx
sqlc

PostgreSQL
PostGIS
pgvector

Python
FastAPI

PostgreSQL-backed jobs

S3-compatible storage

MapLibre
```

AI V1:

- embeddings
- hybrid retrieval
- Arabic normalization
- suggestion classification
- basic entity resolution
- basic contradiction detection
- source-grounded Q&A

No requirement yet for:

- Neo4j
- Kafka
- Kubernetes
- OpenSearch
- dedicated vector database
- complex multi-agent architecture

---

# 37. Phase 2 Architecture Evolution

Add when justified:

- GraphRAG
- stronger entity resolution
- source-dependency analysis
- temporal models
- migration inference
- richer research workspace
- Jev semantic routing
- more sophisticated workers
- durable workflows if needed

---

# 38. Phase 3 Architecture Evolution

Potential:

```text
PostgreSQL
    |
    +--> Neo4j graph projection

Search
    |
    +--> OpenSearch if needed

Ephemeral infrastructure
    |
    +--> Redis if shared caching, distributed rate limiting,
         pub/sub, or queue throughput justifies it

Async
    |
    +--> Temporal if needed

AI
    |
    +--> domain-specific Arabic models
    +--> specialized entity resolution
    +--> research-agent orchestration
```

These are evolution paths, not mandatory components.

---

# 39. Architectural Decisions

## ADR-001: PostgreSQL is the initial system of record

Reason:

- transactional integrity
- versioning
- relational domain model
- PostGIS
- pgvector
- operational simplicity

---

## ADR-002: Use PostGIS for historical geography

Reason:

Mapping is a core product capability and requires proper spatial modeling.

---

## ADR-003: Use pgvector initially

Reason:

Avoid introducing a separate vector database until scale requires one.

---

## ADR-004: Use Go `net/http` for the core API

Reason:

The domain is complex enough without adding framework complexity.

---

## ADR-005: Use Python/FastAPI for research intelligence

Reason:

Python has the strongest ecosystem for NLP, ML, statistics, graph tooling, and AI experimentation.

---

## ADR-006: Do not introduce Neo4j in V1

Reason:

Graph workloads should first prove that they require a dedicated graph database.

---

## ADR-007: AI findings are not accepted facts

Reason:

The product must maintain provenance and uncertainty.

---

## ADR-008: No DNA or genetic genealogy subsystem

Reason:

DNA-based lineage research is explicitly outside product scope.

---

## ADR-009: Keep services coarse-grained initially

Reason:

Premature microservices would create operational overhead without improving product capability.

---

# 40. Target Architecture Summary

```text
                                 USERS
                                   |
                                   v
                         React + Vite + TanStack
                                   |
                                   v
                    +-----------------------------+
                    |        Go Core API          |
                    |          net/http           |
                    +-----------------------------+
                       |         |          |
                       |         |          |
                       v         v          v
                 PostgreSQL          Object Storage
                  |   |   |
                  |   |   +--> pgvector
                  |   +------> PostGIS
                  |   +------> job queue
                  |
                  +--> Product + Research State
                       |
                       v
                +-----------------------+
                | Python AI / Research  |
                |       FastAPI         |
                +-----------------------+
                  |       |       |
                  v       v       v
                Jev   Specialized   LLM
                      Models
                  \       |       /
                   \      |      /
                    v     v     v
                      Findings
                         |
                         v
                     Human Review

Future:
PostgreSQL -> Neo4j graph projection
```

The architecture should remain simple enough that the team can understand the complete system, while leaving clear extension points for graph-native research, advanced retrieval, and AI-assisted lineage analysis as the product grows.
