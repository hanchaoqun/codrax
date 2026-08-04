# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T08:29:09Z
- sweep_start_ts: 20260804-012908
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260804-012909 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 37 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | exact-window authority、系统投影和 finalizer context 均为 20.000ms；模型仍把无绑定的 20.020ms supporting aggregate 写入结论。B69/B70 曾在同合同下正确，判为模型波动，不追加硬门或答案改写。一次错误 relation roster 被 exact typed validator 正常拒绝，下一次即闭环，不是旧格式风暴。 |
| 2 | read_combo_command_current_source_explanation | PASS | eval/results/read_combo_command_current_source_explanation-20260804-012909 | answer_regex | none | 260s | 25 | read=3,repo_map=1,list=2,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | typed route fallback 已激活，主路径已正确区分 observation ledger 与 AnswerAggregateFact 并行载体；局部仍把普通 command measurement 错说成 history/VCS 分支，属于已给精确上下文后的模型波动。答案统计 maxdepth=1 得 168，而用例隐含期望递归 253；用例正文未声明递归且 runner 不验动态计数，另立 eval-quality GAP。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

### Trace：系统权限修复有效，残余为可重复对照下的模型波动

四次 trace_query 的 selected-window partition、target/principal state 和 blocking authority 都是 20.000ms；finalizer 还明确把
Explorer 无 support refs 的 20.020ms aggregate 降成
`supporting_coverage / demoted:runtime_observation_advisory_aggregate`。完整唤醒链、11.000ms IO 主席、真实占时与现规则可消双轴、
自动补齐和因果投影均在。模型仍在摘要中选择 20.020ms，但 B69/B70 在相同 typed 合同下连续选择 20.000ms，因此不再添加
answer-prose 扫描、强制重试、系统代写或单指标特例；保留为模型波动观察项。

模型第一次主动提交的 relation roster 不等于 deterministic authority 的五态 roster，精确 validator 拒绝后，下一次按 typed roster
闭环。该拒绝校验的是主动声明的精确关系，不要求 optional carrier 必须出现，属于正确的精确信号硬校验。

### Read：route fallback 已验收，剩余一处模型偏差和一处用例合同缺口

Explorer 与 finalizer 都收到了 `Command Measurement Evidence Path`，最终答案的主链已经正确：
`ToolResult.CommandMeasurement -> observationRecordForCommandMeasurement -> compiled ObservationLedger`；同时明确
`AnswerAggregateFact` 是模型 completion 的并行载体，deterministic reconcile 只是单向交叉校准，
`EmitInvestigationComplete` 只负责闭环控制。`EVAL-B70-CMDPROFILE1` 因此关闭。

正文局部仍把 ordinary measurement 误说成 `Origin=VCSMetadata, History=true`，而 prompt 已逐字给出普通分支是
`Origin=CommandMeasurement, History=false`。这不再是上下文缺失，按模型波动保留，不用硬门拟合。

本轮实际执行 `find internal/tool -maxdepth 1 ... | wc -l` 得 168，并披露了 maxdepth 边界；当前递归计数是 253。case 请求只写
“internal/tool 下”，没有声明“递归”，旧注释却隐含递归语义，runner 的 oracle 也不验证动态 count。该问题应在 eval 层修：题面显式写
递归/仅直属二选一，oracle 从同一 checkout 动态计算 typed expected count；禁止把 253 固化为常量。
