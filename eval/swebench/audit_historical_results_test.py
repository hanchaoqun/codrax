#!/usr/bin/env python3
"""Unit tests for historical SWE-bench audit helpers."""

from __future__ import annotations

import json
import tempfile
import unittest
import sys
from pathlib import Path

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import audit_historical_results as audit


class AuditHistoricalResultsTest(unittest.TestCase):
    def test_parse_diff_files_and_source_classes(self) -> None:
        patch = """diff --git a/src/pkg/mod.py b/src/pkg/mod.py
--- a/src/pkg/mod.py
+++ b/src/pkg/mod.py
@@ -1 +1 @@
-old()
+new()
diff --git a/tests/test_mod.py b/tests/test_mod.py
--- a/tests/test_mod.py
+++ b/tests/test_mod.py
@@ -1 +1 @@
-assert old()
+assert new()
"""
        summary = audit.diff_summary(patch)
        self.assertEqual(summary.files, ["src/pkg/mod.py", "tests/test_mod.py"])
        self.assertEqual(summary.source_files, ["src/pkg/mod.py"])
        self.assertEqual(summary.test_files, ["tests/test_mod.py"])
        self.assertIn("new", summary.tokens)

    def test_empty_patch_fails(self) -> None:
        verdict, reason = audit.classify_theoretical_fit(
            {"patch_bytes": 0},
            audit.diff_summary(""),
            audit.diff_summary("diff --git a/pkg/x.py b/pkg/x.py\n--- a/pkg/x.py\n+++ b/pkg/x.py\n@@ -1 +1 @@\n-a\n+b\n"),
        )
        self.assertEqual((verdict, reason), ("fail", "empty_patch"))

    def test_no_oracle_source_overlap_fails(self) -> None:
        model = audit.diff_summary("diff --git a/pkg/a.py b/pkg/a.py\n--- a/pkg/a.py\n+++ b/pkg/a.py\n@@ -1 +1 @@\n-foo\n+bar\n")
        oracle = audit.diff_summary("diff --git a/pkg/b.py b/pkg/b.py\n--- a/pkg/b.py\n+++ b/pkg/b.py\n@@ -1 +1 @@\n-foo\n+bar\n")
        verdict, reason = audit.classify_theoretical_fit({"patch_bytes": 10}, model, oracle)
        self.assertEqual((verdict, reason), ("fail", "wrong_source_surface_no_oracle_overlap"))

    def test_high_source_and_token_overlap_passes(self) -> None:
        model = audit.diff_summary(
            "diff --git a/pkg/a.py b/pkg/a.py\n"
            "--- a/pkg/a.py\n+++ b/pkg/a.py\n@@ -1 +1 @@\n"
            "-return old_value\n+return normalized_value\n"
        )
        oracle = audit.diff_summary(
            "diff --git a/pkg/a.py b/pkg/a.py\n"
            "--- a/pkg/a.py\n+++ b/pkg/a.py\n@@ -1 +1 @@\n"
            "-return old_value\n+return normalized_value\n"
        )
        verdict, reason = audit.classify_theoretical_fit({"patch_bytes": 10}, model, oracle)
        self.assertEqual((verdict, reason), ("pass", "oracle_source_and_token_overlap_high"))

    def test_typed_verify_failure_overrides_oracle_similarity(self) -> None:
        model = audit.diff_summary(
            "diff --git a/pkg/a.py b/pkg/a.py\n"
            "--- a/pkg/a.py\n+++ b/pkg/a.py\n@@ -1 +1 @@\n"
            "-return old_value\n+return normalized_value\n"
        )
        oracle = audit.diff_summary(
            "diff --git a/pkg/a.py b/pkg/a.py\n"
            "--- a/pkg/a.py\n+++ b/pkg/a.py\n@@ -1 +1 @@\n"
            "-return old_value\n+return normalized_value\n"
        )
        verdict, reason = audit.classify_theoretical_fit(
            {"patch_bytes": 10, "verify_status": "failed", "verify_failure_kind": "tests_failed"},
            model,
            oracle,
        )
        self.assertEqual((verdict, reason), ("fail", "typed_verify_failed"))

    def test_local_verify_pass_overrides_low_token_overlap(self) -> None:
        model = audit.diff_summary("diff --git a/pkg/a.py b/pkg/a.py\n--- a/pkg/a.py\n+++ b/pkg/a.py\n@@ -1 +1 @@\n-a\n+b\n")
        oracle = audit.diff_summary("diff --git a/pkg/a.py b/pkg/a.py\n--- a/pkg/a.py\n+++ b/pkg/a.py\n@@ -1 +1 @@\n-x\n+y\n")
        verdict, reason = audit.classify_theoretical_fit(
            {
                "patch_bytes": 10,
                "local_acceptance_verdict": "pass",
                "local_acceptance_source": "local_verify",
            },
            model,
            oracle,
        )
        self.assertEqual((verdict, reason), ("pass", "local_verify_passed"))

    def test_audit_rows_prefers_typed_final_report(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            instance_dir = root / "instances" / "repo__case-1"
            plan_dir = instance_dir / "repo" / ".codrax" / "plans"
            plan_dir.mkdir(parents=True)
            (instance_dir / "prediction.json").write_text(
                json.dumps(
                    {
                        "model_patch": (
                            "diff --git a/pkg/a.py b/pkg/a.py\n"
                            "--- a/pkg/a.py\n+++ b/pkg/a.py\n@@ -1 +1 @@\n"
                            "-old\n+new\n"
                        )
                    }
                ),
                encoding="utf-8",
            )
            (instance_dir / "instance.json").write_text(
                json.dumps({"repo": "example/repo", "problem_statement": "fix a"}),
                encoding="utf-8",
            )
            final_path = plan_dir / "plan-1.final.json"
            final_path.write_text(
                json.dumps(
                    {
                        "kind": "final_report",
                        "completion": {"verdict": "unverified"},
                        "reasoning_graph": {
                            "event_count": 6,
                            "repair_event_count": 2,
                            "llm_event_count": 1,
                            "last_reason_code": "proof_coverage_weak",
                            "event_refs": ["rg:000001"],
                        },
                        "proof_ledger": {
                            "uncovered_count": 1,
                            "reason_codes": ["proof_missing"],
                        },
                        "residual_risks": [{"code": "verification_unavailable"}],
                    }
                ),
                encoding="utf-8",
            )
            rows = audit.audit_rows(
                [
                    {
                        "instance_id": "repo__case-1",
                        "instance_dir": str(instance_dir),
                        "patch_bytes": 10,
                    }
                ],
                {
                    "repo__case-1": {
                        "patch": (
                            "diff --git a/pkg/a.py b/pkg/a.py\n"
                            "--- a/pkg/a.py\n+++ b/pkg/a.py\n@@ -1 +1 @@\n"
                            "-old\n+new\n"
                        )
                    }
                },
            )

        self.assertEqual(len(rows), 1)
        self.assertEqual(rows[0]["final_answer_audit_surface"], "write_final_report")
        self.assertTrue(rows[0]["final_answer_typed_artifact_present"])
        self.assertEqual(rows[0]["final_report_path"], str(final_path))
        self.assertEqual(rows[0]["final_report_completion_verdict"], "unverified")
        self.assertEqual(rows[0]["final_report_residual_risk_codes"], ["verification_unavailable"])
        self.assertTrue(rows[0]["graph_present"])
        self.assertEqual(rows[0]["graph_event_count"], 6)
        self.assertEqual(rows[0]["graph_repair_event_count"], 2)
        self.assertEqual(rows[0]["final_report_reasoning_graph_last_reason_code"], "proof_coverage_weak")
        self.assertEqual(rows[0]["graph_missing_p0_evidence_count"], 1)
        self.assertEqual(
            rows[0]["graph_unverified_reason_codes"],
            ["proof_missing", "verification_unavailable", "proof_coverage_weak"],
        )
        rendered = audit.render_markdown(rows, {"input_row_count": 1}, dataset_name="SWE-bench/SWE-bench_Lite", split="test")
        self.assertIn("Reasoning graph telemetry is present for 1/1", rendered)
        self.assertIn("proof_coverage_weak", rendered)


if __name__ == "__main__":
    unittest.main()
