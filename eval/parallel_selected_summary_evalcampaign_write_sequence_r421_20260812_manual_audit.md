# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T04:41:57Z
- sweep_start_ts: 20260812-214156
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260812-214157 | write_apply,answer_regex | none | 129s | 23 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | partial | 源码把 `_prefault` truthiness 改成 exact property-existence，并新增 false/0/empty string 与 existing-default 测试，改动正确；Node 不可用时只得到 source_static，终局诚实 `accept_unverified`，不能降杆。新确认 B707：系统已创建 `ready_to_plan` 的 direct-runtime proof-followup 批，但 action projection 仍提供 `verify_batch`，模型跳过 plan 直接复跑同一静态验证，批被错误标 complete，后置 normalizer 才挡住 all_verified。 |
| 2 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260812-214157 | answer_regex,answer_contains | none | 304s | 35 | read=11,repo_map=2,list=0,trace=0,source_lens=0 | midloop=9,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | 用户显式要求 sequenceDiagram+表。Router reasoning、Analyzer 与 presentation directive 均识别图要求，但 single-shot typed `requires_diagram=false`，Analyzer 正确把 `diagram_hint.required=true` 降为 optional；第一稿未证边被正确拒绝，第二稿重复一条 call-site 被正确拒绝，optional repair 最终允许删图，runner/human 均 FAIL。B706 根因是 load-bearing structured bool 被错误输出/遗漏后以 Go 零值静默接线，不是关系证据门过严；不能扫描 request/directive 修补。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 结论与施工状态

- `B706-REQUIREDDIAGRAMBOOLZERO1`（P0）：已施工。`emit_turn_policy` 对
  `requires_diagram` 启用精确字段存在性校验；缺失时只做一次同 schema 的紧凑结构修复，修复后仍
  不合法则 fail-loud。显式 `false` 原样保留，Codrax 不扫描 current request、presentation
  directive 或答案文字来翻转硬门。路由日志新增 `diagram_required` 与 presentation 遥测，便于生产
  区分“模型明确 false”和“字段缺失已修复”。专项 pin 覆盖 missing→typed true、explicit false
  不重推断、未 opt-in legacy 字段不被意外硬化。
- `B707-PROOFFOLLOWUPACTIONPROJECTION1`（P1）：confirmed，下一批施工。proof-followup 的
  `ready_to_plan` 状态只能先 plan/explore，不能暴露 `verify_batch` 让模型跳过探针计划；终局
  source_static 降级门保持不变。
- 4ms 专审：本轮 4ms 是末端解析/校验耗时，不是流式连接年龄门。已有行为 pin 证明 active byte
  stream 跨 4ms 仍完整返回；合法终止仍仅为 caller cancel/deadline、no-first-byte、真实 byte
  stall、transport/decode failure。本批不新增任何“4ms 无答案即降级”路径。
- Read/Trace 路径未改。显式窗、因果投影、自动补齐、typed 链上-only 主因、实际占用/业务修向与
  规则计价可消量双轴保持；邻近/背景仍不能晋升主因，系统不代写或替换模型结论。
