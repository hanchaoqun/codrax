# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T18:47:54Z
- sweep_start_ts: 20260731-114753
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260731-114754 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 145s | 28 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | typed finalizer hint 已明确进入 prompt，正文各边也写 causal_link=unproven，系统覆盖块保持 relation=temporal_sequence/edges=3；但首段仍宣称“形成完整 UI→RenderService→GPU 跨线程 flow”，与后文“真实因果依赖未经确认”自相矛盾。r1/r3/r4 三次复现、r2 正常，按模型对软权限不稳定服从记录；不扫描“形成/flow”等正文做硬门。 |
| 2 | read_combo_trace_current_code_boundary | PASS | eval/results/read_combo_trace_current_code_boundary-20260731-114754 | trace_attachment,answer_regex | perf_triage+trace_query | 149s | 36 | read=5,repo_map=0,list=0,trace=2,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 批 X1/X2 生效：内容寻址 trace 路径未再误作源码，错误 `[absence: H:RenderService:DoFrame]` 已消失，B/E artifact selector 与源码行分席正确，86.111ms > 50ms 正确。仍把 perf_triage 的 best-guess `heavy-compute` 当作已证触发原因，并将 model-extracted PerfJank reason 作为 runtime_artifact principal/direct-cause 传播；这是 typed authority gap，已列批 Y1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
