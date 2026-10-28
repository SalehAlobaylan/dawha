import importlib.util
import os
from pathlib import Path
import tempfile
import subprocess
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("pipeline_guard", Path(__file__).with_name("check-name-pipeline-test-env.py"))
guard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guard)


class PipelineTestEnvironmentTests(unittest.TestCase):
    def setUp(self):
        self.storage = tempfile.TemporaryDirectory(prefix="dawha-pipeline-test.")
        self.addCleanup(self.storage.cleanup)
        self.environment = {
            "PIPELINE_TEST_DB_NAME": "review_pipeline_test",
            "PIPELINE_TEST_DATABASE_URL": "postgres://tester:fixture@localhost:55432/review_pipeline_test",
            "PIPELINE_TEST_STORAGE_DIR": self.storage.name,
            "PATH": os.environ["PATH"],
        }

    def test_valid_environment_derives_every_database_and_storage_setting(self):
        child = guard.validate_environment(self.environment)
        self.assertEqual(child["POSTGRES_DB"], "review_pipeline_test")
        self.assertEqual(child["DATABASE_URL"], self.environment["PIPELINE_TEST_DATABASE_URL"])
        self.assertEqual(child["SOURCE_STORAGE_DIR"], str(Path(self.storage.name).resolve()))
        self.assertEqual(child["NAME_READER_MODE"], "off")
        self.assertEqual(child["CHART_VISION_ENABLED"], "false")
        self.assertEqual(child["GOFLAGS"], "-p=1")

    def test_connection_overrides_and_live_provider_credentials_are_not_inherited(self):
        dirty = dict(self.environment, PGHOST="elsewhere", PGDATABASE="dawha", PGSERVICE="live",
                     PGOPTIONS="override", DATABASE_URL="live", SOURCE_STORAGE_DIR="live",
                     POSTGRES_DB="dawha", E2E_DATABASE_URL="live", AI_RESEARCH_URL="live",
                     DEEPSEEK_API_KEY="fixture", PYTHONPATH="private-runtime",
                     GOFLAGS="-p=8", STORAGE_DRIVER="s3", VITE_API_URL="http://localhost:8080", MAKEFLAGS="POSTGRES_DB=dawha")
        child = guard.validate_environment(dirty)
        for key in ("PGHOST", "PGDATABASE", "PGSERVICE", "PGOPTIONS", "E2E_DATABASE_URL",
                    "AI_RESEARCH_URL", "DEEPSEEK_API_KEY", "PYTHONPATH", "VITE_API_URL", "MAKEFLAGS"):
            self.assertNotIn(key, child)
        self.assertEqual(child["POSTGRES_DB"], "review_pipeline_test")
        self.assertEqual(child["STORAGE_DRIVER"], "local")
        self.assertEqual(child["E2E_API_PORT"], "8181")

    def test_rejects_missing_real_mismatched_remote_and_driver_override_urls(self):
        invalid = [
            {}, {"PIPELINE_TEST_DB_NAME": "dawha"},
            {"PIPELINE_TEST_DATABASE_URL": "postgres://tester@localhost/dawha"},
            {"PIPELINE_TEST_DATABASE_URL": "postgres://tester@remote/review_pipeline_test"},
            {"PIPELINE_TEST_DATABASE_URL": "postgres://tester@localhost/review_pipeline_test?dbname=dawha"},
            {"PIPELINE_TEST_DATABASE_URL": "postgres://tester@localhost/review_pipeline_test?host=remote"},
            {"PIPELINE_TEST_DATABASE_URL": "postgres://tester@localhost/review_pipeline_test?service=live"},
            {"PIPELINE_TEST_DATABASE_URL": "postgres://tester@localhost/review_pipeline_test#override"},
        ]
        for values in invalid:
            environment = {**self.environment, **values} if values else {}
            with self.subTest(values=tuple(values)):
                with self.assertRaises(guard.InvalidTestEnvironment):
                    guard.validate_environment(environment)

    def test_rejects_nonempty_unmarked_or_symlinked_storage(self):
        Path(self.storage.name, "existing-source").write_text("fixture")
        with self.assertRaises(guard.InvalidTestEnvironment):
            guard.validate_environment(self.environment)
        Path(self.storage.name, "existing-source").unlink()
        link = Path(self.storage.name, "redirect")
        link.symlink_to(self.storage.name, target_is_directory=True)
        with self.assertRaises(guard.InvalidTestEnvironment):
            guard.validate_environment({**self.environment, "PIPELINE_TEST_STORAGE_DIR": str(link)})

    def test_rejects_missing_each_required_variable_and_real_storage(self):
        for key in ("PIPELINE_TEST_DB_NAME", "PIPELINE_TEST_DATABASE_URL", "PIPELINE_TEST_STORAGE_DIR"):
            with self.subTest(key=key):
                with self.assertRaises(guard.InvalidTestEnvironment):
                    guard.validate_environment({**self.environment, key: ""})
        with self.assertRaises(guard.InvalidTestEnvironment):
            guard.validate_environment({**self.environment, "PIPELINE_TEST_STORAGE_DIR": str(guard.ROOT / ".data/source-storage")})

    def test_marked_test_storage_can_be_reused(self):
        Path(self.storage.name, guard.MARKER).write_text(guard.MARKER_CONTENT)
        Path(self.storage.name, "fixture-output").write_text("fixture")
        guard.validate_environment(self.environment)

    def test_gate_is_whitelisted_and_package_arguments_cannot_be_commands(self):
        self.assertEqual(guard.gate_command("verify-full", "")[0], ["make", "verify-full"])
        self.assertEqual(guard.gate_command("packages", "./internal/sourceprocessing")[0][-1], "./internal/sourceprocessing")
        for gate, packages in [("shell", ""), ("packages", "-exec command"), ("packages", "./../outside")]:
            with self.assertRaises(guard.InvalidTestEnvironment):
                guard.gate_command(gate, packages)

    def test_nested_make_reuses_the_running_database_service(self):
        docker = Path(self.storage.name, "docker")
        output = Path(self.storage.name, "docker-arguments")
        docker.write_text('#!/bin/sh\nprintf "%s\\n" "$*" >> "$DOCKER_TEST_OUTPUT"\n')
        docker.chmod(0o700)
        environment = {**os.environ, "PATH": self.storage.name + os.pathsep + os.environ["PATH"],
                       "PIPELINE_TEST_DB_NAME": "review_pipeline_test", "POSTGRES_DB": "review_pipeline_test",
                       "DOCKER_TEST_OUTPUT": str(output)}
        subprocess.run(["make", "db-up"], cwd=guard.ROOT, env=environment, check=True,
                       capture_output=True)
        arguments = output.read_text()
        self.assertIn("pg_isready", arguments)
        self.assertNotIn("compose up", arguments)

    def test_compose_port_mismatch_stops_before_migration_or_fixture_write(self):
        result = subprocess.CompletedProcess([], 0, stdout="0.0.0.0:9999\n")
        environment = {**self.environment, "NAME_PIPELINE_TEST_GATE": "verify-full"}
        with patch.dict(os.environ, environment, clear=True), patch.object(
            guard.subprocess, "run", return_value=result
        ) as run:
            self.assertEqual(guard.main([]), 2)
            self.assertEqual(run.call_count, 1)
            self.assertEqual(run.call_args.args[0], ["docker", "compose", "port", "db", "5432"])
        self.assertFalse(Path(self.storage.name, guard.MARKER).exists())

    def test_compose_port_matches_the_validated_driver_url(self):
        result = subprocess.CompletedProcess([], 0, stdout="0.0.0.0:55432\n[::]:55432\n")
        with patch.object(guard.subprocess, "run", return_value=result):
            guard.validate_compose_port(guard.validate_environment(self.environment))

    def test_package_gate_requires_every_selected_database_package(self):
        selected = guard.selected_required_packages(["./internal/sourceprocessing", "./internal/httpapi"])
        self.assertIn("/internal/sourceprocessing\n", selected)
        self.assertIn("/internal/httpapi\n", selected)
        self.assertNotIn("/internal/trees\n", selected)
        self.assertIn("/internal/trees\n", guard.selected_required_packages(["./..."]))
        self.assertIn("/internal/trees\n", guard.selected_required_packages(["./internal/..."]))

    def test_invalid_configuration_never_starts_a_subprocess(self):
        with patch.dict(os.environ, {}, clear=True), patch.object(guard.subprocess, "run") as run:
            self.assertEqual(guard.main([]), 2)
            run.assert_not_called()


if __name__ == "__main__":
    unittest.main()
