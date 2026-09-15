#!/usr/bin/env python3
"""Resolve PLAN_EXPECT's source without changing the final verification plan.

Only a typed terminal proof-only plan can delegate the plan-content oracle to
the source owners in its formal final delivery. No history search, prose, or
regex match chooses those owners. This is eval bookkeeping, not product proof.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess


class InvalidDelivery(ValueError):
    pass


def require(condition, reason):
    if not condition:
        raise InvalidDelivery(reason)


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, "ambiguous_json")
        result[key] = value
    return result


def load(path):
    try:
        raw = path.read_text(encoding="utf-8")
        value = json.loads(raw, object_pairs_hook=unique_object)
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        raise InvalidDelivery("artifact_unreadable") from exc
    require(isinstance(value, dict), "artifact_not_object")
    return value, raw


def identifier(value):
    require(isinstance(value, str) and re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]*", value) is not None, "invalid_plan_id")
    return value


def paths(value):
    require(isinstance(value, list) and value, "source_paths_missing")
    out = set()
    for item in value:
        require(isinstance(item, str) and item and not any(c in item for c in "\x00\n\r\t\\"), "invalid_source_path")
        path = PurePosixPath(item)
        require(not path.is_absolute() and ".." not in path.parts and str(path) == item and item != ".", "invalid_source_path")
        out.add(item)
    return out


def exact_artifact(roots, name):
    """Use the first standard artifact root, not arbitrary archived plans."""
    for root in roots:
        candidate = root / name
        if candidate.exists():
            require(candidate.is_file() and candidate.resolve().is_relative_to(root.resolve()), "artifact_outside_root")
            return candidate
    raise InvalidDelivery("artifact_missing:" + name)


def git(repo, *args, optional=False):
    result = subprocess.run(["git", "-C", str(repo), *args], capture_output=True)
    if result.returncode:
        if optional:
            return None
        raise InvalidDelivery("git_receipt_unresolved")
    return result.stdout


def resolve_commit(repo, revision):
    # Revision strings only come from checked SHA/ref carriers, never prose.
    value = git(repo, "rev-parse", "--verify", revision + "^{commit}", optional=True)
    return value.decode().strip() if value else ""


def checked_commit(repo, plan, owned_paths):
    plan_id = identifier(plan.get("id"))
    require(plan.get("status") == "applied", "source_owner_not_applied:" + plan_id)
    sha = plan.get("applied_commit_sha", "")
    require(isinstance(sha, str) and re.fullmatch(r"[0-9a-fA-F]{40,64}", sha) is not None, "source_commit_missing:" + plan_id)
    commit = resolve_commit(repo, sha)
    require(commit and commit.lower() == sha.lower(), "source_commit_unresolved:" + plan_id)
    canonical_ref = "refs/codrax/applied/" + plan_id
    ref_commit = resolve_commit(repo, canonical_ref)
    require(not ref_commit or ref_commit == commit, "source_ref_mismatch:" + plan_id)
    checkpoint = plan.get("apply_checkpoint")
    if checkpoint is not None:
        require(isinstance(checkpoint, dict), "source_checkpoint_invalid:" + plan_id)
        require(checkpoint.get("commit_sha") == commit and checkpoint.get("recovery_ref") == canonical_ref, "source_checkpoint_mismatch:" + plan_id)
        require(owned_paths <= paths(checkpoint.get("committed_paths")), "source_checkpoint_paths_mismatch:" + plan_id)
    if "applied_paths" in plan:
        require(owned_paths <= paths(plan["applied_paths"]), "source_applied_paths_mismatch:" + plan_id)
    touched = git(repo, "diff-tree", "--root", "--no-commit-id", "--name-only", "--no-renames", "-r", "-z", commit)
    changed = set(touched.decode().rstrip("\x00").split("\x00"))
    require(owned_paths <= changed, "source_commit_paths_mismatch:" + plan_id)
    return commit


def verify_delivered_paths(repo, apply_source, owners, source_paths):
    require(apply_source.is_dir(), "applied_tree_missing")
    for path in sorted(source_paths):
        commits = {owner["commit"] for owner in owners if path in owner["paths"]}
        maximal = [commit for commit in commits if not any(commit != other and git(repo, "merge-base", "--is-ancestor", commit, other, optional=True) is not None for other in commits)]
        require(len(maximal) == 1, "source_path_owner_ambiguous:" + path)
        tree_entry = git(repo, "ls-tree", "-z", maximal[0], "--", path)
        delivered = apply_source / path
        require(delivered.parent.resolve().is_relative_to(apply_source.resolve()), "applied_path_outside_tree:" + path)
        if not tree_entry:
            require(not os.path.lexists(delivered), "applied_tree_mismatch:" + path)
            continue
        metadata, actual_path = tree_entry.rstrip(b"\x00").split(b"\t", 1)
        mode, kind, blob = metadata.decode().split()
        require(kind == "blob" and actual_path.decode() == path, "source_tree_entry_unsupported:" + path)
        if mode == "120000":
            require(delivered.is_symlink(), "applied_tree_mismatch:" + path)
            content = os.fsencode(os.readlink(delivered))
        else:
            require(mode in {"100644", "100755"} and delivered.is_file() and not delivered.is_symlink(), "applied_tree_mismatch:" + path)
            content = delivered.read_bytes()
            require(bool(delivered.stat().st_mode & 0o111) == (mode == "100755"), "applied_mode_mismatch:" + path)
        expected = git(repo, "cat-file", "blob", blob)
        require(content == expected, "applied_tree_mismatch:" + path)


def resolve(plan_path, outdir, scratch, apply_source, snapshot_path):
    plan, _ = load(plan_path)
    final_id = plan.get("id")
    receipt = {"status": "resolved", "selection": "terminal_plan", "final_plan_id": final_id, "oracle_path": str(plan_path)}
    # Ordinary plan/apply mode retains the existing exact file and oracle.
    if plan.get("persistence_kind") != "proof_probe_only":
        return receipt
    final_id = identifier(final_id)
    require(plan.get("changes") is None or plan.get("changes") == [], "proof_plan_has_changes")
    require(isinstance(plan.get("verification_probes"), list) and plan["verification_probes"], "proof_plan_missing_probes")
    roots = [outdir, plan_path.parent, scratch / ".codrax/plans"]
    worktree = plan.get("worktree_path")
    if isinstance(worktree, str) and worktree:
        roots.append(Path(worktree) / ".codrax/plans")
    final_path = exact_artifact(roots, final_id + ".final.json")
    report_path = exact_artifact(roots, final_id + ".report.json")
    final, _ = load(final_path)
    report, _ = load(report_path)
    require(final.get("kind") == "final_report" and final.get("plan", {}).get("id") == final_id, "final_plan_mismatch")
    require(report.get("plan_id") == final_id and report.get("channel") == "post_apply_verify", "report_plan_mismatch")
    delivery = final.get("delivery")
    require(isinstance(delivery, dict), "delivery_missing")
    require(delivery.get("status") == "coherent" and delivery.get("relation") in {"source_plan_owns_final_delivery", "source_plan_with_later_validation_followup"}, "delivery_not_coherent")
    require(delivery.get("final_plan_id") == final_id and delivery.get("report_plan_id") == final_id, "delivery_plan_mismatch")
    ids = delivery.get("source_owner_plan_ids")
    require(isinstance(ids, list) and ids and len(ids) == len(set(identifier(value) for value in ids)), "source_owners_missing_or_duplicate")
    require(delivery.get("primary_source_plan_id") in ids and final_id not in ids, "primary_source_owner_mismatch")
    if final.get("source_authority") is not None:
        authority = final["source_authority"]
        require(isinstance(authority, dict) and authority.get("plan_id") == delivery["primary_source_plan_id"], "source_authority_owner_mismatch")
    source_paths = paths(delivery.get("source_paths"))
    owners, raw_plans, covered = [], [], set()
    for owner_id in ids:
        source_path = exact_artifact(roots, owner_id + ".json")
        source, raw = load(source_path)
        require(source.get("id") == owner_id, "source_owner_id_mismatch:" + owner_id)
        changes = source.get("changes")
        require(isinstance(changes, list) and changes and all(isinstance(row, dict) for row in changes), "source_owner_has_no_changes:" + owner_id)
        change_paths = paths([value for row in changes for value in [row.get("path"), row.get("new_path")] if value])
        owned = source_paths & change_paths
        require(owned, "source_owner_paths_mismatch:" + owner_id)
        commit = checked_commit(scratch, source, owned)
        owners.append({"plan_id": owner_id, "plan_path": str(source_path), "commit": commit, "paths": sorted(owned), "artifact_sha256": hashlib.sha256(raw.encode()).hexdigest()})
        raw_plans.append(raw.strip())
        covered.update(owned)
    require(covered == source_paths, "delivery_source_paths_not_owned")
    verify_delivered_paths(scratch, apply_source, owners, source_paths)
    # Keep original owner-plan JSON bytes inside one array; do not synthesize
    # a patch, rewrite regexes, or search a plan selected by its matching text.
    snapshot_path.write_text("[\n" + ",\n".join(raw_plans) + "\n]\n", encoding="utf-8")
    receipt.update(selection="final_delivery_source_plans", oracle_path=str(snapshot_path), final_report_path=str(final_path), report_path=str(report_path),
                   primary_source_plan_id=delivery["primary_source_plan_id"], source_owners=owners, source_paths=sorted(source_paths))
    return receipt


def main():
    parser = argparse.ArgumentParser()
    for name in ["plan", "outdir", "scratch", "apply-source", "receipt", "snapshot"]:
        parser.add_argument("--" + name, required=True, type=Path)
    args = parser.parse_args()
    try:
        receipt = resolve(args.plan, args.outdir, args.scratch, args.apply_source, args.snapshot)
    except (InvalidDelivery, OSError, ValueError, TypeError, AttributeError) as exc:
        receipt = {"status": "invalid", "reason": str(exc) if isinstance(exc, InvalidDelivery) else "invalid_delivery_artifact"}
    args.receipt.write_text(json.dumps(receipt, indent=2) + "\n", encoding="utf-8")
    if receipt["status"] != "resolved":
        return 1
    print(receipt["oracle_path"])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
