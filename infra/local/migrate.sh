#!/bin/sh
set -eu

user="${POSTGRES_USER:-dawha}"
database="${POSTGRES_DB:-dawha}"

psql_exec() {
  docker compose exec -T db psql -U "$user" -d "$database" "$@"
}

container_id="$(docker compose ps -q db)"
attempt=0
until [ "$(docker inspect --format '{{.State.Health.Status}}' "$container_id" 2>/dev/null || true)" = "healthy" ]; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 45 ]; then
    printf '%s\n' "database did not become healthy" >&2
    exit 1
  fi
  sleep 1
done

attempt=0
until psql_exec -c "SELECT 1" >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 30 ]; then
    printf '%s\n' "database did not become ready" >&2
    exit 1
  fi
  sleep 1
done

psql_exec -c "CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());" >/dev/null

for file in db/migrations/*.sql; do
  version="$(basename "$file" .sql)"
  applied="$(psql_exec -Atc "SELECT 1 FROM schema_migrations WHERE version = '$version'")"
  if [ "$applied" = "1" ]; then
    continue
  fi
  psql_exec -v ON_ERROR_STOP=1 < "$file"
  psql_exec -c "INSERT INTO schema_migrations (version) VALUES ('$version') ON CONFLICT (version) DO NOTHING;" >/dev/null
done
