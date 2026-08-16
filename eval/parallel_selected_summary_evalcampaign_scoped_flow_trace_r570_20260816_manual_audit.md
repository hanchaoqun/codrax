# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T14:23:12Z
- sweep_start_ts: 20260816-072310
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260816-072312 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 190s | 37 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | B909 正证：逐 CPU 矩阵没有跨 CPU 拼接；正文四态、CPU12/CPU4 数值及“目标频率受 policy 限制未证”正确。Runner 只因旧 limit-row 正则未命中而 FAIL。新确认 B910：runtime_selection_profile=false 仍保留 discover endpoint，错误触发源码选择证据补采/两次 completion。新确认 B911：系统把“96.081ms 占 total running 157.248ms 的 61.1%”强套窗长 233.190ms，追加 41.203% 的错误算术异议。正文还复制 raw enum，属次级展示债。 |
| 1 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260816-072312 | answer_regex,answer_contains | none | 369s | 35 | read=7,repo_map=0,list=0,trace=0,source_lens=0 | midloop=7,inv=4/0,fin_reject=2,unavail=0,prune=0 | partial | B908 N2a 半正证：finalizer 正确为 context_only，且 Analyzer 第一次失败后用完整关系从句修正 provenance；但 participant 完备门允许两个工具分别用 Name() return 边满足 incidence，最终图只剩四条互不相连的内部边，没有表达“两工具在 finalizer 内的调度关系”。这是关系作用域/相关性缺口 B908-N2b，不是模型随机波动；两次成文拒绝均正确阻止未证桥，但修复建议把模型导向任意 participant 邻边，耗时 369s。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. 本批严格并行恰好两个 case，使用同一不可变 binary。未观察到 active stream 因 4ms 或固定累计年龄降级。
2. H4 是有限事实/单一影响判定，零 root-cause/wakeup/blocking/full causal projection 是正确 breadth；不能为了 oracle 变绿强制完整 Trace 因果投影。真正 causal_diagnosis 的显式窗、自动补齐和 typed on-chain-only 主因合同未改变。
3. B910 与 B911 都是系统确定性问题：前者由 typed carrier 跨域，后者由系统附注选错分母。它们不能归为模型波动，也不能靠答案关键词特判。
4. Combo 的 runner PASS 只证明字面答案维度存在，不证明关系图完成用户任务。关系候选必须同时满足请求关系作用域，不能用“任意 incident edge”替代；系统仍不得代画关系。
