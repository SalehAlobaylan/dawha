#!/bin/sh
# The five migration cases the plan asks for, proved against a real PostgreSQL:
#
#   1. success    a fresh database takes every migration, and every applied row
#                 carries the checksum of the file it came from.
#   2. failure    a migration that fails halfway leaves no table and no version
#                 row, and the database is still usable afterwards.
#   3. rerun      a second run over the same database writes nothing, and a run
#                 over a partially migrated database resumes and completes.
#   4. checksum   an edit to an already-applied migration is refused; a row with
#                 no recorded checksum - the state every pre-existing database is
#                 in - is backfilled from the file, and the backfill is recorded
#                 as such.
#   5. concurrent two runners started together: the second WAITS on the lock -
#                 read out of pg_locks, not inferred from a duration - and the
#                 migration is applied exactly once.
#
# It needs a database and a container, so it is deliberately NOT part of
# `make verify`. `make verify-full` and the CI database job run it, because the
# properties it checks are exactly the ones a unit test cannot fake.
#
#   COMPOSE_PROJECT_NAME=dawha infra/local/migration_test.sh
#
# Every case runs against a scratch database this script creates and drops. It
# never points at the database in POSTGRES_DB.
set -eu

repository_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
migrations="$repository_root/db/migrations"
runner="$repository_root/infra/local/migrate.sh"

user="${POSTGRES_USER:-dawha}"
admin="${MIGRATION_TEST_ADMIN_DB:-postgres}"
scratch="dawha_migration_test_$$"
work="$(mktemp -d)"
failures=0

# How long the concurrency fixture holds its transaction. Long enough that the
# poller can see a runner waiting on it, short enough to keep this case quick.
concurrency_sleep_seconds=3

admin_exec() {
  docker compose exec -T db psql -U "$user" -d "$admin" -v ON_ERROR_STOP=1 "$@"
}

scalar() {
  docker compose exec -T db psql -U "$user" -d "$scratch" -Atc "$1"
}

cleanup() {
  admin_exec -q -c "DROP DATABASE IF EXISTS $scratch WITH (FORCE);" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT
trap 'cleanup; exit 130' INT TERM

start_case() {
  printf '\n== case %s ==\n' "$1"
}

pass() {
  printf '   ok   %s\n' "$1"
}

fail() {
  printf '   FAIL %s\n' "$1" >&2
  failures=$((failures + 1))
}

assert_equal() {
  if [ "$2" = "$3" ]; then
    pass "$1 ($2)"
  else
    fail "$1: expected [$3], observed [$2]"
  fi
}

assert_nonzero_exit() {
  if [ "$2" -ne 0 ]; then
    pass "$1 (exit $2)"
  else
    fail "$1: expected a non-zero exit, observed 0"
  fi
}

# Usage: count_matches PATTERN [FILE]. grep exits non-zero on zero matches, and
# an assertion that reads "expected 0, observed nothing" is worse than one that
# says zero, so the count is always printed.
count_matches() {
  if [ "$#" -ge 2 ]; then
    grep -c "$1" "$2" 2>/dev/null || true
  else
    grep -c "$1" || true
  fi
}

# The same three-tool hash dance the runner uses, so the test's expectation is
# computed exactly the way the runner computes the value it is checking.
checksum_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d' ' -f1
  else
    openssl dgst -sha256 "$1" | sed 's/^.*= *//'
  fi
}

# The real runner, against the scratch database, with only MIGRATIONS_DIR moved.
# Never a modified copy of the runner: the properties under test are this file's.
run_migrate() {
  (cd "$repository_root" && MIGRATIONS_DIR="$1" POSTGRES_DB="$scratch" POSTGRES_USER="$user" sh "$runner" "${2:-apply}")
}

# The runner's exit status, without tripping `set -e` on the failure path.
migrate_status() {
  if run_migrate "$1" "${2:-apply}" >/dev/null 2>&1; then
    printf '0'
  else
    printf '%s' "$?"
  fi
}

migration_count="$(ls "$migrations"/*.sql | wc -l | tr -d ' ')"
first_checksum="$(checksum_of "$migrations/0001_extensions.sql")"

docker compose up -d db >/dev/null
attempt=0
# `up -d` returns before PostgreSQL accepts connections, and this script's very
# next statement is a connection. The runner waits for readiness for its own
# reasons; the first statement here happens before any runner is involved.
until admin_exec -Atc "SELECT 1" >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 60 ]; then
    printf 'the database never became reachable\n' >&2
    exit 1
  fi
  sleep 1
done
admin_exec -q -c "DROP DATABASE IF EXISTS $scratch WITH (FORCE);" >/dev/null
admin_exec -q -c "CREATE DATABASE $scratch;" >/dev/null

# ---------------------------------------------------------------- 1. success
start_case "1. success: a fresh database takes every migration, checksummed"
if first_output="$(run_migrate "$migrations" 2>&1)"; then
  pass "the first run over an empty database succeeded"
else
  printf '%s\n' "$first_output" >&2
  fail "the first run over an empty database failed"
fi
assert_equal "migrations applied" "$(scalar "SELECT count(*) FROM schema_migrations")" "$migration_count"
assert_equal "applied rows carry a checksum recorded at apply time" \
  "$(scalar "SELECT count(*) FROM schema_migrations WHERE checksum IS NULL OR checksum_origin <> 'applied_with_file' OR checksum_recorded_at IS NULL")" \
  "0"
assert_equal "and the checksum is the file's own sha256" \
  "$(scalar "SELECT checksum FROM schema_migrations WHERE version = '0001_extensions'")" "$first_checksum"
assert_equal "the drift gate agrees with the database" "$(migrate_status "$migrations" check)" "0"

# ---------------------------------------------------------------- 2. failure
start_case "2. failure: a failing migration leaves no partial schema and no version row"
mkdir -p "$work/case2"
cp "$migrations"/*.sql "$work/case2/"
# The first statement succeeds and the second fails. If the runner applied the
# file outside a transaction the table would survive; if it recorded the version
# in a second call the row would too.
cat >"$work/case2/9999_deliberate_failure.sql" <<'SQL'
-- Plan 008 case 2: a migration that cannot finish.
CREATE TABLE dawha_case2_partial_marker (id integer PRIMARY KEY);
SELECT 1 / 0;
SQL
if case2_output="$(run_migrate "$work/case2" 2>&1)"; then
  case2_status=0
else
  case2_status=$?
fi
assert_nonzero_exit "the run fails" "$case2_status"
if printf '%s' "$case2_output" | grep -q 'was rolled back; the database is unchanged'; then
  pass "and it says the file was rolled back"
else
  fail "the failure did not say what happened: $case2_output"
fi
assert_equal "the table the failed migration created is gone" \
  "$(scalar "SELECT count(*) FROM information_schema.tables WHERE table_name = 'dawha_case2_partial_marker'")" "0"
assert_equal "no version row was written for it" \
  "$(scalar "SELECT count(*) FROM schema_migrations WHERE version = '9999_deliberate_failure'")" "0"
assert_equal "the migrations before it are still applied" \
  "$(scalar "SELECT count(*) FROM schema_migrations")" "$migration_count"
rm "$work/case2/9999_deliberate_failure.sql"
assert_equal "the same database still migrates afterwards" "$(migrate_status "$work/case2")" "0"

# ------------------------------------------------------------------ 3. rerun
start_case "3. rerun: a second run writes nothing, and a partial set resumes"
if second_output="$(run_migrate "$migrations" 2>&1)"; then
  pass "the second run over an already migrated database succeeded"
else
  printf '%s\n' "$second_output" >&2
  fail "the second run over an already migrated database failed"
fi
assert_equal "the second run applies nothing" "$(printf '%s\n' "$second_output" | count_matches '^applied ')" "0"
assert_equal "the second run backfills nothing" "$(printf '%s\n' "$second_output" | count_matches '^backfilled ')" "0"
assert_equal "and the row count is unchanged" "$(scalar "SELECT count(*) FROM schema_migrations")" "$migration_count"

admin_exec -q -c "DROP DATABASE $scratch WITH (FORCE);" >/dev/null
admin_exec -q -c "CREATE DATABASE $scratch;" >/dev/null
mkdir -p "$work/case3"
i=0
for file in "$migrations"/*.sql; do
  i=$((i + 1))
  [ "$i" -gt 4 ] && break
  cp "$file" "$work/case3/"
done
assert_equal "the partial set migrated cleanly" "$(migrate_status "$work/case3")" "0"
assert_equal "and applied four of them" "$(scalar "SELECT count(*) FROM schema_migrations")" "4"
if resume_output="$(run_migrate "$migrations" 2>&1)"; then
  pass "the resuming run succeeded"
else
  printf '%s\n' "$resume_output" >&2
  fail "the resuming run failed"
fi
assert_equal "the resuming run applied only what was missing" \
  "$(printf '%s\n' "$resume_output" | count_matches '^applied ')" "$((migration_count - 4))"
assert_equal "the resumed database is complete" "$(scalar "SELECT count(*) FROM schema_migrations")" "$migration_count"

# --------------------------------------------------------------- 4. checksum
start_case "4. checksum: an edit is refused, and a pre-existing row is backfilled"
# This is the state every database in existence is in on the day the checksum
# arrives: applied, with nothing recorded about which bytes were applied.
scalar "UPDATE schema_migrations SET checksum = NULL, checksum_origin = NULL, checksum_recorded_at = NULL;" >/dev/null
if backfill_output="$(run_migrate "$migrations" 2>&1)"; then
  pass "a database with no recorded checksums still migrates"
else
  printf '%s\n' "$backfill_output" >&2
  fail "backfilling a database with no recorded checksums failed"
fi
assert_equal "every row was backfilled" "$(scalar "SELECT count(*) FROM schema_migrations WHERE checksum IS NULL")" "0"
assert_equal "the backfill says where it came from" \
  "$(scalar "SELECT DISTINCT checksum_origin FROM schema_migrations")" "backfilled_from_file"
assert_equal "and it is timestamped" "$(scalar "SELECT count(*) FROM schema_migrations WHERE checksum_recorded_at IS NULL")" "0"
assert_equal "and the checksums are the files' own" \
  "$(scalar "SELECT checksum FROM schema_migrations WHERE version = '0001_extensions'")" "$first_checksum"
assert_equal "backfilling reports itself once per file" \
  "$(printf '%s\n' "$backfill_output" | count_matches '^backfilled ')" "$migration_count"

mkdir -p "$work/case4"
cp "$migrations"/*.sql "$work/case4/"
printf '\n-- edited after it was applied\n' >>"$work/case4/0002_identity_and_trees.sql"
if edit_output="$(run_migrate "$work/case4" 2>&1)"; then
  edit_status=0
else
  edit_status=$?
fi
assert_nonzero_exit "editing an applied migration fails the run" "$edit_status"
if printf '%s' "$edit_output" | grep -q 'was applied as checksum'; then
  pass "the failure names the migration and both checksums"
else
  fail "the failure did not explain itself: $edit_output"
fi
assert_equal "the recorded checksum is untouched by the refusal" \
  "$(scalar "SELECT count(*) FROM schema_migrations WHERE checksum IS NULL")" "0"
assert_equal "a refused edit half-applies nothing" \
  "$(scalar "SELECT count(*) FROM schema_migrations")" "$migration_count"
assert_equal "the drift gate reports the same edit" "$(migrate_status "$work/case4" check)" "1"

# -------------------------------------------------------------- 5. concurrent
start_case "5. concurrent: a second runner waits on the lock instead of racing"
admin_exec -q -c "DROP DATABASE $scratch WITH (FORCE);" >/dev/null
admin_exec -q -c "CREATE DATABASE $scratch;" >/dev/null
mkdir -p "$work/case5"
cp "$migrations"/*.sql "$work/case5/"
cat >"$work/case5/0043_case5_slow_marker.sql" <<SQL
-- Plan 008 case 5: holds its transaction long enough for a wait to be visible.
SELECT pg_sleep($concurrency_sleep_seconds);
CREATE TABLE dawha_case5_marker (id integer PRIMARY KEY);
SQL

run_migrate "$work/case5" >"$work/first.log" 2>&1 &
first_pid=$!
sleep 1
run_migrate "$work/case5" >"$work/second.log" 2>&1 &
second_pid=$!

# The wait is read out of pg_locks rather than inferred from a duration, because a
# duration can be matched by two runners that simply raced through the same slow
# migration, and racing is the failure this case exists to catch.
saw_wait=0
polls=0
while kill -0 "$first_pid" 2>/dev/null || kill -0 "$second_pid" 2>/dev/null; do
  waiting="$(admin_exec -Atc "SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted" 2>/dev/null || printf '0')"
  case "$waiting" in
    '' | *[!0-9]*) waiting=0 ;;
  esac
  if [ "$waiting" -gt 0 ]; then
    saw_wait=1
  fi
  polls=$((polls + 1))
  if [ "$polls" -gt 900 ]; then
    break
  fi
  sleep 0.1
done
if wait "$first_pid"; then first_status=0; else first_status=$?; fi
if wait "$second_pid"; then second_status=0; else second_status=$?; fi

assert_equal "the first runner succeeded" "$first_status" "0"
assert_equal "the second runner succeeded" "$second_status" "0"
if [ "$saw_wait" = "1" ]; then
  pass "pg_locks held an ungranted advisory lock, so a runner really waited ($polls samples)"
else
  fail "no ungranted advisory lock was ever observed, so the two runners were not serialized"
fi
assert_equal "the second runner recorded no checksum" "$(count_matches '^backfilled ' "$work/second.log")" "0"
# Across both runners every migration is applied exactly once. Without the lock
# both would run the slow migration and one would lose on the version row; with
# it, the loser reports that it skipped and the row count stays at one.
total_applied=$(( $(count_matches '^applied ' "$work/first.log") + $(count_matches '^applied ' "$work/second.log") ))
assert_equal "the two runners together applied every migration exactly once" \
  "$total_applied" "$((migration_count + 1))"
if [ "$(count_matches '^skipped ' "$work/second.log")" -gt 0 ]; then
  pass "the second runner reported skipping what the first had already applied"
else
  fail "the second runner never met an already-applied migration, so it never waited on one either"
fi
assert_equal "the slow migration is applied exactly once" \
  "$(scalar "SELECT count(*) FROM schema_migrations WHERE version = '0043_case5_slow_marker'")" "1"
assert_equal "its table exists once" \
  "$(scalar "SELECT count(*) FROM information_schema.tables WHERE table_name = 'dawha_case5_marker'")" "1"
assert_equal "the whole set is applied exactly once" \
  "$(scalar "SELECT count(*) FROM schema_migrations")" "$((migration_count + 1))"
assert_equal "and the two runners agree with the drift gate" "$(migrate_status "$work/case5" check)" "0"

printf '\n'
if [ "$failures" -ne 0 ]; then
  printf 'migration test: %s assertion(s) failed\n' "$failures" >&2
  exit 1
fi
printf 'migration test: all five cases passed against %s\n' "$scratch"
