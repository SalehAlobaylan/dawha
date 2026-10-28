#!/usr/bin/env python3
"""Validate an isolated name-pipeline test environment before running a gate."""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import tempfile
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]
MARKER = ".dawha-name-pipeline-test-storage"
MARKER_CONTENT = "dawha-name-pipeline-test-storage-v1\n"


class InvalidTestEnvironment(ValueError):
    pass


def validate_environment(environment: dict[str, str]) -> dict[str, str]:
    name = environment.get("PIPELINE_TEST_DB_NAME", "")
    if not re.fullmatch(r"[a-z][a-z0-9_]*_pipeline_test", name) or len(name) > 63:
        raise InvalidTestEnvironment("a dedicated *_pipeline_test database is required")
    raw_url = environment.get("PIPELINE_TEST_DATABASE_URL", "")
    try:
        url = urlsplit(raw_url)
        port = url.port or 5432
        valid = (
            url.scheme in {"postgres", "postgresql"}
            and url.hostname in {"localhost", "127.0.0.1", "::1"}
            and url.path == "/" + name
            and not url.query
            and not url.fragment
            and bool(url.username)
            and 0 < port < 65536
        )
    except ValueError:
        valid = False
    if not valid:
        raise InvalidTestEnvironment("an exact local test URL without overrides is required")

    raw_storage = environment.get("PIPELINE_TEST_STORAGE_DIR", "")
    storage = Path(raw_storage)
    try:
        resolved = storage.resolve(strict=True)
        temporary_root = Path(tempfile.gettempdir()).resolve()
        valid_storage = (
            storage.is_absolute()
            and storage.is_dir()
            and not storage.is_symlink()
            and resolved.is_relative_to(temporary_root)
            and resolved.name.startswith("dawha-pipeline-test.")
            and resolved != (ROOT / ".data/source-storage").resolve()
        )
        marker = resolved / MARKER
        if marker.exists() or marker.is_symlink():
            valid_storage = valid_storage and not marker.is_symlink() and marker.read_text() == MARKER_CONTENT
        elif any(resolved.iterdir()):
            valid_storage = False
    except (OSError, ValueError):
        valid_storage = False
    if not valid_storage:
        raise InvalidTestEnvironment("fresh dedicated temporary test storage is required")

    # libpq/pgx connection environment and existing test overrides must not
    # redirect a validated URL or make a test touch the review storage.
    child = {
        key: value for key, value in environment.items()
        if not key.startswith("PG") and not key.startswith("POSTGRES_")
        and key not in {
            "DATABASE_URL", "SOURCE_STORAGE_DIR", "E2E_DATABASE_URL",
            "AI_RESEARCH_URL", "DEEPSEEK_API_KEY", "JEV_API_KEY", "LIQUID_API_KEY",
            "PYTHONPATH", "GOFLAGS", "MAKEFLAGS", "MFLAGS", "MAKEOVERRIDES",
            "MIGRATIONS_DIR", "SEEDS_DIR", "STORAGE_DRIVER", "STORAGE_TEST_ENDPOINT", "VITE_API_URL",
            "E2E_API_PORT", "E2E_AI_PORT", "E2E_PORT",
        }
    }
    child.update({
        "DATABASE_URL": raw_url,
        # Shared-table contract tests must not race another package's fixtures.
        "GOFLAGS": "-p=1",
        "POSTGRES_DB": name,
        "POSTGRES_USER": unquote(url.username or ""),
        "POSTGRES_PASSWORD": unquote(url.password or ""),
        "POSTGRES_HOST": url.hostname or "localhost",
        "POSTGRES_PORT": str(port),
        "SOURCE_STORAGE_DIR": str(resolved),
        "STORAGE_DRIVER": "local",
        "E2E_API_PORT": "8181",
        "E2E_AI_PORT": "8182",
        "E2E_PORT": "4173",
        "COMPOSE_PROJECT_NAME": "dawha",
        "NAME_READER_MODE": "off",
        "CHART_VISION_ENABLED": "false",
        "NAME_JUDGE_MODE": "off",
        "NAME_JUDGE_IMAGES": "false",
    })
    return child


def gate_command(gate: str, packages: str) -> tuple[list[str], Path]:
    if gate in {"verify-full", "e2e"}:
        return ["make", gate], ROOT
    if gate != "packages":
        raise InvalidTestEnvironment("unknown test gate")
    try:
        selected = shlex.split(packages or "./...")
    except ValueError as error:
        raise InvalidTestEnvironment("invalid test package list") from error
    if not selected or len(selected) > 100 or any(
        not re.fullmatch(r"\./[A-Za-z0-9_./-]+", package)
        or ".." in package.split("/") for package in selected
    ):
        raise InvalidTestEnvironment("test packages must be local Go package paths")
    return ["go", "run", "./tools/dbtestguard", "--", *selected], ROOT / "services/core-api"


def selected_required_packages(packages: list[str]) -> str:
    module = "github.com/SalehAlobaylan/dawha/services/core-api"
    manifest = ROOT / "services/core-api/internal/testsupport/testdata/db-required-packages.txt"
    required = manifest.read_text().splitlines()
    if "./..." in packages:
        return "\n".join(required) + "\n"
    selected = []
    for name in required:
        for package in packages:
            prefix = module + "/" + package[2:]
            if name == prefix or (prefix.endswith("/...") and name.startswith(prefix[:-3])):
                selected.append(name)
                break
    return "\n".join(selected) + "\n"


def validate_compose_port(child: dict[str, str]) -> None:
    # Migrations use Compose's running PostgreSQL, while Go uses the URL.
    # Prove their local port is the same before migrating or seeding anything.
    try:
        result = subprocess.run(["docker", "compose", "port", "db", "5432"], cwd=ROOT,
                                env=child, capture_output=True, text=True, timeout=5, check=False)
        ports = {line.rsplit(":", 1)[-1].strip() for line in result.stdout.splitlines()}
        if result.returncode != 0 or child["POSTGRES_PORT"] not in ports:
            raise InvalidTestEnvironment("test URL must address the running Compose database port")
    except (OSError, subprocess.SubprocessError) as error:
        raise InvalidTestEnvironment("the existing local Compose database service is required") from error


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--gate", default=os.getenv("NAME_PIPELINE_TEST_GATE", "packages"))
    parser.add_argument("--check-only", action="store_true")
    arguments = parser.parse_args(argv)
    try:
        child = validate_environment(dict(os.environ))
        command, cwd = gate_command(arguments.gate, os.getenv("NAME_PIPELINE_TEST_PACKAGES", "./..."))
    except InvalidTestEnvironment as error:
        print(f"name-pipeline test guard: {error}", file=sys.stderr)
        return 2
    if arguments.check_only:
        print(json.dumps({"valid": True, "gate": arguments.gate}))
        return 0
    if arguments.gate in {"verify-full", "e2e"}:
        try:
            validate_compose_port(child)
        except InvalidTestEnvironment as error:
            print(f"name-pipeline test guard: {error}", file=sys.stderr)
            return 2
    marker = Path(child["SOURCE_STORAGE_DIR"]) / MARKER
    if not marker.exists():
        descriptor = os.open(marker, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "w") as stream:
            stream.write(MARKER_CONTENT)
    if arguments.gate == "packages":
        selected = command[command.index("--") + 1:]
        manifest = Path(child["SOURCE_STORAGE_DIR"]) / ".required-db-packages.txt"
        manifest.write_text(selected_required_packages(selected))
        manifest.chmod(0o600)
        command[command.index("--"):command.index("--")] = ["--require", str(manifest)]
    # The local DB service must already be running. Skip db-up so a different
    # POSTGRES_DB cannot make Compose reconfigure/restart the shared container.
    # verify-full runs unit/DB tests before its own migration prerequisites.
    # Bootstrap the validated, already-created database first, never create it.
    if arguments.gate in {"verify-full", "e2e"}:
        prepared = subprocess.run(["make", "db-migrate", "db-seed"], cwd=ROOT, env=child, check=False)
        if prepared.returncode:
            return prepared.returncode
    return subprocess.run(command, cwd=cwd, env=child, check=False).returncode


if __name__ == "__main__":
    raise SystemExit(main())
