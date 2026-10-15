# Selected parallel eval sweep

- date: 2026-10-15T06:33:05Z
- sweep_start_ts: 20261014-233303
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_sample_only_completion_20261014

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_binary_sample_only | PASS | - | 119s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_sample_only_completion_20261014/trace_binary_sample_only-20261014-233305 |
| 2 | native_registration_commandless | PASS | - | 141s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_sample_only_completion_20261014/native_registration_commandless-20261014-233305 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

## Independent manual audit

Machine PASS does not close the capability. Each case ran once, concurrently; there was no third model run.

| case | manual verdict | accepted scope | remaining boundary |
|---|---|---|---|
| trace_binary_sample_only | FAIL | Full original binary prepared; exact thread/window returns 164 samples, 259 cycles weight; both 82-sample hotspots are retained | Prose falsely promotes unknown sample kind to on-CPU, mixes count/weight/time, and expands partial CPU 0 evidence to all samples; full answer not accepted |
| native_registration_commandless | PASS, ordinary delivery | Retained native tests, one-line source fix, 3 actual unittest methods; durable finish records 3 verification-result rows; summary uses the actual plan and materialized product | Read-only registration NOT_TRIGGERED; no live restored REPL run; prior HMC243 failure remains unchanged |

Overall manual acceptance: **1/2**, not 2/2. Detailed evidence and frozen-version boundaries: [manual audit](parallel_selected_summary_hmc_sample_only_completion_20261014_manual_audit.md), [central ledger §244](../docs/design/hmosperf_capability_gap_audit_20260917.md). Raw logs, original failures and payloads are retained locally under the results root; they are ignored artifacts, not claimed as pushed files.
