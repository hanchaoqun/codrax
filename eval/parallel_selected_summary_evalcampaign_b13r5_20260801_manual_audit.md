# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T00:35:47Z
- sweep_start_ts: 20260731-173546
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | PASS | eval/results/real_trace_h2_dstate_dma_fence_triform-20260731-173547 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 189s | 48 | read=5,repo_map=0,list=0,trace=3,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=2 | pass | AL1 真实生效：finalizer prompt 收到 exact 11-row recap，正文主答改为 11 次/36.757ms/dma_fence，完整 Trace 因果投影无回退。残余：无 typed relation 却猜 12-record/11-occurrence 差值来自窗边界；仍把四个 per-CPU aggregate group 描述成单次分布，记 P2。 |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260731-173547 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 193s | 47 | read=2,repo_map=0,list=0,trace=10,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=1 | fail | AL1 recap 已在 finalizer prompt 明确 lower_bound_only、>=1、>=1.409ms；模型末句也承认 coverage truncated，但首段/标题仍写“只有/唯一 1 次、总时长 1.409ms”，同答越权。显式窗投影与根因榜完整。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case audit

1. AL1 wiring is proven, not a no-op: both raw finalizer prompts contain the
   late `Runtime Trace Principal Values` section. H2 consumed its complete
   occurrence row and corrected the principal count from prior 12/8 to 11.
2. H1 is now a wording-permission failure rather than missing data. The model
   saw `permission=lower_bound_only` but described the bound as an exact total,
   then contradicted itself with the correct coverage caveat.
3. General next step: render a language-aware, typed-derived principal
   conclusion sentence for every complete/lower-bound authority. For a lower
   bound it must say “at least N / at least X; full-window total unknown”; for
   complete it may say exact N/X. This remains soft prompt guidance.
4. Cross-caliber deltas have no relation authority by default. Add a generic
   instruction not to explain record-vs-occurrence/partition differences as
   boundary overlap, precision, or missing closure unless a typed relation
   explicitly proves that mechanism.
