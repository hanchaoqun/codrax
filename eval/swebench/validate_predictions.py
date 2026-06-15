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


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("predictions", type=Path, help="Predictions JSONL path")
    parser.add_argument("--instances-jsonl", type=Path, help="Optional instance JSONL whose IDs must be covered")
    parser.add_argument("--require-nonempty-patch", action="store_true", help="Fail if any model_patch is empty")
    parser.add_argument("--allow-extra", action="store_true", help="Allow predictions for IDs not in --instances-jsonl")
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

    print(f"validated {count} prediction(s); empty_patch={empty_patch}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
