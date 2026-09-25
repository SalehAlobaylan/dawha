# Plan 005: Complete the Phase 2 identity CRUD surface

> **Executor instructions**: Implement only the explicit CRUD surface described here. Do not invent automatic identity resolution or historical truth.

## Status

- **Priority**: P1
- **Effort**: L
- **Risk**: HIGH
- **Depends on**: plans/001-enforce-resource-visibility.md
- **Category**: direction
- **Planned at**: commit `a6397ae`, 2026-09-25

## Why this matters

`IMPLEMENTATION_PLAN.md:305-418` requires create/edit people, aliases, families, tribes, branches, places, and generic entity relationships. The current API exposes only tree-scoped person and relationship mutations (`router.go:117-119`), while the rest of the product relies on seeded or indirectly-created identity rows. This leaves the V1 dictionary, search, map, and research workspace dependent on incomplete write surfaces.

## Current state

- `db/migrations/0002_identity_and_trees.sql:24-84` defines `people`, aliases, `families`, and `tribes` tables.
- `router.go:117-120` exposes add person, add relationship, update relationship, and publish; no CRUD routes for aliases, families, tribes, branches, places, or generic entity relationships.
- `services/core-api/internal/trees/service.go:162-180` creates people only while constructing a tree.
- Existing normalization conventions are in `services/core-api/internal/identity/normalization.go`; existing permission checks are in `services/core-api/internal/auth/permissions.go`.
- `PRODUCT.md:1397-1415` requires explicit privacy rules for living/unknown people before broad public exposure.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Focused | `go test ./internal/identity ./internal/trees ./internal/auth ./internal/httpapi -count=1` | all pass |
| Full | `make verify` | exit 0 |
| DB | `DATABASE_URL='postgres://dawha:dawha_local@localhost:55432/dawha' go test ./... -count=1` | all pass |

## Scope

**In scope**:
- Identity/alias/family/tribe/branch/place services and handlers
- Generic entity relationship creation/update/delete where the plan requires it
- Authorization and audit tests
- `apps/web` forms only where required for the new CRUD surface
- A migration only if an explicit field or constraint is required

**Out of scope**:
- Automatic identity merging (plan/entity-resolution behavior).
- New living-person policy beyond the visibility contract from plan 001.
- Tree publication rules or relationship impact algorithms.

## Git workflow

- Branch: `advisor/005-identity-crud`.
- Commit per entity family; do not push unless instructed.

## Steps

### Step 1: Inventory the existing domain schema and permissions

Write a table mapping each Phase 2 entity/alias table to its current read path, expected mutation, owner/permission, and audit action. Reuse existing services and do not create a second generic repository layer.

**Verify**: the table is reviewed in the PR description and every entity has a named handler/service target.

### Step 2: Add permission-checked CRUD services

Implement create/update/delete/list operations for people, aliases, families, tribes, branches, and places. Validate Arabic names, optional dates, alias types, and place geometry through existing domain validators. Use one transaction for the entity, alias/relationship changes, and audit event.

**Verify**: table-driven unit tests for validation and role/owner/collaborator permissions; focused `go test` passes.

### Step 3: Add API routes and request contracts

Register explicit routes with bounded request bodies and stable JSON error mapping. Do not expose raw SQL mass assignment. Ensure direct-ID reads use the plan-001 visibility policy.

**Verify**: HTTP tests cover unauthenticated denial, owner success, collaborator permission, unrelated denial, invalid UUID/date/alias, and audit creation.

### Step 4: Add minimal UI flows

Add forms only for the core identity operations needed by the existing workspace. Reuse current Arabic form components and React Query invalidation patterns. Do not build a large new admin surface in this plan.

**Verify**: `npm run lint && npm run typecheck && npm run test` → all pass.

### Step 5: Run full acceptance tests

Run the DB suite and the V1 E2E suite from plan 004 for tree/dictionary/search/map flows.

**Verify**: `make verify-full` passes and `git diff --check` is clean.

## Test plan

- CRUD happy paths for every entity.
- Alias normalization and uniqueness conflicts.
- Approximate dates and optional geometry.
- Permission/ownership/collaborator matrix.
- Audit rollback when the second write fails.
- Private-draft/public-published visibility regression from plan 001.

## Done criteria

- [ ] Phase 2 entity CRUD routes and services exist.
- [ ] Every mutation is authorized and audited atomically.
- [ ] Direct-ID reads obey the visibility policy.
- [ ] Arabic normalization and approximate dates are preserved.
- [ ] Full verification and DB/E2E tests pass.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if the requested CRUD would require exposing raw entity IDs without a visibility policy.
- Stop if a family/tribe/place operation would silently mutate a published tree.
- Stop if a verification gate fails twice.

## Maintenance notes

Every new identity type needs a dictionary/search visibility rule and an audit action. Reviewers should verify that tree-scoped writes and standalone identity writes cannot bypass one another.
