# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T22:34:47Z
- sweep_start_ts: 20260805-153445
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_system_inventory | PASS | eval/results/operation_system_inventory-20260805-153447 | log_regex,answer_regex | none | 37s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | operation_rounds=1, commands=4, all_exit=0 | pass | Requested inventory is correct and concise: `sw_vers` grounds macOS 26.5.2/25F84; three sysctl values ground 18 total/physical/logical cores; 137438953472 bytes is exactly 128 GiB; SPDisplays grounds M5 Max GPU, 40 cores, Metal 4 and the display. The answer's additional CPU model label `Apple M5 Max` is inferred from the GPU/SoC name rather than an explicit CPU-model command; keep as P2 model-embellishment telemetry, not a failure of the requested core-count result. |
| 1 | data_json_strict_ids | FAIL | eval/results/data_json_strict_ids-20260805-153447 | log_regex,answer_regex | none | 355s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | data_rounds=11,repair=6,prior_errors=8 | fail | The first custom transform already emitted the correct typed payload `{"ids":["u1","u3"]}`. A needlessly broad ledger contract then forced rule/decision/entity/contribution/reconcile ranks; the model used `operation=count` for a member-list answer, so assemble_answer projected the count group as `{"true":"2"}`. The evaluator correctly emitted typed `repair_node(action_kind=assemble_answer)`, but the direct-candidate fast path skipped the contest and published the rejected answer after repair budget exhaustion. Confirmed generic fail-open plus list/count teaching debt; no prose/keyword oracle is needed. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
