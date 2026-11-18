#!/usr/bin/env python3
"""Run or read a ``go test -json`` stream and report it the way batch reports quote it.

  suite_stats.py run  --label full --out-dir DIR [--repo DIR] [--env K=V ...] [-- <command ...>]
  suite_stats.py read --log FILE [--exit N] [--out FILE]

``run`` without a command executes the one frozen full-repo command

    go test -json ./... -count=1 -timeout=30m

from the repository root, stdin closed, stdout+stderr into
``<out-dir>/<label>.log`` (exclusive create), and writes ``<label>-intent.json``
before the process starts, ``<label>-receipt.json`` (formal exit code or
signal, source digest before/after, log SHA-256) and ``<label>-stats.json``
after it ends. The same entry is used for targeted / adjacent / race /
whole-original checks by passing a command after ``--``; ``--raw`` skips the
JSON statistics for non-``go test -json`` commands such as ``make``.

Counting rule (identical to the sealed HMC370 ``full_stats``):

* an event is keyed by ``(Package, Test)``; ``start``/``run`` opens the key,
  ``pass``/``fail``/``skip`` closes it;
* ``package_*``: Test absent; ``top_*``: Test without ``/``; ``sub_*``: with ``/``;
  ``package_skip`` is "no test files";
* ``duplicates``: a key closed more than once; ``unfinished``: a key opened and
  never closed (non-terminal); ``non_json``: lines that are not a JSON object;
* ``failed``: every key whose terminal action is ``fail`` (subtests, their
  parents and the package are all listed - nothing is deduplicated).

The process exit code is recorded separately and is never inferred from the
stream. A FAIL is reported as it is; the tool has no retry.
"""
import argparse
import json
import os
import subprocess
import sys

sys.dont_write_bytecode = True  # before the sibling import: no __pycache__ in the repo
import hmc_common as c  # noqa: E402

FULL_COMMAND = ["go", "test", "-json", "./...", "-count=1", "-timeout=30m"]
SCHEMA = "hmc.suite_stats.v2"


def stats_from_lines(lines):
    """Pure: an iterable of byte lines -> the statistics document."""
    counts, active, terminal, duplicates, other_actions = {}, {}, {}, [], {}
    non_json = events = 0
    for raw in lines:
        try:
            event = json.loads(raw.decode("utf-8"))
            if not isinstance(event, dict):
                raise ValueError("not an object")
        except (ValueError, UnicodeDecodeError):
            non_json += 1
            continue
        events += 1
        key = (event.get("Package"), event.get("Test"))
        action = event.get("Action")
        if action in ("start", "run"):
            active[key] = True
        if action not in ("pass", "fail", "skip"):
            if action not in ("start", "run", "output", "pause", "cont"):
                other_actions[str(action)] = other_actions.get(str(action), 0) + 1
            continue
        if key in terminal:
            duplicates.append(list(key))
        terminal[key] = action
        active.pop(key, None)
        kind = "package" if key[1] is None else ("sub" if "/" in key[1] else "top")
        name = kind + "_" + action
        counts[name] = counts.get(name, 0) + 1
    return {"counts": counts, "non_json": non_json, "duplicates": duplicates,
            "unfinished": [list(key) for key in active],
            "failed": [list(key) for key, action in terminal.items() if action == "fail"],
            "events": events, "other_actions": other_actions}


def stats_from_log(path):
    with open(path, "rb") as handle:
        lines = handle.read().split(b"\n")
    if lines and lines[-1] == b"":
        lines.pop()  # the piece after the final newline is not a line
    result = stats_from_lines(lines)
    result["sha256"] = c.sha256_file(path)
    return result


def report_lines(stats):
    n = lambda key: stats["counts"].get(key, 0)
    exit_text = "exit %s" % stats["exit"] if stats.get("signal") is None else "signal %s" % stats["signal"]
    packages = n("package_pass") + n("package_fail")
    lines = [
        "%s，%s 包通过、%s 无测试、%s 失败；%s 顶层、%s 子通过（顶层/子失败 %s/%s，跳过 %s/%s）；"
        "重复 %s、未终态 %s、非 JSON %s 行" % (
            exit_text, n("package_pass"), n("package_skip"), n("package_fail"),
            c.thousands(n("top_pass")), c.thousands(n("sub_pass")), n("top_fail"), n("sub_fail"),
            n("top_skip"), n("sub_skip"), len(stats["duplicates"]), len(stats["unfinished"]), stats["non_json"]),
        "%s 包 %s/%s" % (packages, n("top_pass"), n("sub_pass")),
    ]
    return lines


def emit(stats, out=None):
    stats = dict(stats, schema=SCHEMA, report=report_lines(stats))
    if out:
        c.save_json(out, stats)
    brief = {key: stats.get(key) for key in ("counts", "non_json", "exit", "signal", "sha256")}
    brief.update(duplicates=len(stats["duplicates"]), unfinished=len(stats["unfinished"]),
                 failed=len(stats["failed"]))
    print(json.dumps(brief))
    for line in stats["report"]:
        print(line)
    for package, test in stats["failed"]:
        print("FAIL %s %s" % (package, test if test is not None else "(package)"))
    for package, test in stats["duplicates"]:
        print("DUPLICATE %s %s" % (package, test))
    for package, test in stats["unfinished"]:
        print("NON-TERMINAL %s %s" % (package, test if test is not None else "(package)"))
    return stats


def cmd_read(args):
    stats = stats_from_log(args.log)
    stats["exit"], stats["signal"] = args.exit, None
    emit(stats, args.out)


def cmd_run(args):
    repo = c.resolve_repo(args.repo)
    if not args.label or not all(ch.isalnum() or ch in "-_." for ch in args.label):
        raise c.ToolError("label must be [A-Za-z0-9._-]+")
    out_dir = os.path.abspath(args.out_dir)
    if os.path.commonpath([os.path.realpath(out_dir), repo]) == repo:
        raise c.ToolError("--out-dir must be outside the repository so the run cannot change its own inputs")
    os.makedirs(out_dir, mode=0o700, exist_ok=True)
    prefix = os.path.join(out_dir, args.label)
    command = args.cmd[1:] if args.cmd and args.cmd[0] == "--" else (args.cmd or [])
    command = command or list(FULL_COMMAND)
    env = c.clean_env()
    overrides = {}
    for item in args.env or []:
        key, sep, value = item.partition("=")
        if not sep or not key:
            raise c.ToolError("--env wants KEY=VALUE: %r" % item)
        overrides[key] = value
    env.update(overrides)

    before, _ = c.source_inputs(repo)
    intent = {"schema": "hmc.check.v2", "label": args.label, "argv": command, "cwd": repo,
              "env_overrides": overrides, "started_at": c.now_utc(), "log": prefix + ".log",
              "head": c.git(repo, "rev-parse", "HEAD").strip(), "source_sha256": c.source_digest(before),
              "freeze_sha256": c.sha256_file(args.freeze) if args.freeze else None}
    c.save_json(prefix + "-intent.json", intent)
    log_fd = os.open(intent["log"], os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        child = subprocess.Popen(command, cwd=repo, env=env, stdin=subprocess.DEVNULL,
                                 stdout=log_fd, stderr=subprocess.STDOUT)
    finally:
        os.close(log_fd)
    code = child.wait()
    after, _ = c.source_inputs(repo)
    receipt = dict(intent, pid=child.pid, finished_at=c.now_utc(),
                   exit_code=code if code >= 0 else None, signal=-code if code < 0 else None,
                   os_terminal_observed=True, source_unchanged=before == after,
                   log_sha256=c.sha256_file(intent["log"]))
    c.save_json(prefix + "-receipt.json", receipt)
    if not args.raw:
        stats = stats_from_log(intent["log"])
        stats["exit"], stats["signal"] = receipt["exit_code"], receipt["signal"]
        emit(stats, prefix + "-stats.json")
    else:
        print(json.dumps({key: receipt[key] for key in ("label", "exit_code", "signal", "source_unchanged", "log_sha256")}))
    sys.exit(0 if code == 0 and receipt["source_unchanged"] else 1)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command", required=True)
    r = sub.add_parser("run", help="run a command once with intent/receipt/log and compute statistics")
    r.add_argument("--label", required=True, help="file stem, e.g. full, focused, race, whole, build")
    r.add_argument("--out-dir", required=True, help="evidence directory outside the repository")
    r.add_argument("--repo")
    r.add_argument("--env", action="append", help="KEY=VALUE added after the operator environment is cleaned")
    r.add_argument("--freeze", help="frozen manifest whose SHA-256 is recorded in the receipt")
    r.add_argument("--raw", action="store_true", help="receipt only; do not parse the log as go test -json")
    r.add_argument("cmd", nargs=argparse.REMAINDER, help="-- command ... (default: the frozen full-repo command)")
    r.set_defaults(func=cmd_run)
    d = sub.add_parser("read", help="statistics of an existing go test -json log")
    d.add_argument("--log", required=True)
    d.add_argument("--exit", type=int, help="formal exit code of the process, if known")
    d.add_argument("--out", help="write the statistics JSON here (exclusive create)")
    d.set_defaults(func=cmd_read)
    args = parser.parse_args(argv)
    try:
        args.func(args)
    except c.ToolError as error:
        c.die(str(error))


if __name__ == "__main__":
    main()
