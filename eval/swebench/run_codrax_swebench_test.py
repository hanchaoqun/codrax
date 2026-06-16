#!/usr/bin/env python3
"""Focused self-tests for the Codrax SWE-bench adapter."""

from __future__ import annotations

import importlib.util
import json
import sys
import tempfile
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


def plan_with_caller_return_adapter() -> dict:
    return {
        "changes": [
            {
                "path": "sklearn/calibration.py",
                "kind": "patch",
                "edits": [
                    {
                        "kind": "replace",
                        "old_text": "proba[:, class_idx] = calibrator.predict(this_pred)",
                        "content": "proba[:, class_idx] = np.ravel(calibrator.predict(this_pred))",
                    }
                ],
            }
        ],
        "behavior_contracts": [
            {
                "id": "contract-1",
                "required": True,
            }
        ],
        "verification_probes": [
            {
                "id": "probe-1",
                "contract_refs": ["contract-1"],
                "changed_symbol_refs": ["sklearn.calibration._CalibratedClassifier"],
            }
        ],
    }


def plan_with_warning_suppression() -> dict:
    return {
        "changes": [
            {
                "path": "sphinx/domains/std.py",
                "kind": "patch",
                "edits": [
                    {
                        "kind": "replace",
                        "old_text": "        except ValueError:\n            logger.warning(_('no number is assigned'), labelid,\n                           location=node)\n            return contnode",
                        "content": "        except ValueError:\n            if figtype != 'table':\n                logger.warning(_('no number is assigned'), labelid,\n                               location=node)\n            return contnode",
                    }
                ],
            }
        ],
        "behavior_contracts": [
            {
                "id": "contract-1",
                "required": True,
            }
        ],
    }


def plan_with_external_private_state_write() -> dict:
    return {
        "changes": [
            {
                "path": "lib/matplotlib/cm.py",
                "kind": "patch",
                "edits": [
                    {
                        "kind": "insert_after",
                        "old_text": "        self.norm = norm",
                        "content": "        self.norm = norm\n        for ref in self._colorbar_cids:\n            colorbar = ref()\n            if colorbar is not None:\n                colorbar._norm = self.norm",
                    }
                ],
            }
        ]
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

    def test_probe_only_pass_downgrades_when_prior_context_is_missing(self) -> None:
        reason = adapter.prediction_confidence_downgrade_reason(
            plan=plan_with_probe(),
            report=probe_only_report(),
            verify_status="passed",
            plan_source_paths=["sympy/ntheory/residue_ntheory.py"],
            plan_context_paths=[],
        )

        self.assertEqual(reason, "verification_probe_changed_source_not_context_covered")

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

    def test_probe_only_pass_downgrades_for_caller_return_adapter_signal(self) -> None:
        signals = adapter.plan_owner_boundary_signals(plan_with_caller_return_adapter())
        reason_codes = [row["reason_code"] for row in signals]

        reason = adapter.prediction_confidence_downgrade_reason(
            plan=plan_with_caller_return_adapter(),
            report=probe_only_report(),
            verify_status="passed",
            plan_source_paths=["sklearn/calibration.py"],
            plan_context_paths=["sklearn/calibration.py"],
            owner_boundary_reason_codes=reason_codes,
        )

        self.assertEqual(reason, "caller_return_shape_adapter")

    def test_project_pass_downgrades_for_structural_patch_quality_signal(self) -> None:
        signals = adapter.plan_owner_boundary_signals(plan_with_warning_suppression())
        reason_codes = [row["reason_code"] for row in signals]
        report = probe_only_report()
        report["test_results"][0]["suite"] = "tests/test_domain_std.py"
        report["executed_commands"][0]["runner"] = "python"

        reason = adapter.prediction_confidence_downgrade_reason(
            plan=plan_with_warning_suppression(),
            report=report,
            verify_status="passed",
            owner_boundary_reason_codes=reason_codes,
        )

        self.assertEqual(reason, "diagnostic_signal_conditionally_suppressed")


class ContextCoverageTests(unittest.TestCase):
    def test_symbol_qualified_context_paths_cover_file_level_change(self) -> None:
        missing = adapter.plan_source_paths_missing_prior_context(
            ["src/_pytest/python_api.py"],
            ["src/_pytest/python_api.py:RaisesContext"],
        )

        self.assertEqual(missing, [])

    def test_missing_context_reports_all_changed_source_paths(self) -> None:
        missing = adapter.plan_source_paths_missing_prior_context(
            ["pkg/a.py", "pkg/tests/test_a.py", "pkg/b.py"],
            [],
        )

        self.assertEqual(missing, ["pkg/a.py", "pkg/b.py"])


class WorkflowAppliedProvenanceTests(unittest.TestCase):
    def test_summarizes_applied_source_and_test_plan_paths(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            repo = Path(raw) / "repo"
            inst = Path(raw) / "inst"
            plans = repo / ".codrax" / "plans"
            plans.mkdir(parents=True)
            inst.mkdir()
            (plans / "plan-source.json").write_text(
                json.dumps({
                    "id": "plan-source",
                    "summary": "source fix",
                    "changes": [{"path": "pkg/fix.py", "kind": "patch"}],
                }),
                encoding="utf-8",
            )
            (plans / "plan-test.json").write_text(
                json.dumps({
                    "id": "plan-test",
                    "summary": "test change",
                    "changes": [{"path": "tests/test_fix.py", "kind": "patch"}],
                }),
                encoding="utf-8",
            )
            workflow = {
                "batches": [
                    {
                        "attempts": [
                            {"kind": "plan", "status": "complete", "plan_id": "plan-source"},
                            {"kind": "apply", "status": "applied", "plan_id": "plan-source"},
                            {"kind": "apply", "status": "failed", "plan_id": "plan-failed"},
                            {"kind": "apply", "status": "applied", "plan_id": "plan-source"},
                            {"kind": "apply", "status": "applied", "plan_id": "plan-test"},
                        ]
                    }
                ]
            }

            got = adapter.summarize_workflow_applied_plan_provenance(
                workflow,
                repo,
                inst,
                final_plan_id="plan-test",
            )

        self.assertEqual(got["workflow_applied_plan_ids"], ["plan-source", "plan-test"])
        self.assertEqual(got["workflow_latest_applied_plan_id"], "plan-test")
        self.assertTrue(got["workflow_final_plan_is_latest_applied"])
        self.assertEqual(got["workflow_applied_source_paths"], ["pkg/fix.py"])
        self.assertEqual(got["workflow_applied_test_paths"], ["tests/test_fix.py"])


class OwnerBoundarySignalTests(unittest.TestCase):
    def test_detects_python_caller_return_shape_adapter(self) -> None:
        signals = adapter.plan_owner_boundary_signals(plan_with_caller_return_adapter())

        self.assertEqual(
            signals,
            [
                {
                    "adapter": "np.ravel",
                    "inner_call": "calibrator.predict",
                    "reason_code": "caller_return_shape_adapter",
                    "path": "sklearn/calibration.py",
                    "edit_index": 0,
                }
            ],
        )

    def test_ignores_non_adapter_call_wrappers(self) -> None:
        plan = plan_with_caller_return_adapter()
        plan["changes"][0]["edits"][0]["content"] = "proba[:, class_idx] = validate(calibrator.predict(this_pred))"

        self.assertEqual(adapter.plan_owner_boundary_signals(plan), [])

    def test_detects_conditionally_suppressed_warning(self) -> None:
        signals = adapter.plan_owner_boundary_signals(plan_with_warning_suppression())

        self.assertEqual(
            signals,
            [
                {
                    "reason_code": "diagnostic_signal_conditionally_suppressed",
                    "path": "sphinx/domains/std.py",
                    "edit_index": 0,
                }
            ],
        )

    def test_detects_external_private_state_sync_workaround(self) -> None:
        signals = adapter.plan_owner_boundary_signals(plan_with_external_private_state_write())

        self.assertEqual(
            signals,
            [
                {
                    "reason_code": "external_private_state_sync_workaround",
                    "targets": ["colorbar._norm"],
                    "path": "lib/matplotlib/cm.py",
                    "edit_index": 0,
                }
            ],
        )


class EmptyPatchReasonTests(unittest.TestCase):
    def test_wall_time_progress_reason_overrides_in_progress_no_plan(self) -> None:
        reason = adapter.empty_patch_reason(
            patch="",
            workflow_status="in_progress",
            plan_path="",
            workflow_latest_reason_code="plan_batch_canceled",
            workflow_latest_message="canceled at stage=write_controller_plan_retry iter=7: write mode wall-time exceeded (600s)",
        )

        self.assertEqual(reason, "write_wall_time_empty_patch")

    def test_legacy_wall_time_progress_message_is_still_auditable(self) -> None:
        reason = adapter.empty_patch_reason(
            patch="",
            workflow_status="in_progress",
            plan_path="",
            workflow_latest_reason_code="plan_batch_failed",
            workflow_latest_message="canceled at stage=write_controller_plan_retry iter=7: write mode wall-time exceeded (600s)",
        )

        self.assertEqual(reason, "write_wall_time_empty_patch")


class PredictionAuditBlockTests(unittest.TestCase):
    def test_verify_infra_block_overrides_stale_failed_plan_status(self) -> None:
        reason = adapter.prediction_audit_block_reason(
            exported_source_paths=["django/forms/models.py"],
            final_plan_source_paths=["django/forms/models.py"],
            final_plan_test_only=False,
            final_plan_covers_exported_source_patch=True,
            workflow_status="blocked",
            verify_status="",
            plan_status="verify_failed",
            workflow_latest_reason_code="verify_infra_retry_budget_exhausted",
        )

        self.assertEqual(reason, "workflow_blocked_after_verify_infra")

        verdict, confidence, blocks = adapter.prediction_verdict(
            "diff --git a/django/forms/models.py b/django/forms/models.py\n",
            "",
            "verify_failed",
            reason,
            "",
        )

        self.assertEqual(verdict, "predicted_audit_blocked")
        self.assertEqual(confidence, "unknown")
        self.assertTrue(blocks)


if __name__ == "__main__":
    unittest.main()
