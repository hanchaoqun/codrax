#!/usr/bin/env python3
"""Summarize Codrax SWE-bench adapter results.jsonl.

This script is deliberately dependency-free and consumes only typed
`results.jsonl` fields emitted by `run_codrax_swebench.py`. It does not parse
issue text, model prose, manual notes, terminal output, or official harness
logs. The official SWE-bench harness remains the only scoring authority; this
summary is for local Codrax triage and audit denominators.
"""

from __future__ import annotations

import argparse
import glob
import json
from pathlib import Path
from typing import Any


CORE_FIELDS = [
    "instance_id",
    "status",
    "patch_bytes",
    "prediction_verdict",
    "prediction_local_confidence",
    "prediction_blocks_local_acceptance",
    "prediction_confidence_downgrade_reason",
    "prediction_audit_block_reason",
    "verify_status",
    "local_acceptance_verdict",
    "local_acceptance_source",
    "manual_audit_verdict",
]

PROOF_COVERAGE_REASON_CODES = {
    "behavior_contract_without_verify_coverage",
    "changed_symbol_without_probe_coverage",
    "verification_probe_baseline_not_run",
    "verification_probe_changed_source_not_context_covered",
    "verification_probe_missing_changed_symbol_ref",
    "verification_probe_missing_required_contract_ref",
    "verification_probe_missing_soft_contract_ref",
}

VERIFY_ENVIRONMENT_REASON_CODES = {
    "accepted_without_local_verify",
    "make_python_module_missing",
    "make_target_missing",
    "project_runner_unavailable",
    "runner_missing",
    "skip_verify",
    "verification_probe_import_error",
    "verification_probe_module_not_found",
}

PROBE_AUTHORING_REASON_CODES = {
    "verification_probe_expected_stdout_missing",
    "verification_probe_name_error",
    "verification_probe_syntax_error",
    "verification_probe_unclassified",
}

IMPACT_TELEMETRY_REASON_CODES = {
    "dependent_surface_without_verify_coverage",
    "related_test_surface_unverified",
}

RESULT_CAUSE_FAMILIES = {
    "accepted_high_confidence": "accepted",
    "accepted_manual_audit": "accepted",
    "manual_audit_failed": "manual_audit",
    "empty_patch_export": "prediction_export",
    "verify_red_tests_or_build": "implementation_or_localization",
    "verify_environment_unavailable": "environment",
    "probe_or_contract_authoring_gap": "probe_generation",
    "proof_coverage_gap": "verification_proof",
    "impact_telemetry_low_confidence": "impact_analysis",
    "actual_diff_or_patch_review_gap": "patch_semantics_or_localization",
    "workflow_state_gap": "workflow_state",
    "adapter_or_runtime_error": "adapter_runtime",
    "low_confidence_unclassified": "unknown",
    "unverified_local_acceptance": "unknown",
    "unknown": "unknown",
}


def load_jsonl(path: Path, source_index: int = 0) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    try:
        source_mtime = path.stat().st_mtime
        handle = path.open(encoding="utf-8")
    except FileNotFoundError as exc:
        raise SystemExit(f"file not found: {path}") from exc
    with handle:
        for line_no, line in enumerate(handle, 1):
            line = line.strip()
            if not line:
                continue
            try:
                row = json.loads(line)
            except json.JSONDecodeError as exc:
                raise SystemExit(f"{path}:{line_no}: invalid JSON: {exc}") from exc
            if not isinstance(row, dict):
                raise SystemExit(f"{path}:{line_no}: expected a JSON object")
            row = dict(row)
            row["__source_path"] = str(path)
            row["__source_index"] = source_index
            row["__source_line"] = line_no
            row["__source_mtime"] = source_mtime
            rows.append(row)
    return rows


def expand_results_paths(paths: list[Path], patterns: list[str]) -> list[Path]:
    out: list[Path] = []
    seen: set[str] = set()
    for path in paths:
        resolved = path.resolve()
        key = str(resolved)
        if key in seen:
            continue
        seen.add(key)
        out.append(resolved)
    for pattern in patterns:
        matches = sorted(Path(match).resolve() for match in glob.glob(pattern, recursive=True))
        if not matches:
            raise SystemExit(f"results glob matched no files: {pattern}")
        for path in matches:
            key = str(path)
            if key in seen:
                continue
            seen.add(key)
            out.append(path)
    return out


def load_results_files(paths: list[Path]) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for idx, path in enumerate(paths):
        rows.extend(load_jsonl(path, idx))
    return rows


def row_order_key(row: dict[str, Any]) -> tuple[float, int, int]:
    mtime = row.get("__source_mtime")
    source_index = row.get("__source_index")
    source_line = row.get("__source_line")
    return (
        float(mtime) if isinstance(mtime, (int, float)) else 0.0,
        int(source_index) if isinstance(source_index, int) else 0,
        int(source_line) if isinstance(source_line, int) else 0,
    )


def dedupe_latest_by_instance(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    latest: dict[str, dict[str, Any]] = {}
    no_instance: list[dict[str, Any]] = []
    for row in rows:
        instance_id = text(row, "instance_id")
        if not instance_id:
            no_instance.append(row)
            continue
        current = latest.get(instance_id)
        if current is None or row_order_key(row) >= row_order_key(current):
            latest[instance_id] = row
    return no_instance + [latest[key] for key in sorted(latest)]


def text(row: dict[str, Any], key: str) -> str:
    return str(row.get(key) or "").strip()


def int_value(row: dict[str, Any], key: str) -> int:
    value = row.get(key)
    if isinstance(value, bool):
        return int(value)
    if isinstance(value, int):
        return max(value, 0)
    if isinstance(value, float):
        return max(int(value), 0)
    return 0


def bool_value(row: dict[str, Any], key: str) -> bool:
    value = row.get(key)
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        return value.strip().lower() == "true"
    return False


def rate(num: int, den: int) -> float | None:
    if den <= 0:
        return None
    return num / den


def percent(value: float | None) -> float | None:
    if value is None:
        return None
    return round(value * 100.0, 4)


def count_by(rows: list[dict[str, Any]], key: str) -> dict[str, int]:
    counts: dict[str, int] = {}
    for row in rows:
        value = text(row, key)
        if value == "":
            value = "<empty>"
        counts[value] = counts.get(value, 0) + 1
    return dict(sorted(counts.items()))


def top_counts(counts: dict[str, int], limit: int = 12) -> list[dict[str, Any]]:
    items = sorted(counts.items(), key=lambda item: (-item[1], item[0]))
    return [{"value": value, "count": count} for value, count in items[:limit]]


def has_non_empty_patch(row: dict[str, Any]) -> bool:
    if int_value(row, "patch_bytes") > 0:
        return True
    return text(row, "status") == "predicted" and text(row, "empty_patch_reason") == ""


def is_high_confidence_local_verify_pass(row: dict[str, Any]) -> bool:
    return (
        text(row, "local_acceptance_verdict") == "pass"
        and text(row, "local_acceptance_source") == "local_verify"
        and text(row, "prediction_local_confidence") == "high"
        and text(row, "prediction_confidence_downgrade_reason") == ""
    )


def is_recorded_local_verify_pass_confidence_mismatch(row: dict[str, Any]) -> bool:
    return (
        text(row, "local_acceptance_verdict") == "pass"
        and text(row, "local_acceptance_source") == "local_verify"
        and not is_high_confidence_local_verify_pass(row)
    )


def is_low_confidence_verify_pass(row: dict[str, Any]) -> bool:
    if text(row, "prediction_verdict") == "predicted_passed_low_confidence":
        return True
    return text(row, "verify_status") == "passed" and text(row, "prediction_confidence_downgrade_reason") != ""


def is_manual_audit_recorded(row: dict[str, Any]) -> bool:
    return text(row, "manual_audit_verdict") in {"pass", "fail", "unknown"}


def normalize_reason_token(raw: str) -> str:
    token = str(raw or "").strip()
    if not token:
        return ""
    if ":" in token:
        token = token.rsplit(":", 1)[1].strip()
    return token


def row_reason_values(row: dict[str, Any]) -> list[str]:
    values: list[str] = []
    for key in (
        "prediction_audit_block_reason",
        "prediction_confidence_downgrade_reason",
        "verify_failure_reason_code",
    ):
        value = text(row, key)
        if value:
            values.append(value)
    raw_confidence = row.get("verify_confidence_reason_codes")
    if isinstance(raw_confidence, list):
        values.extend(str(item).strip() for item in raw_confidence if str(item).strip())
    return values


def row_reason_tokens(row: dict[str, Any]) -> set[str]:
    out: set[str] = set()
    for value in row_reason_values(row):
        for part in str(value).split(","):
            token = normalize_reason_token(part)
            if token:
                out.add(token)
    return out


def first_row_reason(row: dict[str, Any]) -> str:
    for value in row_reason_values(row):
        for part in str(value).split(","):
            token = normalize_reason_token(part)
            if token:
                return token
    verdict = text(row, "prediction_verdict")
    if verdict:
        return verdict
    verify_status = text(row, "verify_status")
    if verify_status:
        return "verify_status:" + verify_status
    return "unknown"


def first_matching_reason(row: dict[str, Any], allowed: set[str]) -> str:
    reasons = row_reason_tokens(row)
    for reason in sorted(reasons):
        if reason in allowed:
            return reason
    return ""


def first_reason_with_prefix(row: dict[str, Any], prefix: str) -> str:
    for value in row_reason_values(row):
        value = str(value or "").strip()
        if value.startswith(prefix):
            return normalize_reason_token(value)
    return ""


def cause_reason_for_row(row: dict[str, Any], category: str) -> str:
    if category == "accepted_high_confidence":
        return "local_verify_passed_high_confidence"
    if category == "accepted_manual_audit":
        return "manual_audit_pass"
    if category == "manual_audit_failed":
        return "manual_audit_fail"
    if category == "empty_patch_export":
        return "empty_patch"
    if category == "verify_red_tests_or_build":
        return text(row, "verify_failure_kind") or "verify_status:failed"
    if category == "workflow_state_gap":
        workflow_reason = first_reason_with_prefix(row, "workflow_")
        if workflow_reason:
            return workflow_reason
        return "workflow_status:" + (text(row, "workflow_status") or "unknown")
    if category == "proof_coverage_gap":
        return first_matching_reason(row, PROOF_COVERAGE_REASON_CODES) or first_row_reason(row)
    if category == "impact_telemetry_low_confidence":
        return first_matching_reason(row, IMPACT_TELEMETRY_REASON_CODES) or first_row_reason(row)
    if category == "actual_diff_or_patch_review_gap":
        return first_reason_with_prefix(row, "patch_review_semantic_uncovered:") or first_row_reason(row)
    if category == "probe_or_contract_authoring_gap":
        return first_matching_reason(row, PROBE_AUTHORING_REASON_CODES) or first_row_reason(row)
    if category == "verify_environment_unavailable":
        return first_matching_reason(row, VERIFY_ENVIRONMENT_REASON_CODES) or "verify_status:unavailable"
    return first_row_reason(row)


def classify_result_cause(row: dict[str, Any]) -> str:
    """Return the typed local-triage cause category for a Codrax result row.

    The classifier consumes adapter enums, status fields, and reason codes only.
    It intentionally avoids issue text, model prose, terminal output, and manual
    notes so it stays an observability aid rather than another brittle router.
    """

    manual = text(row, "manual_audit_verdict")
    verify_status = text(row, "verify_status")
    verify_failure_kind = text(row, "verify_failure_kind")
    prediction_verdict = text(row, "prediction_verdict")
    workflow_status = text(row, "workflow_status")
    reasons = row_reason_tokens(row)

    if is_high_confidence_local_verify_pass(row):
        return "accepted_high_confidence"
    if manual == "pass" and text(row, "local_acceptance_verdict") == "pass":
        return "accepted_manual_audit"
    if manual == "fail":
        return "manual_audit_failed"
    if not has_non_empty_patch(row):
        return "empty_patch_export"
    if text(row, "status") == "error" or prediction_verdict == "adapter_error":
        return "adapter_or_runtime_error"
    if verify_status == "failed" and verify_failure_kind in {
        "tests_failed",
        "build_failure",
        "preexisting_build_failure",
        "timeout",
        "oom",
        "cpu_limit",
        "crash",
    }:
        return "verify_red_tests_or_build"
    if workflow_status in {"blocked", "in_progress"} or any(
        value.startswith("workflow_") for value in row_reason_values(row)
    ):
        return "workflow_state_gap"
    if reasons & PROOF_COVERAGE_REASON_CODES:
        return "proof_coverage_gap"
    if reasons & IMPACT_TELEMETRY_REASON_CODES:
        return "impact_telemetry_low_confidence"
    if any(value.startswith("patch_review_semantic_uncovered:") for value in row_reason_values(row)):
        return "actual_diff_or_patch_review_gap"
    if reasons & PROBE_AUTHORING_REASON_CODES:
        return "probe_or_contract_authoring_gap"
    if verify_status == "unavailable" or reasons & VERIFY_ENVIRONMENT_REASON_CODES:
        return "verify_environment_unavailable"
    if is_low_confidence_verify_pass(row):
        return "low_confidence_unclassified"
    if text(row, "local_acceptance_verdict") in {"", "unknown"}:
        return "unverified_local_acceptance"
    return "unknown"


def result_cause_rows(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    for row in rows:
        category = classify_result_cause(row)
        item = {
            "instance_id": text(row, "instance_id"),
            "category": category,
            "family": RESULT_CAUSE_FAMILIES.get(category, "unknown"),
            "reason": cause_reason_for_row(row, category),
            "verify_status": text(row, "verify_status"),
            "prediction_verdict": text(row, "prediction_verdict"),
        }
        if text(row, "__source_path"):
            item["source_path"] = text(row, "__source_path")
        if int_value(row, "__source_line") > 0:
            item["source_line"] = int_value(row, "__source_line")
        out.append(item)
    return out


def count_cause_field(causes: list[dict[str, Any]], key: str) -> dict[str, int]:
    counts: dict[str, int] = {}
    for cause in causes:
        value = str(cause.get(key) or "").strip() or "<empty>"
        counts[value] = counts.get(value, 0) + 1
    return dict(sorted(counts.items()))


def cause_examples(causes: list[dict[str, Any]], limit_per_category: int = 5) -> dict[str, list[dict[str, Any]]]:
    out: dict[str, list[dict[str, Any]]] = {}
    for cause in causes:
        category = str(cause.get("category") or "").strip()
        if not category:
            continue
        bucket = out.setdefault(category, [])
        if len(bucket) >= limit_per_category:
            continue
        bucket.append(cause)
    return dict(sorted(out.items()))


def core_field_presence(rows: list[dict[str, Any]]) -> dict[str, int]:
    return {field: sum(1 for row in rows if field in row) for field in CORE_FIELDS}


def rows_missing_core_fields(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    for idx, row in enumerate(rows, 1):
        missing = missing_core_fields(row)
        if not missing:
            continue
        item = {
            "row": idx,
            "instance_id": text(row, "instance_id"),
            "missing_fields": missing,
        }
        if text(row, "__source_path"):
            item["source_path"] = text(row, "__source_path")
        if int_value(row, "__source_line") > 0:
            item["source_line"] = int_value(row, "__source_line")
        out.append(item)
    return out


def missing_core_fields(row: dict[str, Any]) -> list[str]:
    return [field for field in CORE_FIELDS if field not in row]


def core_missing_field_counts(rows: list[dict[str, Any]]) -> dict[str, int]:
    counts: dict[str, int] = {field: 0 for field in CORE_FIELDS}
    for row in rows:
        for field in missing_core_fields(row):
            counts[field] += 1
    return {field: count for field, count in counts.items() if count > 0}


def summarize_results(
    rows: list[dict[str, Any]],
    *,
    input_row_count: int | None = None,
    input_paths: list[str] | None = None,
    dedupe_mode: str = "none",
) -> dict[str, Any]:
    total = len(rows)
    input_total = total if input_row_count is None else input_row_count
    instance_ids = [text(row, "instance_id") for row in rows if text(row, "instance_id")]
    unique_ids = sorted(set(instance_ids))
    duplicate_ids = sorted({instance_id for instance_id in instance_ids if instance_ids.count(instance_id) > 1})

    non_empty_patch = sum(1 for row in rows if has_non_empty_patch(row))
    empty_patch = total - non_empty_patch
    high_conf_local_verify = sum(1 for row in rows if is_high_confidence_local_verify_pass(row))
    local_verify_confidence_mismatch = sum(1 for row in rows if is_recorded_local_verify_pass_confidence_mismatch(row))
    low_conf_verify = sum(1 for row in rows if is_low_confidence_verify_pass(row))
    local_acceptance_pass = sum(1 for row in rows if text(row, "local_acceptance_verdict") == "pass")
    local_acceptance_fail = sum(1 for row in rows if text(row, "local_acceptance_verdict") == "fail")
    local_acceptance_unknown = sum(1 for row in rows if text(row, "local_acceptance_verdict") in {"", "unknown"})
    manual_pass = sum(1 for row in rows if text(row, "manual_audit_verdict") == "pass")
    manual_fail = sum(1 for row in rows if text(row, "manual_audit_verdict") == "fail")
    manual_unknown = sum(1 for row in rows if text(row, "manual_audit_verdict") == "unknown")
    manual_recorded = sum(1 for row in rows if is_manual_audit_recorded(row))
    local_blocked = sum(1 for row in rows if bool_value(row, "prediction_blocks_local_acceptance"))
    causes = result_cause_rows(rows)
    current_core_complete = sum(1 for row in rows if not missing_core_fields(row))
    local_acceptance_evaluable = sum(1 for row in rows if "local_acceptance_verdict" in row)
    manual_audit_evaluable = sum(1 for row in rows if "manual_audit_verdict" in row)

    return {
        "schema_version": 1,
        "source": "codrax_swebench_results",
        "dedupe_mode": dedupe_mode,
        "input_row_count": input_total,
        "input_results_paths": input_paths or [],
        "row_count": total,
        "unique_instance_count": len(unique_ids),
        "duplicate_instance_count": len(duplicate_ids),
        "duplicate_instance_ids": duplicate_ids,
        "current_core_complete_instances": current_core_complete,
        "current_core_incomplete_instances": total - current_core_complete,
        "non_empty_patch_instances": non_empty_patch,
        "empty_patch_instances": empty_patch,
        "high_confidence_local_verify_pass_instances": high_conf_local_verify,
        "recorded_local_verify_pass_confidence_mismatch_instances": local_verify_confidence_mismatch,
        "low_confidence_verify_pass_instances": low_conf_verify,
        "local_acceptance_pass_instances": local_acceptance_pass,
        "local_acceptance_fail_instances": local_acceptance_fail,
        "local_acceptance_unknown_instances": local_acceptance_unknown,
        "typed_manual_audit_recorded_instances": manual_recorded,
        "typed_manual_audit_pass_instances": manual_pass,
        "typed_manual_audit_fail_instances": manual_fail,
        "typed_manual_audit_unknown_instances": manual_unknown,
        "local_audit_blocked_instances": local_blocked,
        "local_acceptance_evaluable_instances": local_acceptance_evaluable,
        "typed_manual_audit_evaluable_instances": manual_audit_evaluable,
        "current_core_complete_rate": rate(current_core_complete, total),
        "non_empty_patch_rate": rate(non_empty_patch, total),
        "high_confidence_local_verify_pass_rate": rate(high_conf_local_verify, total),
        "high_confidence_local_verify_pass_rate_evaluable": rate(high_conf_local_verify, local_acceptance_evaluable),
        "low_confidence_verify_pass_rate": rate(low_conf_verify, total),
        "local_acceptance_pass_rate": rate(local_acceptance_pass, total),
        "local_acceptance_pass_rate_evaluable": rate(local_acceptance_pass, local_acceptance_evaluable),
        "typed_manual_audit_recorded_rate": rate(manual_recorded, total),
        "typed_manual_audit_recorded_rate_evaluable": rate(manual_recorded, manual_audit_evaluable),
        "current_core_complete_percent": percent(rate(current_core_complete, total)),
        "non_empty_patch_percent": percent(rate(non_empty_patch, total)),
        "high_confidence_local_verify_pass_percent": percent(rate(high_conf_local_verify, total)),
        "high_confidence_local_verify_pass_percent_evaluable": percent(rate(high_conf_local_verify, local_acceptance_evaluable)),
        "low_confidence_verify_pass_percent": percent(rate(low_conf_verify, total)),
        "local_acceptance_pass_percent": percent(rate(local_acceptance_pass, total)),
        "local_acceptance_pass_percent_evaluable": percent(rate(local_acceptance_pass, local_acceptance_evaluable)),
        "typed_manual_audit_recorded_percent": percent(rate(manual_recorded, total)),
        "typed_manual_audit_recorded_percent_evaluable": percent(rate(manual_recorded, manual_audit_evaluable)),
        "prediction_verdict_counts": count_by(rows, "prediction_verdict"),
        "prediction_local_confidence_counts": count_by(rows, "prediction_local_confidence"),
        "verify_status_counts": count_by(rows, "verify_status"),
        "workflow_status_counts": count_by(rows, "workflow_status"),
        "local_acceptance_verdict_counts": count_by(rows, "local_acceptance_verdict"),
        "local_acceptance_source_counts": count_by(rows, "local_acceptance_source"),
        "manual_audit_verdict_counts": count_by(rows, "manual_audit_verdict"),
        "top_confidence_downgrade_reasons": top_counts(count_by(rows, "prediction_confidence_downgrade_reason")),
        "top_audit_block_reasons": top_counts(count_by(rows, "prediction_audit_block_reason")),
        "result_cause_category_counts": count_cause_field(causes, "category"),
        "result_cause_family_counts": count_cause_field(causes, "family"),
        "top_result_cause_reasons": top_counts(count_cause_field(causes, "reason")),
        "result_cause_examples": cause_examples(causes),
        "core_field_presence": core_field_presence(rows),
        "core_missing_field_counts": core_missing_field_counts(rows),
        "rows_missing_core_fields": rows_missing_core_fields(rows),
    }


def pct_text(value: Any) -> str:
    return "n/a" if value is None else f"{value:.2f}%"


def format_summary(summary: dict[str, Any]) -> str:
    total = summary["row_count"]
    return (
        f"codrax_results rows={total} input_rows={summary.get('input_row_count', total)} "
        f"dedupe={summary.get('dedupe_mode', 'none')} unique={summary['unique_instance_count']} "
        f"current_core={summary['current_core_complete_instances']}/{total} "
        f"({pct_text(summary.get('current_core_complete_percent'))}) "
        f"non_empty_patch={summary['non_empty_patch_instances']}/{total} "
        f"({pct_text(summary.get('non_empty_patch_percent'))}) "
        f"high_conf_local_verify={summary['high_confidence_local_verify_pass_instances']}/{total} "
        f"({pct_text(summary.get('high_confidence_local_verify_pass_percent'))}) "
        f"low_conf_verify={summary['low_confidence_verify_pass_instances']}/{total} "
        f"({pct_text(summary.get('low_confidence_verify_pass_percent'))}) "
        f"manual_audit_recorded={summary['typed_manual_audit_recorded_instances']}/{total} "
        f"({pct_text(summary.get('typed_manual_audit_recorded_percent'))})"
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--results-jsonl",
        type=Path,
        action="append",
        default=[],
        help="Codrax adapter results.jsonl; may be passed multiple times",
    )
    parser.add_argument(
        "--results-glob",
        action="append",
        default=[],
        help="Glob for Codrax adapter results.jsonl files; may be passed multiple times",
    )
    parser.add_argument(
        "--dedupe",
        choices=["none", "latest-by-file-mtime"],
        default="none",
        help="Optional duplicate instance_id handling across multiple result files",
    )
    parser.add_argument(
        "--require-current-core",
        action="store_true",
        help="Fail if any summarized row lacks the current core result fields used for local acceptance denominators",
    )
    parser.add_argument("--output-json", type=Path, help="Write normalized summary JSON to this path")
    parser.add_argument("--json-only", action="store_true", help="Print only normalized JSON to stdout")
    args = parser.parse_args()

    paths = expand_results_paths(args.results_jsonl, args.results_glob)
    if not paths:
        raise SystemExit("at least one --results-jsonl or --results-glob is required")
    input_rows = load_results_files(paths)
    rows = input_rows
    if args.dedupe == "latest-by-file-mtime":
        rows = dedupe_latest_by_instance(input_rows)
    summary = summarize_results(
        rows,
        input_row_count=len(input_rows),
        input_paths=[str(path) for path in paths],
        dedupe_mode=args.dedupe,
    )
    if args.require_current_core and summary["current_core_incomplete_instances"] > 0:
        raise SystemExit(
            "current core result fields missing for "
            f"{summary['current_core_incomplete_instances']}/{summary['row_count']} summarized rows; "
            "rerun Codrax SWE-bench with the current adapter or omit --require-current-core for legacy telemetry"
        )
    if len(paths) == 1:
        summary["results_path"] = str(paths[0])
    encoded = json.dumps(summary, indent=2, sort_keys=True)
    if args.output_json:
        args.output_json.write_text(encoded + "\n", encoding="utf-8")
    if args.json_only:
        print(encoded)
    else:
        print(format_summary(summary))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
