"""The test suite's own honesty about what it is testing.

`make verify` runs this suite, so the suite has an interest in its own
credibility: a test that passes because it imported a *different copy* of the
code than the one in the working tree is not evidence about the working tree. It
looks identical on the console and it is worth nothing.

That is not hypothetical. This suite was run for a long time by a Makefile line
that invoked pytest from the repository root:

    services/ai-research/.venv/bin/pytest

From there, pytest's rootdir is the repository, `services/ai-research/pyproject.toml`
is never read, and `[tool.pytest.ini_options] pythonpath = ["."]` never applies.
`services/ai-research/tests/` has no `__init__.py`, so pytest's prepend import mode
puts the *tests* directory on `sys.path` and not the service root - which means
`from app.main import app` could only ever resolve against whatever `app` the
virtualenv happened to have installed. On a machine whose virtualenv carried a
stale non-editable install from before the packaging fix, `app` resolved to that
stale copy and `evaluation` - which the old install did not provide - was missing
entirely. The suite therefore passed or failed according to the state of an
installed package rather than the state of the source, and the pre-existing tests
were as affected as the new ones.

The Makefile now runs this suite from the service directory. These four tests are
what stops that from being a fact about one line of a Makefile that only a careful
reader would notice: if the working directory regresses, they fail, and they say
which import came from where.
"""

from __future__ import annotations

import importlib
import sys
from pathlib import Path

import pytest

# SERVICE_ROOT is this service's directory: the parent of tests/. Everything the
# suite imports has to come from inside it.
SERVICE_ROOT = Path(__file__).resolve().parent.parent

# The two real top-level packages of the service. `tests` is deliberately absent:
# it is a suite, not something to import.
SERVICE_PACKAGES = ("app", "evaluation")

# One real module from each package. The packages themselves are PEP 420
# namespace packages with no `__init__.py`, so their own `__file__` is None and
# `__path__` is a merge of every directory of that name on sys.path. These
# modules have files, so `__file__` says exactly which copy was executed.
SERVICE_MODULES = (("app", "main"), ("evaluation", "evaluate"), ("evaluation", "thresholds"))


def package_paths(name: str) -> list[Path]:
    """The filesystem directories a package resolves to.

    An editable install also contributes a synthetic
    `__editable__....__path_hook__` entry, which names no directory and is
    filtered out here rather than being asserted against.
    """
    module = __import__(name)
    return [
        Path(entry).resolve() for entry in module.__path__ if not entry.startswith("__editable__")
    ]


def is_inside(path: Path, parent: Path) -> bool:
    try:
        path.relative_to(parent)
        return True
    except ValueError:
        return False


def test_the_pytest_configuration_of_this_service_is_the_one_in_effect(
    pytestconfig: pytest.Config,
) -> None:
    """The defect itself, asserted directly.

    If the suite is run from anywhere but the service directory, pytest's rootdir
    is somewhere else, this service's `pyproject.toml` is not the configfile, and
    `pythonpath` and `testpaths` come back empty. That is the state in which the
    imports below resolve against an installed copy instead of the tree, so it is
    checked before anything relies on it.
    """
    pythonpath = [Path(entry).resolve() for entry in pytestconfig.getini("pythonpath")]
    testpaths = pytestconfig.getini("testpaths")
    assert pythonpath == [SERVICE_ROOT], (
        f"pytest is not applying this service's pythonpath to the service directory (got "
        f"{[str(path) for path in pythonpath]}), so the suite imports whatever is installed "
        f"rather than the working tree. Run it from {SERVICE_ROOT}; `make verify` does."
    )
    assert testpaths == ["tests"], (
        f"pytest is not applying this service's testpaths (got {testpaths!r}); run it "
        f"from {SERVICE_ROOT}."
    )


@pytest.mark.parametrize("package", SERVICE_PACKAGES)
def test_the_service_packages_are_reachable_from_the_working_tree(package: str) -> None:
    """Each package must have the source directory among the places it resolves to."""
    paths = package_paths(package)
    assert any(is_inside(path, SERVICE_ROOT) for path in paths), (
        f"{package} does not resolve to the working tree at {SERVICE_ROOT}; it resolved to "
        f"{[str(path) for path in paths]}. The suite is testing an installed copy."
    )


@pytest.mark.parametrize(
    ("package", "module_name"),
    SERVICE_MODULES,
    ids=[f"{package}.{module_name}" for package, module_name in SERVICE_MODULES],
)
def test_the_suite_executes_the_working_tree(package: str, module_name: str) -> None:
    """The module the tests run must be the file in the working tree.

    This is the assertion that has teeth. A namespace package merges every
    directory of its name on `sys.path`, so `app` resolving to the tree says
    nothing on its own: if site-packages also holds an `app`, both directories
    belong to the same namespace package and a change in ordering would silently
    start executing the copy. `__file__` of a real module is what actually ran,
    and that is what this checks.

    A stale installed copy fails here with a path in the message, which is the
    diagnosis somebody needs: the symptom is otherwise a behaviour difference
    between two copies of the same file, which is close to undebuggable.
    """
    imported = importlib.import_module(f"{package}.{module_name}")
    origin = getattr(imported, "__file__", None)
    assert origin is not None, f"{package}.{module_name} has no __file__; cannot tell what ran"
    resolved = Path(origin).resolve()
    assert is_inside(resolved, SERVICE_ROOT), (
        f"{package}.{module_name} executed {resolved}, which is not in the working tree at "
        f"{SERVICE_ROOT}. The suite is testing an installed copy"
        + (
            f"; {package} also resolves to {[str(path) for path in package_paths(package)]}"
            if package_paths(package)
            else ""
        )
        + f". Reinstall the service with `pip install -e '.[dev]'` from {SERVICE_ROOT}, and run "
        "the suite from that directory so the tree precedes site-packages."
    )


def test_the_service_directory_is_on_the_import_path() -> None:
    """The mechanism the tests above depend on, stated as a fact.

    `pythonpath = ["."]` is relative to the rootdir, so it names the service
    directory only while the suite is run from there. This is the assertion that
    says so, in the failure message of a run somebody is looking at.
    """
    resolved = SERVICE_ROOT.resolve()
    on_path = [Path(entry).resolve() for entry in sys.path if entry]
    assert any(path == resolved for path in on_path), (
        f"{resolved} is not on sys.path, so `import app` and `import evaluation` resolve "
        f"from somewhere else. sys.path starts {[str(path) for path in on_path[:6]]}."
    )
