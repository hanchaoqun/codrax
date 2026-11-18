#!/usr/bin/env python3
"""Launch exactly two eval cases in parallel, once each, behind one start barrier.

  launch.py run --root <batch-root> --case A=<case-file> --case B=<case-file> \\
                --bin <absolute frozen codrax binary> \\
                --providers-args '--providers /abs/providers.yaml' \\
                [--env A:KEY=VALUE] [--env KEY=VALUE] [--freeze manifest.json] \\
                [--timeout 1800] [--barrier-timeout 90] [--repo DIR]

What it does, and nothing more:

1. creates ``<root>/{A,B,barrier}`` exclusively - an existing ``A``, ``B``,
   ``barrier`` or ``launch-*`` entry is a refusal, so a batch root can be
   launched once and there is no code path that starts a third run;
2. starts ``bash eval/run.sh <case> 1`` for A and B from the repository root
   with a cleaned operator environment plus ``CODRAX_BIN=<barrier wrapper>``,
   ``CODRAX_PROVIDER_ARGS_RAW``, ``EVAL_RESULTS_ROOT=<root>/A|B`` and the
   per-case ``--env`` values (e.g. ``A:CODRAX_HMC365_HONEYBIRD=/abs/trace``);
3. the wrapper is the product entry run.sh executes. It records its exact
   argv in ``barrier/<side>.json`` and waits. Only when BOTH sides have
   arrived does the launcher write ``barrier/release.json``; each wrapper
   then ``exec``s the real binary. If one side never arrives, nothing is
   released and no model process starts on either side;
4. for a read case (no ``--mode``/``--write-*`` argument) the wrapper appends
   ``--root-causes-out <result-dir>/run-1.root-causes.json`` - the same
   guaranteed-delivery root-cause side file the HMC370-375 wrapper requested;
5. enforces an outer wall-clock limit per case (process-group SIGTERM, then
   SIGKILL after 10 s) and writes ``launch-<side>-v1-receipt.json`` with the
   formal exit code or signal, never a verdict of its own.

The launcher never reads the providers file, never edits a case, a question
or an oracle, and never looks at an answer. Simultaneous start is NOT
evidence of parallel model traffic; run overlap.py for that.
"""
import argparse
import json
import os
import shlex
import signal
import subprocess
import sys
import time

sys.dont_write_bytecode = True  # before the sibling import: no __pycache__ in the repo
import hmc_common as c  # noqa: E402

RUNS_PER_CASE = "1"  # the only value this tool can pass to eval/run.sh
BARRIER_EXIT = 97    # wrapper: the two entries did not meet
SHAPE_EXIT = 98      # wrapper: run.sh called the product in an unexpected shape


# --------------------------------------------------------------------------
# wrapper side (executed by eval/run.sh as $CODRAX_BIN)
# --------------------------------------------------------------------------

def option_values(argv, name):
    return [argv[i + 1] for i, arg in enumerate(argv[:-1]) if arg == name]


def worker(raw):
    try:
        split = raw.index("--")
        side, root, real_bin, freeze_sha, timeout = raw[:split]
    except ValueError:
        c.die("worker SIDE ROOT REAL_BIN FREEZE_SHA256 BARRIER_SECONDS -- <product argv>", SHAPE_EXIT)
    argv = raw[split + 1:]
    freeze_sha = None if freeze_sha == "-" else freeze_sha
    barrier = os.path.join(root, "barrier")
    if "--request" not in argv:
        os.execv(real_bin, [real_bin] + argv)  # e.g. `version`; not a case run
    logs = option_values(argv, "--log-dir")
    if argv.count("--request") != 1 or len(logs) != 1:
        c.die("unexpected product argv shape (--request/--log-dir)", SHAPE_EXIT)
    log_dir = os.path.realpath(logs[0])
    run_dir = os.path.dirname(log_dir)
    side_root = os.path.realpath(os.path.join(root, side))
    if os.path.dirname(run_dir) != side_root or os.path.basename(log_dir) != "run-1.logs":
        c.die("product entry escaped %s or is not run 1: %s" % (side_root, log_dir), SHAPE_EXIT)
    read_lane = not any(arg.startswith(("--mode", "--write-")) for arg in argv)
    if read_lane and not any(arg == "--root-causes-out" or arg.startswith("--root-causes-out=") for arg in argv):
        argv = argv + ["--root-causes-out", os.path.join(run_dir, "run-1.root-causes.json")]
    record = {"side": side, "pid": os.getpid(), "argv": [real_bin] + argv, "cwd": os.getcwd(),
              "run_dir": run_dir, "log_dir": log_dir, "read_lane": read_lane, "at": c.now_utc(),
              "freeze_sha256": freeze_sha, "entry": 1}
    first = os.path.join(barrier, side + ".json")
    release = os.path.join(barrier, "release.json")
    try:
        c.save_json(first, record)
    except c.ToolError:
        # A later product entry of the same single run (plan -> apply lanes).
        # The two cases already met; it is recorded and not held again.
        if not os.path.isfile(release):
            c.die("first entry of side %s was never released" % side, BARRIER_EXIT)
        number = 2
        while True:
            record["entry"] = number
            try:
                c.save_json(os.path.join(barrier, "%s.entry-%d.json" % (side, number)), record)
                break
            except c.ToolError:
                number += 1
        os.execv(real_bin, [real_bin] + argv)
    deadline = time.monotonic() + float(timeout)
    aborted = os.path.join(barrier, "aborted.json")
    while not os.path.isfile(release):
        if os.path.isfile(aborted) or time.monotonic() > deadline:
            c.die("two independent CLI entries did not meet; no model process started", BARRIER_EXIT)
        time.sleep(0.005)  # start skew between the two sides is bounded by this poll
    if c.load_json(release).get("freeze_sha256") != freeze_sha:
        c.die("release does not belong to this frozen generation", BARRIER_EXIT)
    c.save_json(os.path.join(barrier, side + ".go.json"), {"side": side, "pid": os.getpid(), "at": c.now_utc()})
    os.execv(real_bin, [real_bin] + argv)


# --------------------------------------------------------------------------
# launcher side
# --------------------------------------------------------------------------

def parse_cases(items):
    cases = {}
    for item in items or []:
        label, sep, path = item.partition("=")
        if not sep or label in cases:
            raise c.ToolError("--case wants A=<file> and B=<file>: %r" % item)
        cases[label] = os.path.realpath(path)
    if sorted(cases) != ["A", "B"]:
        raise c.ToolError("exactly two cases labelled A and B are required (got %s)" % sorted(cases))
    for label, path in cases.items():
        if not c.regular_file(path):
            raise c.ToolError("case %s is not a regular file: %s" % (label, path))
    return cases


def parse_env(items):
    env = {"A": {}, "B": {}}
    for item in items or []:
        key, sep, value = item.partition("=")
        if not sep or not key:
            raise c.ToolError("--env wants [A:|B:]KEY=VALUE: %r" % item)
        sides = ("A", "B")
        if key[:2] in ("A:", "B:"):
            sides, key = (key[0],), key[2:]
        if key in ("CODRAX_BIN", "CODRAX_PROVIDER_ARGS_RAW", "EVAL_RESULTS_ROOT"):
            raise c.ToolError("%s is owned by the launcher" % key)
        for side in sides:
            env[side][key] = value
    return env


def kill_group(pid, sig):
    try:
        os.killpg(pid, sig)
    except (ProcessLookupError, PermissionError):
        pass


def single_run_dir(side_root):
    entries = sorted(name for name in os.listdir(side_root) if os.path.isdir(os.path.join(side_root, name)))
    return (os.path.join(side_root, entries[0]) if len(entries) == 1 else None), entries


def frozen(manifest, repo):
    if not manifest:
        return None, None
    import freeze
    _, problems = freeze.verify(manifest, repo)
    return not problems, (problems[0] if problems else None)


def cmd_run(args):
    repo = c.resolve_repo(args.repo)
    runner = os.path.join(repo, "eval", "run.sh")
    if not c.regular_file(runner):
        raise c.ToolError("eval/run.sh not found under %s" % repo)
    cases = parse_cases(args.case)
    extra = parse_env(args.env)
    real_bin = args.bin
    if not os.path.isabs(real_bin) or os.path.islink(real_bin) or not c.regular_file(real_bin) \
            or not os.access(real_bin, os.X_OK):
        # run.sh silently falls back to ./codrax + `make build` when the snapshot is unusable.
        raise c.ToolError("--bin must be an absolute, regular, executable file: %s" % real_bin)
    if not args.providers_args.strip():
        raise c.ToolError("--providers-args must not be empty (run.sh would pick its own default)")
    freeze_sha = c.sha256_file(args.freeze) if args.freeze else None
    if args.freeze and c.load_json(args.freeze).get("external", {}).get(os.path.realpath(real_bin)) \
            != c.sha256_file(real_bin):
        raise c.ToolError("--bin is not a frozen external input of %s (stale or unfrozen binary)" % args.freeze)

    root = os.path.abspath(args.root)
    os.makedirs(root, mode=0o700, exist_ok=True)
    root = os.path.realpath(root)
    stale = [name for name in os.listdir(root) if name in ("A", "B", "barrier") or name.startswith("launch-")]
    if stale:
        raise c.ToolError("this batch root was already launched (%s); a case is run once, "
                          "never again" % ", ".join(sorted(stale)))
    ok, problem = frozen(args.freeze, repo)
    if ok is False:
        raise c.ToolError("frozen inputs changed before launch: %s" % problem)
    for name in ("A", "B", "barrier"):
        os.mkdir(os.path.join(root, name), 0o700)

    me = os.path.realpath(__file__)
    jobs = {}
    for side in ("A", "B"):
        wrapper = os.path.join(root, "barrier", "wrapper-" + side)
        line = "exec %s -B %s worker %s -- \"$@\"\n" % (
            shlex.quote(sys.executable), shlex.quote(me),
            " ".join(shlex.quote(str(x)) for x in (side, root, real_bin, freeze_sha or "-", args.barrier_timeout)))
        c.write_exclusive(wrapper, ("#!/bin/bash\n" + line).encode("utf-8"), 0o700)
        argv = ["bash", "eval/run.sh", cases[side], RUNS_PER_CASE]
        owned = {"CODRAX_BIN": wrapper, "CODRAX_PROVIDER_ARGS_RAW": args.providers_args,
                 "EVAL_RESULTS_ROOT": os.path.join(root, side)}
        jobs[side] = {"label": "live-" + side, "side": side, "argv": argv, "case": cases[side],
                      "case_sha256": c.sha256_file(cases[side]), "cwd": repo,
                      "env": dict(extra[side], **owned), "wrapper_sha256": c.sha256_file(wrapper),
                      "log": os.path.join(root, "launch-%s-v1.log" % side),
                      "logger_utc_offset_seconds": c.local_utc_offset_seconds(),
                      "source_verified_before": ok, "freeze_sha256": freeze_sha,
                      "outer_timeout_seconds": args.timeout}
    intent = {"schema": "hmc.launch.v2", "at": c.now_utc(), "repo": repo, "root": root,
              "head": c.git(repo, "rev-parse", "HEAD").strip(),
              "bin": {"path": real_bin, "sha256": c.sha256_file(real_bin)},
              "runs_per_case": int(RUNS_PER_CASE), "barrier_timeout_seconds": args.barrier_timeout,
              "tool": {"launch.py": c.sha256_file(me), "hmc_common.py": c.sha256_file(os.path.realpath(c.__file__))},
              "jobs": jobs}
    c.save_json(os.path.join(root, "launch-intent-v1.json"), intent)

    children = {}
    for side, job in jobs.items():
        env = c.clean_env()
        env.update(job["env"])
        fd = os.open(job["log"], os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        try:
            job["started_at"] = c.now_utc()
            children[side] = subprocess.Popen(job["argv"], cwd=repo, env=env, stdin=subprocess.DEVNULL,
                                              stdout=fd, stderr=subprocess.STDOUT, start_new_session=True)
        finally:
            os.close(fd)
        job["started_monotonic"] = time.monotonic()

    def stop_all(signum, _frame):
        for child in children.values():
            kill_group(child.pid, signal.SIGTERM)
        sys.exit(128 + signum)
    signal.signal(signal.SIGINT, stop_all)
    signal.signal(signal.SIGTERM, stop_all)
    print(json.dumps({"spawned": {side: child.pid for side, child in children.items()}}), flush=True)

    barrier = os.path.join(root, "barrier")
    arrived = lambda: all(os.path.isfile(os.path.join(barrier, side + ".json")) for side in ("A", "B"))
    deadline = time.monotonic() + args.barrier_timeout
    while not arrived() and time.monotonic() < deadline and all(ch.poll() is None for ch in children.values()):
        time.sleep(0.02)
    released = arrived()
    if released:
        body = {"at": c.now_utc(), "freeze_sha256": freeze_sha,
                "entries": {side: c.sha256_file(os.path.join(barrier, side + ".json")) for side in ("A", "B")}}
        c.save_json(os.path.join(barrier, "release.json.partial"), body)
        os.rename(os.path.join(barrier, "release.json.partial"), os.path.join(barrier, "release.json"))
    else:
        c.save_json(os.path.join(barrier, "aborted.json"),
                    {"at": c.now_utc(), "reason": "both product entries did not arrive",
                     "arrived": [s for s in ("A", "B") if os.path.isfile(os.path.join(barrier, s + ".json"))]})
    print(json.dumps({"barrier_released": released}), flush=True)

    receipts, pending = {}, dict(children)
    killed_at = {}
    while pending:
        for side, child in list(pending.items()):
            job = jobs[side]
            code = child.poll()
            elapsed = time.monotonic() - job["started_monotonic"]
            if code is None:
                if elapsed > args.timeout and side not in killed_at:
                    kill_group(child.pid, signal.SIGTERM)
                    killed_at[side] = time.monotonic()
                elif side in killed_at and time.monotonic() - killed_at[side] > 10:
                    kill_group(child.pid, signal.SIGKILL)
                continue
            del pending[side]
            kill_group(child.pid, signal.SIGKILL)  # no grandchild may outlive its case
            after, problem = frozen(args.freeze, repo)
            run_dir, entries = single_run_dir(os.path.join(root, side))
            verdict = None
            if run_dir and os.path.isfile(os.path.join(run_dir, "run-1.verdict")):
                with open(os.path.join(run_dir, "run-1.verdict"), "r", encoding="utf-8", errors="replace") as handle:
                    verdict = handle.readline().rstrip("\n")
            receipt = {key: value for key, value in job.items() if key != "started_monotonic"}
            receipt.update(pid=child.pid, finished_at=c.now_utc(),
                           exit_code=code if code >= 0 else None, signal=-code if code < 0 else None,
                           os_terminal_observed=True, outer_timeout_hit=side in killed_at,
                           barrier_released=released, source_verified_after=after, verification_error=problem,
                           run_dir=run_dir, result_directories=entries, machine_verdict_line=verdict,
                           log_sha256=c.sha256_file(job["log"]))
            c.save_json(os.path.join(root, "launch-%s-v1-receipt.json" % side), receipt)
            receipts[side] = receipt
            print(json.dumps({key: receipt[key] for key in ("label", "exit_code", "signal", "outer_timeout_hit",
                                                            "run_dir", "machine_verdict_line")}), flush=True)
        time.sleep(0.05)

    summary = {"schema": "hmc.launch_receipt.v2", "at": c.now_utc(), "root": root,
               "intent_sha256": c.sha256_file(os.path.join(root, "launch-intent-v1.json")),
               "barrier_released": released, "runs_per_case": int(RUNS_PER_CASE),
               "product_entries": {side: len([n for n in os.listdir(barrier)
                                              if n == side + ".json" or n.startswith(side + ".entry-")])
                                   for side in ("A", "B")},
               "receipts": {side: c.sha256_file(os.path.join(root, "launch-%s-v1-receipt.json" % side))
                            for side in ("A", "B")}}
    c.save_json(os.path.join(root, "launch-receipt-v1.json"), summary)
    clean = released and all(r["exit_code"] == 0 and not r["outer_timeout_hit"] and r["run_dir"]
                             and r["source_verified_after"] is not False for r in receipts.values())
    sys.exit(0 if clean else 1)


def main(argv=None):
    argv = sys.argv[1:] if argv is None else argv
    if argv[:1] == ["worker"]:
        worker(argv[1:])
        return
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command", required=True)
    r = sub.add_parser("run", help="start A and B once each behind the barrier and wait for both")
    r.add_argument("--root", required=True, help="new batch root, e.g. eval/results/hmc_<topic>_<date>_v1")
    r.add_argument("--case", action="append", required=True, help="A=<case-file> and B=<case-file>")
    r.add_argument("--bin", required=True, help="absolute path of the frozen codrax binary copy")
    r.add_argument("--providers-args", required=True,
                   help="value of CODRAX_PROVIDER_ARGS_RAW, e.g. '--providers /abs/providers.yaml'")
    r.add_argument("--env", action="append",
                   help="KEY=VALUE for both cases or A:KEY=VALUE / B:KEY=VALUE for one (repeatable)")
    r.add_argument("--freeze", help="frozen manifest verified before launch and after each case")
    r.add_argument("--timeout", type=float, default=1800.0, help="outer seconds per case from spawn (default 1800)")
    r.add_argument("--barrier-timeout", type=float, default=90.0,
                   help="seconds both product entries have to arrive (default 90)")
    r.add_argument("--repo")
    args = parser.parse_args(argv)
    try:
        cmd_run(args)
    except c.ToolError as error:
        c.die(str(error))


if __name__ == "__main__":
    main()
