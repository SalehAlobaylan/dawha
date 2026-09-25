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

## What each service needs

| Service | Port | Environment it requires |
| --- | --- | --- |
| PostgreSQL | 55432 | migrated and seeded by `make db-migrate db-seed`; postgis, pg_trgm, pgcrypto and vector must be present |
| `ai-research` | 8182 | `services/ai-research/.venv`; no credentials, the provider is deterministic |
| `core-api` | 8181 | `DATABASE_URL`, `AI_RESEARCH_URL`, `SOURCE_STORAGE_DIR`, `WEB_ORIGIN`, `CORE_API_PORT` |
| source-processing worker | none | the same as `core-api`, plus `SOURCE_WORKER_POLL_INTERVAL`; supervised by the API process and started with it |
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
| `journeys/04-invite-collaborator.spec.ts` | invite a collaborator, revoke one; accepting is a recorded fixme, see the test |
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
Nothing is deleted at the end of a run; a synthetic row left by a failed run
cannot collide with the next one, and it is the only record that the journey ran.
