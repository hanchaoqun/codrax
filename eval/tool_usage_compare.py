#!/usr/bin/env python3
"""Compare per-case tool-call counts between two eval result windows.

The §14.8 efficiency lens: run.sh records tool_* counters per case
(run-1.metrics.txt); this tool pairs each case's latest run in a
BASELINE time window against its latest run in a CANDIDATE window and
reports per-tool deltas — quantifying steering/teaching changes
(fewer grep/read_file round trips, repo_map adoption) without an LLM
judge. Verdicts are compared alongside so an efficiency win that
costs correctness is visible immediately.

Usage:
  python3 eval/tool_usage_compare.py \
      --results eval/results \
      --baseline-before 20260612-150000 \
      --candidate-after 20260612-180000

Windows are matched against the timestamp suffix of result dirs
(<case>-<YYYYMMDD-HHMMSS>). For each case the LATEST dir inside each
window wins.
"""

import argparse
import collections
import os
import re
import sys

DIR_RE = re.compile(r"^(?P<case>.+)-(?P<ts>\d{8}-\d{6})$")
TOOLS = [
    "tool_repo_map", "tool_grep", "tool_read_file", "tool_list_files",
    "tool_trace_query", "tool_keyword_search",
]


def load_runs(root):
    runs = {}
    for name in os.listdir(root):
        m = DIR_RE.match(name)
        if not m:
            continue
        metrics_path = os.path.join(root, name, "run-1.metrics.txt")
        verdict_path = os.path.join(root, name, "run-1.verdict")
        if not os.path.exists(metrics_path):
            continue
        metrics = {}
        with open(metrics_path) as f:
            for line in f:
                k, _, v = line.strip().partition("=")
                if k.startswith("tool_") and v.isdigit():
                    metrics[k] = int(v)
        verdict = ""
        if os.path.exists(verdict_path):
            with open(verdict_path) as f:
                verdict = f.readline().strip()
        runs.setdefault(m.group("case"), []).append(
            (m.group("ts"), metrics, verdict))
    return runs


def pick(runs, case, lo, hi):
    best = None
    for ts, metrics, verdict in runs.get(case, []):
        if lo <= ts <= hi and (best is None or ts > best[0]):
            best = (ts, metrics, verdict)
    return best


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", default="eval/results")
    ap.add_argument("--baseline-before", required=True,
                    help="baseline window = (-inf, THIS]")
    ap.add_argument("--candidate-after", required=True,
                    help="candidate window = [THIS, +inf)")
    args = ap.parse_args()

    runs = load_runs(args.results)
    paired = []
    for case in sorted(runs):
        base = pick(runs, case, "00000000-000000", args.baseline_before)
        cand = pick(runs, case, args.candidate_after, "99999999-999999")
        if base and cand:
            paired.append((case, base, cand))

    if not paired:
        print("no paired cases between the two windows", file=sys.stderr)
        sys.exit(1)

    totals = collections.defaultdict(lambda: [0, 0])
    verdict_flips = []
    print(f"paired cases: {len(paired)}\n")
    print(f"{'case':44s} {'tool':18s} {'base':>5s} {'cand':>5s} {'Δ':>5s}")
    for case, (bts, bm, bv), (cts, cm, cv) in paired:
        if bv != cv:
            verdict_flips.append((case, bv, cv))
        for tool in TOOLS:
            b, c = bm.get(tool, 0), cm.get(tool, 0)
            totals[tool][0] += b
            totals[tool][1] += c
            if b != c:
                print(f"{case:44s} {tool:18s} {b:5d} {c:5d} {c - b:+5d}")

    print("\n== aggregate (paired cases only) ==")
    for tool in TOOLS:
        b, c = totals[tool]
        if b == 0 and c == 0:
            continue
        pct = (f"{(c - b) * 100.0 / b:+.1f}%" if b else "n/a")
        print(f"{tool:18s} baseline={b:5d} candidate={c:5d} delta={c - b:+5d} ({pct})")

    print("\n== verdict flips ==")
    if not verdict_flips:
        print("none")
    for case, bv, cv in verdict_flips:
        print(f"{case}: {bv} -> {cv}")


if __name__ == "__main__":
    main()
