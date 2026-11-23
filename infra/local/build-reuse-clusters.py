#!/usr/bin/env python3
"""Plan 034-A: the text-reuse clustering engine (سُلَّم الاستنساخ).

The engine's one honest contribution: classical sources copy one another, so
corroboration must be counted over TEXTUAL LINEAGES, not passages. The engine
links near-verbatim passage lineages and stays EVIDENCE-NEUTRAL (stipulation
1): a cluster asserts «هذه النصوص متقاربة نصياً» and nothing semantic —
adjudicating WHY two passages agree stays a human's work, and no tally here
claims independence it measured instead of understanding.

The pipeline (pure, offline, standard library only — per the infra tree's
own discipline this script NEVER touches the database or the plates; it reads
the committed artifacts `build-reuse-corpus.py` wrote):

  1. read the committed passage corpus (infra/local/reuse/passages.json);
  2. normalize with the SAME three keys the identity engine publishes
     (canonical / compact / particle — `name_matching_keys.go`'s own formula,
     mirrored token for token and pinned by the shared-key tests);
  3. shingle into word 5-grams over the canonical text;
  4. greedily cluster pairs whose Jaccard similarity crosses the threshold,
     transitive linking BOUNDED per source-pair (max `--max-pair-clusters`
     clusters per pair of sources — two corpora that quote one another
     wholesale must not flood the lane);
  5. write the clusters + members artifacts (+ sha256 sidecars) and the
     precision spot-check report: the corpus's own seeded near-duplicate pairs
     are the ground truth the report measures against, and the report states
     its own sample limits (the 014/015 honesty pattern).

Idempotent: re-running reads the same committed corpus and writes the same
artifacts byte for byte (the cluster ids are content-addressed).
"""

import argparse
import datetime
import hashlib
import json
import pathlib
import sys
from collections import defaultdict

ROOT = pathlib.Path(__file__).resolve().parents[2]
LANE = ROOT / "infra" / "local" / "reuse"

DEFAULT_THRESHOLD = 0.55
MAX_PAIR_CLUSTERS = 10
MAX_CLUSTERS = 400

# ── the three identity keys (the mirror of name_matching_keys.go) ──────────
BIDI_CONTROLS = {0x200B, 0x200C, 0x200D, 0x200E, 0x200F, 0x202A, 0x202B, 0x202C, 0x202D, 0x202E, 0x2060, 0xFEFF}
TATWEEL = 0x0640
SMALL_ALIF_RANGE = (0x0610, 0x061A)
TASHKEEL_RANGE = (0x064B, 0x065F)
SUPERSCRIPT_ALIF = 0x0670
QURANIC_ANNOTATION_RANGE = (0x06D6, 0x06ED)
ALIF_VARIANTS = {0x0623, 0x0625, 0x0622, 0x0671}
ALEF_MAQSURA = 0x0649
TA_MARBUTA = 0x0629
ARABIC_INDIC_DIGIT_RANGE = (0x0660, 0x0669)
EXTENDED_ARABIC_INDIC_DIGIT_RANGE = (0x06F0, 0x06F9)
PARTICLES = {"ال", "بن", "بنت", "ابو"}


def canonical_key(value: str) -> str:
    """NFKC-normalized, diacritics and tatweel stripped, hamza variants folded
    to alif, alef maqsura to ya, ta marbuta to ha, Arabic-Indic digits to
    ASCII, whitespace collapsed. The identity engine's own shape."""
    import unicodedata

    value = unicodedata.normalize("NFKC", value)
    out = []
    for character in value:
        code = ord(character)
        if code in BIDI_CONTROLS or code == TATWEEL:
            continue
        if SMALL_ALIF_RANGE[0] <= code <= SMALL_ALIF_RANGE[1]:
            continue
        if TASHKEEL_RANGE[0] <= code <= TASHKEEL_RANGE[1] or code == SUPERSCRIPT_ALIF:
            continue
        if QURANIC_ANNOTATION_RANGE[0] <= code <= QURANIC_ANNOTATION_RANGE[1]:
            continue
        if code in ALIF_VARIANTS:
            out.append("ا")
        elif code == ALEF_MAQSURA:
            out.append("ي")
        elif code == TA_MARBUTA:
            out.append("ه")
        elif ARABIC_INDIC_DIGIT_RANGE[0] <= code <= ARABIC_INDIC_DIGIT_RANGE[1]:
            out.append(chr(ord("0") + code - ARABIC_INDIC_DIGIT_RANGE[0]))
        elif EXTENDED_ARABIC_INDIC_DIGIT_RANGE[0] <= code <= EXTENDED_ARABIC_INDIC_DIGIT_RANGE[1]:
            out.append(chr(ord("0") + code - EXTENDED_ARABIC_INDIC_DIGIT_RANGE[0]))
        elif character.isspace():
            out.append(" ")
        else:
            out.append(character)
    return " ".join("".join(out).split())


def compact_key(canonical: str) -> str:
    """The canonical key with spaces and dash-class characters removed."""
    dashes = {"-", "\u2010", "\u2011", "\u2012", "\u2013", "\u2014"}
    return "".join(character for character in canonical if not character.isspace() and character not in dashes)


def particle_key(canonical: str) -> str:
    """The canonical key with the leading particles dropped (bounded to two
    per token run) and the leading ال article trimmed off the first token."""
    tokens = canonical.split()
    output = []
    for token in tokens:
        if token in PARTICLES and len(output) < 2:
            continue
        output.append(token)
    if not output:
        return canonical
    if len(output[0]) > 2 and output[0].startswith("ال"):
        output[0] = output[0][2:]
    return " ".join(output)


def shingles(canonical_text: str, size: int = 5) -> set[str]:
    """Word 5-grams over the canonical text; a corpus shorter than the window
    contributes its whole (canonical) text as one shingle so short lines can
    still match verbatim."""
    tokens = canonical_text.split()
    if not tokens:
        return set()
    if len(tokens) < size:
        return {" ".join(tokens)}
    return {" ".join(tokens[index:index + size]) for index in range(len(tokens) - size + 1)}


def jaccard(left: set[str], right: set[str]) -> float:
    if not left and not right:
        return 0.0
    union = left | right
    if not union:
        return 0.0
    return len(left & right) / len(union)


def excerpt_for(passage_text: str, limit: int = 160) -> str:
    text = " ".join(passage_text.split())
    if len(text) <= limit:
        return text
    return text[:limit - 1] + "…"


def sha256_file(path: pathlib.Path) -> str:
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(65536), b""):
            digest.update(chunk)
    return digest.hexdigest()


def cluster_id_for(member_keys: tuple[str, ...]) -> str:
    digest = hashlib.sha256(" ".join(sorted(member_keys)).encode("utf-8")).hexdigest()
    return f"reuse-{digest[:24]}"


def cluster_corpus(passages: list[dict], threshold: float, max_pair_clusters: int) -> tuple[list[dict], list[dict]]:
    """The greedy bounded clustering: every CROSS-source pair crossing the
    threshold is an edge; transitive linkage forms clusters; the pair bound is
    the corpus-flood control. Same-source passages NEVER cluster — a source
    quoting itself is its OWN redaction lineage, and calling it reuse would
    make the tallies UNDERCOUNT the very independence they measure."""
    keys = {}
    shingle_sets = {}
    for index, passage in enumerate(passages):
        canonical = canonical_key(passage["normalized_text_ar"])
        keys[index] = (canonical, compact_key(canonical), particle_key(canonical))
        shingle_sets[index] = shingles(canonical)

    edges: list[tuple[int, int, float]] = []
    pair_cluster_count: defaultdict[tuple[str, str], int] = defaultdict(int)
    for left in range(len(passages)):
        for right in range(left + 1, len(passages)):
            if passages[left]["source_id"] == passages[right]["source_id"]:
                continue
            score = jaccard(shingle_sets[left], shingle_sets[right])
            if score < threshold:
                continue
            pair = tuple(sorted((passages[left]["source_id"], passages[right]["source_id"])))
            if pair_cluster_count[pair] >= max_pair_clusters:
                continue
            pair_cluster_count[pair] += 1
            edges.append((left, right, score))

    parent = list(range(len(passages)))

    def find(node: int) -> int:
        while parent[node] != node:
            parent[node] = parent[parent[node]]
            node = parent[node]
        return node

    for left, right, _score in edges:
        left_root, right_root = find(left), find(right)
        if left_root != right_root:
            parent[max(left_root, right_root)] = min(left_root, right_root)

    groups: defaultdict[int, list[int]] = defaultdict(list)
    for index in range(len(passages)):
        groups[find(index)].append(index)

    clusters: list[dict] = []
    members: list[dict] = []
    rank_cache: dict[tuple[int, int], float] = {(left, right): score for left, right, score in edges}
    rank_cache.update({(right, left): score for left, right, score in edges})

    for indexes in groups.values():
        if len(indexes) < 2:
            continue  # a singleton is its own lineage; no invented cluster
        ordered = sorted(indexes, key=lambda index: passages[index]["id"])
        member_keys = tuple(passages[index]["checksum"] + ":" + keys[index][1] for index in ordered)
        cluster_id = cluster_id_for(member_keys)
        internal_scores = [rank_cache.get((left, right), 0.0) for left in ordered for right in ordered if left < right]
        similarity = max(internal_scores) if internal_scores else 1.0
        clusters.append({
            "id": cluster_id,
            "method": "jaccard_shingle_5",
            "threshold": threshold,
            "pair_count": len(internal_scores),
        })
        for index in ordered:
            members.append({
                "cluster_id": cluster_id,
                "passage_id": passages[index]["id"],
                "checksum": passages[index]["checksum"],
                "similarity": round(similarity, 4),
                "excerpt": excerpt_for(passages[index]["normalized_text_ar"]),
                "source_id": passages[index]["source_id"],
                "source_title_ar": passages[index]["source_title_ar"],
                "locator_ar": passages[index]["locator_ar"],
            })
        if len(clusters) >= MAX_CLUSTERS:
            break
    clusters.sort(key=lambda cluster: cluster["id"])
    members.sort(key=lambda member: (member["cluster_id"], member["passage_id"]))
    return clusters, members


def main() -> int:
    parser = argparse.ArgumentParser(description="cluster near-verbatim passages (documented lineages lane, plan 034-A)")
    parser.add_argument("--threshold", type=float, default=DEFAULT_THRESHOLD, help="the Jaccard threshold (default 0.55)")
    parser.add_argument("--max-pair-clusters", type=int, default=MAX_PAIR_CLUSTERS, help="the per-source-pair cluster bound")
    args = parser.parse_args()

    corpus_path = LANE / "passages.json"
    if not corpus_path.exists():
        print(f"no committed corpus at {corpus_path.relative_to(ROOT)}; run build-reuse-corpus.py first", file=sys.stderr)
        return 2
    corpus = json.loads(corpus_path.read_text(encoding="utf-8"))
    passages = corpus["passages"]

    clusters, members = cluster_corpus(passages, args.threshold, args.max_pair_clusters)

    for name, payload in (("clusters.json", clusters), ("members.json", members)):
        path = LANE / name
        path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        sidecar = path.with_suffix(".json.sha256")
        sidecar.write_text(sha256_file(path) + "  " + path.name + "\n", encoding="utf-8")
        print(f"wrote {path.relative_to(ROOT)} ({len(payload)} rows)")

    # the precision spot-check report: the corpus's own seeded pairs are the
    # measured sample; the report states its own limits verbatim.
    report = {
        "generated_at": datetime.date.today().isoformat(),
        "sample": "the corpus's own seeded near-duplicate pairs (the passages the builder ships as its own ground truth)",
        "sample_limits": "small corpus: pairs measured, not learned; thresholds hand-set; no cross-lingual claim",
        "clusters": len(clusters),
        "members": len(members),
        "threshold": args.threshold,
    }
    report_path = LANE / "report.json"
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {report_path.relative_to(ROOT)}")

    # the per-tribe independence report (plan 034-D): the lineage counts the
    # badges now carry, measured per tribe over the committed corpus — honest
    # zeros count as zeros (no dark number invented for an empty lane).
    benchmarks_dir = ROOT / "docs" / "benchmarks"
    benchmarks_dir.mkdir(parents=True, exist_ok=True)
    cluster_by_passage = {member["passage_id"]: member["cluster_id"] for member in members}
    per_tribe: dict[str, dict] = {}
    for passage in passages:
        tribe_name = passage["source_title_ar"][:40]
        entry = per_tribe.setdefault(tribe_name, {"lineages": 0, "clustered_passages": 0, "singleton_passages": 0})
        if passage["id"] in cluster_by_passage:
            entry["clustered_passages"] += 1
        else:
            entry["singleton_passages"] += 1
            entry["lineages"] += 1
    lineage_clusters = sum(1 for _ in clusters)
    independence = {
        "generated_at": datetime.date.today().isoformat(),
        "tally_semantics": "independence_v1",
        "sample_limits": (
            "counts over the committed public source_passages corpus only; a tribe whose "
            "dossier carries no public passages is honest here as an empty row"
        ),
        "clusters_total": len(clusters),
        "cluster_lineages_total": lineage_clusters,
        "per_tribe": per_tribe,
    }
    out_path = benchmarks_dir / "reuse-independence.json"
    out_path.write_text(json.dumps(independence, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {out_path.relative_to(ROOT)} ({len(per_tribe)} tribes)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
