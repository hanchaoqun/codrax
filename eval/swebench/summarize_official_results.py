#!/usr/bin/env python3
"""Summarize official SWE-bench harness JSON results.

This script is deliberately dependency-free. It reads the JSON artifacts written
by the official harness (`report.<run_id>.json` and per-instance `report.json`
files) and reports `resolved/total` style metrics without importing SWE-bench or
parsing terminal prose.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any


def load_json(path: Path) -> dict[str, Any]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise SystemExit(f"file not found: {path}") from exc
    except json.JSONDecodeError as exc:
        raise SystemExit(f"{path}: invalid JSON: {exc}") from exc
    if not isinstance(data, dict):
        raise SystemExit(f"{path}: expected a JSON object")
    return data


def load_predictions(path: Path | None) -> dict[str, dict[str, Any]]:
    if path is None:
        return {}
    out: dict[str, dict[str, Any]] = {}
    with path.open(encoding="utf-8") as handle:
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
            instance_id = row.get("instance_id")
            if not isinstance(instance_id, str) or not instance_id.strip():
                raise SystemExit(f"{path}:{line_no}: instance_id must be a non-empty string")
            if instance_id in out:
                raise SystemExit(f"{path}:{line_no}: duplicate instance_id {instance_id!r}")
            out[instance_id] = row
    return out


def ids(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    out = sorted({str(item).strip() for item in value if str(item).strip()})
    return out


def int_field(data: dict[str, Any], key: str, fallback: int = 0) -> int:
    value = data.get(key)
    if isinstance(value, bool):
        return int(value)
    if isinstance(value, int):
        return max(value, 0)
    return fallback


def rate(num: int, den: int) -> float | None:
    if den <= 0:
        return None
    return num / den


def percent(value: float | None) -> float | None:
    if value is None:
        return None
    return round(value * 100.0, 4)


def summarize_run_report(report: dict[str, Any]) -> dict[str, Any]:
    resolved_ids = ids(report.get("resolved_ids"))
    unresolved_ids = ids(report.get("unresolved_ids"))
    error_ids = ids(report.get("error_ids"))
    empty_patch_ids = ids(report.get("empty_patch_ids"))
    completed_ids = ids(report.get("completed_ids"))
    submitted_ids = ids(report.get("submitted_ids"))
    incomplete_ids = ids(report.get("incomplete_ids"))

    resolved = int_field(report, "resolved_instances", len(resolved_ids))
    unresolved = int_field(report, "unresolved_instances", len(unresolved_ids))
    completed = int_field(report, "completed_instances", len(completed_ids) or resolved + unresolved)
    submitted = int_field(report, "submitted_instances", len(submitted_ids))
    total = int_field(report, "total_instances", submitted)
    errors = int_field(report, "error_instances", len(error_ids))
    empty = int_field(report, "empty_patch_instances", len(empty_patch_ids))

    return {
        "schema_version": 1,
        "source": "swebench_official_harness",
        "total_instances": total,
        "submitted_instances": submitted,
        "completed_instances": completed,
        "resolved_instances": resolved,
        "unresolved_instances": unresolved,
        "empty_patch_instances": empty,
        "error_instances": errors,
        "incomplete_instances": int_field(report, "incomplete_instances", len(incomplete_ids)),
        "resolved_rate_submitted": rate(resolved, submitted),
        "resolved_rate_completed": rate(resolved, completed),
        "resolved_rate_total": rate(resolved, total),
        "resolved_percent_submitted": percent(rate(resolved, submitted)),
        "resolved_percent_completed": percent(rate(resolved, completed)),
        "resolved_percent_total": percent(rate(resolved, total)),
        "resolved_ids": resolved_ids,
        "unresolved_ids": unresolved_ids,
        "empty_patch_ids": empty_patch_ids,
        "error_ids": error_ids,
        "completed_ids": completed_ids,
        "submitted_ids": submitted_ids,
        "incomplete_ids": incomplete_ids,
    }


def summarize_instance_reports(log_dir: Path, run_id: str, predictions: dict[str, dict[str, Any]], model_name: str = "") -> dict[str, Any]:
    run_root = log_dir / run_id
    if not run_root.is_dir():
        raise SystemExit(f"official harness log dir not found: {run_root}")
    reports: dict[str, dict[str, Any]] = {}
    model_dirs = [run_root / model_name.replace("/", "__")] if model_name else [p for p in run_root.iterdir() if p.is_dir()]
    for model_dir in model_dirs:
        if not model_dir.is_dir():
            continue
        for report_path in model_dir.glob("*/report.json"):
            report = load_json(report_path)
            for instance_id, row in report.items():
                if isinstance(row, dict):
                    reports[str(instance_id)] = row

    submitted_ids = sorted(predictions) if predictions else sorted(reports)
    empty_patch_ids = sorted(
        instance_id
        for instance_id, row in predictions.items()
        if not str(row.get("model_patch") or "").strip()
    )
    completed_ids = sorted(reports)
    resolved_ids = sorted(instance_id for instance_id, row in reports.items() if bool(row.get("resolved")))
    unresolved_ids = sorted(instance_id for instance_id in reports if instance_id not in set(resolved_ids))
    error_ids = sorted(set(submitted_ids) - set(completed_ids) - set(empty_patch_ids))
    run_report = {
        "total_instances": len(submitted_ids) if submitted_ids else len(completed_ids),
        "submitted_instances": len(submitted_ids),
        "completed_instances": len(completed_ids),
        "resolved_instances": len(resolved_ids),
        "unresolved_instances": len(unresolved_ids),
        "empty_patch_instances": len(empty_patch_ids),
        "error_instances": len(error_ids),
        "completed_ids": completed_ids,
        "submitted_ids": submitted_ids,
        "resolved_ids": resolved_ids,
        "unresolved_ids": unresolved_ids,
        "empty_patch_ids": empty_patch_ids,
        "error_ids": error_ids,
    }
    return summarize_run_report(run_report)


def format_summary(summary: dict[str, Any]) -> str:
    resolved = summary["resolved_instances"]
    submitted = summary["submitted_instances"]
    completed = summary["completed_instances"]
    total = summary["total_instances"]
    submitted_pct = summary.get("resolved_percent_submitted")
    completed_pct = summary.get("resolved_percent_completed")
    total_pct = summary.get("resolved_percent_total")
    def pct_text(value: Any) -> str:
        return "n/a" if value is None else f"{value:.2f}%"
    return (
        f"official_resolved submitted={resolved}/{submitted} ({pct_text(submitted_pct)}) "
        f"completed={resolved}/{completed} ({pct_text(completed_pct)}) "
        f"total={resolved}/{total} ({pct_text(total_pct)}) "
        f"errors={summary['error_instances']} empty_patch={summary['empty_patch_instances']}"
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-report", type=Path, help="Official harness run report JSON, e.g. codrax.<run_id>.json")
    parser.add_argument("--log-dir", type=Path, default=Path("logs/run_evaluation"), help="Official harness per-instance log root")
    parser.add_argument("--run-id", help="Run ID used by the official harness")
    parser.add_argument("--model-name", default="", help="Optional model name/path used in predictions")
    parser.add_argument("--predictions-jsonl", type=Path, help="Optional predictions JSONL for submitted/empty/error denominators")
    parser.add_argument("--output-json", type=Path, help="Write normalized summary JSON to this path")
    parser.add_argument("--json-only", action="store_true", help="Print only normalized JSON to stdout")
    args = parser.parse_args()

    predictions = load_predictions(args.predictions_jsonl)
    if args.run_report:
        summary = summarize_run_report(load_json(args.run_report))
    else:
        if not args.run_id:
            raise SystemExit("--run-id is required when --run-report is omitted")
        summary = summarize_instance_reports(args.log_dir, args.run_id, predictions, args.model_name)
    if args.run_id:
        summary["run_id"] = args.run_id
    if args.model_name:
        summary["model_name_or_path"] = args.model_name
    if args.predictions_jsonl:
        summary["predictions_path"] = str(args.predictions_jsonl)
        summary["prediction_count"] = len(predictions)

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
