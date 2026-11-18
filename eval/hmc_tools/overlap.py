#!/usr/bin/env python3
"""Actual model-request overlap between the two cases of a batch.

  overlap.py --root <batch-root> [--repo DIR] [--out FILE]
  overlap.py --a-log FILE [--a-log FILE ...] --a-started-at ISO --a-finished-at ISO \\
             --b-log FILE [--b-log FILE ...] --b-started-at ISO --b-finished-at ISO \\
             [--utc-offset-seconds N] [--repo DIR] [--out FILE]

"A13/B24, 25 对重叠、最长 14.948 秒，歧义 0" is produced here and nowhere else.
All parsing and all interval arithmetic stay in the repository's single
source of truth, ``eval/request_intervals.py`` (``read_nonsecret``,
``parse_log``, ``overlaps``); this file only feeds it the two cases and
counts the result:

* completed  uniquely proven request/response intervals per case;
* ambiguous  busy groups that request_intervals.py refused to pair - they are
             reported and excluded, never guessed;
* overlaps   every (A interval, B interval) pair with a positive intersection
             in integer microseconds; ``max_seconds`` is the longest.

With ``--root`` the process lifetime and logger clock offset come from
``launch-A-v1-receipt.json`` / ``launch-B-v1-receipt.json`` written by
launch.py, and the logs are ``<run_dir>/run-1.logs/codrax-*.log``. Starting two
processes together proves nothing: only this file's ``actual_overlap_proven``
may be quoted as parallel model traffic.
"""
import argparse
import glob
import json
import os
import sys

sys.dont_write_bytecode = True  # before the sibling import: no __pycache__ in the repo
import hmc_common as c  # noqa: E402

SCHEMA = "hmc.actual_request_overlap.v2"


def load_parser(repo):
    path = os.path.join(repo, "eval")
    if not os.path.isfile(os.path.join(path, "request_intervals.py")):
        raise c.ToolError("eval/request_intervals.py not found under %s" % repo)
    sys.dont_write_bytecode = True
    sys.path.insert(0, path)
    try:
        import request_intervals
    finally:
        sys.path.pop(0)
    return request_intervals, c.sha256_file(os.path.join(path, "request_intervals.py"))


def side_intervals(parser, side, logs, started_at, finished_at, offset):
    completed, ambiguous, diagnostics, sources = [], [], [], []
    if not logs:
        raise c.ToolError("case %s has no product log" % side)
    for log in logs:
        path, raw = parser.read_nonsecret(log)
        result = parser.parse_log(raw, started_at, finished_at, log=str(path), utc_offset_seconds=offset)
        if result["schema"] != "codrax.request_intervals.v1" or result["log_sha256"] != c.sha256_file(str(path)):
            raise c.ToolError("parser schema or log bytes changed: %s" % log)
        completed += result["completed"]
        ambiguous += [dict(group, side=side) for group in result["ambiguous"]]
        diagnostics += [dict(item, side=side) for item in result["diagnostics"]]
        sources.append({"path": str(path), "sha256": result["log_sha256"], "counts": result["counts"],
                        "unresolved_request_count": result["unresolved_request_count"]})
    return completed, ambiguous, diagnostics, sources


def from_root(root):
    sides = {}
    for side in ("A", "B"):
        receipt_path = os.path.join(root, "launch-%s-v1-receipt.json" % side)
        receipt = c.load_json(receipt_path)
        if not (receipt.get("os_terminal_observed") is True and receipt.get("finished_at")
                and (receipt.get("exit_code") is not None or receipt.get("signal") is not None)):
            raise c.ToolError("case %s has no formal terminal receipt" % side)
        run_dir = receipt.get("run_dir")
        if not run_dir or not os.path.realpath(run_dir).startswith(os.path.realpath(os.path.join(root, side)) + os.sep):
            raise c.ToolError("case %s run directory is not under %s/%s" % (side, root, side))
        sides[side] = {"logs": sorted(glob.glob(os.path.join(run_dir, "run-1.logs", "codrax-*.log"))),
                       "started_at": receipt["started_at"], "finished_at": receipt["finished_at"],
                       "offset": receipt.get("logger_utc_offset_seconds"),
                       "receipt_sha256": c.sha256_file(receipt_path)}
    return sides


def compute(parser, parser_sha, sides):
    completed, ambiguous, diagnostics, sources = {}, [], [], {}
    for side in ("A", "B"):
        spec = sides[side]
        done, amb, diag, src = side_intervals(parser, side, spec["logs"], spec["started_at"],
                                              spec["finished_at"], spec["offset"])
        completed[side], sources[side] = done, src
        ambiguous += amb
        diagnostics += diag
    pairs = parser.overlaps(completed["A"], completed["B"])
    return {"schema": SCHEMA, "completed": completed, "ambiguous": ambiguous, "diagnostics": diagnostics,
            "actual_overlap_proven": bool(pairs), "overlaps": pairs,
            "max_seconds": max((pair["seconds"] for pair in pairs), default=None),
            "logs": sources, "process": {side: {key: sides[side].get(key) for key in
                                                ("started_at", "finished_at", "offset", "receipt_sha256")}
                                         for side in ("A", "B")},
            "parser": {"path": "eval/request_intervals.py", "sha256": parser_sha},
            "scope": "Uniquely proven logger intervals only; barrier or process overlap is not model overlap"}


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--root", help="batch root with launch-A/B-v1-receipt.json")
    ap.add_argument("--repo", help="repository whose eval/request_intervals.py is used")
    ap.add_argument("--out", help="write the full evidence JSON here (exclusive create)")
    for side in ("a", "b"):
        ap.add_argument("--%s-log" % side, action="append")
        ap.add_argument("--%s-started-at" % side)
        ap.add_argument("--%s-finished-at" % side)
    ap.add_argument("--utc-offset-seconds", type=int,
                    help="logger clock offset for naive log timestamps (manual mode)")
    args = ap.parse_args(argv)
    try:
        repo = os.path.realpath(args.repo or c.default_repo())
        parser, parser_sha = load_parser(repo)
        if args.root:
            sides = from_root(os.path.realpath(args.root))
        else:
            sides = {}
            for side in ("a", "b"):
                logs, start, finish = (getattr(args, "%s_log" % side), getattr(args, "%s_started_at" % side),
                                       getattr(args, "%s_finished_at" % side))
                if not (logs and start and finish):
                    raise c.ToolError("give --root, or --%s-log/--%s-started-at/--%s-finished-at for both cases"
                                      % (side, side, side))
                sides[side.upper()] = {"logs": logs, "started_at": start, "finished_at": finish,
                                       "offset": args.utc_offset_seconds}
        result = compute(parser, parser_sha, sides)
        if args.out:
            c.save_json(args.out, result)
    except (c.ToolError, ValueError, OSError) as error:
        c.die(str(error))
    counts = {"completed": {side: len(rows) for side, rows in result["completed"].items()},
              "ambiguous": len(result["ambiguous"]), "diagnostics": len(result["diagnostics"]),
              "overlaps": len(result["overlaps"]), "max": result["max_seconds"],
              "actual_overlap_proven": result["actual_overlap_proven"]}
    print(json.dumps(counts))
    print("实际模型请求 A%d/B%d，%d 对重叠、最长 %s 秒，歧义 %d" % (
        counts["completed"]["A"], counts["completed"]["B"], counts["overlaps"],
        "%.3f" % counts["max"] if counts["max"] is not None else "0", counts["ambiguous"]))
    sys.exit(0)


if __name__ == "__main__":
    main()
