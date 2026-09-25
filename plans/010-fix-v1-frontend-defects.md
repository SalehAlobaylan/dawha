# Plan 010: Fix the V1 frontend defects the browser suite exposed

> **Executor instructions**: This is a small production fix plan. Plan 004 built the browser suite and stopped on a defect it was not allowed to fix; this plan is that fix. Change production code here, keep the assertions honest, and do not weaken the suite to make it pass.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: plan 004
- **Category**: bug
- **Planned at**: commit `fac52ed`, 2026-09-26

## Why this matters

`make e2e` covers the V1 journeys in a real browser. The first full run found three defects that no unit test could see, because they are all in code paths that only exist once a route, a query string, and a real response meet each other. One of them blocks a declared V1 journey outright: an invited collaborator cannot accept an invitation through the UI.

## Current state

- `apps/web/src/routes/invitation.tsx:8-19` — `InvitationPage({ token = "" })` takes the token from a prop, but `apps/web/src/route-tree.tsx:134` registers it as a route component and `@tanstack/react-router` renders route components with no props. The accept button therefore issues `POST /api/v1/invitations//accept` (empty token) and the API answers 404; the page shows the generic failure. `apps/web/src/routes/tree-route.tsx:1-10` already does this correctly with `useParams({ from: "/tree/$treeId" })`.
- `apps/web/e2e/journeys/04-invite-collaborator.spec.ts:110` — the acceptance journey is a `test.fixme` with the diagnosis recorded in the test body. Everything else in that spec (send, pending, revoke, permission copy) passes.
- `apps/web/src/routes/tree.tsx:182-186` — the publish success message reports `published.selectedVersion.number`, which after publishing is the newly opened draft, so publishing version 1 announces "نُشرت النسخة 2".
- `apps/web/src/routes/places.tsx:35` — the place search input is `<input placeholder="ابحث عن موضع" />` with no `value` and no `onChange`, so typing does nothing. `places.tsx:20` binds a period `<select>` to state that never filters anything.

## Commands you will need

| Purpose | Command | Expected |
|---|---|---|
| Fast gate | `make verify` | exit 0 |
| Browser journeys | `COMPOSE_PROJECT_NAME=dawha make e2e` | exit 0, zero fixme |

## Scope

**In scope**:
- `apps/web/src/routes/invitation.tsx`
- `apps/web/src/routes/tree.tsx`
- `apps/web/src/routes/places.tsx`
- `apps/web/e2e/journeys/04-invite-collaborator.spec.ts` and any new assertions
- Unit/component tests in `apps/web` if that is where the behaviour can be pinned cheaply

**Out of scope**:
- Any backend change. The API already accepts the same token; `TestInvitedCollaboratorCanEditTheDraft` proves it.
- Redesigning the pages, adding new features, or refactoring the router.
- Turning the places filter into a real query (that is plan 005's write surface, not a bug fix). Make the control do what its label says, or remove it.

## Git workflow

- Branch: `advisor/010-v1-frontend-defects`.
- One commit per defect. Do not push unless instructed.

## Steps

### Step 1: Read the invitation token from the route

Replace the prop with `useParams({ from: "/invitation/$token" })`, following `apps/web/src/routes/tree-route.tsx`. Handle a missing token explicitly: render the neutral failure state instead of posting an empty token, and keep the existing `returnTo` login link working with the real token.

**Verify**: convert `04-invite-collaborator.spec.ts:110` from `test.fixme` into a real test that accepts an invitation through the browser and asserts the accepted state. The suite must report **zero** fixme and zero skip. Run `make e2e` → exit 0.

### Step 2: Report the version that was actually published

Fix `apps/web/src/routes/tree.tsx:186` so the success message names the published version, not the draft the API opened. Read the published number from the publish response or the version list before the new draft is selected, and do not invent a number: if the response does not carry it, say which version was published from the state you already have.

**Verify**: add an assertion to `02-create-publish-tree.spec.ts` that the banner names the version that was published, and that it is not the number of the draft opened afterwards. Run `make e2e` → exit 0.

### Step 3: Make the place search input do something

Wire `apps/web/src/routes/places.tsx:35` to state and filter the index list it already renders. Keep the existing card markup and selection behaviour. Apply the same treatment to the period control: if it cannot filter with the data the page has, remove it rather than leave a dead control.

**Verify**: add an assertion to `08-browse-map.spec.ts` that typing a place name narrows the list to matching places. Run `make e2e` → exit 0.

### Step 4: Prove the suite is still honest

The point of this plan is that a green suite means something. Confirm the suite still fails when a journey is broken: temporarily break one assertion or one page, watch `make e2e` fail, then restore it. Report the exact failure you observed.

**Verify**: `make verify` exit 0, `make e2e` exit 0 with 0 fixme and 0 skip, `git diff --check` clean.

## Test plan

- Invitation acceptance through the browser: real token, accepted state shown, collaborator gains access.
- Publish banner names the published version, not the new draft.
- Place search narrows the rendered list; the period control is either functional or gone.
- The whole suite still fails when a journey breaks.

## Done criteria

- [ ] An invited collaborator can accept an invitation through the UI.
- [ ] The browser suite has zero `test.fixme` and zero skipped journeys.
- [ ] The publish banner names the published version.
- [ ] The place search input filters, and no dead control is left behind.
- [ ] `make verify` and `make e2e` pass; `git diff --check` passes.
- [ ] `plans/README.md` updated.

## STOP conditions

- Stop if fixing the invitation page requires changing the router configuration or the API contract; report the smallest seam.
- Stop if the publish response genuinely does not identify the published version and no existing state can supply it; report what would have to change instead of guessing.
- Stop if a verification gate fails twice.

## Maintenance notes

The E2E suite is the only thing that caught these. When a route component grows a prop, check how the router actually calls it; TanStack Router passes route params through `useParams`, not props.
