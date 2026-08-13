# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T14:02:58Z
- sweep_start_ts: 20260813-070256
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h7_self_seat_full_spectrum | FAIL | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-070258 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 162s | 47 | read=4,repo_map=0,list=0,trace=3,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Analyzer 将明确的“根因排序+大小贡献来源”错铸为 bounded_fact_set；补齐据此跳过 families_present，最终 projection=0，答案仅余状态卡，双轴、链上完整榜、邻近隔离和枚举边界均丢失。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-070258 | answer_regex,answer_contains,mermaid_edge_count | none | 247s | 34 | read=9,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=3/0,fin_reject=4,unavail=2,prune=0 | partial | 最终图合法并保留三段顺序、三条真实局部 call；本轮没有复现 B725 collision tuple，4 次拒绝来自首次大量未证边、boundary JSON/可见性/保留错误。关系真实但仍可进一步降低模型心智。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

- Trace 的 FAIL 不是 B725 改动造成，也不是 4ms/连接年龄降级。模型 160s 内持续产出，最终正常
  `emit_answer_document`，零 finalizer reject；缺失发生在 Analyzer→supplement→projection 的 typed
  路由。Analyzer 明明列出必答维度“根因排序”“小贡献来源”，却发
  `runtime_question_profile.scope=bounded_fact_set`，并把 artifact scope 错记成 full_artifact 而非用户
  给出的 exact window。因 profile 被视为窄事实，supplement 在 `families_present` 直接跳过；最终
  `trace_query_final_projection_blocks=0`，因此 49.623/0.033 同源二分、完整链上榜、供给折算基准、
  未计价真实占用与 enumeration_status 均没有进入答案。模型仍给出部分正确数值，但把 off-chain
  logd/hilogcat 称为“小贡献来源”，也没有发布链上-only 因果板，人工判 fail。
- 新立 B726（P0）：当前 runtime breadth consistency 只检查“causal_diagnosis 是否有诊断 carrier”，
  不检查反向矛盾“bounded_fact_set 是否承载用户必答因果归因”。最优根修是新增 schema-valid
  requested dimension role `causal_attribution`；Analyzer 对根因/瓶颈或 causes/contributors 排名使用该
  role。required causal_attribution 与 bounded_fact_set 同时出现时，生产入口精确拒绝并要求完整重发；
  不静默扩大 scope，不扫描 raw request/model answer/tool prose。普通状态、次数、时长、记录理由继续
  保持 bounded。
- Logic 本轮没有自然命中 B725：第一稿直接画了多条未证关系，校验器正确拒绝；修补后主要在
  participant_boundaries 的 JSON 层级、Mutable 可见节点和 boundary 保留间往返。最终图保留三条
  analyzer→explorer→extractor→finalizer precedence 与三条真实 local call，并把 BusContext/Mutable
  作为断开未证 participant，未造假。B725 unique collision map 的行为仍由生产 precheck pin 覆盖，
  等待更匹配的自然生产 witness；不能把本轮 PASS 虚报为 positive。
- 两条 active streams 均超过 4ms 且持续产出；没有 fixed-age answer degradation。合法终止/有披露恢复
  条件仍限 caller cancel/deadline、no-first-byte、byte stall、transport/decode failure 或重试耗尽。
