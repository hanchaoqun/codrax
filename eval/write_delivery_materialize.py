#!/usr/bin/env python3
"""Reconstruct a workflow's durable delivery, never its live worktree.

The runner fixes the seed before the first model call. A system-authored final
receipt supplies retained roots and historical checkpoint candidates. Git
ancestry selects the retained closure; timestamps and arbitrary refs do not.
This receipt is independent of final verification and PLAN_EXPECT ownership.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import stat
import tempfile

from write_plan_oracle import InvalidDelivery, exact_artifact, git, identifier, load, require, resolve_commit


def write_json(path, value, exclusive=False):
    with path.open("x" if exclusive else "w", encoding="utf-8") as handle:
        json.dump(value, handle, indent=2, ensure_ascii=False)
        handle.write("\n")


def repo_root(repo):
    return Path(os.fsdecode(git(repo, "rev-parse", "--show-toplevel")).strip()).resolve()


def capture(repo, seed_path):
    repo = repo_root(repo)
    sha = resolve_commit(repo, "HEAD")
    require(sha, "seed_commit_missing")
    write_json(seed_path, {"schema_version": 1, "repo_root": str(repo), "commit_sha": sha,
                           "tree_sha": git(repo, "rev-parse", sha + "^{tree}").decode().strip()}, exclusive=True)


def safe_path(value):
    require(isinstance(value, str) and value and not any(c in value for c in "\x00\r\n\t\\"), "invalid_delivery_path")
    path = PurePosixPath(value)
    require(not path.is_absolute() and str(path) == value and value != "." and ".." not in path.parts
            and not any(part.lower() == ".git" for part in path.parts), "invalid_delivery_path")
    return value


def path_set(value):
    require(isinstance(value, list), "checkpoint_paths_missing")
    result = {safe_path(item) for item in value}
    require(len(result) == len(value), "duplicate_checkpoint_path")
    return result


def ids(value):
    require(isinstance(value, list), "owner_ids_missing")
    result = [identifier(item) for item in value]
    require(len(result) == len(set(result)), "duplicate_owner_id")
    return result


class Repository:
    def __init__(self, path):
        self.path = path
        self.trees = {}
        self.ancestors = {}

    def ancestor(self, left, right):
        key = (left, right)
        if key not in self.ancestors:
            self.ancestors[key] = git(self.path, "merge-base", "--is-ancestor", left, right, optional=True) is not None
        return self.ancestors[key]

    def tree(self, commit):
        if commit not in self.trees:
            result = {}
            for row in git(self.path, "ls-tree", "-r", "--full-tree", "-z", commit).split(b"\0"):
                if not row:
                    continue
                meta, raw_path = row.split(b"\t", 1)
                mode, kind, oid = meta.decode("ascii").split()
                path = safe_path(os.fsdecode(raw_path))
                require(kind == "blob" and mode in {"100644", "100755", "120000"}, "unsupported_tree_entry:" + path)
                result[path] = (mode, oid)
            self.trees[commit] = result
        return self.trees[commit]


def owner_checkpoint(repository, roots, claim):
    require(isinstance(claim, dict), "invalid_owner")
    plan_id = identifier(claim.get("plan_id"))
    plan, _ = load(exact_artifact(roots, plan_id + ".json"))
    require(plan.get("id") == plan_id, "owner_id_mismatch:" + plan_id)
    checkpoint = plan.get("apply_checkpoint")
    require(isinstance(checkpoint, dict), "checkpoint_missing:" + plan_id)
    require(not checkpoint.get("partial") and not checkpoint.get("commit_error"), "checkpoint_incomplete:" + plan_id)
    sha = claim.get("commit_sha")
    require(isinstance(sha, str) and re.fullmatch(r"[0-9a-f]{40}|[0-9a-f]{64}", sha), "invalid_owner_commit:" + plan_id)
    require(plan.get("applied_commit_sha") == sha == checkpoint.get("commit_sha"), "checkpoint_commit_mismatch:" + plan_id)
    require(resolve_commit(repository.path, sha) == sha, "owner_commit_unresolved:" + plan_id)
    ref = "refs/codrax/applied/" + plan_id
    recovery_ref = checkpoint.get("recovery_ref", "")
    require(isinstance(recovery_ref, str) and recovery_ref in {"", ref}, "checkpoint_ref_mismatch:" + plan_id)
    resolved_ref = resolve_commit(repository.path, ref)
    require(not resolved_ref or resolved_ref == sha, "owner_ref_mismatch:" + plan_id)
    declared = path_set(claim.get("paths"))
    # The Go checkpoint uses omitempty; an omitted staged set is the typed
    # empty slice, valid only when the actual Git delta below is also empty.
    staged = checkpoint.get("committed_paths")
    require(declared == path_set([] if staged is None else staged), "checkpoint_paths_mismatch:" + plan_id)
    parents = git(repository.path, "rev-list", "--parents", "-n", "1", sha).decode().split()[1:]
    require(len(parents) == 1, "unsupported_checkpoint_parents:" + plan_id)
    before, after = repository.tree(parents[0]), repository.tree(sha)
    delta = {path: after.get(path) for path in before.keys() | after.keys() if before.get(path) != after.get(path)}
    require(delta.keys() <= declared, "checkpoint_delta_not_owned:" + plan_id)
    return {"plan_id": plan_id, "commit_sha": sha, "parent": parents[0], "delta": delta}


def latest(repository, owners, path):
    writers = [owner for owner in owners if path in owner["delta"]]
    maxima = [owner for owner in writers if not any(owner is not other and repository.ancestor(owner["commit_sha"], other["commit_sha"])
                                                  for other in writers)]
    require(len(maxima) <= 1, "incomparable_path_owners:" + path)
    return maxima[0] if maxima else None


def verify_tree(directory, tree):
    # Verify the actual filesystem, not an assumed case/Unicode normalization
    # model. A valid Git tree may be unrepresentable on the destination volume.
    leaves = set()
    for root, directories, files in os.walk(directory, followlinks=False):
        for name in files + [name for name in directories if (Path(root) / name).is_symlink()]:
            leaves.add((Path(root) / name).relative_to(directory).as_posix())
    require(leaves == tree.keys(), "materialized_literal_paths_mismatch")
    for path, (mode, oid) in tree.items():
        target = directory / path
        metadata = target.lstat()
        if mode == "120000":
            require(stat.S_ISLNK(metadata.st_mode), "materialized_type_mismatch:" + path)
            content = os.fsencode(os.readlink(target))
        else:
            require(stat.S_ISREG(metadata.st_mode), "materialized_type_mismatch:" + path)
            require(bool(metadata.st_mode & 0o111) == (mode == "100755"), "materialized_mode_mismatch:" + path)
            content = target.read_bytes()
        digest = hashlib.sha1 if len(oid) == 40 else hashlib.sha256
        require(digest(b"blob " + str(len(content)).encode("ascii") + b"\0" + content).hexdigest() == oid,
                "materialized_blob_mismatch:" + path)


def publish(repository, tree, destination):
    # All leaves are known up front. A symlink can never be a parent we follow.
    for path in tree:
        require(not any(str(parent) in tree for parent in PurePosixPath(path).parents if str(parent) != "."),
                "delivery_path_prefix_conflict:" + path)
    temporary = Path(tempfile.mkdtemp(prefix=destination.name + ".tmp-", dir=destination.parent))
    backup = None
    try:
        for path, (mode, oid) in sorted(tree.items()):
            target = temporary / path
            target.parent.mkdir(parents=True, exist_ok=True)
            content = git(repository.path, "cat-file", "blob", oid)
            if mode == "120000":
                os.symlink(os.fsdecode(content), target)
            else:
                target.write_bytes(content)
                target.chmod(0o755 if mode == "100755" else 0o644)
        verify_tree(temporary, tree)
        if os.path.lexists(destination):
            require(destination.is_dir() and not destination.is_symlink(), "unsafe_existing_destination")
            backup = Path(tempfile.mkdtemp(prefix=destination.name + ".old-", dir=destination.parent))
            backup.rmdir()
            destination.rename(backup)
        try:
            temporary.rename(destination)
        except OSError:
            if backup is not None:
                backup.rename(destination)
                backup = None
            raise
    finally:
        if temporary.exists():
            shutil.rmtree(temporary)
        if backup is not None:
            shutil.rmtree(backup)


def materialize(plan_path, outdir, scratch, run_id):
    seed, _ = load(outdir / ("run-" + run_id + ".delivery-seed.json"))
    repo = repo_root(scratch)
    require(seed.get("schema_version") == 1 and seed.get("repo_root") == str(repo), "seed_repo_mismatch")
    seed_sha = seed.get("commit_sha", "")
    require(isinstance(seed_sha, str) and re.fullmatch(r"[0-9a-f]{40}|[0-9a-f]{64}", seed_sha), "seed_commit_invalid")
    require(resolve_commit(repo, seed_sha) == seed_sha, "seed_commit_unresolved")
    require(git(repo, "rev-parse", seed_sha + "^{tree}").decode().strip() == seed.get("tree_sha"), "seed_tree_mismatch")
    plan, _ = load(plan_path)
    final_id = identifier(plan.get("id"))
    roots = [outdir, plan_path.parent, scratch / ".codrax/plans"]
    worktree = plan.get("worktree_path")
    if isinstance(worktree, str) and worktree:
        roots.append(Path(worktree) / ".codrax/plans")
    final, _ = load(exact_artifact(roots, final_id + ".final.json"))
    require(final.get("kind") == "final_report" and isinstance(final.get("plan"), dict)
            and final["plan"].get("id") == final_id, "final_plan_mismatch")
    delivery = final.get("delivery")
    require(isinstance(delivery, dict), "delivery_missing")
    receipt = delivery.get("materialization")
    require(isinstance(receipt, dict) and receipt.get("schema_version") == 1, "materialization_receipt_missing")
    require(receipt.get("status") == "available", "materialization_unavailable:" + str(receipt.get("reason_code", "missing")))
    require(isinstance(final.get("run_id"), str) and final["run_id"] and receipt.get("run_id") == final["run_id"], "materialization_run_mismatch")
    require(receipt.get("final_plan_id") == final_id, "materialization_plan_mismatch")
    candidates = receipt.get("owners")
    require(isinstance(candidates, list), "materialization_owners_missing")
    repository = Repository(repo)
    owners = [owner_checkpoint(repository, roots, claim) for claim in candidates]
    owner_ids = ids([owner["plan_id"] for owner in owners])
    require(len({owner["commit_sha"] for owner in owners}) == len(owners), "duplicate_owner_commit")
    retained_ids = ids(receipt.get("retained_plan_ids"))
    require(set(retained_ids) <= set(owner_ids), "retained_owner_missing")
    retained = [owner for owner in owners if owner["plan_id"] in retained_ids]
    selected = [owner for owner in owners if any(repository.ancestor(owner["commit_sha"], root["commit_sha"]) for root in retained)]
    initial = repository.tree(seed_sha)
    for owner in selected:
        sha = owner["commit_sha"]
        require(sha != seed_sha and repository.ancestor(seed_sha, sha), "owner_outside_seed:" + owner["plan_id"])
        ancestors = [other for other in selected if other is not owner and repository.ancestor(other["commit_sha"], sha)]
        for path in owner["delta"]:
            prior = latest(repository, ancestors, path)
            expected = prior["delta"][path] if prior else initial.get(path)
            require(repository.tree(owner["parent"]).get(path) == expected, "unauthorized_parent_preimage:" + path)
    for index, left in enumerate(selected):
        for right in selected[index + 1:]:
            if repository.ancestor(left["commit_sha"], right["commit_sha"]) or repository.ancestor(right["commit_sha"], left["commit_sha"]):
                continue
            for path in left["delta"]:
                require(not any(path == other or path.startswith(other + "/") or other.startswith(path + "/") for other in right["delta"]),
                        "incomparable_path_owners:" + path)
    tree = dict(initial)
    touched = {path for owner in selected for path in owner["delta"]}
    for path in touched:
        entry = latest(repository, selected, path)["delta"][path]
        if entry is None:
            tree.pop(path, None)
        else:
            tree[path] = entry
    destination = outdir / ("run-" + run_id + ".applied-tree")
    publish(repository, tree, destination)
    return {"status": "resolved", "seed_commit_sha": seed_sha, "run_id": final["run_id"], "final_plan_id": final_id,
            "retained_plan_ids": retained_ids, "selected_plan_ids": [owner["plan_id"] for owner in selected], "path": str(destination)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    seed = commands.add_parser("capture")
    seed.add_argument("--repo", required=True, type=Path)
    seed.add_argument("--seed", required=True, type=Path)
    export = commands.add_parser("materialize")
    export.add_argument("--plan", required=True, type=Path)
    export.add_argument("--outdir", required=True, type=Path)
    export.add_argument("--scratch", required=True, type=Path)
    export.add_argument("--run-id", required=True)
    args = parser.parse_args()
    if args.command == "capture":
        capture(args.repo, args.seed)
        return 0
    identifier(args.run_id)
    try:
        receipt = materialize(args.plan, args.outdir, args.scratch, args.run_id)
    except (InvalidDelivery, OSError, UnicodeError, ValueError) as exc:
        receipt = {"status": "invalid", "reason_code": str(exc)}
    write_json(args.outdir / ("run-" + args.run_id + ".materialization.json"), receipt)
    if receipt["status"] == "resolved":
        print(receipt["path"])
        return 0
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
