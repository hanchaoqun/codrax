# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T12:31:10Z
- sweep_start_ts: 20260819-053108
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_nlohmann_long_double_symptom | FAIL | eval/results/github_issue_nlohmann_long_double_symptom-20260819-053110 | write_apply,answer_regex | none | 236s | 26 | read=7,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | fail | B1159-B2 的 fail-closed 正臂：`make check` 虽绿但无 `project_test_observations[]`，终态正确为 unverified。补丁本身也不正确：把 `long double/%.*lg/1.25L` 全改成 `double/%.*g/1.25`，绕掉用户要求的 long-double 行为且仍无普通 float/double 非回归断言；正确方向应保留 long-double 并使用 `%.*Lg`。根因含系统自冲突：planner typed section 要求 observation carrier，而 source-language mismatch repair 明令删 Go probe 后只留 acceptance_tests，模型逐字照做。|
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-053110 | answer_regex,answer_contains,mermaid_edge_count | none | 689s | 49 | read=18,repo_map=4,list=0,trace=0,source_lens=0 | midloop=12,inv=5/0,fin_reject=6,unavail=0,prune=0 | partial | 组件责任正文基本准确，终稿保留四阶段、BusContext 参数流、BuildAgentContext→Mutable 调用及 Mutable→AgentContext.Mutable 数据流；但耗时 689s、6 次 finalizer reject/7 次 Mermaid 修补。首轮已有 typed participant candidates，模型仍先发 unproven boundary、随后反转 call、残留 boundary、拆岛，说明 recipe 心智仍偏高。终图把内部 enum `precedence/argument_flow/data_flow` 直接显示给用户，虽然 edge_anchors 已有业务 `visible_label`；另有“系统补充：输出维度核对”内部块。关系事实可用但展示与成文效率未闭环。|

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
