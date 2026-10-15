#!/usr/bin/env python3
"""Read-only summary handoff for artifacts already produced by one eval run.

This does not discover plans, materialize trees, or grant verification authority.
The writer binds the actual execution plan and existing materialization receipts;
the reader checks those bindings before displaying the existing delivered tree.
"""

import argparse
import hashlib
import json
from pathlib import Path
import re

from write_plan_oracle import InvalidDelivery, load, require


def path(value):
    require(isinstance(value, str) and value and not any(c in value for c in "\n\r\t"), "invalid_path")
    return Path(value).resolve()


def fingerprint(file):
    return hashlib.sha256(file.read_bytes()).hexdigest()


def identity(outdir, scratch, run_id):
    require(re.fullmatch(r"[1-9][0-9]*", run_id) is not None, "invalid_run")
    out, repo = path(outdir), path(scratch)
    require(repo.is_relative_to(out), "foreign_scratch")
    return out, repo, {"outdir": str(out), "scratch": str(repo), "run_id": run_id}


def owned_plan(plan, out, repo, run_id):
    # Explicit --plan-out or the commandless product's own runtime directory.
    require(plan == out / f"run-{run_id}.plan.json" or
            plan.is_relative_to(repo / ".codrax"), "foreign_plan")


def materialized_source(out, repo, run_id, plan_id, receipt):
    materialization = out / f"run-{run_id}.materialization.json"
    seed_path = out / f"run-{run_id}.delivery-seed.json"
    mat, _ = load(materialization)
    seed, _ = load(seed_path)
    final_path = path(receipt.get("final_report_path"))
    require(final_path == out / (plan_id + ".final.json") or
            final_path.is_relative_to(repo / ".codrax"), "foreign_final")
    final, _ = load(final_path)
    require(final.get("kind") == "final_report" and final.get("plan", {}).get("id") == plan_id,
            "final_plan_mismatch")
    require(mat.get("status") == "resolved" and mat.get("final_plan_id") == plan_id,
            "materialization_unavailable")
    require(final.get("run_id") and mat.get("run_id") == final["run_id"], "workflow_mismatch")
    require(seed.get("schema_version") == 1 and path(seed.get("repo_root")) == repo and
            seed.get("commit_sha") and mat.get("seed_commit_sha") == seed["commit_sha"], "seed_mismatch")
    source = out / f"run-{run_id}.applied-tree"
    require(path(mat.get("path")) == source and source.is_dir() and not source.is_symlink(),
            "source_unavailable")
    return {"source_path": str(source), "materialization_sha256": fingerprint(materialization),
            "seed_sha256": fingerprint(seed_path), "final_path": str(final_path),
            "final_sha256": fingerprint(final_path)}


def capture(receipt_path, plan_path, outdir, scratch):
    """Extend the existing run receipt without changing its verification fields."""
    receipt, _ = load(Path(receipt_path))
    binding = {"status": "unavailable"}
    try:
        match = re.fullmatch(r"run-([1-9][0-9]*)\.write-apply\.json", Path(receipt_path).name)
        require(match is not None, "non_run_receipt")
        run_id = match[1]
        out, repo, owner = identity(outdir, scratch, run_id)
        require(path(receipt_path) == out / f"run-{run_id}.write-apply.json", "foreign_receipt")
        plan = path(plan_path)
        owned_plan(plan, out, repo, run_id)
        value, _ = load(plan)
        plan_id = value.get("id")
        require(isinstance(plan_id, str) and re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]*", plan_id), "invalid_plan_id")
        require(receipt.get("plan_written") is True and receipt.get("plan_id") == plan_id, "plan_mismatch")
        binding = {"status": "resolved", **owner, "plan_path": str(plan),
                   "plan_id": plan_id, "plan_sha256": fingerprint(plan)}
        try:
            binding["materialization"] = materialized_source(out, repo, run_id, plan_id, receipt)
        except (InvalidDelivery, OSError, ValueError, TypeError, AttributeError) as error:
            binding["materialization_unavailable"] = str(error)
    except (InvalidDelivery, OSError, ValueError, TypeError, AttributeError) as error:
        binding["reason"] = str(error)
    receipt["summary_artifacts"] = binding
    Path(receipt_path).write_text(json.dumps(receipt, indent=2) + "\n", encoding="utf-8")


def resolve(outdir, scratch, run_id):
    """Return independently available plan/source paths; never mutate artifacts."""
    result = {"plan_path": "", "source_path": ""}
    try:
        out, repo, owner = identity(outdir, scratch, run_id)
        receipt, _ = load(out / f"run-{run_id}.write-apply.json")
        binding = receipt.get("summary_artifacts", {})
        require(binding.get("status") == "resolved" and all(binding.get(k) == v for k, v in owner.items()),
                "summary_binding_unavailable")
        plan = path(binding.get("plan_path"))
        owned_plan(plan, out, repo, run_id)
        value, _ = load(plan)
        require(receipt.get("plan_written") is True and
                receipt.get("plan_id") == binding.get("plan_id") == value.get("id") and
                fingerprint(plan) == binding.get("plan_sha256"), "stale_plan")
        result["plan_path"] = str(plan)
        expected = binding.get("materialization")
        require(isinstance(expected, dict), "materialization_binding_unavailable")
        actual = materialized_source(out, repo, run_id, value["id"], receipt)
        require(actual == expected, "stale_materialization")
        result["source_path"] = actual["source_path"]
    except (InvalidDelivery, OSError, ValueError, TypeError, AttributeError) as error:
        result["reason"] = str(error)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("capture", "resolve"))
    parser.add_argument("--outdir", required=True)
    parser.add_argument("--scratch", required=True)
    parser.add_argument("--receipt")
    parser.add_argument("--plan")
    parser.add_argument("--run-id")
    args = parser.parse_args()
    if args.action == "capture":
        capture(args.receipt, args.plan, args.outdir, args.scratch)
    else:
        result = resolve(args.outdir, args.scratch, args.run_id)
        # Fixed two-line protocol preserves an unavailable plan without shifting source.
        print(result["plan_path"])
        print(result["source_path"])


if __name__ == "__main__":
    main()
