// Package testsupport holds the shared PostgreSQL fixtures and the skip audit
// used by the acceptance gate in tools/dbtestguard.
//
// It is test-only support code. Nothing under cmd/ imports it, so it never
// reaches a shipped binary, and the fast `make verify` gate still runs without
// a database because every entry point here reads DatabaseURLEnv first.
//
// This file holds the shared vocabulary the acceptance gate and its audit
// agree on. Keeping the literals here rather than in the tests means the audit
// can look for them without its own source tripping the checks it enforces.
package testsupport

// DatabaseURLEnv is the environment variable that turns the database-backed
// tests on. `make verify` deliberately leaves it unset; `make verify-full` and
// the CI database job set it.
const DatabaseURLEnv = "DATABASE_URL"

// AIResearchURLEnv is the optional second gate on the one AI-backed Go test.
// The deterministic ai-research service needs no credentials, so setting this is
// a convenience rather than a requirement.
const AIResearchURLEnv = "AI_RESEARCH_URL"

// DBSkipMessage is the canonical skip message for a test that needs PostgreSQL.
//
// It is a contract, not a convention. tools/dbtestguard fails the run when a
// test skipped with this message while DATABASE_URL was set, and
// TestDBSkipMessagesStayAuditable fails when a test file gates on DATABASE_URL
// with any other wording. Together they make "the database suite silently
// skipped" a build failure instead of a green tick.
const DBSkipMessage = DatabaseURLEnv + " is not set"

// RequiredExtensions are the extensions db/migrations/0001_extensions.sql needs.
// A database without them cannot host the schema, so the audit reports it as a
// broken environment rather than letting the affected tests skip.
var RequiredExtensions = []string{"pg_trgm", "pgcrypto", "postgis", "vector"}
