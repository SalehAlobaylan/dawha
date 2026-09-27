# OCR, and what a reliable family-tree import would actually need

**Status: open decision, deliberately not taken here.** Written 2026-09-27 so the OCR
question stops sitting inside Phase 14's status, and so a family-tree import feature is
designed against what is true rather than against what is hoped for.

This repository currently **refuses** scanned documents. `internal/sourceprocessing/upload.go`
accepts `text/*`, `application/json` and `application/xml`; a PDF, JPEG, PNG, TIFF or
`application/octet-stream` upload is answered with `415` and a body naming the supported
matrix, before any object is stored or any job is enqueued. That was a deliberate V1
decision (`plans/README.md`, plan 003) taken because the only implemented extractor reads
text, and a PDF that was accepted and then failed in a worker consumed storage, retries and
a researcher's patience while telling them nothing until the run went red.

`IMPLEMENTATION_PLAN.md:1145` lists `text/OCR extraction` in the Phase 14 pipeline. That
stage does not exist. This document is where that gap is now discussed, so
`docs/phase-status.md` does not have to carry it as a permanent asterisk.

## Two questions that are not the same question

The request behind this document is a family-tree import that is reliable. That is mostly
**not** an OCR problem, and treating it as one is how an importer ends up unreliable.

1. **Getting text into the platform.** Where does the text come from? A file the researcher
   already transcribed, a structured genealogy export, a database dump, or a scan that
   something has to read. Only the last one is OCR.
2. **Getting a tree into the platform, correctly.** Given text or structured records, how
   does a person become a row, a relationship become a relationship, and an existing person
   not be duplicated six times? This is validation, identity matching, conflict handling and
   reversibility. It is the part that decides whether an import is trustworthy, and it is
   almost entirely independent of where the characters came from.

A reliable importer built on structured input is achievable now. A reliable importer built on
scanned manuscripts additionally needs a reading step this repository has not evaluated, and
that step introduces a vendor, a credential and a data-sharing question.

## What an import needs to be reliable, whatever the input

These are the properties that make an import trustworthy, and none of them require OCR:

- **A dry run that changes nothing.** Parse, validate and report before any write: how many
  people, how many relationships, how many that would collide with an existing person, and
  what the importer could not interpret. A researcher must be able to see the shape of the
  damage before committing to it.
- **A preview of every write.** The platform's existing rule is that a change is reviewable:
  a claim is a claim, a tree is an interpretation, an AI output is never a fact. An import is
  no exception. It should produce reviewable rows and reviewed records, not a published tree.
- **Explicit conflict handling.** "This is probably the same person as that one" is a
  suggestion for a human, never a silent merge. The existing entity-resolution surface
  already requires explicit confirmation for a merge and can reverse it; an importer should
  produce candidates for that surface, not bypass it.
- **Idempotency and reversibility.** Re-running an import must not duplicate; undoing one must
  not require a database restore. A run identifier, an import record, and a per-run undo are
  the minimum.
- **The existing prohibitions, unchanged.** An import must not publish a tree, accept a claim,
  merge people, or resolve a dispute without an explicit authorized action by a person. Those
  restrictions already bind the research agent (`IMPLEMENTATION_PLAN.md:1599-1607`) and they
  bind an importer for the same reason: bulk input is exactly where an unreviewed conclusion
  would do the most damage at once.
- **Provenance on every row.** Every imported person and relationship should carry where it came
  from, so a later correction or a retraction is possible. The schema already has the
  vocabulary for this (`created_by`, audit events, source statements).

If the import is driven by an LLM rather than a parser, the existing rule applies with no
exception: the model's output is validated, never trusted, and the parts it could not parse
are reported rather than guessed.

## The OCR question, when you take it up

OCR is a decision about an external dependency, and it has three parts that must be decided
together, because any one of them alone makes the answer "no" for a public launch:

1. **Who reads the scan.** A hosted OCR or vision API, a self-hosted model, or a manual
   transcription path. The first two add a vendor, a credential, and a data-egress question
   for documents that may concern **living people** — which `PRODUCT.md` and the plan's own
   security milestones single out as needing explicit privacy rules before broad exposure.
2. **What leaves the machine.** If scans of family records go to a third party, that is a
   data-processing decision, not a technical one, and it needs to be made by a person and
   written down. This repository has no answer to it and should not acquire one by default.
3. **What the platform does with a low-confidence reading.** A genealogy product that renders
   an uncertain name as a fact has broken its own thesis. Low-confidence extractions have to
   land in a reviewable state with the confidence attached, and the review queue has to be
   able to reject them without leaving a trace.

The shape of a defensible answer, when you get to it: OCR produces **candidates**, never rows
that bypass review; the extraction carries page numbers and offsets so a reading is traceable
to the image; and the format contract in `upload.go` becomes a matrix with a confidence
dimension rather than a yes/no list. That is a change to the contract, its error copy, the
worker, and the review surface — and it is exactly why it should be a decision rather than a
quiet extension of the extractor.

## What is deliberately not decided here

- No OCR vendor, model, or credential is proposed or configured.
- No import feature is specified, scheduled, or implied by this document.
- `docs/phase-status.md` continues to record Phase 14 as implemented with the OCR stage
  absent, and now points here rather than carrying the explanation inline.

**The question to decide, when you want to:** does a family-tree import need to read scanned
documents at launch, or is structured and transcribed input enough to prove the feature? That
answer decides whether OCR is on the critical path or a later capability — and it is a
product decision about whose documents may leave the machine.
