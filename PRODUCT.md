# PRODUCT.md

# Arabic Lineage Intelligence Platform

## 1. Product Summary

The product is a public Arabic-first platform for building, publishing, exploring, comparing, and researching family trees, families, tribes, branches, historical relationships, and geographic lineage data.

It is not intended to be only a family-tree builder or genealogy directory.

The long-term product direction is an **AI-assisted lineage research platform** that combines:

- collaborative family-tree building
- family and tribe dictionaries
- advanced indexes
- documentary source management
- claim and evidence tracking
- knowledge graphs
- hybrid retrieval and GraphRAG
- Arabic information extraction
- entity resolution
- statistical and temporal analysis
- geospatial and migration analysis
- contradiction detection
- research-question generation
- AI-assisted investigation
- public contribution workflows

The platform must preserve a strict separation between:

1. what a source explicitly states
2. what a contributor or researcher claims
3. what an accepted tree or publication currently represents
4. what the platform computes or infers
5. what remains unresolved or disputed

The platform must help users investigate lineage questions without forcing conclusions where the available evidence does not justify one.

---

## 2. Product Vision

Build the most useful Arabic digital environment for structured lineage research.

The system should allow a researcher to move naturally between:

- a person
- a family
- a tribe
- a branch
- a tree
- a historical source
- a claim
- a disputed claim
- a place
- a migration path
- a timeline
- an open question

without losing the provenance of the information.

The tree is therefore not the database itself.

It is one view over a larger lineage knowledge system.

---

## 3. Product Thesis

Traditional family-tree software usually assumes that the tree represents known truth.

Lineage research rarely works that way.

Historical genealogy contains:

- conflicting sources
- incomplete records
- name ambiguity
- competing family traditions
- uncertain dates
- uncertain places
- different versions of the same lineage
- undocumented links
- duplicated people
- migrations
- disputed branches
- questions that may remain unresolved for years

The product should model that uncertainty directly rather than hiding it.

The product's core thesis is:

> A lineage platform should preserve evidence, interpretations, alternatives, disputes, uncertainty, and research history instead of reducing everything to one canonical tree.

AI then becomes useful because the platform has enough structure to help researchers:

- retrieve evidence
- compare claims
- discover contradictions
- identify possible duplicates
- detect unusual chronology
- analyze geography
- propose hypotheses
- surface unresolved questions
- prioritize investigations

without pretending that the model itself is the authority.

---

## 4. Non-Negotiable Product Principles

### 4.1 Evidence before conclusion

AI may generate findings, signals, comparisons, and hypotheses.

AI must not silently convert them into established genealogical facts.

### 4.2 Preserve disagreement

Conflicting claims must be allowed to coexist.

The system must not require every dispute to be resolved before information can be represented.

### 4.3 Open questions are first-class knowledge

"Unknown", "disputed", and "needs investigation" are valid outcomes.

The platform should actively preserve unresolved questions.

### 4.4 Provenance is mandatory

Every significant research claim should be traceable to:

- source
- contributor
- accepted tree
- platform analysis
- or explicit hypothesis

### 4.5 Platform output is visibly distinct

A platform-produced finding must never be presented as if it were a historical source.

### 4.6 Human-governed publication

AI can assist research and moderation, but accepted lineage structures and published interpretations remain governed by humans.

### 4.7 No DNA or genetic genealogy

DNA analysis, genetic ancestry, genetic matching, haplogroups, or genetic inference are outside the scope of this product.

The platform instead relies on:

- documentary evidence
- historical sources
- structured lineage data
- graph analysis
- statistics
- temporal reasoning
- geospatial analysis
- AI-assisted analysis

### 4.8 Arabic-first design

Arabic names, Arabic source material, Arabic search behavior, Arabic naming patterns, and Arabic historical terminology must be treated as primary product requirements.

---

## 5. Primary Users

### 5.1 Public visitor

Can:

- browse public family trees
- browse family and tribe dictionary entries
- search indexes
- explore maps
- view sources where public
- view disputes and open questions
- compare public tree versions
- submit plain-text suggestions on nodes

Cannot:

- directly modify published trees
- accept research claims
- merge identities
- resolve disputes

### 5.2 Registered user

Can additionally:

- save trees
- follow families, tribes, people, places, or questions
- fork public trees
- maintain personal or draft trees
- submit richer suggestions
- participate in permitted research discussions

### 5.3 Invited collaborator

Can:

- edit a tree
- review public suggestions
- accept or reject proposed node changes
- add sources
- create claims
- mark claims as disputed
- create open questions
- review AI findings
- approve or reject entity merges
- compare tree versions

### 5.4 Tree owner / maintainer

Can additionally:

- manage collaborators
- publish tree versions
- define tree visibility
- accept changes from forks
- manage moderation rules
- lock sensitive or high-impact nodes
- resolve tree-level conflicts

### 5.5 Researcher

A role or capability set focused on:

- source review
- claim comparison
- evidence analysis
- open questions
- graph analysis
- historical geography
- AI-assisted investigations

### 5.6 Platform moderator / administrator

Can:

- manage abuse reports
- moderate public contributions
- manage dictionary records
- manage source ingestion policies
- manage indexing
- review platform-level quality issues
- inspect AI extraction failures
- manage research-taxonomy settings

---

## 6. Core Domain Model

The product should be built around an evidence-aware lineage graph.

### 6.1 Person

Represents an individual identity.

Possible attributes:

- canonical Arabic name
- alternative names
- kunyah
- laqab
- nisbah
- gender where known
- approximate birth date
- approximate death date
- locations
- biographies
- aliases
- identity confidence metadata
- associated sources
- associated claims

A person record must be capable of representing uncertainty.

### 6.2 Family

Represents a family-level identity.

Possible attributes:

- canonical name
- alternative spellings
- known branches
- historical locations
- origin claims
- related families
- source references
- tree references
- dictionary description
- unresolved questions

### 6.3 Tribe

Represents a tribal identity or higher-order lineage grouping.

The system must not assume every family-to-tribe relationship is undisputed.

### 6.4 Branch

Represents a lineage subdivision within a family or tribe.

Branches may have:

- historical time range
- location
- parent branch
- associated tree
- disputed relationships
- migration history

### 6.5 Place

A first-class geographic entity.

Fields may include:

- Arabic name
- alternative names
- historical names
- modern name
- place type
- parent region
- coordinates
- polygon or approximate area
- historical notes
- source references

### 6.6 Source

A first-class research entity.

Examples:

- book
- manuscript
- family document
- archive record
- newspaper
- article
- research paper
- oral testimony
- website
- existing published tree
- historical registry

A source should store:

- title
- author
- date
- edition
- pages
- source type
- location
- metadata
- citation information
- digitized text where permitted
- derivative/dependency relationships

### 6.7 Source Statement

Represents what a source explicitly says.

This must be separate from whether the statement is historically correct.

Example:

> Source A states that Abdullah is the son of Muhammad.

That statement can be recorded even if another source disagrees.

### 6.8 Claim

A structured proposition about lineage or historical identity.

Examples:

- X is the father of Y
- X belongs to branch Y
- family X originated in place Y
- branch X migrated to place Y
- person X and person Y are the same historical identity

A claim may have:

- subject
- predicate
- object
- date range
- place
- supporting evidence
- counter-evidence
- claimant
- status
- version history

### 6.9 Interpretation

Represents a researcher's or publication's interpretation of one or more claims.

Different interpretations may coexist.

### 6.10 Platform Finding

Represents an analytical result produced by the system.

Examples:

- possible duplicate identity
- chronological anomaly
- potential contradiction
- likely geographic continuity
- unusual generation pattern
- likely source dependency
- missing-link candidate

Platform findings must remain visibly separate from accepted historical claims.

### 6.11 Open Question

A first-class research object.

Examples:

- Who was the father of Person X?
- Are Record A and Record B the same person?
- Did Branch X originate in Place A or Place B?
- Is this tree branch missing one generation?
- Are two apparently independent sources actually dependent?

An open question may contain:

- title
- description
- linked entities
- linked claims
- evidence for
- evidence against
- missing evidence
- participants
- status
- history
- platform findings
- proposed next investigations

---

## 7. Epistemic Segmentation

Every important piece of information must clearly belong to one of the following layers.

### Layer 1: Source Record

What an original source explicitly contains.

### Layer 2: Research Claim

A proposition derived from a source, contributor, or researcher.

### Layer 3: Accepted Interpretation

What a maintained or published tree currently represents.

### Layer 4: Platform Finding

What AI, statistics, graph analysis, or other platform systems produced.

### Layer 5: Open Question

What remains unresolved.

These layers must never be silently merged.

---

## 8. Claim Status Model

Candidate statuses:

- documented
- supported
- contested
- disputed
- inferred
- platform-generated
- unresolved
- contradicted
- rejected
- superseded
- unknown

The exact taxonomy should be kept intentionally small enough to remain understandable.

---

## 9. Family Tree Product

### 9.1 Public tree browsing

Users should be able to:

- browse published trees
- expand and collapse branches
- navigate by person
- open person profiles
- inspect supporting sources
- inspect disputed relationships
- see unresolved nodes
- inspect branch geography
- view change history

### 9.2 Tree creation

Users should be able to create trees from:

- scratch
- imported structured data
- forked public trees
- AI-assisted source extraction
- collaborator invitations

### 9.3 Forking

A public tree may be forked.

A fork should preserve:

- original tree reference
- original version
- fork creator
- difference history
- source changes
- relationship changes
- merge history

### 9.4 Lineage diff

The platform should provide structured comparison between trees.

Example output:

- 14 people added
- 3 people removed
- 2 identities merged
- 1 parent relationship changed
- 6 sources added
- 2 new disputes introduced
- 1 contradiction resolved

High-impact relationship changes should show downstream impact.

### 9.5 Collaborative editing

Collaborators should receive:

- edit permissions
- suggestion review queue
- change history
- version comparison
- conflict review
- source review tools
- AI-assisted change summaries

### 9.6 Publication

A maintained tree should support:

- draft versions
- published versions
- publication notes
- historical version access
- rollback
- optional moderation before publication

---

## 10. Public Contribution System

Public users should not directly edit a maintained tree by default.

Instead, users can submit plain-text Arabic suggestions per node.

Example:

> والد عبدالله هو صالح وليس محمد، وقد ورد ذلك في كتاب كذا، الجزء الثاني، صفحة 121.

The platform should preserve the original submission and optionally extract:

- suggestion type
- subject
- current relationship
- proposed relationship
- referenced source
- referenced page
- affected nodes
- possible duplicate suggestion
- potential contradiction

Collaborators review and:

- accept
- reject
- request clarification
- convert to research question
- link to existing dispute

AI extraction must not replace the contributor's original text.

---

## 11. Family and Tribe Dictionary

The dictionary should be a structured research surface, not a standalone encyclopedia.

A family entry may contain:

- name
- variants
- description
- historical references
- branches
- related families
- tribe relationships
- known locations
- migration history
- earliest indexed references
- notable people
- source list
- published trees
- disputed relationships
- open questions
- map
- timeline

The dictionary should be generated from and linked to the underlying knowledge graph.

---

## 12. Advanced Indexes

Indexes should support more than alphabetical browsing.

Potential indexes:

### Identity

- families
- tribes
- branches
- people
- alternative names
- nisbas
- kunyahs

### Geography

- regions
- cities
- villages
- historical settlements
- families by place
- tribes by place

### Time

- century
- Hijri period
- Gregorian period
- earliest documented reference
- migration period

### Research

- disputed lineages
- unresolved identities
- open questions
- contradictory claims
- unverified branches
- recently revised claims
- sources needing review

### Sources

- people by source
- families by source
- claims by source
- places by source
- source dependency chains

### Semantic / AI indexes

- similar biographies
- likely duplicate identities
- related unresolved questions
- geographic anomalies
- chronological anomalies

---

## 13. Mapping and Geo-Temporal Research

Mapping is a core research capability.

### 13.1 Family distribution map

Display:

- origin claims
- historical settlements
- branch locations
- documented presence
- inferred presence
- disputed presence
- current known distribution where appropriate

### 13.2 Migration map

Display routes over time.

Example:

Shagra → Ushaiqer → Riyadh

Each migration relationship should support:

- date range
- evidence
- certainty
- status
- source
- interpretation

### 13.3 Historical map layers

Potential layers:

- documented presence
- interpreted presence
- platform-inferred presence
- disputed presence
- unresolved areas
- source locations
- migration routes

### 13.4 Time slider

Users should be able to view geographic data by:

- year
- approximate period
- century
- Hijri range

### 13.5 Place pages

A place page should show:

- families documented there
- tribes documented there
- known migrations
- people associated with the place
- relevant sources
- open questions
- historical aliases

### 13.6 Map uncertainty

The platform must avoid representing uncertain historical territory as precise fact.

Prefer:

- evidence points
- approximate regions
- confidence gradients
- disputed overlays
- explicit source-backed boundaries

over unsupported hard borders.

---

## 14. AI Product Strategy

AI is not one feature.

It is a set of specialized capabilities serving lineage research.

The platform should avoid a design where every task becomes:

`input -> general LLM -> answer`

Instead:

`deterministic systems -> semantic decision models -> specialized models -> graph/statistical analysis -> reasoning model -> human review`

---

## 15. AI Capability: Hybrid RAG

Retrieval should combine:

- lexical search
- Arabic full-text search
- vector semantic search
- graph retrieval
- temporal filters
- geographic filters
- source filters
- claim filters

The RAG system should retrieve evidence, not manufacture a canonical truth.

Answers should distinguish:

- source statements
- accepted interpretations
- conflicting claims
- platform findings
- unresolved questions

---

## 16. AI Capability: Knowledge Graph

The lineage knowledge graph is a foundational AI and product layer.

It should connect:

- person
- family
- tribe
- branch
- place
- source
- source statement
- claim
- interpretation
- event
- open question
- platform finding

This enables multi-hop queries that vector retrieval alone cannot reliably answer.

---

## 17. AI Capability: GraphRAG

GraphRAG should combine:

- relevant source passages
- entity relationships
- claim relationships
- source provenance
- geographic context
- temporal context
- conflicts
- open questions

Example query:

> What evidence connects Family X with Branch Y?

The system should return:

- relevant claims
- supporting source passages
- graph paths
- counter-evidence
- unresolved gaps
- source dependencies

---

## 18. AI Capability: Arabic Information Extraction

The platform should extract structured information from Arabic historical and genealogical material.

Target entities:

- people
- families
- tribes
- branches
- places
- dates
- events
- relationships
- titles
- kunyahs
- nisbas
- source references

Target relationships:

- father
- mother
- child
- sibling
- family membership
- branch membership
- tribal attribution
- place association
- migration
- residence
- historical event association

AI-extracted information must remain marked as unreviewed until validated.

---

## 19. AI Capability: Claim Extraction

The extraction system should produce structured claims with provenance.

Example:

Subject: Muhammad  
Predicate: father_of  
Object: Abdullah  
Source: Book X  
Page: 81  
Extraction method: AI  
Extraction confidence: 0.94  
Research status: unreviewed

The model confidence must not be presented as historical certainty.

---

## 20. AI Capability: Entity Resolution

The system should identify possible duplicate identities across:

- different trees
- different sources
- spelling variants
- aliases
- incomplete names

Signals may include:

- name similarity
- father
- grandfather
- siblings
- children
- location
- time period
- branch
- occupation
- source context

Output should be:

- likely different
- possible match
- strong merge candidate

Final merges require human review.

---

## 21. AI Capability: Temporal Reasoning

The platform should detect:

- impossible parent/child dates
- suspicious generation intervals
- overlapping identities
- impossible life-event ordering
- unusual generational density
- inconsistent migration timing

These should create findings such as:

> Chronology deserves investigation.

They should not automatically reject the claim.

---

## 22. AI Capability: Statistical Analysis

Once enough validated data exists, the system may model:

- generation intervals
- age-at-parenthood distributions
- branch expansion
- naming frequency
- geographic persistence
- migration patterns
- surname / nisbah evolution
- source density
- documentation gaps

Statistics should be used to surface anomalies and patterns, not to convert correlation into historical fact.

---

## 23. AI Capability: Contradiction Detection

The platform should detect possible contradictions such as:

- different fathers for the same person
- conflicting family attribution
- conflicting tribal attribution
- incompatible dates
- incompatible places
- duplicate identities
- inconsistent migration paths
- source disagreement
- incompatible branch structures

A contradiction should become a reviewable research object.

---

## 24. AI Capability: Hypothesis Generation

The platform may propose:

- missing intermediary people
- likely identity matches
- possible branch relationships
- possible migration sequences
- potentially related sources
- likely source dependencies

All such outputs must be labeled as hypotheses.

---

## 25. AI Capability: Research Agent

A future research agent should be capable of decomposing complex lineage questions.

Example:

> Investigate the possibility that Branch X is connected to Family Y during the 12th Hijri century.

Possible workflow:

1. normalize entity names
2. retrieve relevant family and branch entities
3. inspect graph paths
4. retrieve source passages
5. compare chronology
6. compare geography
7. inspect known relatives
8. identify conflicting evidence
9. inspect source dependency
10. generate research report
11. list unresolved gaps
12. suggest next investigations

The output should be a research package, not a definitive historical verdict.

---

## 26. AI Capability: Research Question Generation

The platform should identify gaps worthy of investigation.

Examples:

- identity ambiguity
- missing generations
- conflicting place history
- unexplained branch split
- unsupported migration
- source disagreement
- unusual chronology

The platform may generate a new Open Question candidate.

Human users decide whether it becomes a maintained research question.

---

## 27. AI Capability: Source Dependency Analysis

The system should attempt to identify when multiple sources are not independent.

Signals:

- explicit citations
- repeated wording
- paraphrasing
- shared unusual errors
- identical claim sequences
- known publication lineage

This prevents five derivative sources from being treated as five independent pieces of evidence.

---

## 28. AI Capability: Source Characterization

The system may characterize sources using descriptive attributes.

Example:

- primary / secondary
- contemporary / later
- author proximity
- citation quality
- known dependencies
- geographic proximity
- independent corroboration count

The system should avoid reducing a source to a simplistic "trusted / untrusted" score.

---

## 29. AI Capability: Semantic Similarity

Embeddings may support:

- RAG
- similar passages
- duplicate biographies
- related research questions
- similar migration narratives
- claim similarity
- cross-source event matching

---

## 30. AI Capability: Graph Algorithms

Graph analysis may support:

- shortest relationship paths
- connected components
- community detection
- graph anomaly detection
- branch structure comparison
- common ancestors
- source dependency networks
- relationship impact analysis

Graph algorithms produce structural findings, not historical conclusions.

---

## 31. AI Capability: Geospatial Intelligence

The platform should support:

- Arabic place-name extraction
- historical place normalization
- place disambiguation
- geographic plausibility analysis
- migration inference
- spatial clustering
- location anomaly detection
- source-geography analysis

---

## 32. AI Capability: AI-Assisted Tree Editing

Users may write natural-language instructions such as:

> Add Muhammad's sons Abdullah, Suleiman, and Nasser. Abdullah moved to Al-Zubair around 1250 AH.

The system may prepare a change preview.

Nothing is applied until the user confirms.

---

## 33. AI Capability: Lineage Diff Explanation

When comparing tree versions or forks, AI should summarize:

- added people
- removed people
- merged identities
- changed relationships
- changed dates
- added or removed sources
- newly introduced disputes
- resolved questions
- downstream impact

---

## 34. AI Capability: Research Recommendations

The platform may recommend research actions such as:

- review Source X
- investigate identity Y
- inspect conflicting branch
- connect unresolved question A with B
- verify a suspicious date
- examine an unreviewed migration claim

The platform should recommend investigation, not beliefs.

---

## 35. Jev / System One Semantic Decision Layer

A fast semantic decision model such as TypeSafe Jev may be used as a low-cost control layer.

Jev should not determine historical truth.

It can make high-volume semantic workflow decisions such as:

- relevant / irrelevant
- likely duplicate / different
- needs review / routine
- potential contradiction / normal
- source-bearing / unsupported
- geographic query / lineage query
- vector search / graph search / hybrid
- cheap path / expensive reasoning path
- continue investigation / enough evidence gathered
- duplicate suggestion / novel suggestion
- high-impact / low-impact change
- open-question candidate / ordinary issue

Recommended architecture:

`deterministic rules -> Jev -> specialized analysis -> reasoning model -> human`

Important distinction:

A Jev probability is a model-routing or classification score.

It is not historical confidence.

---

## 36. AI Output Classes

The product UI and backend should preserve distinct output types.

### Source-derived

"Book X states that A is the son of B."

### Research claim

"A may be the son of B."

### Platform finding

"The A -> B relationship is more chronologically consistent with currently indexed evidence."

### Accepted interpretation

The relationship currently published in a maintained tree.

### Open question

"Available evidence does not currently resolve whether A's father was B or C."

---

## 37. Research Workspace

A dedicated research workspace should combine:

- source search
- claim graph
- evidence comparison
- tree view
- map
- timeline
- contradictions
- open questions
- AI findings
- notes
- investigation history

Researchers should be able to move between evidence and visualization without losing context.

---

## 38. Search

Search should support:

- exact Arabic name search
- fuzzy Arabic name search
- semantic search
- transliteration where useful
- alias search
- family search
- tribe search
- place search
- source search
- date filtering
- geographic filtering
- tree filtering
- disputed-only filtering
- open-question filtering

---

## 39. Versioning and Auditability

Versioning is required for:

- trees
- claims
- dictionary entries
- interpretations
- source metadata
- open questions
- accepted merges

The platform should preserve:

- what changed
- who changed it
- when
- why
- supporting evidence
- previous value
- current value

---

## 40. Research History

The platform should make it possible to understand:

- what was previously believed
- what source supported it
- what changed
- what new source appeared
- which interpretation replaced another
- which questions were reopened

This history is part of the research value of the product.

---

## 41. Notifications

Potential notifications:

- tree suggestion received
- collaborator invited
- claim disputed
- source added to followed question
- followed open question updated
- fork created
- fork contains significant changes
- possible duplicate detected
- new contradiction detected
- new source references followed family
- maintained tree published

---

## 42. Moderation and Abuse

The product needs moderation for:

- harassment
- fabricated claims
- spam
- defamatory descriptions
- deliberate vandalism
- mass low-quality suggestions
- impersonation
- source manipulation

Genealogy can involve living individuals and sensitive family claims.

The platform should support:

- reporting
- rate limits
- restricted visibility
- contributor reputation signals
- moderation queues
- revision history
- private drafts

---

## 43. Privacy

The platform must distinguish between:

- historical/deceased people
- living people
- uncertain living status

The product should minimize exposure of unnecessary personal information for living individuals.

Potential restrictions:

- exact birth dates
- addresses
- contact information
- sensitive family notes
- unpublished branches

Privacy policy and data visibility rules must be explicit before broad public launch.

---

## 44. Non-Goals

The product is not intended to become:

- DNA genealogy software
- genetic ancestry analysis
- ancestry ethnicity estimation
- a generic social network
- a general historical encyclopedia
- a platform where AI autonomously decides lineage truth
- a system that hides disputes to produce one canonical answer
- a pure genealogy chatbot
- a static family directory

---

## 45. Functional Requirements

### Tree

- create tree
- edit tree
- fork tree
- publish tree
- version tree
- compare tree versions
- invite collaborators
- manage permissions
- review suggestions

### Research

- create claims
- attach sources
- attach counter-evidence
- create disputes
- create open questions
- link claims to questions
- record interpretations
- review platform findings

### Sources

- create source record
- attach pages/passages
- link entities
- link claims
- track dependencies
- search sources

### Dictionary

- family entries
- tribe entries
- branch entries
- source references
- map
- timeline
- related entities

### Mapping

- points
- regions
- routes
- layers
- time filters
- family filters
- branch filters
- source filters
- uncertainty visualization

### AI

- extraction
- entity resolution
- contradiction detection
- retrieval
- GraphRAG
- source characterization
- hypothesis generation
- research assistance
- semantic routing
- geospatial analysis
- temporal analysis

---

## 46. Non-Functional Requirements

### Auditability

Every high-impact change must be traceable.

### Explainability

Platform findings should explain which evidence or signals produced them.

### Arabic quality

Arabic search, normalization, display, names, and historical text handling are first-class quality requirements.

### Performance

Large public trees and maps must remain responsive.

### Scalability

The architecture should support:

- large graphs
- millions of relationships
- large source corpora
- vector indexes
- map datasets
- asynchronous extraction jobs

### Security

The system must enforce:

- authorization
- collaborator permissions
- tree visibility
- moderation capabilities
- secure source uploads
- rate limits

### Reproducibility

Where practical, research findings should be reproducible from:

- source version
- graph version
- algorithm version
- model version

---

## 47. Suggested System Boundaries

Potential high-level services:

### Core API

Handles:

- users
- trees
- people
- families
- tribes
- branches
- claims
- questions
- collaboration

### Knowledge Service

Handles:

- graph relationships
- claim graph
- source graph
- graph queries

### Search Service

Handles:

- lexical search
- vector search
- hybrid retrieval
- indexes

### AI / Research Service

Handles:

- extraction
- entity resolution
- contradiction analysis
- research-agent workflows
- GraphRAG
- Jev semantic routing

### Mapping Service

Handles:

- places
- geographic relationships
- migrations
- map layers
- spatiotemporal queries

### Source Processing Workers

Handle:

- OCR
- text extraction
- chunking
- embeddings
- entity extraction
- claim extraction
- indexing

This is a product-level boundary proposal, not a requirement to use microservices from day one.

---

## 48. Product Metrics

Avoid measuring only engagement.

Useful product-quality metrics include:

### Research quality

- percentage of published relationships with source references
- percentage of AI extractions reviewed
- accepted vs rejected entity-resolution suggestions
- contradiction resolution rate
- number of useful open questions created
- number of sources linked per research claim

### Collaboration

- suggestions reviewed
- fork-to-merge activity
- collaborator participation
- average suggestion review time

### Search

- successful search sessions
- evidence opened after search
- source retrieval precision
- query reformulation rate

### AI

- extraction precision
- entity-resolution acceptance rate
- contradiction precision
- research-agent citation coverage
- cost per processed source
- percentage of requests resolved without expensive reasoning

### Knowledge growth

- verified people
- verified relationships
- indexed sources
- mapped places
- open questions
- resolved questions
- published trees

---

## 49. MVP

The MVP should prove the core knowledge model and collaboration model before attempting the full research-agent vision.

### MVP capabilities

#### Public

- browse published trees
- browse person pages
- browse family pages
- basic family dictionary
- search
- public node suggestions

#### Tree management

- create/edit tree
- invite collaborators
- publish tree
- fork tree
- version history
- basic tree diff

#### Research model

- sources
- source statements
- claims
- claim status
- evidence links
- disputed claims
- open questions

#### Mapping

- place entities
- person/family-place relationships
- basic map
- historical presence points
- migration events

#### AI

- Arabic name normalization
- basic semantic search
- hybrid RAG over approved sources
- source-assisted Q&A with citations
- suggestion classification
- basic duplicate-person candidates
- basic contradiction detection

Jev may be introduced for routing and triage where it demonstrably reduces cost or latency.

---

## 50. Phase 2

- GraphRAG
- richer entity resolution
- map timelines
- migration paths
- source dependency analysis
- tree comparison intelligence
- advanced research indexes
- research workspace
- AI-assisted tree editing
- statistical chronology analysis
- open-question recommendations

---

## 51. Phase 3

- full lineage research agent
- advanced hypothesis generation
- spatiotemporal clustering
- source lineage/dependency graph
- research recommendation engine
- learned ranking from expert review
- advanced graph analytics
- historical place-name resolution
- large-scale source ingestion
- domain-specific Arabic lineage models

---

## 52. Key Product Risks

### False certainty

The UI may accidentally make AI or community claims look established.

Mitigation:

- strict epistemic labels
- provenance everywhere
- separate platform findings
- human-governed publication

### Source duplication

Several derivative sources may appear to provide independent evidence.

Mitigation:

- source dependency graph
- citation extraction
- dependency analysis

### Identity merging errors

Arabic naming ambiguity can cause incorrect merges.

Mitigation:

- merge proposals
- explainable signals
- human confirmation
- reversible merges

### Geographic overclaiming

Historical territory may be represented too precisely.

Mitigation:

- uncertainty layers
- approximate regions
- source-backed mapping
- disputed overlays

### Model bias

AI may favor majority or frequently repeated interpretations.

Mitigation:

- retrieve counter-evidence
- preserve minority documented claims
- do not rank historical truth solely from frequency
- expose source dependence

### Living-person privacy

Public trees may expose private information.

Mitigation:

- living-person rules
- privacy controls
- restricted fields
- reporting

---

## 53. Open Product Questions

These should be resolved during product discovery.

1. What exact claim-status taxonomy should be exposed to users?
2. Can a published tree contain unresolved parent relationships?
3. Should one person entity be shared globally across trees, or should trees initially maintain their own identity records?
4. How should disputed tribal relationships be displayed?
5. How should living individuals be handled?
6. What source types are acceptable for public citation?
7. How should oral testimony be represented?
8. Should contributor reputation influence moderation priority?
9. When should a platform finding automatically generate an Open Question candidate?
10. Should AI-generated claims require explicit human approval before entering the research graph?
11. How should historical place names map to modern geography?
12. How should approximate dates and Hijri/Gregorian uncertainty be stored?
13. Should forks share source records with the upstream tree?
14. What constitutes a sufficiently significant tree change to notify followers?
15. Which graph operations must be real-time versus asynchronous?
16. Which AI capabilities require domain-specific training rather than general models?
17. How much of the research workspace should be public?
18. Should tree maintainers be able to define their own interpretation while still displaying competing claims from the wider knowledge graph?

---

## 54. Product Identity

This product should be understood internally as:

> An Arabic lineage intelligence and research platform.

The family tree is one core interface.

The dictionary is one core interface.

The map is one core interface.

The source and claim graph is the knowledge foundation.

AI is the research machinery around that foundation.

The platform succeeds when it helps people understand:

- what is documented
- what is claimed
- what is disputed
- what the system found
- what remains unknown
- what deserves investigation next

without pretending that uncertainty does not exist.

---

## 55. North-Star Principle

> AI may infer a hypothesis, but only evidence and human research governance can establish a published genealogical interpretation.

