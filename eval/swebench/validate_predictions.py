#!/usr/bin/env python3
"""Validate a SWE-bench predictions JSONL file.

This is intentionally lightweight and dependency-free. The official harness is
the scoring authority; this script only proves that Codrax produced the shape
the harness expects: one JSON object per prediction with `instance_id` and
`model_patch` strings.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


def load_expected_ids(path: Path) -> set[str]:
    ids: set[str] = set()
    with path.open(encoding="utf-8") as handle:
        for line_no, line in enumerate(handle, 1):
            line = line.strip()
            if not line:
                continue
            try:
                row = json.loads(line)
            except json.JSONDecodeError as exc:
                raise SystemExit(f"{path}:{line_no}: invalid JSON: {exc}") from exc
            instance_id = row.get("instance_id")
            if isinstance(instance_id, str) and instance_id:
                ids.add(instance_id)
    return ids


def require_string_list(row: dict, path: Path, line_no: int, field: str) -> list[str]:
    value = row.get(field)
    if not isinstance(value, list):
        raise SystemExit(f"{path}:{line_no}: {field} must be a list")
    out: list[str] = []
    for item in value:
        if not isinstance(item, str) or not item.strip():
            raise SystemExit(f"{path}:{line_no}: {field} entries must be non-empty strings")
        out.append(item.strip())
    return out


def validate_results_jsonl(
    path: Path,
    prediction_ids: set[str],
    *,
    require_final_report_audit: bool = False,
) -> None:
    if not path.is_file():
        raise SystemExit(f"results file not found: {path}")

    seen: set[str] = set()
    count = 0
    with path.open(encoding="utf-8") as handle:
        for line_no, line in enumerate(handle, 1):
            line = line.strip()
            if not line:
                continue
            count += 1
            try:
                row = json.loads(line)
            except json.JSONDecodeError as exc:
                raise SystemExit(f"{path}:{line_no}: invalid JSON: {exc}") from exc
            if not isinstance(row, dict):
                raise SystemExit(f"{path}:{line_no}: result row must be a JSON object")
            instance_id = row.get("instance_id")
            if not isinstance(instance_id, str) or not instance_id.strip():
                raise SystemExit(f"{path}:{line_no}: instance_id must be a non-empty string")
            if instance_id in seen:
                raise SystemExit(f"{path}:{line_no}: duplicate instance_id {instance_id!r}")
            seen.add(instance_id)

            if require_final_report_audit:
                if row.get("final_report_present") is not True:
                    raise SystemExit(f"{path}:{line_no}: final_report_present must be true")
                for field in (
                    "final_report_path",
                    "final_report_completion_verdict",
                    "final_report_verification_status",
                    "final_report_loop_status",
                    "final_report_loop_truth_state",
                ):
                    value = row.get(field)
                    if not isinstance(value, str) or not value.strip():
                        raise SystemExit(f"{path}:{line_no}: {field} must be a non-empty string")
                event_refs = require_string_list(row, path, line_no, "final_report_loop_event_refs")
                patch_families = require_string_list(row, path, line_no, "final_report_patch_language_families")
                runner_families = require_string_list(row, path, line_no, "final_report_verification_runner_families")
                if not event_refs:
                    raise SystemExit(f"{path}:{line_no}: final_report_loop_event_refs must not be empty")
                if not patch_families:
                    raise SystemExit(f"{path}:{line_no}: final_report_patch_language_families must not be empty")
                if not runner_families:
                    raise SystemExit(f"{path}:{line_no}: final_report_verification_runner_families must not be empty")

    if count == 0:
        raise SystemExit(f"{path}: no result rows found")

    missing = sorted(prediction_ids - seen)
    if missing:
        raise SystemExit(f"{path}: missing results for {len(missing)} prediction(s): {', '.join(missing[:10])}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("predictions", type=Path, help="Predictions JSONL path")
    parser.add_argument("--instances-jsonl", type=Path, help="Optional instance JSONL whose IDs must be covered")
    parser.add_argument("--require-nonempty-patch", action="store_true", help="Fail if any model_patch is empty")
    parser.add_argument("--allow-extra", action="store_true", help="Allow predictions for IDs not in --instances-jsonl")
    parser.add_argument("--results-jsonl", type=Path, help="Optional Codrax adapter results JSONL to validate against predictions")
    parser.add_argument(
        "--require-final-report-audit",
        action="store_true",
        help="When --results-jsonl is set, require final-report loop/truth/language telemetry for every prediction",
    )
    args = parser.parse_args()

    if not args.predictions.is_file():
        raise SystemExit(f"predictions file not found: {args.predictions}")

    expected_ids = load_expected_ids(args.instances_jsonl) if args.instances_jsonl else set()
    seen: set[str] = set()
    count = 0
    empty_patch = 0

    with args.predictions.open(encoding="utf-8") as handle:
        for line_no, line in enumerate(handle, 1):
            line = line.strip()
            if not line:
                continue
            count += 1
            try:
                row = json.loads(line)
            except json.JSONDecodeError as exc:
                raise SystemExit(f"{args.predictions}:{line_no}: invalid JSON: {exc}") from exc
            instance_id = row.get("instance_id")
            model_patch = row.get("model_patch")
            if not isinstance(instance_id, str) or not instance_id.strip():
                raise SystemExit(f"{args.predictions}:{line_no}: instance_id must be a non-empty string")
            if instance_id in seen:
                raise SystemExit(f"{args.predictions}:{line_no}: duplicate instance_id {instance_id!r}")
            seen.add(instance_id)
            if not isinstance(model_patch, str):
                raise SystemExit(f"{args.predictions}:{line_no}: model_patch must be a string")
            if not model_patch.strip():
                empty_patch += 1
                if args.require_nonempty_patch:
                    raise SystemExit(f"{args.predictions}:{line_no}: model_patch is empty for {instance_id}")
            model_name = row.get("model_name_or_path")
            if model_name is not None and not isinstance(model_name, str):
                raise SystemExit(f"{args.predictions}:{line_no}: model_name_or_path must be a string when present")

    if count == 0:
        raise SystemExit(f"{args.predictions}: no predictions found")

    if expected_ids:
        missing = sorted(expected_ids - seen)
        extra = sorted(seen - expected_ids)
        if missing:
            raise SystemExit(f"missing predictions for {len(missing)} instance(s): {', '.join(missing[:10])}")
        if extra and not args.allow_extra:
            raise SystemExit(f"unexpected predictions for {len(extra)} instance(s): {', '.join(extra[:10])}")

    if args.require_final_report_audit and not args.results_jsonl:
        raise SystemExit("--require-final-report-audit requires --results-jsonl")
    if args.results_jsonl:
        validate_results_jsonl(
            args.results_jsonl,
            seen,
            require_final_report_audit=args.require_final_report_audit,
        )

    print(f"validated {count} prediction(s); empty_patch={empty_patch}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
