#!/usr/bin/env python3
from __future__ import annotations

import importlib.util
import json
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
        self.assertEqual(summary["typed_manual_audit_pass_instances"], 1)
        self.assertEqual(summary["typed_manual_audit_fail_instances"], 1)
        self.assertEqual(summary["local_audit_blocked_instances"], 1)
        self.assertEqual(summary["prediction_verdict_counts"]["predicted_passed_low_confidence"], 1)
        self.assertEqual(summary["top_confidence_downgrade_reasons"][0]["value"], "<empty>")

    def test_reports_missing_core_fields_for_old_schema_rows(self) -> None:
        rows = [
            {"instance_id": "repo__old-1", "status": "predicted"},
            {"instance_id": "repo__new-1", **{field: "" for field in summary_mod.CORE_FIELDS if field != "instance_id"}},
        ]

        summary = summary_mod.summarize_results(rows)

        self.assertEqual(summary["core_field_presence"]["prediction_verdict"], 1)
        self.assertEqual(len(summary["rows_missing_core_fields"]), 1)
        self.assertEqual(summary["rows_missing_core_fields"][0]["instance_id"], "repo__old-1")
        self.assertIn("prediction_verdict", summary["rows_missing_core_fields"][0]["missing_fields"])

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
            self.assertEqual(saved["results_path"], str(results))


if __name__ == "__main__":
    unittest.main()
