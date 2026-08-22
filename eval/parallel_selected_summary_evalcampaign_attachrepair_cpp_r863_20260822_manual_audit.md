# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T13:58:19Z
- sweep_start_ts: 20260822-065818
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_py_registry_dispatch | FAIL | eval/results/sr_py_registry_dispatch-20260822-065819 | answer_regex,answer_contains | none | 691s | 29 | read=3,repo_map=1,list=0,trace=0,source_lens=1 | midloop=7,inv=2/0,fin_reject=9,unavail=0,prune=0 | partial | 正文正确覆盖装饰器注册、REGISTRY 查找、实例化、run_in_executor 与 MRO；最终恢复稿仍带六参与者关系图。但 9 次 finalizer reject 后降级，主要由两类确定性合同死锁造成：系统提示可删除 optional diagram，live lease schema/执行器却拒绝 whole_remove；同一原子 patch 中模型又反复把即将重加的 EX 边与 remove_if_isolated 并置，正确被拒。B1347 的“本代无 attach，禁止组合 refs”已生效，未再出现非法 attach。引用 resolve 主项仍偏定义行而非 lookup/instantiate 行。 |
| 2 | sr_cpp_virtual_chain | FAIL | eval/results/sr_cpp_virtual_chain-20260822-065819 | answer_regex,answer_contains | none | 1146s | 63 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=12,inv=1/0,fin_reject=20,unavail=1,prune=0 | partial | 正文较完整且业务表达可用：Logger.log→Sink.write 多态分发→ConsoleSink.write→fputs/fputc，并说明 SinkRegistry::create 的 console 分支。但四条 live failure 为 target_carrier=unknown、allowed_actions=[]，系统仍发局部租约并同时禁止 replace/remove，模型在第 19 轮明确识别无解，20 次拒绝后降级。恢复稿 Mermaid 把多个 participant/Note 折进引号，语法/结构不可用，证明降级恢复路径仍需独立 Mermaid fail-safe 审计（B1349）。B1347 同样生产生效。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
