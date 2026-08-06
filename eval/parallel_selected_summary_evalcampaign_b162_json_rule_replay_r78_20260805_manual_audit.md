# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T04:30:53Z
- sweep_start_ts: 20260805-213050
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_basic_sum_with_rules | PASS | eval/results/data_basic_sum_with_rules-20260805-213053 | log_regex,answer_regex | none | 46s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 严格终稿 `17`；一次 bounded custom transform 实际读取 orders/rules 两份材料并完成。相较 r77 的 127s/6 批/2 repair 收敛到 46s/1 批/1 repair，但首个 plan 仍把未读取的 rules.md 声明为 script_consumed，修复后才读取。summary 的 repair=0 漏计 typed `data_repair_rounds=1`，是 eval 观测 GAP。 |
| 2 | hilog_arkts_panic | PASS | eval/results/hilog_arkts_panic-20260805-213053 | log_attachment,answer_contains | log_triage | 92s | 19 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | log triage 和 Analyzer 均首轮合法，JSON shape 修复生产闭环；直接触发点、位置、堆栈、sig=6 正确。但正文把“某对象在 line 42 为 undefined”进一步写成“传递给 UserCard 的用户/状态数据为 undefined/null”，日志并未证明变量身份或 caller 数据流；末段又正确降回“具体上游需源码”。typed 上下文已明确 stack caller 只证明调用顺序，记模型越界单 witness，不加 prose hard gate。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
