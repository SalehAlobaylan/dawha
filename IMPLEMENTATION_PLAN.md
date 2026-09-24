# IMPLEMENTATION_PLAN.md

# Arabic Lineage Intelligence Platform

## 1. Purpose

This document defines the implementation plan for the Arabic Lineage Intelligence Platform described in:

- `PRODUCT.md`
- `ARCHITECTURE.md`

The goal is to deliver the product in a sequence that validates the most important domain assumptions early:

1. lineage and identity modeling
2. source and evidence modeling
3. collaborative tree building
4. disputes and open questions
5. mapping and historical geography
6. retrieval and AI assistance
7. advanced research intelligence

The plan intentionally avoids introducing infrastructure before it is needed.

Initial constraints:

- no DNA or genetic genealogy
- PostgreSQL is the system of record
- PostGIS handles geography
- pgvector handles embeddings
- Go `net/http` powers the core API
- Python + FastAPI powers AI/research capabilities
- PostgreSQL-backed jobs handle async work
- object storage holds source files
- Neo4j, Redis, OpenSearch, Kafka, Kubernetes, and Temporal are not V1 requirements

---

# 2. Implementation Strategy

The project should be implemented in vertical slices rather than building every backend subsystem first.

Each major phase should produce a usable product capability.

Recommended sequence:

```text
Foundation
    ↓
Identity + Tree Model
    ↓
Versioning + Collaboration
    ↓
Sources + Claims + Evidence
    ↓
Disputes + Open Questions
    ↓
Dictionary + Indexes
    ↓
Mapping
    ↓
Search
    ↓
AI Retrieval
    ↓
Entity Resolution + Contradictions
    ↓
Research Workspace
    ↓
GraphRAG + Advanced Intelligence
```

The most important rule is:

> Do not build advanced AI before the claim/evidence/provenance model is stable.

Without a strong research model, AI will only automate ambiguity.

---

# 3. Repository Setup

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
│   ├── ARCHITECTURE.md
│   └── IMPLEMENTATION_PLAN.md
│
├── infra/
│   └── local/
│
├── docker-compose.yml
└── README.md
```

---

# 4. Phase 0: Engineering Foundation

## Goal

Create a development environment that supports fast iteration without infrastructure complexity.

## Backend

Create:

```text
services/core-api
```

Use:

- Go
- `net/http`
- `pgx`
- `sqlc`
- `slog`
- OpenTelemetry-ready structure

Initial packages:

```text
/internal/auth
/internal/users
/internal/health
/platform/db
/platform/jobs
/platform/storage
/platform/telemetry
```

## Frontend

Create:

```text
apps/web
```

Use:

- React
- Vite
- TanStack Router
- TanStack Query
- Tailwind CSS
- shared component system

## AI Service

Create:

```text
services/ai-research
```

Use:

- Python
- FastAPI
- Pydantic
- PostgreSQL client
- model-provider abstraction

Do not implement advanced AI yet.

## Database

Set up PostgreSQL with:

- PostGIS
- pgvector

Create migration tooling.

## Local development

Docker Compose should provide:

- PostgreSQL
- local S3-compatible storage if useful
- optional local AI dependencies

Do not add Redis.

## CI

Add:

- frontend lint/test/build
- Go format/vet/test
- Python lint/test
- migration validation

## Acceptance criteria

- web app starts locally
- Go API starts locally
- Python AI service starts locally
- PostgreSQL includes PostGIS and pgvector
- health checks work
- CI passes
- one command starts local environment

---

# 5. Phase 1: Authentication and Authorization

## Goal

Establish identity and permission boundaries before collaborative features.

## Implement

### Users

```text
users
user_profiles
```

### Roles

Initial capability model:

```text
public
registered
tree_owner
collaborator
researcher
moderator
admin
```

Prefer permission checks over hard-coded role branching.

Example permissions:

```text
tree.read
tree.create
tree.edit
tree.publish
tree.invite
claim.create
claim.review
source.create
source.review
question.manage
moderation.review
```

## Authentication

Choose either:

- application-managed authentication
- external OIDC provider

Do not build a custom identity system unless necessary.

## Authorization

Every protected handler must resolve:

```text
actor
resource
permission
```

## Acceptance criteria

- registration/login works
- protected routes work
- collaborator permissions can be represented
- authorization is enforced server-side
- permission tests exist

---

# 6. Phase 2: Identity Model

## Goal

Implement the core entities before building full trees.

## Entities

### Person

Initial fields:

```text
id
canonical_name_ar
gender_optional
birth_date_from
birth_date_to
death_date_from
death_date_to
notes
created_at
updated_at
```

Do not assume dates are exact.

### Person aliases

```text
person_aliases
```

Support:

- alternative names
- kunyah
- laqab
- nisbah
- source spelling

### Family

```text
families
family_aliases
```

### Tribe

```text
tribes
tribe_aliases
```

### Branch

```text
branches
```

### Place

```text
places
historical_place_names
```

Use PostGIS geometry.

## Relationship model

Implement generic typed relationships carefully.

Candidate structure:

```text
entity_relationships
- id
- subject_type
- subject_id
- predicate
- object_type
- object_id
- valid_from
- valid_to
- status
- created_by
```

However, high-value domain relationships may later deserve dedicated tables.

Avoid over-generalizing too early.

## Arabic normalization

Implement deterministic normalization utilities for:

- whitespace
- common Arabic character variants
- optional diacritic removal for search
- normalized search forms

Never destroy the original display value.

## Acceptance criteria

- create/edit people
- aliases work
- create families/tribes/branches
- create places
- approximate dates supported
- entity relationships work
- Arabic normalized search fields generated

---

# 7. Phase 3: Tree Model

## Goal

Deliver the first usable family-tree experience.

## Core tables

```text
trees
tree_versions
tree_nodes
tree_relationships
```

A tree is an interpretation, not the universal truth graph.

## Tree requirements

Support:

- create tree
- add person
- connect parent/child
- branch navigation
- private draft
- public published version

## Versioning

Published versions should be immutable.

Workflow:

```text
Draft
  ↓
Publish
  ↓
Version N
  ↓
Edit creates new draft
  ↓
Publish
  ↓
Version N+1
```

## Frontend

Build:

- tree explorer
- node drawer/profile
- add person flow
- add parent/child flow
- edit relationship
- draft/published badge

## Important restriction

Tree relationships at this stage represent:

> this tree's current interpretation

They do not yet represent globally accepted historical fact.

## Acceptance criteria

- user can create a tree
- user can add people
- parent-child relations render
- user can publish immutable versions
- public users can browse published trees
- unpublished drafts remain private

---

# 8. Phase 4: Collaboration

## Goal

Support invited editing without opening public direct modification.

## Tables

```text
tree_collaborators
tree_invitations
tree_change_log
```

## Features

- invite collaborator
- accept invitation
- permission assignment
- remove collaborator
- audit edits
- collaborator activity view

## Change log

Record semantic actions:

```text
person_added
person_updated
relationship_added
relationship_changed
relationship_removed
source_linked
claim_created
```

Do not rely only on raw database audit diffs.

## Acceptance criteria

- owner can invite collaborators
- collaborators can edit according to permission
- unauthorized users cannot edit
- edits are auditable
- owner can revoke access

---

# 9. Phase 5: Forking and Tree Diff

## Goal

Allow competing interpretations without destructive conflict.

## Forking

Add:

```text
tree_forks
```

Fields:

```text
tree_id
parent_tree_id
parent_version_id
forked_by
forked_at
```

Fork starts from a specific published version.

## Diff engine

Implement semantic comparison:

- people added
- people removed
- relationship changes
- date changes
- source additions
- source removals

Do not diff raw row IDs only.

## Downstream impact

For parentage changes, compute affected descendant count.

## Frontend

Add:

- fork button
- upstream reference
- compare with upstream
- structured diff view

## Acceptance criteria

- public tree can be forked
- fork is independently editable
- original remains unchanged
- diff explains meaningful changes
- parentage impact is visible

---

# 10. Phase 6: Source and Evidence Model

## Goal

Build the research foundation before advanced AI.

## Tables

### Sources

```text
sources
source_files
source_passages
source_citations
```

### Source statements

```text
source_statements
```

A source statement represents:

> what the source explicitly says

not:

> what the platform accepts as historically true

### Claims

```text
claims
claim_evidence
claim_counter_evidence
claim_versions
```

## Claim structure

Minimum:

```text
id
subject
predicate
object
status
created_by
created_at
```

Optional:

```text
place
time_range
notes
```

## Evidence links

Evidence should connect claims to:

- source passage
- source statement
- another claim
- researcher note

## Source upload

Initial workflow:

```text
Upload source
  ↓
Store file
  ↓
Create source metadata
  ↓
Manual passage entry initially
```

AI extraction comes later.

## Acceptance criteria

- source can be created
- source file can be uploaded
- passage can be cited
- source statement can be recorded
- claim can reference evidence
- counter-evidence supported
- claim version history preserved

---

# 11. Phase 7: Disputes and Open Questions

## Goal

Make unresolved research a first-class product feature.

## Tables

```text
open_questions
question_entities
question_claims
question_sources
question_findings
question_notes
```

## Features

Open question contains:

- title
- description
- linked entities
- competing claims
- evidence
- counter-evidence
- missing evidence
- status
- contributors

Statuses:

```text
open
under_investigation
resolved
reopened
archived
```

## Disputed claims

Allow several competing claims to coexist.

Example:

```text
Claim A:
Abdullah -> father -> Muhammad

Claim B:
Abdullah -> father -> Saleh
```

Do not require one to be deleted.

## Frontend

Build:

- dispute panel
- evidence comparison
- open question page
- question activity
- resolution history

## Acceptance criteria

- competing claims can coexist
- open question can link to both
- evidence can be attached to each side
- resolving question preserves history
- question can be reopened

---

# 12. Phase 8: Public Suggestions

## Goal

Allow public participation without direct tree edits.

## Tables

```text
suggestions
suggestion_reviews
```

## Workflow

```text
Public user
  ↓
plain-text Arabic suggestion
  ↓
stored unchanged
  ↓
review queue
  ↓
accept / reject / convert to question
```

## Initial implementation

Do not require AI to launch this feature.

First implement manual review.

## Later AI enrichment

AI may extract:

- suggestion type
- referenced person
- proposed relationship
- source reference
- page
- potential duplicate

But original text remains canonical input.

## Acceptance criteria

- public user can submit suggestion on allowed node
- collaborator sees review queue
- collaborator can accept/reject
- accepted suggestion can create domain changes
- review decision is auditable

---

# 13. Phase 9: Dictionary and Indexes

## Goal

Make the product useful beyond individual trees.

## Dictionary pages

### Family

Display:

- names/aliases
- branches
- locations
- published trees
- relevant claims
- relevant sources
- open questions

### Tribe

Same pattern with branch relationships.

### Place

Display:

- historical names
- associated families
- associated tribes
- people
- migrations
- sources
- open questions

## Indexes

Implement:

- alphabetical family index
- tribe index
- branch index
- people index
- place index
- source index
- open question index
- disputed claim index

## Acceptance criteria

- public dictionary pages work
- indexes are filterable
- entries are graph-backed, not duplicate editorial silos
- aliases are searchable

---

# 14. Phase 10: Historical Mapping

## Goal

Make geography a primary research surface.

## Data model

Implement:

```text
geographic_associations
migration_events
historical_regions
spatial_evidence
```

Each geographic relation should support:

```text
entity
place
relation_type
time_from
time_to
status
source/evidence
```

## Frontend

Use MapLibre.

Initial map layers:

- documented place points
- family locations
- branch locations
- migrations
- source locations

## Time filtering

Add:

- year/range filter
- Hijri or Gregorian display support where practical

## Uncertainty

Do not render uncertain historical territory as hard fact.

Support:

- approximate regions
- disputed layer
- inferred layer
- documented layer

## Acceptance criteria

- family page shows map
- place page works
- migration routes render
- map can filter by period
- evidence is inspectable from map items
- status is visually distinguishable

---

# 15. Phase 11: Search V1

## Goal

Deliver strong Arabic search without extra infrastructure.

## Implement

### PostgreSQL full-text / lexical search

For:

- exact names
- source titles
- aliases
- claims
- passages

### Structured filtering

By:

- person
- family
- tribe
- branch
- place
- period
- source
- claim status

### pgvector

Add semantic search for:

- source passages
- family descriptions
- person descriptions
- research questions

## Retrieval API

Build a unified search endpoint with ranked result groups.

## Acceptance criteria

- exact Arabic name search works
- alias search works
- fuzzy/normalized name search works
- semantic passage search works
- filters work
- no OpenSearch required

---

# 16. Phase 12: PostgreSQL-Backed Job System

## Goal

Support asynchronous AI and document processing without Redis.

## Table

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

## Claiming

Workers should use:

```sql
FOR UPDATE SKIP LOCKED
```

## Capabilities

- retry
- exponential backoff
- dead/failed state
- idempotency key
- priority where necessary
- scheduled execution

## Use for

- source processing
- embeddings
- AI extraction
- contradiction scans
- entity resolution
- research investigations

## Acceptance criteria

- multiple workers cannot claim same job
- retry works
- job failures visible
- stuck jobs recoverable
- workers are idempotent where practical

---

# 17. Phase 13: AI Research Service Foundation

## Goal

Introduce AI behind stable interfaces.

## API contracts

Examples:

```text
POST /normalize-name
POST /embed
POST /classify
POST /extract/entities
POST /extract/claims
POST /resolve/entity
POST /analyze/contradiction
POST /research/query
```

## Model-provider abstraction

Avoid hard-coding one vendor.

Interfaces should support:

- embeddings
- generation
- reranking
- semantic classification

## Structured outputs

All AI outputs must be validated through schemas.

Never trust free-form model output for persistence.

## Acceptance criteria

- Go can call AI service
- model providers are replaceable
- all outputs validated
- timeouts/retries configured
- AI failures do not corrupt product state

---

# 18. Phase 14: Source Processing Pipeline

## Goal

Turn uploaded Arabic sources into reviewable structured candidates.

## Pipeline

```text
Source upload
  ↓
text/OCR extraction
  ↓
page segmentation
  ↓
Arabic normalization
  ↓
entity extraction
  ↓
claim extraction
  ↓
embedding
  ↓
candidate linking
  ↓
review queue
```

## Output

AI creates candidates, not accepted truth.

Candidate statuses:

```text
unreviewed
accepted
rejected
needs_review
```

## Human review

Researchers should see:

- original passage
- extracted entity
- extracted relation
- extraction confidence
- proposed existing entity match

## Acceptance criteria

- source can be processed asynchronously
- extracted passage remains traceable to page
- AI candidates can be accepted/rejected
- accepted candidates create explicit reviewed records
- rejected candidates remain auditable

---

# 19. Phase 15: Hybrid RAG

## Goal

Answer research questions from indexed evidence.

## Retrieval pipeline

```text
Question
  ↓
query classification
  ↓
lexical search
+ vector search
+ structured filters
  ↓
reranking
  ↓
evidence package
  ↓
LLM synthesis
```

## Response contract

The answer must distinguish:

- source statement
- accepted tree interpretation
- competing claim
- platform finding
- unresolved question

## Citations

Every research answer should cite relevant source passages.

## Acceptance criteria

- RAG answers cite evidence
- conflicting sources are represented
- system can answer "evidence is insufficient"
- responses never silently promote AI output to fact

---

# 20. Phase 16: Entity Resolution V1

## Goal

Detect likely duplicate people and families.

## Candidate generation

Use deterministic blocking:

- normalized name
- father
- grandfather
- place
- date range

## Scoring

Combine:

- string similarity
- embedding similarity
- relationship overlap
- geography
- chronology

## Output

```text
likely_different
possible_match
strong_candidate
```

## Human review

Merging must always require explicit confirmation.

## Merge operation

Must be:

- transactional
- audited
- reversible

## Acceptance criteria

- duplicate candidates generated
- explanation shows matching/conflicting signals
- merge requires human review
- merge can be reversed

---

# 21. Phase 17: Contradiction Detection V1

## Goal

Surface issues that deserve investigation.

## Deterministic checks

Implement first:

- multiple incompatible fathers
- parent born after child
- parent died too early where clearly impossible
- event before birth
- event after death
- duplicate identities in same branch
- impossible relationship loops

## Statistical checks

Add after enough validated data exists:

- unusual generation interval
- unusual age at parenthood
- unusual branch density

## Output

Create:

```text
platform_findings
```

Never mutate the underlying claim automatically.

## Acceptance criteria

- findings generated asynchronously
- finding links to affected claims/entities
- user can dismiss/confirm/investigate
- finding can create open question

---

# 22. Phase 18: Jev Semantic Control Layer

## Goal

Reduce cost and unnecessary deep-model calls.

Do not add Jev until there are enough real workflows to optimize.

## Candidate decisions

- query type
- suggestion type
- research relevance
- duplicate-suggestion detection
- cheap path vs deep reasoning
- potential contradiction routing
- source-bearing vs unsupported
- continue investigation vs stop

## Guardrail

Jev output is operational classification.

It must never be stored as historical confidence.

## Acceptance criteria

- measurable reduction in expensive model calls
- routing quality evaluated against test set
- fallback path exists
- no historical status depends directly on Jev output

---

# 23. Phase 19: Research Workspace

## Goal

Combine all research surfaces into one investigator experience.

## Workspace

Display together:

- selected person/family/branch
- tree context
- claims
- evidence
- counter-evidence
- map
- timeline
- relevant sources
- contradictions
- open questions
- platform findings
- notes

## Actions

Researcher can:

- create claim
- dispute claim
- link evidence
- create question
- resolve identity candidate
- run investigation
- attach finding to question

## Acceptance criteria

- researcher can investigate without jumping across unrelated screens
- all findings are traceable to evidence
- edits remain permission-controlled
- research history is preserved

---

# 24. Phase 20: GraphRAG V1

## Goal

Use graph structure in retrieval without introducing Neo4j yet.

## Implement in PostgreSQL first

Graph retrieval may use:

- recursive CTEs
- relationship tables
- bounded graph traversal
- source/claim relationships

## Query types

Examples:

- common ancestor path
- evidence connecting two families
- all claims around one branch
- people connected to one source
- geographic path over time

## Combined retrieval

```text
vector retrieval
+
lexical retrieval
+
graph retrieval
=
evidence package
```

## Acceptance criteria

- multi-hop research questions improve over vector-only RAG
- graph traversal remains performant for bounded depth
- evidence path is explainable

---

# 25. Phase 21: Source Dependency Analysis

## Goal

Avoid counting derivative sources as independent evidence.

## Detect

- explicit citation
- repeated unusual phrasing
- paraphrasing
- shared claim sequence
- common source references

## Data model

```text
source_dependencies
```

Types:

```text
cites
derived_from
likely_paraphrase
shared_origin
unknown
```

## Acceptance criteria

- source dependency graph visible
- evidence views warn about dependent sources
- dependency does not automatically invalidate a source

---

# 26. Phase 22: Advanced Temporal and Statistical Analysis

## Goal

Use validated data to identify unusual structures.

## Models

Potential:

- generation interval distribution
- age-at-parenthood distribution
- branch growth patterns
- naming frequency
- geographic persistence
- migration timing
- documentation density

## Rule

Statistical anomaly means:

> investigate

not:

> false

## Acceptance criteria

- statistics based only on qualified data
- reference population documented
- anomaly explanations show comparison basis
- no score is mislabeled as historical probability

---

# 27. Phase 23: Advanced Geospatial Intelligence

## Goal

Move from map display to research analysis.

Implement:

- historical place-name resolution
- place disambiguation
- spatial clustering
- migration inference
- geographic contradiction detection
- source geography analysis

## Example output

```text
Possible migration sequence:
Place A -> Place B -> Place C

Evidence:
Source 1
Source 2
Tree Version 4

Status:
platform hypothesis
```

## Acceptance criteria

- all inferred geography is labeled
- source-backed and inferred layers remain separate
- ambiguous historical places remain unresolved when necessary

---

# 28. Phase 24: Research Agent

## Goal

Perform multi-stage evidence-based investigations.

## Agent capabilities

- decompose question
- search sources
- search graph
- inspect geography
- inspect chronology
- compare claims
- inspect source dependency
- retrieve counter-evidence
- generate evidence package
- identify missing evidence
- recommend next investigation

## Agent restrictions

The agent may not:

- publish a tree
- accept a claim
- merge people
- resolve a dispute
- modify source evidence

without explicit authorized user action.

## Acceptance criteria

- agent produces traceable investigation steps
- citations support outputs
- counter-evidence included
- unresolved state is allowed
- actions are bounded by permissions

---

# 29. Neo4j Decision Gate

Do not add Neo4j by date.

Add it only if measurements show PostgreSQL graph retrieval is becoming limiting.

## Trigger conditions

Examples:

- frequent deep multi-hop traversals
- graph algorithms are core to product use
- recursive SQL becomes difficult to maintain
- relationship counts become very large
- graph-native exploration becomes latency-sensitive
- community detection becomes important
- source dependency networks become large

## Migration strategy

Prefer:

```text
PostgreSQL = authoritative
Neo4j = derived graph projection
```

until there is a strong reason to change ownership.

---

# 30. Redis Decision Gate

Redis is not required initially.

Consider Redis only when there is a measured need for:

- distributed rate limiting
- shared low-latency cache
- pub/sub
- realtime fan-out
- high-throughput ephemeral jobs
- queue workloads that outgrow PostgreSQL

Do not add Redis merely because it is common in web stacks.

---

# 31. OpenSearch Decision Gate

Consider OpenSearch only when:

- PostgreSQL search quality is insufficient
- Arabic indexing needs exceed Postgres capabilities
- search volume grows substantially
- complex ranking becomes difficult
- independent search scaling is needed

---

# 32. Temporal Decision Gate

Consider Temporal when workflows become:

- long-running
- multi-step
- retry-heavy
- human-in-the-loop
- externally dependent
- difficult to recover manually

Until then, PostgreSQL-backed jobs remain preferred.

---

# 33. Testing Strategy

## Unit tests

Focus on:

- claim rules
- permissions
- tree versioning
- merge logic
- diff logic
- date logic
- normalization

## Integration tests

Use real PostgreSQL with:

- PostGIS
- pgvector

Test:

- migrations
- repository behavior
- spatial queries
- vector queries
- job claiming

## API tests

Test:

- permissions
- tree workflows
- claim workflows
- suggestion workflows
- source workflows

## AI evaluation tests

Maintain datasets for:

- entity extraction
- claim extraction
- entity resolution
- contradiction classification
- retrieval quality
- Jev routing

AI should be evaluated separately from ordinary unit tests.

## E2E tests

Critical paths:

- create tree
- publish tree
- fork tree
- invite collaborator
- submit public suggestion
- review suggestion
- attach source
- create dispute
- create open question
- browse map
- research query

---

# 34. Seed Data

Create a small synthetic Arabic lineage dataset.

It should intentionally contain:

- aliases
- duplicate people
- disputed parentage
- approximate dates
- two competing sources
- migration
- one unresolved question
- one source dependency
- one forked tree

This dataset becomes:

- local demo
- test fixture
- AI evaluation fixture
- UI development fixture

Avoid using sensitive living-person data.

---

# 35. Observability Milestones

From early stages, capture:

- request latency
- error rate
- database latency
- slow queries
- job queue depth
- job failures

When AI launches, add:

- model latency
- token/cost usage
- extraction acceptance rate
- entity-resolution acceptance rate
- contradiction precision
- retrieval hit rate

---

# 36. Security Milestones

Before public launch:

- authorization audit
- upload validation
- signed object access
- rate limiting
- CSRF protection where applicable
- secure cookie policy
- secrets management
- dependency scanning
- audit logging
- prompt-injection protections for source processing

---

# 37. Suggested Delivery Milestones

## Milestone A: Usable tree platform

Includes:

- auth
- identity model
- tree creation
- publication
- collaboration

This proves the basic product.

## Milestone B: Research-aware genealogy

Includes:

- sources
- claims
- evidence
- disputes
- open questions
- forks
- diffing

This proves the core product thesis.

## Milestone C: Geographic research

Includes:

- places
- PostGIS
- maps
- migrations
- time filters

This proves mapping as a first-class capability.

## Milestone D: AI-assisted research

Includes:

- semantic search
- embeddings
- RAG
- source processing
- entity resolution
- contradiction detection

This proves useful AI without over-automation.

## Milestone E: Lineage intelligence

Includes:

- GraphRAG
- source dependency analysis
- statistical analysis
- research workspace
- advanced geospatial analysis
- Jev routing

## Milestone F: Research agent

Includes:

- multi-stage investigations
- evidence packages
- counter-evidence retrieval
- next-investigation recommendations

---

# 38. Recommended First Build Order

If implementation starts immediately, build in this order:

```text
1. Repository + CI
2. PostgreSQL/PostGIS/pgvector
3. Auth + permissions
4. Person/Family/Tribe/Branch/Place models
5. Tree model
6. Tree UI
7. Publishing/versioning
8. Collaboration
9. Forking + diff
10. Sources
11. Claims + evidence
12. Disputes
13. Open questions
14. Public suggestions
15. Dictionary
16. Indexes
17. Mapping
18. Search
19. PostgreSQL job queue
20. Python AI service
21. Embeddings + hybrid retrieval
22. AI source extraction
23. Entity resolution
24. Contradiction detection
25. Research workspace
26. GraphRAG
27. Jev routing
28. Statistical/geospatial intelligence
29. Research agent
```

---

# 39. What Not to Build First

Do not start with:

- Neo4j
- Kafka
- Redis
- Kubernetes
- OpenSearch
- a separate vector database
- multi-agent orchestration
- custom foundation models
- complex event-driven microservices
- automatic lineage conclusions
- full historical map reconstruction
- large-scale statistical inference

These may become useful later, but they do not validate the product's core assumptions.

---

# 40. Definition of V1 Complete

V1 is complete when a user can:

1. register
2. create a family tree
3. invite collaborators
4. publish a version
5. fork another published tree
6. compare two tree versions
7. attach a historical source
8. create a claim from that source
9. represent competing claims
10. create an open research question
11. receive a public Arabic suggestion
12. review and accept/reject that suggestion
13. browse family/tribe dictionary pages
14. search names and sources
15. view historical locations on a map
16. view migration relationships
17. ask a source-grounded research question
18. see clearly separated:
   - source statements
   - research claims
   - tree interpretation
   - platform findings
   - unresolved questions

V1 does not require autonomous research agents or graph-native infrastructure.

---

# 41. Definition of Product Architecture Success

The implementation is successful if the system can grow from:

```text
family tree builder
```

into:

```text
Arabic lineage research platform
```

without requiring a rewrite of the core evidence model.

The most important architectural investment is therefore not a framework, database, or AI provider.

It is the correctness of the domain model around:

```text
identity
+
tree interpretation
+
source
+
claim
+
evidence
+
dispute
+
open question
+
platform finding
+
place
+
time
```

If those concepts are modeled correctly, the AI and research layers can evolve substantially without undermining the integrity of the product.
