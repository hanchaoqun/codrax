#!/usr/bin/env python3
"""Focused self-tests for the Codrax SWE-bench adapter."""

from __future__ import annotations

import importlib.util
import io
import json
import os
import subprocess
import sys
import tempfile
import unittest
from types import SimpleNamespace
from contextlib import redirect_stderr
from unittest import mock
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
    def test_probe_only_pass_recognizes_typed_multilanguage_suites(self) -> None:
        for suite in [
            "verification_probe/javascript",
            "verification_probe/ruby",
            "verification_probe/go",
        ]:
            with self.subTest(suite=suite):
                report = probe_only_report()
                report["test_results"][0]["suite"] = suite

                self.assertTrue(adapter.report_passed_by_verification_probe(report))
                reason = adapter.prediction_confidence_downgrade_reason(
                    plan=plan_with_probe(),
                    report=report,
                    verify_status="passed",
                )

                self.assertEqual(reason, "")

    def test_probe_only_pass_rejects_untyped_or_mixed_suites(self) -> None:
        for suite in [
            "verification_probe",
            "verification_probe/",
            "verification_probe/python/extra",
            "tests/test_widget.py",
        ]:
            with self.subTest(suite=suite):
                report = probe_only_report()
                report["test_results"][0]["suite"] = suite

                self.assertFalse(adapter.report_passed_by_verification_probe(report))

        mixed = probe_only_report()
        mixed["test_results"].append({"suite": "tests/test_widget.py", "passed": True})
        self.assertFalse(adapter.report_passed_by_verification_probe(mixed))

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

    def test_probe_only_pass_downgrades_soft_fallback_contracts_separately(self) -> None:
        plan = {
            "behavior_contracts": [
                {"id": "outcome-1", "required": True, "source": "expected_outcome_fallback"},
                {
                    "id": "soft-satisfies",
                    "required": True,
                    "source": "write_analyzer",
                    "operator": "satisfies",
                    "polarity": "expected",
                },
            ],
            "verification_probes": [
                {"id": "probe-1", "language": "python", "changed_symbol_refs": ["pkg.entrypoint"]},
            ],
        }

        reason = adapter.prediction_confidence_downgrade_reason(
            plan=plan,
            report=probe_only_report(),
            verify_status="passed",
        )

        self.assertEqual(reason, "verification_probe_missing_soft_contract_ref")

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


class LocalAcceptanceTests(unittest.TestCase):
    def test_raw_pass_without_authoritative_status_is_not_local_acceptance(self) -> None:
        report = {
            "passed": True,
            "failure_kind": "no_tests",
            "no_tests_runners": ["pytest"],
            "test_results": [],
        }

        self.assertEqual(adapter.report_verification_status(report), "unavailable")
        self.assertFalse(adapter.report_authoritative_verify_passed(report))

        verdict, source = adapter.local_acceptance_verdict(
            verify_status=adapter.report_verification_status(report),
            prediction_blocks_local_acceptance=False,
            manual_audit_verdict="",
        )

        self.assertEqual((verdict, source), ("unknown", ""))

    def test_local_verify_pass_counts_as_internal_acceptance(self) -> None:
        verdict, source = adapter.local_acceptance_verdict(
            verify_status="passed",
            prediction_blocks_local_acceptance=False,
        )

        self.assertEqual((verdict, source), ("pass", "local_verify"))

    def test_low_confidence_verify_pass_is_not_internal_acceptance(self) -> None:
        verdict, source = adapter.local_acceptance_verdict(
            verify_status="passed",
            prediction_blocks_local_acceptance=False,
            confidence_downgrade_reason="patch_review_semantic_unverified:behavior_contract_without_verify_coverage",
        )

        self.assertEqual((verdict, source), ("unknown", ""))

    def test_manual_pass_can_accept_low_confidence_verify_pass(self) -> None:
        verdict, source = adapter.local_acceptance_verdict(
            verify_status="passed",
            prediction_blocks_local_acceptance=False,
            confidence_downgrade_reason="verification_probe_changed_source_not_context_covered",
            manual_audit_verdict="pass",
        )

        self.assertEqual((verdict, source), ("pass", "manual_audit"))

    def test_manual_fail_overrides_local_verify_pass(self) -> None:
        verdict, source = adapter.local_acceptance_verdict(
            verify_status="passed",
            prediction_blocks_local_acceptance=False,
            manual_audit_verdict="fail",
        )

        self.assertEqual((verdict, source), ("fail", "manual_audit"))

    def test_manual_pass_counts_only_when_local_verify_is_not_failed(self) -> None:
        verdict, source = adapter.local_acceptance_verdict(
            verify_status="unavailable",
            prediction_blocks_local_acceptance=False,
            manual_audit_verdict="pass",
        )

        self.assertEqual((verdict, source), ("pass", "manual_audit"))

    def test_failed_verify_is_not_overridden_by_manual_pass(self) -> None:
        verdict, source = adapter.local_acceptance_verdict(
            verify_status="failed",
            prediction_blocks_local_acceptance=False,
            manual_audit_verdict="pass",
        )

        self.assertEqual((verdict, source), ("fail", "local_verify"))

    def test_hard_audit_block_is_not_overridden_by_manual_pass(self) -> None:
        verdict, source = adapter.local_acceptance_verdict(
            verify_status="unavailable",
            prediction_blocks_local_acceptance=True,
            manual_audit_verdict="pass",
        )

        self.assertEqual((verdict, source), ("fail", "local_audit_block"))

    def test_manual_audit_loader_accepts_typed_verdicts(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "manual.jsonl"
            path.write_text(
                json.dumps({
                    "instance_id": "proj__repo-1",
                    "verdict": "pass",
                    "reason_code": "manual_patch_review",
                    "source": "human",
                    "notes": "free-form note is audit-only",
                }) + "\n",
                encoding="utf-8",
            )

            got = adapter.load_manual_audits(path)

        self.assertEqual(got["proj__repo-1"]["verdict"], "pass")
        self.assertEqual(got["proj__repo-1"]["reason_code"], "manual_patch_review")
        self.assertEqual(got["proj__repo-1"]["notes"], "free-form note is audit-only")


class FinalReportProjectionTests(unittest.TestCase):
    def test_plan_localization_review_summary_projects_typed_plan_fields(self) -> None:
        got = adapter.plan_localization_review_summary({
            "localization_review": {
                "status": "missing",
                "reason_codes": ["plan_source_paths_missing_prior_context", ""],
                "source_paths": ["pkg/caller.py"],
                "supported_paths": [],
                "missing_paths": ["pkg/caller.py"],
                "prior_context_paths": ["pkg/owner.py"],
            }
        })

        self.assertEqual(got["plan_localization_review_status"], "missing")
        self.assertEqual(got["plan_localization_review_reason_codes"], ["plan_source_paths_missing_prior_context"])
        self.assertEqual(got["plan_localization_source_paths"], ["pkg/caller.py"])
        self.assertEqual(got["plan_localization_missing_paths"], ["pkg/caller.py"])
        self.assertEqual(got["plan_localization_prior_context_paths"], ["pkg/owner.py"])

    def test_load_and_project_final_report_fields(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            plans = Path(raw)
            plan_path = plans / "plan-1.json"
            plan_path.write_text(json.dumps({"id": "plan-1"}), encoding="utf-8")
            final_path = plans / "plan-1.final.json"
            final_path.write_text(
                json.dumps({
                    "schema_version": 1,
                    "kind": "final_report",
                    "completion": {"verdict": "unverified", "reason_code": "runner_missing"},
                    "plan": {
                        "id": "plan-1",
                        "localization": {
                            "status": "weak",
                            "reason_codes": ["plan_source_paths_without_prior_context"],
                            "missing_paths": ["src/app.py"],
                            "supported_paths": ["src/owner.py"],
                        },
                    },
                    "patch": {"diff_fingerprint": "abc123"},
                    "verification": {"status": "unavailable"},
                    "patch_review": {"verdict": "unverified"},
                    "handoff": {
                        "top_items": [
                            {"evidence_ref": "src/app.py:10", "fingerprint": "fp-1"},
                            {"fingerprint": "fp-2"},
                        ]
                    },
                    "residual_risks": [
                        {"code": "verification_unavailable"},
                        {"code": "patch_review_semantic_unverified"},
                    ],
                }),
                encoding="utf-8",
            )

            final_report, loaded_path = adapter.load_final_report_for_plan(plan_path)
            result: dict[str, object] = {}
            adapter.apply_final_report_result_fields(result, final_report, loaded_path)

        self.assertTrue(result["final_report_present"])
        self.assertEqual(result["final_report_path"], str(final_path))
        self.assertEqual(result["final_report_completion_verdict"], "unverified")
        self.assertEqual(result["final_report_completion_reason_code"], "runner_missing")
        self.assertEqual(result["final_report_verification_status"], "unavailable")
        self.assertEqual(result["final_report_patch_review_verdict"], "unverified")
        self.assertEqual(result["final_report_localization_status"], "weak")
        self.assertEqual(
            result["final_report_localization_reason_codes"],
            ["plan_source_paths_without_prior_context"],
        )
        self.assertEqual(result["final_report_localization_missing_paths"], ["src/app.py"])
        self.assertEqual(result["final_report_localization_supported_paths"], ["src/owner.py"])
        self.assertEqual(result["final_report_plan_id"], "plan-1")
        self.assertEqual(result["final_report_patch_fingerprint"], "abc123")
        self.assertEqual(
            result["final_report_residual_risk_codes"],
            ["verification_unavailable", "patch_review_semantic_unverified"],
        )
        self.assertEqual(
            result["final_report_handoff_evidence_refs"],
            ["src/app.py:10", "fp-1", "fp-2"],
        )

    def test_missing_final_report_fields_are_explicitly_empty(self) -> None:
        result: dict[str, object] = {}
        adapter.apply_final_report_result_fields(result, {}, None)

        self.assertFalse(result["final_report_present"])
        self.assertEqual(result["final_report_path"], "")
        self.assertEqual(result["final_report_completion_verdict"], "")
        self.assertEqual(result["final_report_localization_status"], "")
        self.assertEqual(result["final_report_localization_missing_paths"], [])
        self.assertEqual(result["final_report_residual_risk_codes"], [])
        self.assertEqual(result["final_report_handoff_evidence_refs"], [])


class PatchReviewSummaryTests(unittest.TestCase):
    def test_patch_review_error_blocks_local_acceptance(self) -> None:
        summary = adapter.plan_patch_review_summary({
            "patch_review": {
                "status": "failed",
                "hard_block": True,
                "coverage_summary": {
                    "verdict": "failed",
                    "block_reason": "patch_review_error:patch_effect_path_outside_plan_scope",
                    "reason_codes": ["patch_effect_path_outside_plan_scope"],
                },
                "findings": [{
                    "code": "patch_effect_path_outside_plan_scope",
                    "severity": "error",
                    "category": "scope",
                }],
            },
        })

        self.assertEqual(summary["block_reason"], "patch_review_error:patch_effect_path_outside_plan_scope")
        self.assertEqual(summary["coverage_verdict"], "failed")
        self.assertTrue(summary["hard_block"])
        self.assertEqual(summary["reason_codes"], ["patch_effect_path_outside_plan_scope"])

    def test_semantic_unverified_patch_review_blocks_local_acceptance(self) -> None:
        summary = adapter.plan_patch_review_summary({
            "patch_review": {
                "status": "passed",
                "findings": [{
                    "code": "changed_symbol_without_probe_coverage",
                    "severity": "warning",
                    "category": "semantic_coverage",
                    "coverage_status": "unverified",
                    "subject_symbol": "pkg.Target",
                }],
            },
        })

        self.assertEqual(
            summary["block_reason"],
            "patch_review_semantic_unverified:changed_symbol_without_probe_coverage",
        )
        self.assertEqual(summary["semantic_unverified_codes"], ["changed_symbol_without_probe_coverage"])
        self.assertEqual(summary["uncovered_impact_kinds"], ["changed_symbol"])
        self.assertEqual(summary["impact_kind_coverage"][0]["kind"], "changed_symbol")

    def test_actual_diff_boundary_patch_review_blocks_local_acceptance(self) -> None:
        summary = adapter.plan_patch_review_summary({
            "patch_review": {
                "status": "passed",
                "coverage_summary": {
                    "verdict": "unverified",
                    "block_reason": "patch_review_semantic_uncovered:python_nested_string_key_direct_access_added",
                    "reason_codes": ["python_nested_string_key_direct_access_added"],
                },
                "findings": [{
                    "code": "python_nested_string_key_direct_access_added",
                    "severity": "warning",
                    "category": "semantic_coverage",
                    "coverage_status": "unverified",
                    "path": "pkg/widget.py",
                }],
            },
        })

        self.assertEqual(
            summary["block_reason"],
            "patch_review_semantic_uncovered:python_nested_string_key_direct_access_added",
        )
        self.assertEqual(summary["semantic_unverified_codes"], ["python_nested_string_key_direct_access_added"])
        self.assertEqual(summary["uncovered_impact_kinds"], ["effect_followup"])

    def test_patch_review_summary_consumes_impact_kind_coverage(self) -> None:
        summary = adapter.plan_patch_review_summary({
            "patch_review": {
                "status": "passed",
                "coverage_summary": {
                    "verdict": "unverified",
                    "impact_kind_coverage": [{
                        "kind": "behavior_contract",
                        "total": 1,
                        "unverified": 1,
                        "reason_codes": ["behavior_contract_without_verify_coverage"],
                    }],
                },
                "findings": [],
            },
        })

        self.assertEqual(summary["impact_kind_coverage"], [{
            "kind": "behavior_contract",
            "total": 1,
            "verified": 0,
            "unverified": 1,
            "unavailable": 0,
            "unknown": 0,
            "advisory": 0,
            "reason_codes": ["behavior_contract_without_verify_coverage"],
        }])

    def test_dependent_surface_unverified_is_confidence_telemetry(self) -> None:
        summary = adapter.plan_patch_review_summary({
            "patch_review": {
                "status": "passed",
                "coverage_summary": {
                    "verdict": "unverified",
                    "block_reason": "patch_review_semantic_uncovered:dependent_surface_without_verify_coverage",
                    "reason_codes": ["dependent_surface_without_verify_coverage"],
                },
                "findings": [{
                    "code": "dependent_surface_without_verify_coverage",
                    "severity": "warning",
                    "category": "semantic_coverage",
                    "coverage_status": "unverified",
                    "related_path": "pkg/tests/test_surface.py",
                }],
            },
        })

        self.assertEqual(summary["block_reason"], "")
        self.assertEqual(summary["semantic_unverified_codes"], [])
        self.assertEqual(summary["semantic_unverified_telemetry_codes"], ["dependent_surface_without_verify_coverage"])
        self.assertEqual(summary["uncovered_impact_kinds"], [])
        self.assertEqual(summary["uncovered_impact_kind_telemetry"], ["dependent"])
        self.assertEqual(
            adapter.patch_review_confidence_downgrade_reason(summary),
            "patch_review_semantic_unverified:dependent_surface_without_verify_coverage",
        )

    def test_verified_semantic_patch_review_is_telemetry_only(self) -> None:
        summary = adapter.plan_patch_review_summary({
            "patch_review": {
                "status": "passed",
                "findings": [{
                    "code": "changed_symbol_without_probe_coverage",
                    "severity": "warning",
                    "category": "semantic_coverage",
                    "coverage_status": "verified",
                    "subject_symbol": "pkg.Target",
                }],
            },
        })

        self.assertEqual(summary["block_reason"], "")
        self.assertEqual(summary["semantic_unverified_codes"], [])
        self.assertEqual(summary["reason_codes"], ["changed_symbol_without_probe_coverage"])

    def test_delivery_patch_review_demotes_stale_proof_blockers_after_passed_report(self) -> None:
        stale_source_plan = {
            "id": "plan-old",
            "patch_review": {
                "status": "passed",
                "coverage_summary": {
                    "verdict": "unverified",
                    "block_reason": "patch_review_semantic_unverified:changed_symbol_without_probe_coverage",
                    "reason_codes": ["changed_symbol_without_probe_coverage"],
                },
                "findings": [{
                    "code": "changed_symbol_without_probe_coverage",
                    "severity": "warning",
                    "category": "semantic_coverage",
                    "coverage_status": "unverified",
                    "subject_symbol": "pkg.Target",
                }],
            },
        }
        final_report_plan = {
            "id": "plan-final",
            "patch_review": {
                "status": "passed",
                "coverage_summary": {
                    "verdict": "unverified",
                    "block_reason": "patch_review_semantic_uncovered:dependent_surface_without_verify_coverage",
                    "reason_codes": ["dependent_surface_without_verify_coverage"],
                },
                "findings": [{
                    "code": "dependent_surface_without_verify_coverage",
                    "severity": "warning",
                    "category": "semantic_coverage",
                    "coverage_status": "unverified",
                    "related_path": "tests/test_target.py",
                }],
            },
        }

        summary = adapter.delivery_patch_review_summary(
            source_owner_plans=[stale_source_plan, final_report_plan],
            delivery_plan=final_report_plan,
            report_plan_id="plan-final",
            verify_status="passed",
            delivery_status="coherent",
        )

        self.assertEqual(summary["block_reason"], "")
        self.assertEqual(summary["semantic_unverified_codes"], [])
        self.assertEqual(summary["uncovered_impact_kinds"], [])
        self.assertEqual(summary["uncovered_impact_kind_telemetry"], ["changed_symbol", "dependent"])
        self.assertEqual(
            summary["semantic_unverified_telemetry_codes"],
            ["changed_symbol_without_probe_coverage", "dependent_surface_without_verify_coverage"],
        )

    def test_patch_review_confidence_uses_final_report_after_passed_delivery(self) -> None:
        delivery_summary = {
            "block_reason": "",
            "coverage_verdict": "unverified",
            "semantic_unverified_telemetry_codes": ["changed_symbol_without_probe_coverage"],
        }
        final_summary = adapter.plan_patch_review_summary({
            "patch_review": {
                "status": "passed",
                "coverage_summary": {"verdict": "verified"},
                "findings": [],
            },
        })

        summary, source = adapter.patch_review_confidence_authority_summary(
            final_summary=final_summary,
            delivery_summary=delivery_summary,
            verify_status="passed",
            delivery_status="coherent",
        )

        self.assertEqual(source, "final_plan")
        self.assertIs(summary, final_summary)
        self.assertEqual(adapter.patch_review_confidence_downgrade_reason(summary), "")

    def test_patch_review_confidence_falls_back_without_final_signal(self) -> None:
        delivery_summary = {
            "block_reason": "",
            "coverage_verdict": "unverified",
            "semantic_unverified_telemetry_codes": ["dependent_surface_without_verify_coverage"],
        }

        summary, source = adapter.patch_review_confidence_authority_summary(
            final_summary=adapter.plan_patch_review_summary({}),
            delivery_summary=delivery_summary,
            verify_status="passed",
            delivery_status="coherent",
        )

        self.assertEqual(source, "delivery")
        self.assertIs(summary, delivery_summary)
        self.assertEqual(
            adapter.patch_review_confidence_downgrade_reason(summary),
            "patch_review_semantic_unverified:dependent_surface_without_verify_coverage",
        )

    def test_delivery_patch_review_keeps_actual_diff_blocker_from_any_owner(self) -> None:
        risky_source_plan = {
            "id": "plan-old",
            "patch_review": {
                "status": "passed",
                "coverage_summary": {
                    "verdict": "unverified",
                    "block_reason": "patch_review_semantic_uncovered:python_nested_string_key_direct_access_added",
                    "reason_codes": ["python_nested_string_key_direct_access_added"],
                },
                "findings": [{
                    "code": "python_nested_string_key_direct_access_added",
                    "severity": "warning",
                    "category": "semantic_coverage",
                    "coverage_status": "unverified",
                    "path": "pkg/widget.py",
                }],
            },
        }
        final_report_plan = {
            "id": "plan-final",
            "patch_review": {
                "status": "passed",
                "coverage_summary": {"verdict": "clean"},
                "findings": [],
            },
        }

        summary = adapter.delivery_patch_review_summary(
            source_owner_plans=[risky_source_plan, final_report_plan],
            delivery_plan=final_report_plan,
            report_plan_id="plan-final",
            verify_status="passed",
            delivery_status="coherent",
        )

        self.assertEqual(
            summary["block_reason"],
            "patch_review_semantic_uncovered:python_nested_string_key_direct_access_added",
        )
        self.assertEqual(summary["semantic_unverified_codes"], ["python_nested_string_key_direct_access_added"])


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


class ChangePlanDiscoveryTests(unittest.TestCase):
    def test_latest_change_plan_includes_probe_only_no_change_plan(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            plans = Path(raw) / "repo" / ".codrax" / "plans"
            plans.mkdir(parents=True)
            source_plan_path = plans / "plan-source.json"
            proof_plan_path = plans / "plan-proof.json"
            source_plan_path.write_text(
                json.dumps({
                    "id": "plan-source",
                    "summary": "source fix",
                    "changes": [{"path": "pkg/fix.py", "kind": "patch"}],
                }),
                encoding="utf-8",
            )
            proof_plan_path.write_text(
                json.dumps({
                    "id": "plan-proof",
                    "summary": "probe-only proof follow-up",
                    "changes": None,
                    "verification_probes": [{"id": "probe", "language": "python"}],
                }),
                encoding="utf-8",
            )
            os.utime(source_plan_path, (100, 100))
            os.utime(proof_plan_path, (200, 200))

            got = adapter.find_latest_change_plan(plans)

        self.assertEqual(got, proof_plan_path)


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

    def test_restore_progress_excludes_stale_applied_source_plans(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            repo = Path(raw) / "repo"
            inst = Path(raw) / "inst"
            plans = repo / ".codrax" / "plans"
            plans.mkdir(parents=True)
            inst.mkdir()
            for plan_id, body in {
                "plan-stale-1": {
                    "id": "plan-stale-1",
                    "changes": [{"path": "pkg/fix.py", "kind": "patch"}],
                    "patch_review": {
                        "status": "failed",
                        "findings": [{
                            "code": "python_unreachable_body_after_added_return",
                            "severity": "error",
                            "category": "structural",
                        }],
                    },
                },
                "plan-stale-2": {
                    "id": "plan-stale-2",
                    "changes": [{"path": "pkg/fix.py", "kind": "patch"}],
                    "patch_review": {"status": "passed", "findings": []},
                },
                "plan-final": {
                    "id": "plan-final",
                    "changes": [{"path": "pkg/fix.py", "kind": "patch"}],
                    "patch_review": {"status": "passed", "findings": []},
                },
            }.items():
                (plans / f"{plan_id}.json").write_text(json.dumps(body), encoding="utf-8")
            workflow = {
                "progress_ledger": [
                    {
                        "batch_id": "batch-1",
                        "reason_code": "checkpoint_restored_before_replan",
                        "at": "2026-06-18T21:56:15+08:00",
                    },
                    {
                        "batch_id": "batch-1",
                        "reason_code": "checkpoint_restored_before_replan",
                        "at": "2026-06-18T21:59:16+08:00",
                    },
                ],
                "batches": [{
                    "id": "batch-1",
                    "attempts": [
                        {
                            "kind": "apply",
                            "status": "applied",
                            "plan_id": "plan-stale-1",
                            "finished_at": "2026-06-18T21:56:07+08:00",
                        },
                        {
                            "kind": "apply",
                            "status": "applied",
                            "plan_id": "plan-stale-2",
                            "finished_at": "2026-06-18T21:58:19+08:00",
                        },
                        {
                            "kind": "apply",
                            "status": "applied",
                            "plan_id": "plan-final",
                            "finished_at": "2026-06-18T22:00:41+08:00",
                        },
                    ],
                }],
            }

            got = adapter.summarize_workflow_applied_plan_provenance(
                workflow,
                repo,
                inst,
                final_plan_id="plan-final",
            )
            candidate = adapter.build_write_delivery_candidate(
                repo_dir=repo,
                inst_dir=inst,
                workflow=workflow,
                final_plan=json.loads((plans / "plan-final.json").read_text(encoding="utf-8")),
                final_plan_path=plans / "plan-final.json",
                exported_source_paths=["pkg/fix.py"],
                exported_test_paths=[],
                patch_source="refs/codrax/applied/plan-final",
            )

        self.assertEqual(got["workflow_applied_plan_ids"], ["plan-final"])
        self.assertEqual(got["workflow_latest_applied_plan_id"], "plan-final")
        self.assertEqual(got["workflow_applied_source_paths"], ["pkg/fix.py"])
        self.assertEqual(candidate["source_owner_plan_ids"], ["plan-final"])
        self.assertEqual(candidate["primary_source_plan_id"], "plan-final")
        summary = adapter.delivery_patch_review_summary(
            source_owner_plans=candidate["source_owner_plans"],
            delivery_plan=candidate["primary_plan"],
            report_plan_id="plan-final",
            verify_status="passed",
            delivery_status="coherent",
        )
        self.assertEqual(summary["block_reason"], "")
        self.assertFalse(summary["hard_block"])

    def test_delivery_candidate_binds_exported_source_patch_to_prior_source_plan(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            repo = Path(raw) / "repo"
            inst = Path(raw) / "inst"
            plans = repo / ".codrax" / "plans"
            plans.mkdir(parents=True)
            inst.mkdir()
            source_plan = {
                "id": "plan-source",
                "summary": "source fix",
                "changes": [{"path": "pkg/fix.py", "kind": "patch"}],
                "patch_review": {"status": "passed", "findings": []},
            }
            test_plan = {
                "id": "plan-test",
                "summary": "test validation",
                "changes": [{"path": "tests/test_fix.py", "kind": "patch"}],
            }
            (plans / "plan-source.json").write_text(json.dumps(source_plan), encoding="utf-8")
            (plans / "plan-test.json").write_text(json.dumps(test_plan), encoding="utf-8")
            (plans / "plan-test.report.json").write_text(
                json.dumps({"verification_status": "passed", "passed": True}),
                encoding="utf-8",
            )
            workflow = {
                "batches": [
                    {"attempts": [
                        {"kind": "apply", "status": "applied", "plan_id": "plan-source"},
                        {"kind": "apply", "status": "applied", "plan_id": "plan-test"},
                    ]}
                ]
            }

            candidate = adapter.build_write_delivery_candidate(
                repo_dir=repo,
                inst_dir=inst,
                workflow=workflow,
                final_plan=test_plan,
                final_plan_path=plans / "plan-test.json",
                exported_source_paths=["pkg/fix.py"],
                exported_test_paths=[],
                patch_source="refs/codrax/applied/plan-test",
            )

        self.assertEqual(candidate["status"], "coherent")
        self.assertEqual(candidate["relation"], "source_plan_with_later_test_followup")
        self.assertEqual(candidate["source_owner_plan_ids"], ["plan-source"])
        self.assertEqual(candidate["primary_source_plan_id"], "plan-source")
        self.assertTrue(candidate["source_plan_covers_exported_source_patch"])
        self.assertEqual(candidate["report_plan_id"], "plan-test")

        reason = adapter.prediction_audit_block_reason(
            exported_source_paths=["pkg/fix.py"],
            final_plan_source_paths=candidate["source_paths"],
            final_plan_test_only=False,
            final_plan_covers_exported_source_patch=candidate["source_plan_covers_exported_source_patch"],
        )

        self.assertEqual(reason, "")

    def test_delivery_candidate_binds_later_probe_only_report_to_prior_source_plan(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            repo = Path(raw) / "repo"
            inst = Path(raw) / "inst"
            plans = repo / ".codrax" / "plans"
            plans.mkdir(parents=True)
            inst.mkdir()
            source_plan = {
                "id": "plan-source",
                "summary": "source fix",
                "changes": [{"path": "pkg/fix.py", "kind": "patch"}],
                "patch_review": {"status": "passed", "findings": []},
            }
            proof_plan = {
                "id": "plan-proof",
                "summary": "probe-only validation",
                "changes": None,
                "verification_probes": [{"id": "probe", "language": "python"}],
            }
            (plans / "plan-source.json").write_text(json.dumps(source_plan), encoding="utf-8")
            (plans / "plan-proof.json").write_text(json.dumps(proof_plan), encoding="utf-8")
            (plans / "plan-proof.report.json").write_text(
                json.dumps({"verification_status": "unavailable", "passed": False}),
                encoding="utf-8",
            )
            workflow = {
                "batches": [
                    {"attempts": [
                        {"kind": "apply", "status": "applied", "plan_id": "plan-source"},
                        {"kind": "verify", "status": "unverified", "plan_id": "plan-proof"},
                    ]}
                ]
            }

            candidate = adapter.build_write_delivery_candidate(
                repo_dir=repo,
                inst_dir=inst,
                workflow=workflow,
                final_plan=proof_plan,
                final_plan_path=plans / "plan-proof.json",
                exported_source_paths=["pkg/fix.py"],
                exported_test_paths=[],
                patch_source="refs/codrax/applied/plan-source",
            )

        self.assertEqual(candidate["status"], "coherent")
        self.assertEqual(candidate["relation"], "source_plan_with_later_validation_followup")
        self.assertEqual(candidate["source_owner_plan_ids"], ["plan-source"])
        self.assertTrue(candidate["source_plan_covers_exported_source_patch"])
        self.assertEqual(candidate["report_plan_id"], "plan-proof")
        self.assertTrue(candidate["final_plan_validation_only"])
        reason = adapter.prediction_audit_block_reason(
            exported_source_paths=["pkg/fix.py"],
            final_plan_source_paths=candidate["source_paths"],
            final_plan_test_only=False,
            final_plan_covers_exported_source_patch=candidate["source_plan_covers_exported_source_patch"],
        )
        self.assertEqual(reason, "")

    def test_delivery_candidate_blocks_exported_source_without_applied_owner(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            repo = Path(raw) / "repo"
            inst = Path(raw) / "inst"
            plans = repo / ".codrax" / "plans"
            plans.mkdir(parents=True)
            inst.mkdir()
            test_plan = {
                "id": "plan-test",
                "summary": "test change",
                "changes": [{"path": "tests/test_fix.py", "kind": "patch"}],
            }
            (plans / "plan-test.json").write_text(json.dumps(test_plan), encoding="utf-8")
            workflow = {
                "batches": [
                    {"attempts": [
                        {"kind": "apply", "status": "applied", "plan_id": "plan-test"},
                    ]}
                ]
            }

            candidate = adapter.build_write_delivery_candidate(
                repo_dir=repo,
                inst_dir=inst,
                workflow=workflow,
                final_plan=test_plan,
                final_plan_path=plans / "plan-test.json",
                exported_source_paths=["pkg/fix.py"],
                exported_test_paths=[],
                patch_source="refs/codrax/applied/plan-test",
            )

        self.assertEqual(candidate["status"], "incoherent")
        self.assertEqual(candidate["reason_code"], "delivery_source_plan_missing")
        self.assertEqual(candidate["source_owner_plan_ids"], [])


class ExportPatchPolicyTests(unittest.TestCase):
    def test_validation_followup_exports_latest_applied_source_plan(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            repo = Path(raw) / "repo"
            inst = Path(raw) / "inst"
            repo.mkdir()
            inst.mkdir()
            plans = repo / ".codrax" / "plans"
            plans.mkdir(parents=True)

            def git(*args: str) -> str:
                result = subprocess.run(
                    ["git", *args],
                    cwd=repo,
                    text=True,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    check=True,
                )
                return result.stdout.strip()

            git("init")
            git("config", "user.email", "codrax@example.com")
            git("config", "user.name", "Codrax Test")
            (repo / "pkg").mkdir()
            (repo / "pkg" / "fix.py").write_text("old = 1\n", encoding="utf-8")
            git("add", ".")
            git("commit", "-m", "base")
            base = git("rev-parse", "HEAD")
            (repo / "pkg" / "fix.py").write_text("new = 1\n", encoding="utf-8")
            git("add", ".")
            git("commit", "-m", "source fix")
            source_commit = git("rev-parse", "HEAD")
            git("update-ref", "refs/codrax/applied/plan-source", source_commit)

            source_plan = {
                "id": "plan-source",
                "summary": "source fix",
                "changes": [{"path": "pkg/fix.py", "kind": "patch"}],
            }
            proof_plan = {
                "id": "plan-proof",
                "summary": "probe-only validation",
                "changes": None,
                "verification_probes": [{"id": "probe", "language": "python"}],
            }
            (plans / "plan-source.json").write_text(json.dumps(source_plan), encoding="utf-8")
            (plans / "plan-proof.json").write_text(json.dumps(proof_plan), encoding="utf-8")
            workflow = {
                "batches": [
                    {"attempts": [
                        {"kind": "apply", "status": "applied", "plan_id": "plan-source"},
                        {"kind": "verify", "status": "unverified", "plan_id": "plan-proof"},
                    ]}
                ]
            }

            export_plan, export_plan_path, reason = adapter.select_prediction_export_plan(
                final_plan=proof_plan,
                final_plan_path=plans / "plan-proof.json",
                workflow=workflow,
                repo_dir=repo,
                inst_dir=inst,
            )
            allowed = adapter.export_allowed_patch_paths(
                workflow,
                repo,
                inst,
                proof_plan,
                include_test_patches=False,
            )
            patch, source, dropped_tests, exported_paths, dropped_unowned = adapter.export_patch(
                repo,
                base,
                export_plan,
                include_test_patches=False,
                allowed_paths=allowed,
            )

        self.assertEqual(export_plan["id"], "plan-source")
        self.assertEqual(export_plan_path, plans / "plan-source.json")
        self.assertEqual(reason, "workflow_latest_applied_source_plan")
        self.assertIn("diff --git a/pkg/fix.py b/pkg/fix.py", patch)
        self.assertEqual(source, "refs/codrax/applied/plan-source")
        self.assertEqual(exported_paths, ["pkg/fix.py"])
        self.assertEqual(dropped_tests, [])
        self.assertEqual(dropped_unowned, [])

    def test_export_patch_between_filters_to_typed_allowed_paths(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            repo = Path(raw) / "repo"
            repo.mkdir()

            def git(*args: str) -> str:
                result = subprocess.run(
                    ["git", *args],
                    cwd=repo,
                    text=True,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    check=True,
                )
                return result.stdout.strip()

            git("init")
            git("config", "user.email", "codrax@example.com")
            git("config", "user.name", "Codrax Test")
            (repo / "pkg").mkdir()
            (repo / "tests").mkdir()
            (repo / "build").mkdir()
            (repo / "pkg" / "fix.py").write_text("old = 1\n", encoding="utf-8")
            (repo / "tests" / "test_fix.py").write_text("def test_old(): pass\n", encoding="utf-8")
            (repo / "build" / "config.log").write_text("old build\n", encoding="utf-8")
            git("add", ".")
            git("commit", "-m", "base")
            base = git("rev-parse", "HEAD")
            (repo / "pkg" / "fix.py").write_text("new = 1\n", encoding="utf-8")
            (repo / "tests" / "test_fix.py").write_text("def test_new(): pass\n", encoding="utf-8")
            (repo / "build" / "config.log").write_text("generated build output\n", encoding="utf-8")
            git("add", ".")
            git("commit", "-m", "changed")

            patch, dropped_tests, exported_paths, dropped_unowned = adapter.export_patch_between(
                repo,
                base,
                "HEAD",
                include_test_patches=False,
                allowed_paths=["pkg/fix.py"],
            )

        self.assertIn("diff --git a/pkg/fix.py b/pkg/fix.py", patch)
        self.assertNotIn("diff --git a/build/config.log b/build/config.log", patch)
        self.assertNotIn("diff --git a/tests/test_fix.py b/tests/test_fix.py", patch)
        self.assertEqual(exported_paths, ["pkg/fix.py"])
        self.assertEqual(dropped_tests, ["tests/test_fix.py"])
        self.assertEqual(dropped_unowned, ["build/config.log"])


class PythonCompatConstraintTests(unittest.TestCase):
    def test_pyproject_build_requires_parses_multiline_build_system_requires(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            repo = Path(raw)
            (repo / "pyproject.toml").write_text(
                """
[project]
name = "demo"

[build-system]
requires = [
    "setuptools",
    "wheel",
    "extension-helpers",
]
build-backend = "setuptools.build_meta"
""",
                encoding="utf-8",
            )

            requires = adapter.pyproject_build_requires(repo)

        self.assertEqual(requires, ["setuptools", "wheel", "extension-helpers"])

    def test_pyproject_build_requires_fallback_parses_without_toml_library(self) -> None:
        requires = adapter.parse_pyproject_build_requires_fallback(
            """
[build-system]
requires = ["setuptools", "wheel",
            "extension-helpers"]

[tool.demo]
requires = ["ignored"]
"""
        )

        self.assertEqual(requires, ["setuptools", "wheel", "extension-helpers"])

    def test_adds_jinja2_ceiling_when_legacy_requirement_has_no_upper_bound(self) -> None:
        constraints = adapter.python_compat_constraints(["Jinja2>=2.3", "numpy>=1.11"])

        by_package = {row["package"]: row for row in constraints}
        self.assertEqual(by_package["jinja2"]["constraint"], "Jinja2<3.1")
        self.assertEqual(by_package["jinja2"]["reason_code"], "legacy_jinja2_filter_api_without_major_ceiling")
        self.assertEqual(by_package["numpy"]["constraint"], "numpy<2")

    def test_does_not_override_declared_jinja2_upper_bound(self) -> None:
        constraints = adapter.python_compat_constraints(["Jinja2>=2.3,<3.0"])

        self.assertEqual([row for row in constraints if row["package"] == "jinja2"], [])


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

    def test_pending_approval_plan_gets_specific_empty_patch_reason(self) -> None:
        reason = adapter.empty_patch_reason(
            patch="",
            workflow_status="in_progress",
            plan_status="pending_approval",
            plan_approval_action="manual_approval",
            plan_path="/tmp/plan.json",
        )

        self.assertEqual(reason, "workflow_pending_approval_empty_patch")

    def test_auto_executable_pending_plan_status_is_not_manual_approval(self) -> None:
        reason = adapter.empty_patch_reason(
            patch="",
            workflow_status="in_progress",
            plan_status="pending_approval",
            plan_approval_action="auto_execute",
            plan_path="/tmp/plan.json",
        )

        self.assertEqual(reason, "workflow_in_progress_empty_patch")


class ProgressHeartbeatTests(unittest.TestCase):
    def test_progress_status_fails_soft_when_workflow_state_is_unavailable(self) -> None:
        with mock.patch.object(adapter, "load_latest_workflow_run", side_effect=RuntimeError("race")):
            line = adapter.codrax_progress_status(Path("/tmp/repo"), "inst-1", 12)

        self.assertIn("progress inst-1 elapsed=12s", line)
        self.assertIn("workflow=unavailable", line)
        self.assertIn("reason=workflow_progress_unavailable", line)
        self.assertNotIn("RuntimeError", line)

    def test_streaming_progress_callback_failure_uses_typed_fallback(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            stderr = io.StringIO()
            with redirect_stderr(stderr):
                result = adapter.run_cmd_streaming_to_file(
                    [sys.executable, "-c", "import time; time.sleep(1.2)"],
                    output_path=Path(tmp) / "out.txt",
                    timeout=5,
                    progress_interval=1,
                    progress_fn=lambda _elapsed: (_ for _ in ()).throw(RuntimeError("boom")),
                )

        self.assertEqual(result.code, 0)
        line = stderr.getvalue()
        self.assertIn("workflow=unavailable", line)
        self.assertIn("reason=progress_callback_unavailable", line)
        self.assertNotIn("RuntimeError", line)


class RunCodraxCommandTests(unittest.TestCase):
    def test_run_codrax_forwards_providers_path(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            repo = root / "repo"
            inst = root / "inst"
            repo.mkdir()
            inst.mkdir()
            providers = root / "providers.yaml"
            providers.write_text("llm: {}\n", encoding="utf-8")
            binary = root / "codrax"
            binary.write_text("#!/bin/sh\n", encoding="utf-8")
            args = SimpleNamespace(
                codrax_bin=str(binary),
                settings="",
                providers=str(providers),
                max_steps=12,
                log_level="info",
                isolate_git_history=True,
                codrax_timeout=30,
                codrax_progress_interval=0,
                request_prefix="",
                include_oracle_in_request=False,
                require_nonempty_patch=False,
            )

            captured: dict[str, object] = {}

            def fake_run(cmd: list[str], **kwargs: object) -> adapter.CommandResult:
                captured["cmd"] = cmd
                captured["kwargs"] = kwargs
                return adapter.CommandResult(0, "")

            with mock.patch.object(adapter, "run_cmd_streaming_to_file", side_effect=fake_run):
                result = adapter.run_codrax(
                    {"instance_id": "repo__issue-1", "problem_statement": "fix it"},
                    repo,
                    inst,
                    args,
                )

        self.assertEqual(result.code, 0)
        cmd = captured["cmd"]
        assert isinstance(cmd, list)
        self.assertEqual(cmd[1:3], ["--providers", str(providers.resolve())])
        self.assertIn("--eval-disable-git-history", cmd)


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

    def test_patch_review_blocker_blocks_local_acceptance_proxy(self) -> None:
        verdict, confidence, blocks = adapter.prediction_verdict(
            "diff --git a/pkg/a.py b/pkg/a.py\n",
            "passed",
            "applied",
            "patch_review_semantic_unverified:changed_symbol_without_probe_coverage",
            "",
        )

        self.assertEqual(verdict, "predicted_audit_blocked")
        self.assertEqual(confidence, "failed")
        self.assertTrue(blocks)


if __name__ == "__main__":
    unittest.main()
