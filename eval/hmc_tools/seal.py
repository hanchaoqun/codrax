#!/usr/bin/env python3
"""Seal batch evidence ("封存 N 件") and re-verify a sealed batch.

  seal.py seal    --root <batch-root> [--tree DIR ...] [--file PATH ...] [--list FILE ...]
                  [--exclude PATH ...] [--freeze manifest.json] [--attach NAME=file.json ...]
                  [--prior <previous-batch-root>] [--generation v1]
  seal.py verify  --root <batch-root> [--generation v1]
  seal.py restore --root <batch-root> --source <recorded source path> --to <new file> [--sha256 HEX]

Layout written by ``seal`` (identical row schema to HMC350-375, so ``verify``
reads old and new batches alike):

  <root>/sealed-v1/NNNN-<basename>     byte copies, read-only, ``.go`` -> ``.go.txt``
  <root>/sealed-index-v1.json          [{source, archived, sha256, bytes}, ...]
  <root>/sealed-receipt-v1.json        count, index SHA-256, freeze SHA-256, the
                                       prior-batch verification result, attached JSON

Pieces are the de-duplicated canonical realpaths of everything listed, sorted
by path, numbered from 0001. Nothing sealed is ever rewritten: an existing
``sealed-v1`` directory, index or receipt makes ``seal`` refuse; a successor
generation uses ``--generation v2``. Credential-named paths and symlinks are
refused, and files already inside a ``sealed-v*/`` directory are never copied
again. ``verify`` and ``restore`` open the sealed root read-only.
"""
import argparse
import json
import os
import re
import sys

sys.dont_write_bytecode = True  # before the sibling import: no __pycache__ in the repo
import hmc_common as c  # noqa: E402

SCHEMA = "hmc.sealed_receipt.v2"
SEALED_DIR = re.compile(r"(^|/)sealed-v\d+(/|$)")


def names(root, generation):
    if not re.fullmatch(r"v\d+", generation):
        raise c.ToolError("generation must look like v1, v2, ...")
    return (os.path.join(root, "sealed-" + generation),
            os.path.join(root, "sealed-index-%s.json" % generation),
            os.path.join(root, "sealed-receipt-%s.json" % generation))


def verify_index(root, generation="v1"):
    """Read-only physical check of every row of a sealed index."""
    root = os.path.realpath(root)
    directory, index_path, receipt_path = names(root, generation)
    index = c.load_json(index_path)
    wanted = re.compile(r"\Asealed-%s/[^/]+\Z" % re.escape(generation))
    result = {"root": root, "generation": generation, "count": len(index), "verified": 0,
              "missing": [], "escaped": [], "size_mismatch": [], "sha_mismatch": [],
              "index_sha256": c.sha256_file(index_path)}
    seen = set()
    for row in index:
        archived = row.get("archived", "")
        if not wanted.match(archived) or archived in seen:
            result["escaped"].append(archived)
            continue
        seen.add(archived)
        path = os.path.join(root, archived)
        if not c.regular_file(path):
            result["missing"].append(archived)
        elif os.path.getsize(path) != row.get("bytes"):
            result["size_mismatch"].append(archived)
        elif c.sha256_file(path, allow_credential_name=True) != row.get("sha256"):
            result["sha_mismatch"].append(archived)
        else:
            result["verified"] += 1
    on_disk = sorted(os.listdir(directory)) if os.path.isdir(directory) else []
    result["unindexed_files"] = [name for name in on_disk
                                 if "sealed-%s/%s" % (generation, name) not in seen]
    if os.path.isfile(receipt_path):
        receipt = c.load_json(receipt_path)
        result["receipt_sha256"] = c.sha256_file(receipt_path)
        result["receipt_count_matches"] = receipt.get("count") == len(index)
        result["receipt_index_sha256_matches"] = receipt.get("index_sha256") == result["index_sha256"]
    else:
        result["receipt_sha256"] = None
    result["all_verified"] = (result["verified"] == len(index) and not result["unindexed_files"]
                              and result.get("receipt_count_matches", True)
                              and result.get("receipt_index_sha256_matches", True))
    return result


def brief(result):
    out = {key: result[key] for key in ("root", "generation", "count", "verified", "index_sha256",
                                        "receipt_sha256", "all_verified")}
    for key in ("missing", "escaped", "size_mismatch", "sha_mismatch", "unindexed_files"):
        out[key] = len(result[key])
    for key in ("receipt_count_matches", "receipt_index_sha256_matches"):
        if key in result:
            out[key] = result[key]
    return out


def cmd_verify(args):
    result = verify_index(args.root, args.generation)
    print(json.dumps(brief(result)))
    for key in ("missing", "escaped", "size_mismatch", "sha_mismatch", "unindexed_files"):
        for name in result[key][:args.show]:
            print("  %s: %s" % (key, name))
    sys.exit(0 if result["all_verified"] else 1)


def collect(args, root):
    files = list(args.file or [])
    for listing in args.list or []:
        with open(listing, "r", encoding="utf-8") as handle:
            files.extend(line.rstrip("\n") for line in handle if line.strip() and not line.startswith("#"))
    explicit = len(files)
    for directory in args.tree or []:
        files.extend(c.tree(directory))
    excluded = {os.path.realpath(path) for path in args.exclude or []}
    pieces = set()
    for position, path in enumerate(files):
        if not os.path.isabs(path):
            raise c.ToolError("absolute path required: %s" % path)
        if os.path.islink(path):
            raise c.ToolError("symlink refused: %s" % path)
        real = os.path.realpath(path)
        if real in excluded or any(real.startswith(prefix + os.sep) for prefix in excluded):
            continue
        if SEALED_DIR.search(real):
            if position < explicit:
                raise c.ToolError("do not copy an old archive into a new one: %s" % real)
            continue  # a tree walk steps over sealed-v*/ of an earlier generation
        if os.path.basename(real).startswith(("sealed-index-", "sealed-receipt-")) and os.path.dirname(real) == root:
            continue
        if c.credential_path(real):
            raise c.ToolError("credential path refused: %s" % real)
        if not c.regular_file(real):
            raise c.ToolError("regular file required: %s" % real)
        pieces.add(real)
    return sorted(pieces)


def cmd_seal(args):
    root = os.path.realpath(args.root)
    if not os.path.isdir(root):
        raise c.ToolError("batch root does not exist: %s" % root)
    directory, index_path, receipt_path = names(root, args.generation)
    for path in (directory, index_path, receipt_path):
        if os.path.lexists(path):
            raise c.ToolError("already sealed, nothing is rewritten: %s" % path)

    prior = None
    if args.prior:
        prior_result = verify_index(args.prior, args.prior_generation)
        prior = brief(prior_result)
        if not prior_result["all_verified"] and not args.allow_prior_mismatch:
            raise c.ToolError("previous batch does not re-verify (%d of %d); record it explicitly with "
                              "--allow-prior-mismatch: %s" % (prior["verified"], prior["count"], args.prior))
    attached = {}
    for item in args.attach or []:
        name, sep, path = item.partition("=")
        if not sep or not name:
            raise c.ToolError("--attach wants NAME=file.json: %r" % item)
        attached[name] = {"path": os.path.realpath(path), "sha256": c.sha256_file(path), "value": c.load_json(path)}

    pieces = collect(args, root)
    if not pieces:
        raise c.ToolError("nothing to seal")
    os.mkdir(directory, 0o700)
    index = []
    for number, source in enumerate(pieces, 1):
        name = "%04d-%s" % (number, os.path.basename(source))
        if name.endswith(".go"):
            name += ".txt"  # sealed sources must never become Go packages of ./...
        before = c.sha256_file(source)
        sha, size = c.copy_exclusive(source, os.path.join(directory, name))
        if sha != before or c.sha256_file(os.path.join(directory, name)) != sha:
            raise c.ToolError("source changed while it was being sealed: %s" % source)
        index.append({"source": source, "archived": "sealed-%s/%s" % (args.generation, name),
                      "sha256": sha, "bytes": size})
    c.save_json(index_path, index)
    check = verify_index(root, args.generation)
    if check["verified"] != len(index):
        raise c.ToolError("fresh archive does not re-verify")
    receipt = {"schema": SCHEMA, "sealed_at": c.now_utc(), "count": len(index),
               "index_sha256": c.sha256_file(index_path),
               "freeze_sha256": c.sha256_file(args.freeze) if args.freeze else None,
               "prior": prior, "attached": attached,
               "tool": {"seal.py": c.sha256_file(os.path.realpath(__file__)),
                        "hmc_common.py": c.sha256_file(os.path.realpath(c.__file__))}}
    c.save_json(receipt_path, receipt)
    print(json.dumps({"count": len(index), "index_sha256": receipt["index_sha256"],
                      "receipt_sha256": c.sha256_file(receipt_path),
                      "prior_verified": prior["verified"] if prior else None}))
    line = "封存 %d 件，索引 SHA256 %s，收据 %s" % (len(index), receipt["index_sha256"], c.sha256_file(receipt_path))
    if prior:
        line += "；前代 %d 件逐份复验 %d 件通过" % (prior["count"], prior["verified"])
    print(line)


def cmd_restore(args):
    """Copy one sealed piece back out (e.g. a lost /private/tmp regression input)."""
    root = os.path.realpath(args.root)
    directory, index_path, _ = names(root, args.generation)
    rows = [row for row in c.load_json(index_path)
            if row.get("source") == args.source and (not args.sha256 or row.get("sha256") == args.sha256)]
    if len(rows) != 1:
        raise c.ToolError("%d sealed rows match source %s" % (len(rows), args.source))
    row = rows[0]
    if not re.fullmatch(r"sealed-%s/[^/]+" % re.escape(args.generation), row["archived"]):
        raise c.ToolError("archive path escapes the sealed directory")
    source = os.path.join(root, row["archived"])
    target = os.path.abspath(args.to)
    if SEALED_DIR.search(os.path.realpath(os.path.dirname(target)) + "/"):
        raise c.ToolError("refusing to write inside a sealed directory")
    if os.path.getsize(source) != row["bytes"] or c.sha256_file(source, allow_credential_name=True) != row["sha256"]:
        raise c.ToolError("sealed piece no longer matches its index row: %s" % row["archived"])
    sha, size = c.copy_exclusive(source, target, mode=0o600)
    if sha != row["sha256"]:
        raise c.ToolError("restore mismatch")
    print(json.dumps({"restored": target, "from": row["archived"], "sha256": sha, "bytes": size}))


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command", required=True)
    s = sub.add_parser("seal", help="byte-copy the listed evidence and write index + receipt")
    s.add_argument("--root", required=True, help="batch root, e.g. eval/results/hmc_<topic>_<date>_v1")
    s.add_argument("--tree", action="append", help="directory; every regular file below it is a piece")
    s.add_argument("--file", action="append", help="one absolute file (repeatable)")
    s.add_argument("--list", action="append", help="text file with one absolute path per line")
    s.add_argument("--exclude", action="append", help="file or directory to leave out (e.g. the frozen CLI binary)")
    s.add_argument("--freeze", help="frozen manifest; its SHA-256 goes into the receipt")
    s.add_argument("--attach", action="append", help="NAME=file.json embedded in the receipt (full, overlap, judgment)")
    s.add_argument("--prior", help="previous batch root, physically re-verified first (read-only)")
    s.add_argument("--prior-generation", default="v1")
    s.add_argument("--allow-prior-mismatch", action="store_true",
                   help="seal anyway and record the failed prior verification")
    s.add_argument("--generation", default="v1")
    s.set_defaults(func=cmd_seal)
    v = sub.add_parser("verify", help="read-only: re-hash every sealed piece against the index")
    v.add_argument("--root", required=True)
    v.add_argument("--generation", default="v1")
    v.add_argument("--show", type=int, default=10)
    v.set_defaults(func=cmd_verify)
    r = sub.add_parser("restore", help="copy one sealed piece to a new file, verified")
    r.add_argument("--root", required=True)
    r.add_argument("--generation", default="v1")
    r.add_argument("--source", required=True, help="the `source` path recorded in the index row")
    r.add_argument("--sha256", help="require this digest (disambiguates repeated sources)")
    r.add_argument("--to", required=True, help="destination; must not exist")
    r.set_defaults(func=cmd_restore)
    args = parser.parse_args(argv)
    try:
        args.func(args)
    except c.ToolError as error:
        c.die(str(error))


if __name__ == "__main__":
    main()
