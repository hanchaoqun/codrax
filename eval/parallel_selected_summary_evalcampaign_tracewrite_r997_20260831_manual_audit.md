# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T02:56:43Z
- sweep_start_ts: 20260831-195643
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-195643 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 197s | 46 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B1523 生产正证：explorer/final 不再把 NetworkService/CookieMonster 的 scheduler rows 称作确定性工作，A→B 唤醒方向与 runnable 语义均正确；显式 10ms 窗、Trace 因果投影和自动补齐保留。过程仍发现 B1524：模型用单字面量 `pattern=VerifyClass\|Jit\|...` 误当正则并据 0 行声称无语义工作，typed supplement 后续救回 VerifyClass；另有 B1525：T7 只有链上 runnable 席，未发布同 CPU relation/overlap/competitor carrier，模型却扩写为“与目标同 CPU 竞争”。两者未改写最终主根因和请求核心关系，故本例人工 pass，但均需泛化修复。 |
| 2 | github_issue_fmt_tm_year_overflow_symptom | PASS | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260831-195643 | write_apply,answer_regex | none | 231s | 27 | read=7,repo_map=0,list=4,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 写入提交 `111460347f6b4a081098e97d43a671597029c1fb` 仅把 calendar year 提升到 long long 并以 1900LL 相加，消除 int 加法溢出；make check 真实编译运行通过，变更路径与计划一致。Analyzer thinking 曾数次误判 INT_MAX+1900 的范围，但计划/补丁/验证最终正确，未见系统性合同 gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case disposition

- `B1523-EXPLORERTRACESEMANTICROLECALIBRATION1`: production-positive；Trace explorer 与最终答案均保持 scheduler-state/semantic-work 分栏和正确唤醒方向。
- `B1524-EVENTSEARCHMULTILITERAL1/P1`: confirmed；单一 literal `pattern` 没有多个精确候选的 typed OR 入口，模型用 `|` 自造正则后形成假 0 和错误缺席结论。最优方案是新增 `patterns[]` literal-OR carrier，绝不重新解释旧 `pattern` 中的 `|`。
- `B1525-RUNNABLECPUCOMPETITIONAUTHORITY1/P1`: confirmed；链上/ranked runnable 只证明可运行等待，不自动证明同 CPU 放置或直接竞争。Explorer handoff 必须保留 typed authority ceiling：同 CPU placement 与 compatible overlap/competitor carrier 缺一不可。
- 两项修复均属于模型上下文校准和 typed tool capability，不扫描用户/模型/最终正文，不硬拒或替换模型结论。
