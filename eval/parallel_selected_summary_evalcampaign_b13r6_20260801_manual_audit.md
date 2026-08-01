# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T00:46:09Z
- sweep_start_ts: 20260731-174608
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260731-174609 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 197s | 43 | read=1,repo_map=0,list=0,trace=7,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | AL2 的中文 lower-bound 主结论已在 finalizer prompt，但模型仍写“只有 1.409ms/只有 1 个产生等待”，并把 70.338ms 其余 sleep 全归给 fscache。系统 leading authority 正确、投影完整；判为稳定模型偏差，不再加词面硬约束。 |
| 2 | real_trace_h2_dstate_dma_fence_triform | PASS | eval/results/real_trace_h2_dstate_dma_fence_triform-20260731-174609 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 225s | 48 | read=7,repo_map=0,list=0,trace=12,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=1 | fail | 主问题仍给出正确 11 次/36.757ms/caller，但同时新增“5 段合计 43.541ms”，又无 typed relation 地把差值归因于跨窗；与系统主值冲突。r5 曾主问题通过，说明正文有明显模型波动；AK3 的 aggregate-group 显示债仍开放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case audit

1. AL2 wiring and language selection are present in the raw finalizer prompts.
   The remaining contradictions are not missing typed data or a dead code path.
2. System-owned leading authority remains correct in both cases, and explicit
   window causal projection/root rank/wakeup chain/critical blocking/eliminable
   amount all remain present.
3. H2 varied from human pass in r5 to fail in r6 while code and typed principal
   values were unchanged. Further prompt hardening on these exact phrases would
   be case/model overfit. Leave the prose residual as model variance.
4. Keep AK3 filed as a generalized typed renderer task: per-CPU aggregate
   groups must not look like occurrences. Do not alter the legacy oracle in
   this batch.
