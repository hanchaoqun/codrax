#!/usr/bin/env python3
from __future__ import annotations

import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "eval" / "swebench" / "summarize_codrax_results.py"

spec = importlib.util.spec_from_file_location("summarize_codrax_results", SCRIPT)
assert spec and spec.loader
summary_mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(summary_mod)


class CodraxResultsSummaryTests(unittest.TestCase):
    def test_summarizes_local_confidence_and_manual_audit_separately(self) -> None:
        rows = [
            {
                "instance_id": "repo__a-1",
                "status": "predicted",
                "patch_bytes": 120,
                "prediction_verdict": "predicted_passed",
                "prediction_local_confidence": "high",
                "prediction_blocks_local_acceptance": False,
                "prediction_confidence_downgrade_reason": "",
                "prediction_audit_block_reason": "",
                "verify_status": "passed",
                "workflow_status": "complete",
                "final_report_present": True,
                "final_report_completion_verdict": "verified",
                "local_acceptance_verdict": "pass",
                "local_acceptance_source": "local_verify",
                "manual_audit_verdict": "",
            },
            {
                "instance_id": "repo__a-2",
                "status": "predicted",
                "patch_bytes": 80,
                "prediction_verdict": "predicted_passed_low_confidence",
                "prediction_local_confidence": "unknown",
                "prediction_blocks_local_acceptance": False,
                "prediction_confidence_downgrade_reason": "verification_probe_changed_source_not_context_covered",
                "prediction_audit_block_reason": "",
                "verify_status": "passed",
                "workflow_status": "complete",
                "final_report_present": True,
                "final_report_completion_verdict": "verified",
                "local_acceptance_verdict": "pass",
                "local_acceptance_source": "local_verify",
                "manual_audit_verdict": "",
            },
            {
                "instance_id": "repo__a-3",
                "status": "predicted",
                "patch_bytes": 60,
                "prediction_verdict": "predicted_unverified",
                "prediction_local_confidence": "unknown",
                "prediction_blocks_local_acceptance": False,
                "prediction_confidence_downgrade_reason": "verification_probe_import_error",
                "prediction_audit_block_reason": "",
                "verify_status": "unavailable",
                "workflow_status": "complete",
                "final_report_present": True,
                "final_report_completion_verdict": "unverified",
                "local_acceptance_verdict": "pass",
                "local_acceptance_source": "manual_audit",
                "manual_audit_verdict": "pass",
            },
            {
                "instance_id": "repo__a-4",
                "status": "empty_patch",
                "patch_bytes": 0,
                "prediction_verdict": "empty_patch",
                "prediction_local_confidence": "none",
                "prediction_blocks_local_acceptance": True,
                "prediction_confidence_downgrade_reason": "",
                "prediction_audit_block_reason": "workflow_in_progress_empty_patch",
                "verify_status": "",
                "workflow_status": "in_progress",
                "final_report_present": False,
                "final_report_completion_verdict": "",
                "local_acceptance_verdict": "fail",
                "local_acceptance_source": "local_audit_block",
                "manual_audit_verdict": "fail",
            },
        ]

        summary = summary_mod.summarize_results(rows)

        self.assertEqual(summary["row_count"], 4)
        self.assertEqual(summary["non_empty_patch_instances"], 3)
        self.assertEqual(summary["high_confidence_local_verify_pass_instances"], 1)
        self.assertEqual(summary["recorded_local_verify_pass_confidence_mismatch_instances"], 1)
        self.assertEqual(summary["low_confidence_verify_pass_instances"], 1)
        self.assertEqual(summary["local_acceptance_pass_instances"], 3)
        self.assertEqual(summary["typed_manual_audit_recorded_instances"], 2)
        self.assertEqual(summary["final_report_present_instances"], 3)
        self.assertEqual(summary["final_report_completion_verdict_counts"]["verified"], 2)
        self.assertEqual(summary["final_report_completion_verdict_counts"]["unverified"], 1)
        self.assertEqual(summary["typed_manual_audit_pass_instances"], 1)
        self.assertEqual(summary["typed_manual_audit_fail_instances"], 1)
        self.assertEqual(summary["local_audit_blocked_instances"], 1)
        self.assertEqual(summary["prediction_verdict_counts"]["predicted_passed_low_confidence"], 1)
        self.assertEqual(summary["top_confidence_downgrade_reasons"][0]["value"], "<empty>")
        self.assertEqual(summary["result_cause_category_counts"]["accepted_high_confidence"], 1)
        self.assertEqual(summary["result_cause_category_counts"]["manual_audit_failed"], 1)
        self.assertEqual(summary["result_cause_category_counts"]["accepted_manual_audit"], 1)
        self.assertEqual(summary["result_cause_category_counts"]["proof_coverage_gap"], 1)
        self.assertEqual(summary["result_cause_family_counts"]["accepted"], 2)

    def test_classifies_typed_failure_causes_without_log_text(self) -> None:
        rows = [
            {
                "instance_id": "repo__red-tests-1",
                "status": "predicted",
                "patch_bytes": 100,
                "prediction_verdict": "predicted_failed_verify",
                "prediction_local_confidence": "failed",
                "prediction_blocks_local_acceptance": True,
                "prediction_confidence_downgrade_reason": "make_target_missing",
                "prediction_audit_block_reason": "",
                "verify_status": "failed",
                "verify_failure_kind": "tests_failed",
                "verify_failure_reason_code": "make_target_missing",
                "workflow_status": "complete",
                "local_acceptance_verdict": "fail",
                "local_acceptance_source": "local_audit_block",
                "manual_audit_verdict": "",
            },
            {
                "instance_id": "repo__env-1",
                "status": "predicted",
                "patch_bytes": 90,
                "prediction_verdict": "predicted_unverified",
                "prediction_local_confidence": "failed",
                "prediction_blocks_local_acceptance": True,
                "prediction_confidence_downgrade_reason": "verification_probe_module_not_found",
                "prediction_audit_block_reason": "",
                "verify_status": "unavailable",
                "workflow_status": "complete",
                "local_acceptance_verdict": "fail",
                "local_acceptance_source": "local_audit_block",
                "manual_audit_verdict": "",
            },
            {
                "instance_id": "repo__boundary-1",
                "status": "predicted",
                "patch_bytes": 80,
                "prediction_verdict": "predicted_audit_blocked",
                "prediction_local_confidence": "failed",
                "prediction_blocks_local_acceptance": True,
                "prediction_confidence_downgrade_reason": "",
                "prediction_audit_block_reason": "patch_review_semantic_uncovered:python_nested_string_key_direct_access_added",
                "verify_status": "passed",
                "workflow_status": "complete",
                "local_acceptance_verdict": "fail",
                "local_acceptance_source": "local_audit_block",
                "manual_audit_verdict": "",
            },
            {
                "instance_id": "repo__probe-1",
                "status": "predicted",
                "patch_bytes": 70,
                "prediction_verdict": "predicted_failed_verify",
                "prediction_local_confidence": "failed",
                "prediction_blocks_local_acceptance": True,
                "prediction_confidence_downgrade_reason": "verification_probe_expected_stdout_missing",
                "prediction_audit_block_reason": "",
                "verify_status": "failed",
                "workflow_status": "complete",
                "local_acceptance_verdict": "fail",
                "local_acceptance_source": "local_audit_block",
                "manual_audit_verdict": "",
            },
            {
                "instance_id": "repo__empty-1",
                "status": "empty_patch",
                "patch_bytes": 0,
                "prediction_verdict": "empty_patch",
                "prediction_local_confidence": "none",
                "prediction_blocks_local_acceptance": True,
                "prediction_confidence_downgrade_reason": "",
                "prediction_audit_block_reason": "empty_patch",
                "verify_status": "",
                "workflow_status": "complete",
                "local_acceptance_verdict": "fail",
                "local_acceptance_source": "local_audit_block",
                "manual_audit_verdict": "",
            },
        ]

        summary = summary_mod.summarize_results(rows)

        counts = summary["result_cause_category_counts"]
        self.assertEqual(counts["verify_red_tests_or_build"], 1)
        self.assertEqual(counts["verify_environment_unavailable"], 1)
        self.assertEqual(counts["actual_diff_or_patch_review_gap"], 1)
        self.assertEqual(counts["probe_or_contract_authoring_gap"], 1)
        self.assertEqual(counts["empty_patch_export"], 1)
        self.assertEqual(summary["result_cause_family_counts"]["implementation_or_localization"], 1)
        self.assertEqual(
            summary["result_cause_examples"]["verify_red_tests_or_build"][0]["reason"],
            "tests_failed",
        )

    def test_reports_missing_core_fields_for_old_schema_rows(self) -> None:
        rows = [{
            "instance_id": "repo__old-1",
            "status": "predicted",
            "__source_path": "/tmp/results-old.jsonl",
            "__source_line": 3,
        },
            {"instance_id": "repo__new-1", **{field: "" for field in summary_mod.CORE_FIELDS if field != "instance_id"}},
        ]

        summary = summary_mod.summarize_results(rows)

        self.assertEqual(summary["core_field_presence"]["prediction_verdict"], 1)
        self.assertEqual(summary["current_core_complete_instances"], 1)
        self.assertEqual(summary["current_core_incomplete_instances"], 1)
        self.assertEqual(summary["current_core_complete_percent"], 50.0)
        self.assertEqual(summary["core_missing_field_counts"]["prediction_verdict"], 1)
        self.assertEqual(len(summary["rows_missing_core_fields"]), 1)
        self.assertEqual(summary["rows_missing_core_fields"][0]["instance_id"], "repo__old-1")
        self.assertEqual(summary["rows_missing_core_fields"][0]["source_path"], "/tmp/results-old.jsonl")
        self.assertEqual(summary["rows_missing_core_fields"][0]["source_line"], 3)
        self.assertIn("prediction_verdict", summary["rows_missing_core_fields"][0]["missing_fields"])

    def test_missing_local_acceptance_fields_make_pass_rate_not_evaluable(self) -> None:
        rows = [
            {
                "instance_id": "repo__legacy-pass",
                "status": "predicted",
                "patch_bytes": 100,
                "prediction_verdict": "predicted_passed",
                "prediction_local_confidence": "high",
                "prediction_blocks_local_acceptance": False,
                "prediction_confidence_downgrade_reason": "",
                "prediction_audit_block_reason": "",
                "verify_status": "passed",
            }
        ]

        summary = summary_mod.summarize_results(rows)

        self.assertEqual(summary["local_acceptance_evaluable_instances"], 0)
        self.assertIsNone(summary["local_acceptance_pass_rate_evaluable"])
        self.assertIsNone(summary["high_confidence_local_verify_pass_rate_evaluable"])
        self.assertEqual(summary["local_acceptance_pass_instances"], 0)
        self.assertEqual(summary["current_core_incomplete_instances"], 1)

    def test_dedupe_latest_by_instance_uses_file_mtime(self) -> None:
        rows = [
            {
                "instance_id": "repo__dup-1",
                "prediction_local_confidence": "high",
                "prediction_confidence_downgrade_reason": "",
                "local_acceptance_verdict": "pass",
                "local_acceptance_source": "local_verify",
                "__source_mtime": 10.0,
                "__source_index": 0,
                "__source_line": 1,
            },
            {
                "instance_id": "repo__dup-1",
                "prediction_local_confidence": "unknown",
                "prediction_confidence_downgrade_reason": "verification_probe_import_error",
                "local_acceptance_verdict": "unknown",
                "local_acceptance_source": "",
                "__source_mtime": 20.0,
                "__source_index": 1,
                "__source_line": 1,
            },
            {
                "instance_id": "repo__solo-1",
                "__source_mtime": 15.0,
                "__source_index": 0,
                "__source_line": 2,
            },
        ]

        got = summary_mod.dedupe_latest_by_instance(rows)

        self.assertEqual([row["instance_id"] for row in got], ["repo__dup-1", "repo__solo-1"])
        self.assertEqual(got[0]["prediction_local_confidence"], "unknown")

    def test_cli_writes_json_summary(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            results = root / "results.jsonl"
            output = root / "summary.json"
            results.write_text(
                "\n".join([
                    json.dumps({
                        "instance_id": "repo__a-1",
                        "status": "predicted",
                        "patch_bytes": 10,
                        "prediction_verdict": "predicted_passed",
                        "prediction_local_confidence": "high",
                        "prediction_blocks_local_acceptance": False,
                        "prediction_confidence_downgrade_reason": "",
                        "prediction_audit_block_reason": "",
                        "verify_status": "passed",
                        "workflow_status": "complete",
                        "local_acceptance_verdict": "pass",
                        "local_acceptance_source": "local_verify",
                        "manual_audit_verdict": "",
                    })
                ])
                + "\n",
                encoding="utf-8",
            )

            proc = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--results-jsonl",
                    str(results),
                    "--output-json",
                    str(output),
                ],
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                check=True,
            )

            self.assertIn("codrax_results rows=1", proc.stdout)
            saved = json.loads(output.read_text(encoding="utf-8"))
            self.assertEqual(saved["high_confidence_local_verify_pass_instances"], 1)
            self.assertEqual(saved["current_core_complete_instances"], 1)
            self.assertEqual(saved["results_path"], str(results.resolve()))

    def test_cli_require_current_core_fails_on_legacy_rows(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            results = root / "results.jsonl"
            results.write_text(
                json.dumps({
                    "instance_id": "repo__legacy",
                    "status": "predicted",
                    "patch_bytes": 10,
                    "prediction_verdict": "predicted_passed",
                    "verify_status": "passed",
                })
                + "\n",
                encoding="utf-8",
            )

            proc = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--results-jsonl",
                    str(results),
                    "--require-current-core",
                ],
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )

            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("current core result fields missing", proc.stderr)

    def test_cli_accepts_multiple_files_and_dedupes_latest(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            old = root / "old.results.jsonl"
            new = root / "new.results.jsonl"
            old.write_text(
                json.dumps({
                    "instance_id": "repo__dup-1",
                    "status": "predicted",
                    "patch_bytes": 10,
                    "prediction_verdict": "predicted_passed",
                    "prediction_local_confidence": "high",
                    "prediction_blocks_local_acceptance": False,
                    "prediction_confidence_downgrade_reason": "",
                    "prediction_audit_block_reason": "",
                    "verify_status": "passed",
                    "workflow_status": "complete",
                    "local_acceptance_verdict": "pass",
                    "local_acceptance_source": "local_verify",
                    "manual_audit_verdict": "",
                })
                + "\n",
                encoding="utf-8",
            )
            new.write_text(
                json.dumps({
                    "instance_id": "repo__dup-1",
                    "status": "predicted",
                    "patch_bytes": 12,
                    "prediction_verdict": "predicted_passed_low_confidence",
                    "prediction_local_confidence": "unknown",
                    "prediction_blocks_local_acceptance": False,
                    "prediction_confidence_downgrade_reason": "verification_probe_import_error",
                    "prediction_audit_block_reason": "",
                    "verify_status": "passed",
                    "workflow_status": "complete",
                    "local_acceptance_verdict": "unknown",
                    "local_acceptance_source": "",
                    "manual_audit_verdict": "",
                })
                + "\n",
                encoding="utf-8",
            )
            os.utime(old, (10, 10))
            os.utime(new, (20, 20))

            proc = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--results-jsonl",
                    str(old),
                    "--results-jsonl",
                    str(new),
                    "--dedupe",
                    "latest-by-file-mtime",
                    "--json-only",
                ],
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                check=True,
            )

            saved = json.loads(proc.stdout)
            self.assertEqual(saved["input_row_count"], 2)
            self.assertEqual(saved["row_count"], 1)
            self.assertEqual(saved["dedupe_mode"], "latest-by-file-mtime")
            self.assertEqual(saved["high_confidence_local_verify_pass_instances"], 0)
            self.assertEqual(saved["low_confidence_verify_pass_instances"], 1)


if __name__ == "__main__":
    unittest.main()
