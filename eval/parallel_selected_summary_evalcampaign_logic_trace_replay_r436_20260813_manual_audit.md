# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T13:17:34Z
- sweep_start_ts: 20260813-061733
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-061735 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 275s | 43 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | partial | 显式窗、链上-only、因果投影、自动补采、两维根因和业务线索完整。模型正文有两处 P1：五态口误成“四态”；把 blocked_reason 记录与状态区间差异过度解释为内核延迟上报。系统后文边界正确且未代写正文。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-061735 | answer_regex,answer_contains,mermaid_edge_count | none | 455s | 44 | read=15,repo_map=4,list=0,trace=0,source_lens=0 | midloop=10,inv=5/0,fin_reject=4,unavail=0,prune=1 | partial | B724 路由生产稳定：kind=mechanism/axis=flow，删 anchor 不再可逃逸。新 B725：同一 node ID 同时承载用户参与者 Mutable 与技术端点 MutableState.AppendEvidence，boundary 校验在缺边界/边界有入边间振荡，四次拒绝后模型删除一条已证 local call。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

- B724 达到本轮生产目标，但应精确记为 route-stable 而非 reject-branch-positive。Analyzer 首个正式
  emit 已显式携带 `question_kind=mechanism` 与 `predicate_axis=flow`；第一次失败是 participant 的
  `source_quote` 不符合当前请求逐字 provenance，第二次修正后 RequestModel 保持 axis=flow。最终图
  的三条 stage precedence 与一条本地 call 均有 typed edge anchors；r435 那种删除 metadata 后仍让
  未证箭头从 presentation-only 车道通过的逃逸不再存在。本轮没有自然触发“漏字段→presence reject”，
  该分支目前由生产入口单测覆盖，继续等待异构生产正形。
- 新确认 B725（P1，高重试 ROI）。模型第二个 patch 已提交真实 local edge
  `appendStageOutputEvidenceToMutable -> MutableState.AppendEvidence`，但把其 `to_node` 复用了用户点名
  的 participant node ID `Mutable`。requested flow 对 Mutable 本身未证，所以必须保留 unproven
  boundary；同一 node 又有可见入边，校验器遂在 `missing_unproven_boundary` 与
  `unproven_boundary_has_visible_incident_edge` 之间振荡。当前 repair action 只有抽象指令“把技术端点
  移到精确节点”，没有给出发生冲突的 typed edge/anchor tuple。模型三轮增删 boundary 后，最终通过
  删除这条真实边收敛；答案未造假，但关系丰富度下降且浪费 4 个成文回合。
- B725 最优解不是放宽 boundary，也不是系统代画边。应从现有 model-authored body edge + exact
  edge_anchor 生成 bounded conflict map：participant、from/to node、canonical from/to identity、冲突
  endpoint side；明确保持原边和 anchor 不变，只把技术端点换到一个新的非 participant node ID，另保留
  exact participant 断开节点与 unproven boundary。该 map 不选择新边、不扫描正文、不改结论；模型仍
  决定显示 label/layout。若没有唯一 body-edge/anchor pair 就 fail-open 保持现有提示。
- Trace `20260813-062207.412-65061.md` 继续证明核心能力稳定：窗口
  `13762.791708..13763.024898`；running 真实占时 74.915ms、规则可消 65.912ms；非 IO D-state
  36.757ms；链上优先级反转、调度/算力/IO 与业务线索保留；邻近/背景明确不入主因，未计价真实占用
  另作新方向。模型正文把五态称为“四态”，并把 blocked_reason 事件记录与 scheduler interval 的
  数量/Σ 差异解释为“内核自身延迟上报口径”；typed 证据只支持两者口径不同，不能支持具体机理。
  这是模型成文 P1，先异构回放，不用原文关键词门或系统改写模型正文。
- 275s/455s 两个 active byte stream 均没有因 4ms 或累计时长降级。终止/有披露恢复权仍仅属于
  caller cancel/deadline、无首字节、真实 byte stall、transport/decode failure 或重试耗尽。
