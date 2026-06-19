#!/usr/bin/env python3
"""Tests for validate_predictions.py."""

from __future__ import annotations

import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("validate_predictions.py")
SPEC = importlib.util.spec_from_file_location("validate_predictions", SCRIPT)
assert SPEC and SPEC.loader
validator = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = validator
SPEC.loader.exec_module(validator)


def write_jsonl(path: Path, rows: list[dict]) -> None:
    path.write_text("".join(json.dumps(row) + "\n" for row in rows), encoding="utf-8")


class ValidatePredictionTests(unittest.TestCase):
    def test_results_audit_telemetry_matches_prediction_ids(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            predictions = root / "predictions.jsonl"
            results = root / "results.jsonl"
            write_jsonl(predictions, [{
                "instance_id": "repo__proj-1",
                "model_name_or_path": "codrax",
                "model_patch": "diff --git a/a.py b/a.py\n",
            }])
            write_jsonl(results, [{
                "instance_id": "repo__proj-1",
                "final_report_present": True,
                "final_report_path": "/tmp/plan.final.json",
                "final_report_completion_verdict": "verified",
                "final_report_verification_status": "passed",
                "final_report_loop_status": "complete",
                "final_report_loop_truth_state": "covered",
                "final_report_loop_event_refs": ["wf:000001:run_seeded"],
                "final_report_patch_language_families": ["python", "arkts", "cangjie", "cpp"],
                "final_report_verification_runner_families": ["python", "javascript", "typescript", "go"],
            }])

            with unittest.mock.patch.object(sys, "argv", [
                "validate_predictions.py",
                str(predictions),
                "--results-jsonl",
                str(results),
                "--require-final-report-audit",
            ]):
                self.assertEqual(validator.main(), 0)

    def test_require_final_report_audit_fails_when_results_are_missing(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            predictions = root / "predictions.jsonl"
            write_jsonl(predictions, [{
                "instance_id": "repo__proj-1",
                "model_patch": "diff --git a/a.py b/a.py\n",
            }])

            with unittest.mock.patch.object(sys, "argv", [
                "validate_predictions.py",
                str(predictions),
                "--require-final-report-audit",
            ]):
                with self.assertRaises(SystemExit) as ctx:
                    validator.main()

        self.assertIn("--results-jsonl", str(ctx.exception))

    def test_results_audit_telemetry_fails_loud_on_missing_language_families(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            predictions = root / "predictions.jsonl"
            results = root / "results.jsonl"
            write_jsonl(predictions, [{
                "instance_id": "repo__proj-1",
                "model_patch": "diff --git a/a.py b/a.py\n",
            }])
            write_jsonl(results, [{
                "instance_id": "repo__proj-1",
                "final_report_present": True,
                "final_report_path": "/tmp/plan.final.json",
                "final_report_completion_verdict": "verified",
                "final_report_verification_status": "passed",
                "final_report_loop_status": "complete",
                "final_report_loop_truth_state": "covered",
                "final_report_loop_event_refs": ["wf:000001:run_seeded"],
                "final_report_patch_language_families": [],
                "final_report_verification_runner_families": ["python"],
            }])

            with self.assertRaises(SystemExit) as ctx:
                validator.validate_results_jsonl(results, {"repo__proj-1"}, require_final_report_audit=True)

        self.assertIn("final_report_patch_language_families", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
