# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T11:51:02Z
- sweep_start_ts: 20260817-045101
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260817-045102 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 204s | 37 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass-with-caveat | 显式用户窗内两次 typed trace_query；模型摘要先给 65.912ms running 供给、36.757ms D-state、链上小额来源，并明确 15 个邻近席只作背景。系统投影、自动补齐、双轴占时/可消除量、业务 span 与覆盖边界完整，`bounded_window_candidate` 未泄漏。模型仍写“全部链上席位”，而 typed enumeration_status=incomplete；且把 callsite 所在 `devhost.elf`解释成 GPU/媒体驱动宿主，超出本窗直接证据。保留 B965 模型服从观察，不加 prose gate/系统代写。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-045102 | answer_regex,answer_contains,mermaid_edge_count | none | 299s | 40 | read=17,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=4/0,fin_reject=2,unavail=0,prune=3 | partial | B969 使真实 `extractor.go:262` 最终进入 evidence 和图，较 r612 降至 299s/2 repo_map/4 completion attempts；但首次/二次 exact repair 仍误导向 cgec/contract 校验文件。深审确认 analyzer 已有唯一 symbol provenance，而共享 identity projection 丢弃 `ResolvedAs`，使导航、关系组件、最终图各用 display/canonical 不同身份。终图诚实保留 stage precedence 与 exact extractor call，Mutable/BusContext 仍断开 unproven；正文继续把未证 BusContext 数据流写成既成事实，归 B965。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
