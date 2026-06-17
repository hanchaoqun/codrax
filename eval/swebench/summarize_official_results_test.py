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
SCRIPT = ROOT / "eval" / "swebench" / "summarize_official_results.py"

spec = importlib.util.spec_from_file_location("summarize_official_results", SCRIPT)
assert spec and spec.loader
summary_mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(summary_mod)


class OfficialResultsSummaryTests(unittest.TestCase):
    def test_summarizes_official_run_report_with_explicit_denominators(self) -> None:
        report = {
            "total_instances": 5,
            "submitted_instances": 3,
            "completed_instances": 2,
            "resolved_instances": 1,
            "unresolved_instances": 1,
            "empty_patch_instances": 1,
            "error_instances": 1,
            "submitted_ids": ["a", "b", "c"],
            "completed_ids": ["a", "b"],
            "resolved_ids": ["a"],
            "unresolved_ids": ["b"],
            "empty_patch_ids": ["c"],
            "error_ids": ["d"],
        }

        summary = summary_mod.summarize_run_report(report)

        self.assertEqual(summary["resolved_instances"], 1)
        self.assertEqual(summary["submitted_instances"], 3)
        self.assertEqual(summary["completed_instances"], 2)
        self.assertEqual(summary["total_instances"], 5)
        self.assertAlmostEqual(summary["resolved_rate_submitted"], 1 / 3)
        self.assertAlmostEqual(summary["resolved_rate_completed"], 1 / 2)
        self.assertAlmostEqual(summary["resolved_rate_total"], 1 / 5)
        self.assertEqual(summary["resolved_ids"], ["a"])
        self.assertEqual(summary["empty_patch_ids"], ["c"])

    def test_collects_per_instance_reports_without_importing_harness(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            log_root = root / "logs" / "run_evaluation" / "run-1" / "codrax"
            for instance_id, resolved in {"a": True, "b": False}.items():
                inst = log_root / instance_id
                inst.mkdir(parents=True)
                (inst / "report.json").write_text(
                    json.dumps({instance_id: {"resolved": resolved}}),
                    encoding="utf-8",
                )
            predictions = root / "predictions.jsonl"
            predictions.write_text(
                "\n".join(
                    [
                        json.dumps({"instance_id": "a", "model_patch": "diff --git a/a b/a"}),
                        json.dumps({"instance_id": "b", "model_patch": "diff --git a/b b/b"}),
                        json.dumps({"instance_id": "c", "model_patch": ""}),
                        json.dumps({"instance_id": "d", "model_patch": "diff --git a/d b/d"}),
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            loaded_predictions = summary_mod.load_predictions(predictions)
            summary = summary_mod.summarize_instance_reports(root / "logs" / "run_evaluation", "run-1", loaded_predictions, "codrax")

        self.assertEqual(summary["submitted_instances"], 4)
        self.assertEqual(summary["completed_instances"], 2)
        self.assertEqual(summary["resolved_instances"], 1)
        self.assertEqual(summary["unresolved_instances"], 1)
        self.assertEqual(summary["empty_patch_instances"], 1)
        self.assertEqual(summary["error_instances"], 1)
        self.assertEqual(summary["resolved_ids"], ["a"])
        self.assertEqual(summary["error_ids"], ["d"])

    def test_cli_writes_normalized_json(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            report = root / "codrax.run-1.json"
            output = root / "summary.json"
            report.write_text(
                json.dumps(
                    {
                        "total_instances": 2,
                        "submitted_instances": 2,
                        "completed_instances": 2,
                        "resolved_instances": 1,
                        "unresolved_instances": 1,
                        "resolved_ids": ["ok"],
                        "unresolved_ids": ["bad"],
                    }
                ),
                encoding="utf-8",
            )

            proc = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--run-report",
                    str(report),
                    "--run-id",
                    "run-1",
                    "--output-json",
                    str(output),
                ],
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                check=True,
            )

            self.assertIn("official_resolved submitted=1/2", proc.stdout)
            saved = json.loads(output.read_text(encoding="utf-8"))
            self.assertEqual(saved["run_id"], "run-1")
            self.assertEqual(saved["resolved_percent_submitted"], 50.0)


if __name__ == "__main__":
    unittest.main()
