# The browser acceptance suite

Playwright drives the ten V1 journeys declared in `IMPLEMENTATION_PLAN.md` against
the deterministic local stack. Nothing here needs a public deployment or an AI
credential: the `ai-research` service is a deterministic provider, and the map
canvas draws from an inline style with no tile server.

## Running it

```sh
COMPOSE_PROJECT_NAME=dawha make db-migrate db-seed
COMPOSE_PROJECT_NAME=dawha make e2e
```

`COMPOSE_PROJECT_NAME=dawha` reuses the already running `db` container. Without
it, `docker compose` derives the project name from the working directory and
tries to start a second PostgreSQL on the port 55432 the first one publishes.

To keep the run off the database you develop against, point it at a scratch one
in the same container:

```sh
docker exec dawha-db-1 createdb -U dawha -T template_postgis dawha_e2e
COMPOSE_PROJECT_NAME=dawha POSTGRES_DB=dawha_e2e make e2e
```

`make e2e` builds the web app with `VITE_API_URL` pointing at the API, then runs
`npx playwright test`. To run the suite without the Makefile, set the variables
below and build first.

## Demo mode is on here, explicitly

`DEMO_MODE=true` is exported to the stack by `make e2e` and defaulted to `"true"`
in `e2e/stack.mjs`. The setting decides whether the API may serve a static
dashboard and a static tree list at all; with it off, both answer a 503 rather
than a workspace nobody created. The browser suite runs with it on for the same
reason the local development stack does - the app's landing page is part of what a
reader sees - and it is set explicitly rather than relied on, because the default
is off and a suite that quietly depended on it would break the moment anybody
tightened the default.

Every demo response is labelled three ways - an `X-Data-Source: demo` header, a
`mode: "demo"` field and a `demo: true` field - and the web app renders a banner
above the numbers. No journey asserts on the dashboard; journey coverage for demo
mode is in `services/core-api/internal/httpapi/demo_mode_test.go` and in
`apps/web/src/lib/api.test.ts`.

## Rate limits are off here, explicitly

`RATE_LIMIT_ENABLED=false` is exported to the stack by `make e2e` and defaulted to
`false` in `e2e/stack.mjs`. The twenty-eight journeys share one API and one client
address, and the register journey alone would spend the whole sign-in budget of 30
requests a minute, so the browser suite is not a place where a rate limit can be
on.

It is set explicitly rather than left to a default for two reasons. A journey that
quietly came to depend on being unthrottled has to show up in a diff rather than in
a flaky failure, and the E2E stack must never be the reason a production limit was
widened. The limits are proved instead by `services/core-api/platform/ratelimit`,
whose tests assert both that a budget is enforced and that the documented local
demo workflow fits inside every one of them, and by the CI database job, which
runs the same API with the limits on.

## What each service needs

| Service | Port | Environment it requires |
| --- | --- | --- |
| PostgreSQL | 55432 | migrated and seeded by `make db-migrate db-seed`; postgis, pg_trgm, pgcrypto and vector must be present |
| `ai-research` | 8182 | `services/ai-research/.venv`; no credentials, the provider is deterministic |
| `core-api` | 8181 | `DATABASE_URL`, `AI_RESEARCH_URL`, `SOURCE_STORAGE_DIR`, `WEB_ORIGIN`, `CORE_API_PORT`, `RATE_LIMIT_ENABLED`, `DEMO_MODE` |
| source-processing worker | none | the same as `core-api`, plus `SOURCE_WORKER_POLL_INTERVAL`; supervised by the API process and started with it |
| analysis worker | none | the same as `core-api`, plus `AI_RESEARCH_URL` and `ANALYSIS_WORKER_POLL_INTERVAL`; drains the identity-scan and research-investigation job types, and is supervised by the API process and started with it |
| web (`vite preview`) | 4173 | `apps/web/dist` built with `VITE_API_URL=http://localhost:8181` |

`WEB_ORIGIN` has to match the web origin exactly, or the browser is refused the
credentialed request that carries the session cookie. Both origins are `localhost`
so the cookie stays same-site; only the port differs.

The API and AI ports are deliberately **not** the development ports 8080 and
8000. A developer with `npm run dev` running already has servers there with a
different `WEB_ORIGIN` and a different `DATABASE_URL`, so `reuseExistingServer`
is off everywhere: a busy port is a loud failure rather than a pass against
somebody else's process.

## The journeys

| File | Journey |
| --- | --- |
| `journeys/01-register.spec.ts` | register, and keep a live session |
| `journeys/02-create-publish-tree.spec.ts` | create a tree, publish version 1 |
| `journeys/03-fork-tree.spec.ts` | fork a published version |
| `journeys/04-invite-collaborator.spec.ts` | invite a collaborator, revoke one, accept one as the invited researcher |
| `journeys/05-attach-source.spec.ts` | attach a `.txt` source and watch it reach a terminal processing run; refuse an unsupported format |
| `journeys/06-suggestion.spec.ts` | submit a public suggestion, review it |
| `journeys/07-question-dispute.spec.ts` | open a question, record a dispute |
| `journeys/08-browse-map.spec.ts` | browse the map |
| `journeys/09-research-query.spec.ts` | run a research query |

## Isolation

Every test registers its own account with a unique address, so no journey can
read another's tree, source or suggestion even though they share one database.
The suite runs one worker for the same reason: the journeys deliberately create
public trees and public suggestions that a second worker would see as its own.

A run also cleans up after itself. Those accounts own real rows - trees, versions,
nodes, sources, uploaded files, questions, invitations, an audit trail - and
before this existed they stayed in the database the run was pointed at, so every
`make e2e` grew the development database by ~1,500 rows. Now:

- `e2e/cleanup.sql` is the only place the deletes live. It removes exactly the
  rows reachable from the synthetic `e2e-*@example.invalid` accounts and nothing
  else, in dependency order, inside one transaction. The order is not decoration:
  about fifty tables reference `users(id)` without `ON DELETE CASCADE`, so
  `DELETE FROM users` on its own would abort. Tables that do cascade are left to
  cascade, and the transaction ends by asserting that no row was left without its
  parent and that no foreign key is deferrable or `NOT VALID` (which is what
  would make an ordered delete insufficient). A failure rolls the whole thing
  back, so a cleanup that cannot finish leaves the database exactly as it found
  it.
- `e2e/global-teardown.ts` runs that file after every run, through `pg`, and
  prints what it removed plus the count of users it did not touch. Audit rows
  are matched on the entity as well as the actor, because the processing worker
  and a signed-out visitor both write audit rows with no actor. Uploaded files
  under `SOURCE_STORAGE_DIR/sources/<source id>` go with their source.
- `make e2e-clean` runs the same file through `psql`, for the runs a teardown
  cannot follow: a run killed mid-flight, a stopped container, or the rows older
  versions of this suite left behind. A teardown failure prints this command.

```sh
COMPOSE_PROJECT_NAME=dawha make e2e-clean
```

One row class is deliberately left behind, and the teardown says so at the end
of every run: a research run started by a signed-out visitor has no `actor_id`
and no `question_id`, so nothing links it to an `e2e-*` account. The only way to
find it is a time window, and a time window is how a cleanup ends up deleting
somebody's own work - one such row, from a developer's own session, is in the
development database right now. It is counted and reported instead:

```sql
DELETE FROM research_runs WHERE actor_id IS NULL AND question_id IS NULL;
```

Point `POSTGRES_DB` at a scratch database, as above, and even that residue lands
somewhere disposable.
