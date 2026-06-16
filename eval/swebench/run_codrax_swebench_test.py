#!/usr/bin/env python3
"""Focused self-tests for the Codrax SWE-bench adapter."""

from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("run_codrax_swebench.py")
SPEC = importlib.util.spec_from_file_location("run_codrax_swebench", SCRIPT)
assert SPEC and SPEC.loader
adapter = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = adapter
SPEC.loader.exec_module(adapter)


def probe_only_report() -> dict:
    return {
        "verification_status": "passed",
        "passed": True,
        "test_results": [
            {
                "suite": "verification_probe/python",
                "passed": True,
            }
        ],
        "executed_commands": [
            {
                "runner": "verification_probe",
                "outcome": "executed",
                "exit_code": 0,
            },
            {
                "runner": "python",
                "outcome": "suite_skipped",
                "exit_code": 0,
            },
        ],
    }


def plan_with_probe() -> dict:
    return {
        "behavior_contracts": [
            {
                "id": "contract-1",
                "required": True,
                "source": "expected_outcome_fallback",
            }
        ],
        "verification_probes": [
            {
                "id": "probe-1",
                "contract_refs": ["contract-1"],
                "changed_symbol_refs": ["pkg.entrypoint"],
            }
        ],
    }


class PredictionConfidenceTests(unittest.TestCase):
    def test_probe_only_pass_downgrades_when_changed_source_lacks_context(self) -> None:
        reason = adapter.prediction_confidence_downgrade_reason(
            plan=plan_with_probe(),
            report=probe_only_report(),
            verify_status="passed",
            plan_source_paths=["pkg/__init__.py"],
            plan_context_paths=["pkg/config/initialization.py", "pkg/runtime.py"],
        )

        self.assertEqual(reason, "verification_probe_changed_source_not_context_covered")

    def test_probe_only_pass_keeps_confidence_when_changed_source_has_context(self) -> None:
        reason = adapter.prediction_confidence_downgrade_reason(
            plan=plan_with_probe(),
            report=probe_only_report(),
            verify_status="passed",
            plan_source_paths=["src/_pytest/python_api.py"],
            plan_context_paths=["src/_pytest/python_api.py:RaisesContext", "src/_pytest/outcomes.py"],
        )

        self.assertEqual(reason, "")

    def test_project_runner_pass_is_not_downgraded_by_context_coverage(self) -> None:
        report = probe_only_report()
        report["executed_commands"].append(
            {
                "runner": "python",
                "outcome": "executed",
                "exit_code": 0,
            }
        )

        reason = adapter.prediction_confidence_downgrade_reason(
            plan=plan_with_probe(),
            report=report,
            verify_status="passed",
            plan_source_paths=["pkg/__init__.py"],
            plan_context_paths=["pkg/config/initialization.py"],
        )

        self.assertEqual(reason, "")


class ContextCoverageTests(unittest.TestCase):
    def test_symbol_qualified_context_paths_cover_file_level_change(self) -> None:
        missing = adapter.plan_source_paths_missing_prior_context(
            ["src/_pytest/python_api.py"],
            ["src/_pytest/python_api.py:RaisesContext"],
        )

        self.assertEqual(missing, [])


if __name__ == "__main__":
    unittest.main()
