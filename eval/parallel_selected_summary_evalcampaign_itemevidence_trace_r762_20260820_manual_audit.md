# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T08:22:30Z
- sweep_start_ts: 20260820-012230
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-012230 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 343s | 36 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=3/1,fin_reject=0,unavail=1,prune=0 | pass（有显示债） | 显式 2.000000..2.020000s 窗、threadpool-400→network-300→cookie-200→app-100 已证链、11.000ms IO 首席、三条互斥 1.000ms 调度供给、实际占时/现规则可消双轴、Trace 因果投影和自动补齐均保留；邻近与背景未加冕。343s 活跃流未按 4ms/4m/总年龄降级。新系统显示 GAP：◎ 背景行把 io_pressure 综合评分 7.000 错写成 7.000ms，同页树/表又正确写“综合评分,非墙钟”。模型另把 absolute_level=not_defined 的 7 分描述为“中高”，上下文已明确禁止绝对等级，暂按模型措辞波动，不扫正文作硬门。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260820-012230 | answer_regex,answer_contains,mermaid_edge_count | none | 714s | 48 | read=32,repo_map=3,list=0,trace=0,source_lens=0 | midloop=23,inv=16/0,fin_reject=4,unavail=2,prune=0 | fail | B1222 生产正证：Mutable/BusContext 均获得可复制 exact candidate，系统未代写边。终稿仍把四阶段 precedence 岛和 BuildAgentContext/Mutable 局部操作岛称为完整数据流，typed handoff 已明确 requested_relation_spine_status=unproven、weak_components=9，却没有结构化整图范围披露义务。B1223 生产未采用：items[].evidence_ids 全缺席，四个阶段条目把 citation_ref 全填成 7，最终四项均误引 StageAnalyze 行；“字段缺席不产生义务”使旧手算错绑继续出厂。714s 模型终稿正常返回，未降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed gaps and disposition

- `B1224-ITEMEVIDENCEADOPTION1`（P1，下一批）：当前源码 lane 中，结构化 item 一旦主动携带 `citation_ref(s)`，必须同时用 accepted `evidence_ids` 选择证据；只校验字段组合，不读 item 文本。字段与所有引用均缺席仍允许，避免强迫无证结论伪造引用。
- `B1225-FLOWSPINEDISCLOSURE1`（P1）：每个 participant 分别有局部 incidence，不等于请求的整体 flow 已连接。消费既有 typed `requested_relation_spine_status=unproven`，要求模型在 diagram 载体上作结构化范围声明；系统不得补边或改结论。
- `B1226-TRACEPRESSUREFAKEUNIT1`（P1，小批）：◎ 背景区仍以 seat/tier 推断口径，未消费 `ObservationRecord.Unit=composite_score`，导致综合评分穿戴 `ms`。统一所有 overview 分流到共享 `runtimeTraceProjNonWallClockValueCaliber`。
- Analyzer 两路各有 3 次 schema 重试；拒绝原因互不相反，当前无“同一字段既必带又必拒”证据，继续按 P2 模型 JSON 遵循波动观察。
