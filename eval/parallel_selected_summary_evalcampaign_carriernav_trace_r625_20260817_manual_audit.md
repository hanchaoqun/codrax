# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T15:03:00Z
- sweep_start_ts: 20260817-080258
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-080300 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 171s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 三次 trace_query 均绑定 1.000..1.010s 与 app-100；完整 Trace 因果投影、链上 worker-200、8.300ms 可消除量、目标 sleep=10.000ms、背景 supply_pressure=3.500 cpu·ms 均保留，背景未晋升主因。Analyzer 两次精确跨字段拒绝后自行形成 causal_diagnosis；无系统静默扩/缩域、无成文拒绝、无 4ms 活跃流降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-080300 | answer_regex,answer_contains,mermaid_edge_count | none | 510s | 42 | read=24,repo_map=3,list=0,trace=0,source_lens=0 | midloop=10,inv=6/1,fin_reject=2,unavail=1,prune=0 | fail | B983 首版仍失败。completion 时阶段参与者已由 precedence 覆盖，missing 只剩 Mutable/BusContext；软导航把 participant groups 也按 missing 过滤，导致已覆盖但正是载体对端的 Extractor/Finalizer 身份不能参与 sibling-argument 排序。连续补读两个无关 helper，模型第 16 轮才自行找到 dispatchStage，终稿仍把 BusContext/Mutable 标成未证并省掉用户要求的数据流。Runner 的阶段名/边数 oracle 再次假阳性。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `B983` 首版仅在“对端也仍 missing”时生效，生产条件不成立；不能收账。
- `B984-FULLREQUESTPARTICIPANTQUALITY1` 已实现：候选发现仍只由 missing carrier 限定，但候选质量使用全部 incident-required 请求参与者分组。这样已经被其他边覆盖的组件仍可证明某处载体 handoff 更值得读取；它不会重新打开已覆盖义务，也不会铸证据/图边/答案。
- 目标测试已改为“Extractor 已覆盖、只有 BusContext 缺失”的真实 completion 形，并在全部支持语言通过；完整 `internal/tool` 套件通过。需要新二进制 r626 才能验证生产效果。
