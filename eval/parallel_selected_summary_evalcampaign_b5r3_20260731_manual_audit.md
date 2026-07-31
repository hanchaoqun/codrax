# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T14:11:28Z
- sweep_start_ts: 20260731-071128
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_e2_cross_trace_asymmetry | PASS | eval/results/real_trace_e2_cross_trace_asymmetry-20260731-071128 | log_regex,answer_regex,answer_contains | none | 193s | 38 | read=2,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | invalid | 本轮 runner 快照的是旧 `./codrax`：binary revision=`11b8e2284`，而计划验证的 HEAD=`4c740bc31`。结果只能作为旧基线，不能用于判定批 G。 |
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260731-071128 | typed_inventory_rowset,dimension_substring,answer_contains | none | 370s | 26 | read=1,repo_map=3,list=0,trace=0,source_lens=3 | midloop=5,inv=2/0,fin_reject=2,unavail=0,prune=0 | invalid | 与上一项相同：旧二进制使本轮不是当前代码回放；自动 FAIL 不作为批 G 验收证据。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit validity

`parallel_selected.sh` 当时只复制工作区已有二进制，不校验它是否由当前 HEAD 构建。人工检查发现该二进制构建于 `main@11b8e2284`，而批 G 已在 `main@4c740bc31`。因此本轮两项统一标为 `invalid`；后续 `20260731-072430` 是显式执行 `make` 后的第一轮当前代码证据。
