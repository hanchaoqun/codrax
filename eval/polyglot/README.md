# Polyglot benchmark drivers

Drivers for running codrax write-mode against [Aider's polyglot
benchmark](https://github.com/Aider-AI/polyglot-benchmark) (sourced
from Exercism language tracks: cpp / go / java / javascript / python /
rust). Used to validate the `--mode=plan` + `--mode=apply` end-to-end
pipeline on real exercises with concrete pass/fail signals.

## Setup

The benchmark dataset is NOT vendored in this repo (large, has its
own license). Clone it once:

```bash
mkdir -p /tmp/codrax-eval
git clone https://github.com/Aider-AI/polyglot-benchmark.git /tmp/codrax-eval/polyglot-benchmark
mkdir -p /tmp/codrax-eval/runs
```

The scripts hardcode `/tmp/codrax-eval/{polyglot-benchmark,runs}`.
Symlink `eval/polyglot/run-*.sh` into `/tmp/codrax-eval/` (or run them
from there) so relative paths resolve:

```bash
cp eval/polyglot/run-*.sh /tmp/codrax-eval/
```

## Run a single task

```bash
cd /tmp/codrax-eval
./run-task.sh /tmp/codrax-eval/polyglot-benchmark/python/exercises/practice/forth forth-py 4
#                                              ^ exercise dir              ^name  ^retry budget
```

Output:
- `runs/<task>-r<retry>.plan.json` — the emitted ChangePlan
- `runs/<task>-r<retry>.plan.out` / `.apply.out` — codrax stdout per stage
- `runs/<plan-id>.report.json` — verify-stage ChangeReport (one per
  successful retry attempt; the final attempt's plan-id is what
  `run-task.sh` uses to extract the verdict)
- `runs/<task>-r<retry>` — the worktree (preserved on success per
  `pipeline_keep_worktree_on_success: true`)

Verdict line printed to stdout:
- `PASS  (tests=N/N plan=Xs apply=Ys)` — `r.passed=true` in the final report
- `FAIL  (tests=k/N plan=Xs apply=Ys)` — `r.passed=false`
- `SKIPPED  (...)` — fixture is design-a-test-suite type (Exercism's
  deprecated `counter` exercise); REQUEST template would be inverted
- `UNKNOWN  (...)` — neither report.json nor stderr regex matched
- `PLAN_FAIL  (...)` — plan stage produced no plan.json (LLM never
  called emit_change_plan)

## Run a batch (parallel)

```bash
cd /tmp/codrax-eval
./run-batch-l.sh
```

Launches every named task with `&` and `wait`s — **18 codrax processes
+ 18 worktrees + 18 test runners**. On a low-RAM host this is very
heavy; with the codrax write-mode supervisor caps in place
(verify_mem_limit_mb default 2 GiB, verify_cpu_limit_seconds default
600), each task is bounded but the aggregate can still be uncomfortable
on hosts with < 8 GiB. Prefer serial when the host has < 8 GiB:

```bash
for spec in $(grep -oE '\[[a-z0-9-]+\]=' run-batch-l.sh | tr -d '[]='); do
  src=$(grep -E "^[[:space:]]+\[$spec\]=" run-batch-l.sh | sed 's/.*"\(.*\)"$/\1/')
  ./run-task.sh "$src" "$spec" 4 > runs/$spec-r4.driver.log 2>&1
done
```

## Verdict-extraction quirks the driver handles

- **retry-aware report lookup** — `--mode=apply` with retries writes a
  fresh `<plan-id>.report.json` per retry attempt; the original
  `plan.json` from the plan-stage names the FIRST attempt's plan-id
  only. `run-task.sh` extracts the LAST plan-id mentioned in the
  apply.out tail (the final report) instead. Without this the driver
  would FAIL retries that ultimately PASSed.
- **kind backfill** — codrax emits `TestResult.Kind = "unit"`; older
  driver versions counted only `kind == "test"` and showed `tests=0/0`
  on every successful run.
- **design-test-suite exception** — Exercism's deprecated `counter`
  exercise asks the learner to WRITE tests, not implement code.
  `run-task.sh` detects the fixture type via `instructions.md` regex
  and emits SKIPPED rather than dragging the LLM into an inverted
  REQUEST.

## Scope

Drivers are scoped to single-shot eval of a single exercise repo OR a
fixed batch list. They do not spider the polyglot tree, do not parallel-
shard, and do not aggregate verdicts across runs (post-process the
`runs/*.driver.log` files for tallies). Two `resummarize*.sh` scripts
in `/tmp/codrax-eval/` (intentionally not vendored — they hardcode a
specific Batch L task list and are throwaway analysis tools) can serve
as templates if you need to write your own aggregator.
