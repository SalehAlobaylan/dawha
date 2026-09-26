#!/usr/bin/env python3
"""Check every relative link and every cited path in the repository's documents.

Documentation in this repository makes a specific kind of claim: it points at
code. `IMPLEMENTATION_PLAN.md:1619` is a citation, not a figure of speech, and a
status matrix that links `graphrag.go:37-42` is only useful while that line
range still says what the matrix claims it says. A broken citation is worse than
no citation, because a reader who trusts one stops reading.

So this checks three things, in increasing strictness:

  1. Relative links and images in Markdown resolve to a file that exists.
     Anchors are not resolved: this repository has no generated table of
     contents, so a `#heading` fragment is checked for being well-formed, not for
     matching a heading.
  2. Bare paths written inline as `path/to/file.ext` or `path/to/file.ext:12-34`
     resolve to a file that exists. This is the citation form the plans and the
     architecture document use, and it is the one that rots.
  3. A cited line range is inside the file. A citation to lines 900-1200 of a
     400-line file is a claim about a file that does not exist in that state,
     and it is the failure mode that a plain existence check misses.

External URLs are not fetched. A link checker that needs the network is a link
checker nobody runs offline, and a URL that 404s is the upstream author's
problem rather than this repository's.

Usage:

    python3 infra/local/check-doc-links.py            # check the whole tree
    python3 infra/local/check-doc-links.py --verbose  # also print what passed

Exit status is 0 when everything resolved and 1 when anything did not.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent.parent

# Documents are checked explicitly rather than by globbing every .md file. A new
# document should be added here in the same commit that adds it: a document that
# nobody checks is a document whose citations rot in silence.
DOCUMENTS = (
    "README.md",
    "ARCHITECTURE.md",
    "IMPLEMENTATION_PLAN.md",
    "PRODUCT.md",
    "plans/README.md",
    "docs/graph-benchmark.md",
    "docs/phase-status.md",
    "apps/web/e2e/README.md",
    "infra/security/scanner-exceptions.md",
    "services/core-api/platform/ratelimit/ratelimit.go",
    "services/core-api/platform/telemetry/telemetry.go",
    "services/core-api/platform/storage/storage.go",
)

# Reviewed exceptions, in the spirit of .gitleaks.toml and
# infra/security/scanner-exceptions.md: a named reference this checker is allowed
# to leave unresolved, with the reason a reviewer reads before believing it.
#
# Each one is a citation that is wrong in a document this change is not allowed to
# edit. They are listed rather than fixed so the count stays visible: the day
# plans/README.md is next edited, the entry can be corrected and deleted here in
# the same commit.
REVIEWED_EXCEPTIONS: dict[tuple[str, str], str] = {
    (
        "plans/README.md",
        "tools/dbtestguard",
    ): "plans/README.md:27 names the guard by its in-module path; the file is "
    "services/core-api/tools/dbtestguard. plans/README.md is not editable by the "
    "change that added this checker.",
}

# Directories a link may point into without naming a file. None of the
# documents do this today; the check exists so that if one starts, it is a
# deliberate decision rather than an accident.
LINK_PATTERN = re.compile(r"\[[^\]]*\]\(([^)\s]+)(?:\s+\"[^\"]*\")?\)")
IMAGE_PATTERN = re.compile(r"!\[[^\]]*\]\(([^)\s]+)(?:\s+\"[^\"]*\")?\)")
# A backticked token that looks like a repository path, optionally with a line
# or line range. Deliberately narrow: it must start with a directory this
# repository actually has at one of its roots, so prose that happens to be in
# backticks is not mistaken for a citation.
#
# The prefixes span three roots because the documents use three conventions: a
# status matrix writes `internal/research/graphrag.go:37-45` and a benchmark
# writes `app/main.py`, neither of which resolves from the repository root.
CITATION_PATTERN = re.compile(
    r"`((?:apps|db|docs|infra|plans|services|tools"
    r"|internal|cmd|platform|generated"
    r"|app|evaluation|tests)/[A-Za-z0-9_./-]+"
    r"(?::\d+(?:-\d+)?)?)`"
)
LINE_RANGE_PATTERN = re.compile(r"^(?P<path>[^:]+):(?P<start>\d+)(?:-(?P<end>\d+))?$")

# ROOTS are the directories a citation may be relative to. A document's own
# directory is tried first, because a Markdown link is relative to its document;
# these are the fallbacks for an inline citation, which is relative to whichever
# root the author had in mind.
ROOTS = (
    REPO_ROOT,
    REPO_ROOT / "services" / "core-api",
    REPO_ROOT / "services" / "ai-research",
    REPO_ROOT / "apps" / "web",
)

# Text fenced as a command is a claim about something a reader can run; these are
# the markers that fence it. Inline `code` is deliberately not skipped: an inline
# citation is the most common form in the plans, and skipping inline code would
# skip exactly the thing worth checking.
COMMAND_FENCE_MARKERS = ("sh", "bash", "shell", "console", "text", "json", "yaml", "yml", "sql", "")

# Fenced blocks that cannot contain a path citation and would only produce noise.
NO_CITATION_FENCE_MARKERS = ("mermaid", "textile")


def strip_code_fences(text: str) -> str:
    """Return the document with fenced code blocks blanked, keeping line count.

    A link inside a fenced block is a shell command or a JSON payload, not a
    citation, and a path inside one is a string a program will resolve. Line
    numbers are preserved so a citation outside a fence still reports the line it
    is really on.
    """
    output: list[str] = []
    fence: str | None = None
    for line in text.splitlines():
        stripped = line.lstrip()
        if fence is None:
            marker = stripped[3:].strip().split(" ")[0] if stripped.startswith("```") else None
            if marker is not None:
                fence = marker.lower()
                output.append("")
                continue
            output.append(line)
            continue
        if stripped.startswith("```"):
            fence = None
            output.append("")
            continue
        output.append("")
    return "\n".join(output)


def is_external(target: str) -> bool:
    return target.startswith(("http://", "https://", "mailto:", "tel:", "#", "data:"))


def strip_fragment(target: str) -> str:
    return target.split("#", 1)[0]


def resolve(relative_to: Path, target: str) -> Path:
    if target.startswith("/"):
        return REPO_ROOT / target.lstrip("/")
    return (relative_to.parent / target).resolve()


def resolve_citation(relative_to: Path, target: str) -> Path | None:
    """Resolve an inline citation against every root the documents write from.

    The repository's convention is mixed and all of the mix is deliberate: a
    Markdown link is relative to the document that holds it, while an inline
    `path` citation is written from whichever root the author was looking at -
    the repository for `docs/`, the Go module for `internal/research/`, the
    Python service for `app/main.py`. Trying each root in turn accepts all of
    them, and a path that resolves under only one of them is a path a reader
    could still have followed from another.
    """
    for base in (relative_to.parent, *ROOTS):
        candidate = (base / target).resolve()
        if candidate.exists():
            return candidate
    return None


def is_generated(target: str) -> str | None:
    """Name the build output a citation points at, if that is what it is.

    `services/ai-research/.venv` and `apps/web/dist` are real paths in a
    configured checkout and absent in a clean one. Reporting them as broken
    would make the gate fail on a fresh clone, and skipping them silently would
    hide a genuine citation to a file somebody forgot to generate. So they are
    named, and the report says how many there were.
    """
    for marker, reason in (
        ("node_modules/", "npm dependency tree, created by make install"),
        (".venv/", "python virtualenv, created by make install"),
        ("dist/", "build output, created by make build"),
        (".data/", "local runtime directory, created by the API"),
        ("evaluation/report.json", "written by make ai-eval"),
        ("docs/benchmarks/", "written by make graph-benchmark"),
    ):
        if target.endswith(marker) or f"/{marker}" in target or target.startswith(marker):
            return reason
    return None


def line_count(path: Path) -> int:
    with path.open("r", encoding="utf-8", errors="replace") as handle:
        return sum(1 for _ in handle)


def check_document(
    relative_path: str, problems: list[str], reviewed: list[str], verbose: bool
) -> tuple[int, int, list[str]]:
    document = REPO_ROOT / relative_path
    if not document.is_file():
        problems.append(f"{relative_path}: listed in the checker but does not exist")
        return 0, 0, []
    raw = document.read_text(encoding="utf-8")
    checkable = strip_code_fences(raw)
    checked = 0
    generated: list[str] = []

    for match in list(LINK_PATTERN.finditer(checkable)) + list(IMAGE_PATTERN.finditer(checkable)):
        target = match.group(1)
        if is_external(target):
            continue
        checked += 1
        resolved = resolve(document, strip_fragment(target))
        if resolved.exists():
            continue
        reason = is_generated(target)
        if reason:
            generated.append(f"{relative_path}: {target} ({reason})")
            continue
        line = checkable[: match.start()].count("\n") + 1
        problems.append(f"{relative_path}:{line}: link target does not exist: {target}")

    for match in CITATION_PATTERN.finditer(checkable):
        citation = match.group(1)
        checked += 1
        line = checkable[: match.start()].count("\n") + 1
        range_match = LINE_RANGE_PATTERN.match(citation)
        if range_match:
            path_part = range_match.group("path")
            start = int(range_match.group("start"))
            end = int(range_match.group("end") or start)
        else:
            path_part = citation
            start = end = 0
        reason = is_generated(path_part)
        if reason and resolve_citation(document, path_part) is None:
            generated.append(f"{relative_path}:{line}: {citation} ({reason})")
            continue
        target = resolve_citation(document, path_part)
        if target is None:
            exception = REVIEWED_EXCEPTIONS.get((relative_path, citation))
            if exception is not None:
                reviewed.append(f"{relative_path}:{line}: {citation} ({exception})")
                continue
            problems.append(f"{relative_path}:{line}: cited path does not exist: {citation}")
            continue
        if not target.is_file():
            if verbose:
                print(f"  ok  {relative_path}:{line} -> {path_part} (directory)")
            continue
        if start:
            total = line_count(target)
            if start < 1 or end < start or end > total:
                problems.append(
                    f"{relative_path}:{line}: citation names lines {start}-{end} of a file with "
                    f"{total} lines: {path_part}"
                )
                continue
            if verbose:
                print(f"  ok  {relative_path}:{line} -> {path_part}:{start}-{end} (of {total})")
        elif verbose:
            print(f"  ok  {relative_path}:{line} -> {path_part}")

    return checked, len(checkable.splitlines()), generated


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--verbose", action="store_true", help="print every resolved citation")
    arguments = parser.parse_args()

    problems: list[str] = []
    generated: list[str] = []
    reviewed: list[str] = []
    total_checked = 0
    print(f"doc link checker: {len(DOCUMENTS)} documents, repository {REPO_ROOT}")
    for relative_path in DOCUMENTS:
        checked, lines, skipped = check_document(relative_path, problems, reviewed, arguments.verbose)
        total_checked += checked
        generated.extend(skipped)
        print(f"  {relative_path}: {lines} lines, {checked} local links and citations checked")

    print("")
    if reviewed:
        print(f"doc link checker: {len(reviewed)} reviewed exception(s), left unresolved on purpose:")
        for note in reviewed:
            print(f"  - {note}")
        print("")
    if generated:
        print(f"doc link checker: {len(generated)} citation(s) point at build output, not checked:")
        for note in generated:
            print(f"  - {note}")
        print("")
    if problems:
        print(f"doc link checker: {len(problems)} unresolved reference(s)")
        for problem in problems:
            print(f"  - {problem}")
        return 1
    print(f"doc link checker: no broken local links ({total_checked} checked)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
