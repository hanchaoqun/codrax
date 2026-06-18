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


def load_jsonl(path: Path) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    try:
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
            rows.append(row)
    return rows


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


def core_field_presence(rows: list[dict[str, Any]]) -> dict[str, int]:
    return {field: sum(1 for row in rows if field in row) for field in CORE_FIELDS}


def rows_missing_core_fields(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    for idx, row in enumerate(rows, 1):
        missing = [field for field in CORE_FIELDS if field not in row]
        if not missing:
            continue
        out.append({
            "row": idx,
            "instance_id": text(row, "instance_id"),
            "missing_fields": missing,
        })
    return out


def summarize_results(rows: list[dict[str, Any]]) -> dict[str, Any]:
    total = len(rows)
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

    return {
        "schema_version": 1,
        "source": "codrax_swebench_results",
        "row_count": total,
        "unique_instance_count": len(unique_ids),
        "duplicate_instance_count": len(duplicate_ids),
        "duplicate_instance_ids": duplicate_ids,
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
        "non_empty_patch_rate": rate(non_empty_patch, total),
        "high_confidence_local_verify_pass_rate": rate(high_conf_local_verify, total),
        "low_confidence_verify_pass_rate": rate(low_conf_verify, total),
        "local_acceptance_pass_rate": rate(local_acceptance_pass, total),
        "typed_manual_audit_recorded_rate": rate(manual_recorded, total),
        "non_empty_patch_percent": percent(rate(non_empty_patch, total)),
        "high_confidence_local_verify_pass_percent": percent(rate(high_conf_local_verify, total)),
        "low_confidence_verify_pass_percent": percent(rate(low_conf_verify, total)),
        "local_acceptance_pass_percent": percent(rate(local_acceptance_pass, total)),
        "typed_manual_audit_recorded_percent": percent(rate(manual_recorded, total)),
        "prediction_verdict_counts": count_by(rows, "prediction_verdict"),
        "prediction_local_confidence_counts": count_by(rows, "prediction_local_confidence"),
        "verify_status_counts": count_by(rows, "verify_status"),
        "workflow_status_counts": count_by(rows, "workflow_status"),
        "local_acceptance_verdict_counts": count_by(rows, "local_acceptance_verdict"),
        "local_acceptance_source_counts": count_by(rows, "local_acceptance_source"),
        "manual_audit_verdict_counts": count_by(rows, "manual_audit_verdict"),
        "top_confidence_downgrade_reasons": top_counts(count_by(rows, "prediction_confidence_downgrade_reason")),
        "top_audit_block_reasons": top_counts(count_by(rows, "prediction_audit_block_reason")),
        "core_field_presence": core_field_presence(rows),
        "rows_missing_core_fields": rows_missing_core_fields(rows),
    }


def pct_text(value: Any) -> str:
    return "n/a" if value is None else f"{value:.2f}%"


def format_summary(summary: dict[str, Any]) -> str:
    total = summary["row_count"]
    return (
        f"codrax_results rows={total} unique={summary['unique_instance_count']} "
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
    parser.add_argument("--results-jsonl", type=Path, required=True, help="Codrax adapter results.jsonl")
    parser.add_argument("--output-json", type=Path, help="Write normalized summary JSON to this path")
    parser.add_argument("--json-only", action="store_true", help="Print only normalized JSON to stdout")
    args = parser.parse_args()

    rows = load_jsonl(args.results_jsonl)
    summary = summarize_results(rows)
    summary["results_path"] = str(args.results_jsonl)
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
