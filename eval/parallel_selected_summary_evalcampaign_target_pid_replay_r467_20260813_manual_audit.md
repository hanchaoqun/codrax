# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T03:57:43Z
- sweep_start_ts: 20260813-205741
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-205743 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 221s | 36 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B762 生产正证：target_window_states、top_running CPU12=96.081ms/CPU4=35.960ms 和 thread_cpu_load 均进入 Finalizer，正文恢复 CPU12=2075MHz，不再声称 CPU 编号全无。新 B763：thread_cpu_load.cpu=12 实为最大状态切片代表，却未自描述为非排他；模型把它写成目标唯一绑定 CPU，漏列同一 target 的 CPU4。策略上限存在/目标影响未证结论正确；自动 FAIL 仅因窄正则不接受等价中文。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-205743 | answer_regex,answer_contains,mermaid_edge_count | none | 600s | 37 | read=32,repo_map=2,list=0,trace=0,source_lens=0 | midloop=16,inv=6/0,fin_reject=1,unavail=0,prune=0 | fail | 自动 PASS 但图只剩 Run→runAnalyzePhase 与 dispatchStage→applyStageOutput 两条 call，用户明确要求的 analyzer/explorer/extractor/finalizer/Mutable/BusContext 均未进入关系图。探索累计 32 reads、16 midloop、6 completion reject；多路恢复重复追逐同一 operation-level transfer 义务，最终 validator 正确删未证边但答案图失去主要指导意义。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
