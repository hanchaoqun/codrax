#!/usr/bin/env python3
"""Frozen input manifest of one HMC batch ("冻结 N 源输入 / M 外部 / K 变更").

  freeze.py write  --base <commit> --out <manifest.json> [options]
  freeze.py verify --manifest <manifest.json> [--head <commit>]
  freeze.py inputs [--repo DIR]            # count + digest only, writes nothing

``write`` records, in one exclusive-create JSON file:

* ``source``   every tracked or untracked-not-ignored build/eval input of the
               working tree with its SHA-256 (rule in hmc_common.INPUT_*);
* ``external`` inputs outside the repository named on the command line
               (user originals, reference-repo files, previous seal headers,
               the frozen CLI copy, these tools) keyed by canonical realpath;
* ``changed``  files that differ from ``--base`` (committed, staged, unstaged
               or untracked) - every one must be a source input unless
               ``--allow-changed-outside-inputs`` is given, in which case the
               others are listed separately and never counted;
* ``snapshots`` optional ``NNN-<name>.txt`` byte copies of the changed files in
               a directory OUTSIDE the repository (``--snapshot-dir``).

After the freeze nothing may be written: ``verify`` re-hashes all three maps
and fails on the first difference. It needs no network and calls no model.
"""
import argparse
import json
import os
import sys

sys.dont_write_bytecode = True  # before the sibling import: no __pycache__ in the repo
import hmc_common as c  # noqa: E402

SCHEMA = "hmc.freeze.v2"


def changed_files(repo, base):
    """Working tree vs ``base``: committed + staged + unstaged + untracked."""
    diff = c.git(repo, "diff", "--name-only", "-z", base, "--").split("\0")
    untracked = c.git(repo, "ls-files", "--others", "--exclude-standard", "-z").split("\0")
    return sorted(set(diff + untracked) - {""})


def external_files(args):
    paths = list(args.external or [])
    for listing in args.external_list or []:
        with open(listing, "r", encoding="utf-8") as handle:
            paths.extend(line.rstrip("\n") for line in handle if line.strip() and not line.startswith("#"))
    for root in args.external_tree or []:
        paths.extend(c.tree(root))
    result = {}
    for path in paths:
        if not os.path.isabs(path):
            raise c.ToolError("external input must be an absolute path: %s" % path)
        real = os.path.realpath(path)
        if os.path.islink(path):
            raise c.ToolError("external input is a symlink: %s" % path)
        result[real] = c.sha256_file(real)
    return dict(sorted(result.items()))


def head_state(repo):
    return {"head": c.git(repo, "rev-parse", "HEAD").strip(),
            "branch": c.git(repo, "branch", "--show-current").strip()}


def cmd_write(args):
    repo = c.resolve_repo(args.repo)
    out = os.path.abspath(args.out)
    if os.path.commonpath([os.path.realpath(os.path.dirname(out)), repo]) == repo and not args.allow_out_in_repo:
        raise c.ToolError("write the manifest outside the repository (or under a git-ignored results root "
                          "with --allow-out-in-repo): %s" % out)
    if os.path.lexists(out):
        raise c.ToolError("refusing to overwrite existing manifest: %s" % out)
    state = head_state(repo)
    base = c.git(repo, "rev-parse", "--verify", args.base + "^{commit}").strip()
    if args.head and state["head"] != c.git(repo, "rev-parse", "--verify", args.head + "^{commit}").strip():
        raise c.ToolError("HEAD is %s, not the expected %s" % (state["head"], args.head))
    if args.branch and state["branch"] != args.branch:
        raise c.ToolError("branch is %r, not the expected %r" % (state["branch"], args.branch))

    source, legacy = c.source_inputs(repo)
    digest = c.source_digest(source)

    for receipt_path in args.require_receipt or []:
        receipt = c.load_json(receipt_path)
        log = receipt.get("log")
        green = (receipt.get("exit_code") == 0 and receipt.get("signal") is None
                 and receipt.get("os_terminal_observed") is True
                 and receipt.get("source_unchanged") is True
                 and receipt.get("source_sha256") == digest
                 and log and c.sha256_file(log) == receipt.get("log_sha256"))
        if not green:
            raise c.ToolError("check receipt is not green for this exact source: %s" % receipt_path)

    changed = changed_files(repo, base)
    deleted = [rel for rel in changed if not os.path.lexists(os.path.join(repo, rel))]
    # A deleted input is still a reviewed change of the candidate; it has no bytes to hash.
    counted = [rel for rel in changed if rel in source or (rel in deleted and c.is_source_input(rel))]
    outside = [rel for rel in changed if rel not in counted]
    if outside and not args.allow_changed_outside_inputs:
        raise c.ToolError("changed files that are not frozen source inputs (review them, or pass "
                          "--allow-changed-outside-inputs): %s" % ", ".join(outside[:20]))

    snapshots = {}
    if args.snapshot_dir:
        snap = os.path.abspath(args.snapshot_dir)
        if os.path.commonpath([os.path.realpath(os.path.dirname(snap)), repo]) == repo:
            # HMC incident: .go snapshots inside the repo became 5 shadow packages of `go test ./...`.
            raise c.ToolError("snapshot directory must be outside the repository: %s" % snap)
        os.mkdir(snap, 0o700)
        for index, rel in enumerate(counted, 1):
            if rel in deleted:
                continue
            target = os.path.join(snap, "%03d-%s.txt" % (index, os.path.basename(rel)))
            sha, _ = c.copy_exclusive(os.path.join(repo, rel), target)
            if sha != source[rel]:
                raise c.ToolError("source changed while snapshotting: %s" % rel)
            snapshots[target] = sha

    cases = {}
    for item in args.case or []:
        label, _, path = item.partition("=")
        if not label or not path:
            raise c.ToolError("--case wants LABEL=PATH: %r" % item)
        cases[label] = os.path.realpath(path)

    manifest = {
        "schema": SCHEMA, "at": c.now_utc(), "repo": repo, "baseline": base,
        "head": state["head"], "branch": state["branch"], "upstream": args.upstream,
        "source_sha256": digest, "source": source, "external": external_files(args),
        "snapshots": snapshots, "changed": counted, "changed_outside_inputs": outside,
        "deleted": deleted, "legacy_name_excluded": legacy, "cases": cases,
        "checks": list(args.require_receipt or []), "runs_per_case": 1,
        "tool": {"freeze.py": c.sha256_file(os.path.realpath(__file__)),
                 "hmc_common.py": c.sha256_file(os.path.realpath(c.__file__))},
    }
    after, _ = c.source_inputs(repo)
    if after != source:
        raise c.ToolError("source changed while the manifest was being built; nothing written")
    c.save_json(out, manifest)
    summary(out, manifest)


def summary(path, manifest):
    counts = {"freeze_sha256": c.sha256_file(path), "inputs": len(manifest["source"]),
              "external": len(manifest["external"]), "changed": len(manifest["changed"])}
    print(json.dumps(counts))
    print("冻结 SHA256 %s：%s 源输入、%s 外部、%s 变更" % (
        counts["freeze_sha256"], c.thousands(counts["inputs"]), c.thousands(counts["external"]),
        c.thousands(counts["changed"])))
    return counts


def verify(manifest_path, repo=None, head=None, check_external=True):
    """Returns a list of human-readable differences; empty means still frozen."""
    manifest = c.load_json(manifest_path)
    repo = c.resolve_repo(repo or manifest.get("repo"))
    problems = []
    if head:
        actual = c.git(repo, "rev-parse", "HEAD").strip()
        if actual != c.git(repo, "rev-parse", "--verify", head + "^{commit}").strip():
            problems.append("HEAD is %s, expected %s" % (actual, head))
    current, _ = c.source_inputs(repo)
    frozen = manifest["source"]
    if manifest.get("schema") != SCHEMA:
        # HMC350-375 Ruby manifests never listed tracked files named providers*.
        current = {rel: sha for rel, sha in current.items() if not c.credential_path(rel)}
    for rel in sorted(set(frozen) | set(current)):
        if frozen.get(rel) != current.get(rel):
            kind = "added" if rel not in frozen else "removed" if rel not in current else "modified"
            problems.append("source %s: %s" % (kind, rel))
    if check_external:
        for key in ("external", "snapshots"):
            for path, sha in manifest.get(key, {}).items():
                try:
                    if c.sha256_file(path) != sha:
                        problems.append("%s modified: %s" % (key, path))
                except (c.ToolError, OSError):
                    problems.append("%s missing or unreadable: %s" % (key, path))
    return manifest, problems


def cmd_verify(args):
    manifest, problems = verify(args.manifest, args.repo, args.head, not args.source_only)
    report = {"manifest": os.path.realpath(args.manifest), "freeze_sha256": c.sha256_file(args.manifest),
              "schema": manifest.get("schema"), "inputs": len(manifest["source"]),
              "external": len(manifest.get("external", {})), "changed": len(manifest.get("changed", [])),
              "external_checked": not args.source_only, "frozen": not problems,
              "differences": len(problems)}
    print(json.dumps(report))
    for line in problems[:args.show]:
        print("  " + line)
    if len(problems) > args.show:
        print("  ... %d more" % (len(problems) - args.show))
    sys.exit(0 if not problems else 1)


def cmd_inputs(args):
    repo = c.resolve_repo(args.repo)
    source, legacy = c.source_inputs(repo)
    print(json.dumps({"repo": repo, "head": c.git(repo, "rev-parse", "HEAD").strip(), "inputs": len(source),
                      "source_sha256": c.source_digest(source), "legacy_name_excluded": legacy}))


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command", required=True)

    w = sub.add_parser("write", help="build the manifest (exclusive create) and print the counts line")
    w.add_argument("--repo", help="repository root (default: the one containing this tool)")
    w.add_argument("--base", required=True, help="baseline commit the candidate is compared with")
    w.add_argument("--out", required=True, help="manifest path; must not exist")
    w.add_argument("--head", help="require HEAD to be exactly this commit")
    w.add_argument("--branch", help="require this branch to be checked out (e.g. main)")
    w.add_argument("--upstream", help="remote main SHA to record (never fetched by this tool)")
    w.add_argument("--external", action="append", help="absolute path of one external input (repeatable)")
    w.add_argument("--external-list", action="append", help="text file with one absolute path per line")
    w.add_argument("--external-tree", action="append", help="directory whose every regular file is external input")
    w.add_argument("--snapshot-dir", help="new directory OUTSIDE the repo for NNN-<name>.txt copies of changed files")
    w.add_argument("--case", action="append", help="LABEL=case-file, recorded only (e.g. A=eval/cases/x.case)")
    w.add_argument("--require-receipt", action="append",
                   help="suite_stats.py receipt that must be green for this exact source (repeatable)")
    w.add_argument("--allow-changed-outside-inputs", action="store_true",
                   help="list changed non-input files separately instead of refusing")
    w.add_argument("--allow-out-in-repo", action="store_true")
    w.set_defaults(func=cmd_write)

    v = sub.add_parser("verify", help="re-hash a manifest against the working tree and external files")
    v.add_argument("--manifest", required=True)
    v.add_argument("--repo", help="override the repository recorded in the manifest")
    v.add_argument("--head", help="also require HEAD to be this commit")
    v.add_argument("--source-only", action="store_true", help="skip external/snapshot files (e.g. old /private/tmp inputs)")
    v.add_argument("--show", type=int, default=20, help="differences to print (default 20)")
    v.set_defaults(func=cmd_verify)

    i = sub.add_parser("inputs", help="print the current source-input count and digest; writes nothing")
    i.add_argument("--repo")
    i.set_defaults(func=cmd_inputs)

    args = parser.parse_args(argv)
    try:
        args.func(args)
    except c.ToolError as error:
        c.die(str(error))


if __name__ == "__main__":
    main()
