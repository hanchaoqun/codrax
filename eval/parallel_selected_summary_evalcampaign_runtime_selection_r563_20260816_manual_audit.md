# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T11:12:16Z
- sweep_start_ts: 20260816-041215
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260816-041216 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 176s | 36 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 四态、per-CPU Running 及 CPU4/CPU12 typed 频点均正确；模型第一稿也写“不能直接证明受限”。终稿却升级成“频率受限不成立”，并以 CPU12=2075MHz “未超出 cpu4 上限 2100MHz”再次跨 CPU 比较。typed same-key join 完整在 prompt，因此属于模型未一致消费精确信息，不授权系统扫描/改写正文。另有独立系统 gap：无关 event_search enumeration incomplete 污染算术附注，把完整 target-window 三个比例都标成 `completeness=incomplete`。 |
| 1 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260816-041216 | answer_regex,answer_contains | none | 467s | 43 | read=20,repo_map=1,list=0,trace=0,source_lens=0 | midloop=20,inv=7/0,fin_reject=3,unavail=0,prune=2 | fail | B897 生效：Analyzer 由 r562 的 6 次身份/前缀冲突降为仅 1 次缺 diagram relation quote。但 accepted payload 仍完全省略 schema-required `call_chain_endpoints`，B896 selection lane 未启动。Explorer 因此扩展到 28 条证据/26 轮，最终只证明 full/patch 的 mutation 路径与汇合点；正文仍把“首次完整/重试 patch”作为工具固有适用时机，未证明每轮 schema refresh、patch-base 与 preference/force-full 选择关系。3 次 diagram repair 后图只呈现局部执行路径，runner PASS 仍是假阳性。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. `B897-DIAGRAMPARTICIPANTPREFIXIDENTITY1` 获生产正证；不再有 snake/Camel 重复 actor 或短 identifier
   命中长前缀的分析拒绝。唯一 Analyzer reject 是精确 diagram relation quote 缺失，下一轮正确修复。
2. `B898-RUNTIMESELECTIONCARRIEROMISSION1` 获生产反证并升级 P1：provider schema 将
   `call_chain_endpoints` 列为 required，但 executor compatibility 只强制 `question_kind`，模型两次均整字段
   省略并仍被接受。于是后续无法知道请求包含 initial/full vs retry/error/patch selection，B896 的正确消费
   链无输入可用。
3. 最优修法不是扫描用户句子，也不是把所有 runtime 请求都增加一轮：仅当 typed 当前源码关系面为
   required diagram、`predicate_axis=flow|call` 或 `question_kind=call_chain` 时，缺 carrier fail-loud；typed
   runtime-artifact-only（显式 runtime scope 且没有 current-source explanation，或 source_mode=exclude）保留
   旧 provider 的 inert false/empty 兼容。模型仍负责 true/false 与 verbatim quote，系统不推断选择结论。
4. 冻结 `B899-ARITHENUMERATIONCROSSQUERYTAINT1`：算术附注把全会话任一 trace_query 的
   `EnumerationAuthority!=complete` 当成每个正文分子的 completeness。H4 的完整 target window state 总账因
   无关 capped event_search 被标 incomplete，属于跨 query/metric 污染。应改成 relation-local typed
   numerator authority；在不能唯一绑定时只报 unknown，禁止用 unrelated incomplete 声称该分子不完整。
