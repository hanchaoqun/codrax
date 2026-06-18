#!/usr/bin/env python3
"""Unit tests for historical SWE-bench audit helpers."""

from __future__ import annotations

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


if __name__ == "__main__":
    unittest.main()
