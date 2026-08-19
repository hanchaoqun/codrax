# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T13:12:10Z
- sweep_start_ts: 20260819-061209
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_nlohmann_long_double_symptom | FAIL | eval/results/github_issue_nlohmann_long_double_symptom-20260819-061211 | write_apply,answer_regex | none | 224s | 26 | read=8,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | B1163 生产正证：planner 不再把 C++ 改成 double，也发出了 `project_test_observations[]`。但补丁仍选 `%.*Lf`，改变了上游 `%.*Lg` 的通用格式语义；正确修向是保留 long double 并用 `%.*Lg`。更严重的是 observation 把只断言两个入口“非空”的 `tests/long_double_format.cpp` 同时绑定给“输出包含 1.25”等更强合同；当前 path-level receipt 将这三条记为 observed，属于潜在误签。终态之所以仍正确 unverified，只是另有六条合同未绑定，不能掩盖 observation 权威粒度 gap。|
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-061211 | answer_regex,answer_contains,mermaid_edge_count | none | 368s | 40 | read=11,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=4/0,fin_reject=4,unavail=0,prune=0 | partial | B1164 生产正向：最终关系保留，reject 从 r730 的 6 降为 4、耗时从 689s 降为 368s；锚点与 Mermaid 标签已一致。残余一：模型把 `precedence/argument_flow` 同时写进两面，结构一致但仍泄漏内部枚举；首轮 typed recipe 已有“随后进入/作为参数传递”，可做精确 recipe→模型标签校验。残余二：终图是“阶段顺序”和“BusContext 局部调用”两个孤岛，没有画出正文声称的各阶段读写共享状态，关系表达仍不完整；不能由系统补画，需提升 parser-owned 数据流证据与首轮上下文。四次拒绝中两次是 incident edge、一次 stale boundary，说明参与者修补心智仍偏高。|

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
