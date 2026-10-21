import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import evaluate  # noqa: E402


def node(identifier, name, x):
    return {"id": identifier, "name_ar": name, "page_id": "p1", "region": {"x": x, "y": 0.2, "width": 0.04, "height": 0.025}}


class EvaluatorTests(unittest.TestCase):
    def test_unicode_cer_and_explicit_search_normalization(self):
        gold = {"document_id": "d1", "source_class": "book", "pages": [{"page_id": "p1", "text_ar": "إبراهيم بن علي", "text_status": "readable"}]}
        pred = {"document_id": "d1", "source_class": "book", "pages": [{"page_id": "p1", "text_ar": "ابراهيم بن علي", "processed": True}]}
        counts = evaluate.document_counts(gold, pred)
        self.assertEqual(counts["character_error_rate_raw"], {"errors": 1, "total": len("إبراهيم بن علي")})
        self.assertEqual(counts["character_error_rate_search_normalized"], {"errors": 0, "total": len("ابراهيم بن علي")})

    def test_repeated_names_align_by_occurrence_not_string_for_relationships(self):
        gold = {
            "document_id": "d2", "source_class": "chart", "pages": [],
            "nodes": [node("older", "محمد", .1), node("younger", "محمد", .6)],
            "edges": [{"subject_id": "older", "predicate": "parent_of", "object_id": "younger"}],
        }
        pred = {
            "document_id": "d2", "source_class": "chart", "pages": [],
            "nodes": [dict(node("n-a", "محمد", .1), reference_id="older"), dict(node("n-b", "محمد", .6), reference_id="younger")],
            "edges": [{"subject_node_id": "n-a", "predicate": "parent_of", "object_node_id": "n-b"}],
        }
        counts = evaluate.document_counts(gold, pred)
        self.assertEqual(counts["exact_name_precision"], {"correct": 2, "total": 2})
        self.assertEqual(counts["exact_name_recall"], {"correct": 2, "total": 2})
        self.assertEqual(counts["relationship_precision"], {"correct": 1, "total": 1})
        self.assertEqual(counts["relationship_recall"], {"correct": 1, "total": 1})

    def test_two_correct_names_out_of_five_hundred_reveals_omissions(self):
        gold_nodes = [node(f"gold-{index}", f"اسم {index}", (index % 20) * .045) for index in range(500)]
        predictions = [dict(node("p-0", "اسم 0", 0), reference_id="gold-0"), dict(node("p-1", "اسم 1", .045), reference_id="gold-1")]
        counts = evaluate.document_counts(
            {"document_id": "many", "source_class": "chart", "pages": [], "nodes": gold_nodes},
            {"document_id": "many", "source_class": "chart", "pages": [], "nodes": predictions},
        )
        self.assertEqual(counts["exact_name_precision"], {"correct": 2, "total": 2})
        self.assertAlmostEqual(counts["exact_name_recall"]["correct"] / counts["exact_name_recall"]["total"], 0.004)
        self.assertEqual(counts["diagnostics"]["readable_name_omissions"], 498)

    def test_unreadable_labels_count_for_detection_but_not_transcription_recall(self):
        unreadable = dict(node("u", "", .2), name_status="unreadable")
        readable = node("r", "سالم", .6)
        detected = dict(node("p-u", "", .2), reference_id="u", status="unreadable")
        counts = evaluate.document_counts(
            {"document_id": "d", "source_class": "chart", "pages": [], "nodes": [unreadable, readable]},
            {"document_id": "d", "source_class": "chart", "pages": [], "nodes": [detected]},
        )
        self.assertEqual(counts["name_detection_recall"], {"correct": 1, "total": 2})
        self.assertEqual(counts["unreadable_label_detection_recall"], {"correct": 1, "total": 1})
        self.assertEqual(counts["exact_name_recall"], {"correct": 0, "total": 1})

    def test_partial_labels_count_as_detected_but_not_exact_readable_names(self):
        readable = node("r", "سالم", .1)
        partial = dict(node("partial", "ناصر", .4), name_status="partial", unreadable_spans_ar="بن ؟")
        unreadable = dict(node("unreadable", "", .7), name_status="unreadable")
        predictions = [
            dict(node("pr", "سالم", .1), reference_id="r"),
            dict(node("pp", "ناصر", .4), reference_id="partial"),
        ]
        counts = evaluate.document_counts(
            {"document_id": "partial", "source_class": "chart", "pages": [], "nodes": [readable, partial, unreadable]},
            {"document_id": "partial", "source_class": "chart", "pages": [], "nodes": predictions},
        )
        self.assertEqual(counts["exact_name_recall"], {"correct": 1, "total": 1})
        self.assertEqual(counts["name_detection_recall"], {"correct": 2, "total": 3})
        self.assertEqual(counts["partial_label_detection_recall"], {"correct": 1, "total": 1})
        self.assertEqual(counts["unreadable_label_detection_recall"], {"correct": 0, "total": 1})

    def test_wrong_location_does_not_count_as_correct_occurrence(self):
        counts = evaluate.document_counts(
            {"document_id": "d", "source_class": "chart", "pages": [], "nodes": [node("g", "سالم", .1)]},
            {"document_id": "d", "source_class": "chart", "pages": [], "nodes": [dict(node("p", "سالم", .7), reference_id="g")]},
        )
        self.assertEqual(counts["exact_name_precision"]["correct"], 0)
        self.assertEqual(counts["name_detection_recall"]["correct"], 0)

    def test_branch_overlap_uses_the_actual_polygon_not_its_bounding_box(self):
        gold = {"document_id": "shape", "source_class": "chart", "pages": [], "branches": [{
            "id": "g", "page_id": "p1", "polygon": [[.1, .1], [.2, .1], [.9, .9], [.8, .9]],
        }]}
        prediction = {"document_id": "shape", "source_class": "chart", "pages": [], "branches": [{
            "id": "p", "page_id": "p1", "reference_id": "g", "region": {"x": .1, "y": .1, "width": .8, "height": .8},
        }]}
        counts = evaluate.document_counts(gold, prediction)
        self.assertEqual(counts["branch_precision"]["correct"], 0)
        self.assertEqual(counts["branch_recall"]["correct"], 0)

    def test_connector_scoring_checks_the_traced_path_not_only_its_endpoints(self):
        gold = {"document_id": "path", "source_class": "chart", "pages": [], "connectors": [{
            "id": "g", "path": [[.1, .1], [.9, .1]],
        }]}
        prediction = {"document_id": "path", "source_class": "chart", "pages": [], "connectors": [{
            "id": "p", "reference_id": "g", "path": [[.1, .1], [.5, .8], [.9, .1]],
        }]}
        counts = evaluate.document_counts(gold, prediction)
        self.assertEqual(counts["connector_precision"]["correct"], 0)
        self.assertEqual(counts["connector_recall"]["correct"], 0)

    def test_book_assertion_requires_quote_attribution_polarity_and_certainty(self):
        gold = {
            "document_id": "d3", "source_class": "book", "pages": [],
            "mentions": [{"id": "m1"}, {"id": "m2"}],
            "assertions": [{"subject_id": "m1", "predicate": "parent_of", "object_id": "m2", "quote_ar": "قيل إن سالم والد ناصر", "attribution_ar": "الراوي", "polarity": "affirmed", "certainty": "reported"}],
        }
        pred = {
            "document_id": "d3", "source_class": "book", "pages": [],
            "mentions": [{"id": "p1", "reference_id": "m1"}, {"id": "p2", "reference_id": "m2"}],
            "assertions": [{"subject_mention_id": "p1", "predicate": "parent_of", "object_mention_id": "p2", "quote_ar": "قيل إن سالم والد ناصر", "attribution_ar": "الراوي", "polarity": "affirmed", "certainty": "asserted"}],
        }
        counts = evaluate.document_counts(gold, pred)
        self.assertEqual(counts["assertion_precision"]["correct"], 0)
        self.assertEqual(counts["assertion_recall"]["correct"], 0)

    def test_missing_output_page_is_uncovered_and_document_sets_must_match(self):
        gold = {"document_id": "d4", "source_class": "mixed", "pages": [{"page_id": "p1", "text_ar": "نص"}]}
        pred = {"document_id": "d4", "source_class": "mixed", "pages": []}
        counts = evaluate.document_counts(gold, pred)
        self.assertEqual(counts["page_coverage"], {"correct": 0, "total": 1})
        with self.assertRaisesRegex(ValueError, "document sets differ"):
            evaluate.evaluate({"d4": gold}, {}, bootstrap_samples=0)

    def test_reaching_point_targets_does_not_automatically_qualify_model(self):
        gold = {}; pred = {}
        for document in range(3):
            identifier = f"held-{document}"
            gold[identifier] = {"document_id": identifier, "source_class": "chart", "split": "held_out", "pages": [], "nodes": [node("gold", "سالم", .2)]}
            pred[identifier] = {"document_id": identifier, "source_class": "chart", "pages": [], "nodes": [dict(node("pred", "سالم", .2), reference_id="gold")]}
        result = evaluate.evaluate(gold, pred, bootstrap_samples=10)
        self.assertTrue(result["held_out_evidence"]["meets_point_targets"])
        self.assertEqual(result["held_out_evidence"]["status"], "pending_independent_review")


if __name__ == "__main__":
    unittest.main()
