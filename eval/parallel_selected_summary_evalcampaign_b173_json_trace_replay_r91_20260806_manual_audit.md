# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T10:42:48Z
- sweep_start_ts: 20260806-034246
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260806-034248 | log_regex,answer_regex | none | 131s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Exact output is `{"ids":["u1","u3"]}`. Typed repair locus stayed `required_material_scheduling`; repair rounds fell from 3 to 1 and the declaration-only decision flag no longer survived. A separate system gap remains: a later filter+assemble continuation was classified as a “complex strict-output plan” solely because it had two actions, which minted decision/contribution/reconcile duties and expanded a pure projection to six execution batches. |
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260806-034248 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 132s | 30 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | Explicit-window projection, wakeup path, rank, semantic span, two decision axes, auto-supplement, and `frame_causality=unproven/frame_evidence_status=absent` all survived. The model nevertheless opened with a proven dropped-frame claim and added 4.6ms+0.8ms despite no pairwise additive carrier; this is adherence variance and must not be “fixed” by final-prose scanning or system rewriting. A deterministic context gap amplified the bad recommendation: perf pre-triage classified the PID-0 `idle/1 next_prio=120` endpoint as observed user-space `ohos_rt`, after which the model called idle a competing high-priority thread and advised lowering its interference. PID-0 idle is an idle placeholder, not runnable competition. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
