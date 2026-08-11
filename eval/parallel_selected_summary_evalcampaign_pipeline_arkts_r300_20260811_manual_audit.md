# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T07:37:56Z
- sweep_start_ts: 20260811-003754
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | arkts_repomap | FAIL | eval/results/arkts_repomap-20260811-003756 | typed_inventory_rowset,answer_contains | none | 174s | 22 | read=6,repo_map=4,list=0,trace=0,source_lens=4 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | `EntryAbility` 的证据和答案都明确写“无 @Entry/@Component”，却被 Explorer 放进 `@Entry` member_set；错误 aggregate 被系统加冕为 authoritative，Finalizer 零拒绝照抄。根因不是 ArkTS parser 漏识别：四条真实 @Entry evidence 均精确，`EntryAbility` 行也没有 `surface_family=@entry`。这是资格型集合缺少逐成员 predicate witness 的合同 gap。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-003756 | answer_regex,answer_contains | none | 491s | 42 | read=25,repo_map=2,list=0,trace=0,source_lens=0 | midloop=11,inv=2/0,fin_reject=2,unavail=0,prune=1 | fail | B510-A3 获生产正证：completion 用 exact LHS/RHS 阻止 closure，模型局部重发后清债。但 call mismatch 仍只有 note + actionable none，真实调用没进入 typed closure；Finalizer 首稿的调用/返回边被关系门拒绝，patch 删除后仅余三条阶段 precedence 与一条 append data_flow，AnalysisIR/EvidenceItems/AnswerDocument 交接缺失。表头也从 typed `stage/输入/输出/状态载体` 退化为 `项目/列 2/列 3/列 4/列 5`；图以 Agent/函数名为主，业务角色表达不足。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch decision

- `B510-A3`: production-positive，关闭。
- `B510-A4-RELREPAIR2/P1-high`: required non-Trace source-relation diagram 下，call 的 parser-owned exact caller/callee 与 assignment 一样进入局部 action-required repair；错误 row 继续 text-reference，系统不补边、不改答案。
- `B513-QUALMEMBER1/P1-high`: 资格型 source inventory 的 principal member_set 必须为每个成员携带同一 typed qualifying family/predicate witness；定义存在或“业务入口”近义不能替代 `@Entry`/annotation/decorator 资格。先设计 typed carrier，禁止扫描 aggregate label、member note 或最终答案字符串。
- `B510-D-BUSINESSLAYER/P1`: 图展示层使用业务角色/动作，源码 exact endpoint 仍是关系 authority；业务分组或 alias 不得变成 evidence endpoint。
- `B510-E-DIMHEADER/P1`: 既有 typed requested dimensions 已精确携带 `stage/输入/输出/状态载体`，但 table schema 未消费，需单源投影到模型 authoring 教学/结构字段；不从最终表头反向猜用户意图。
- Trace explicit-window causal projection、auto-supplement、on-chain-only root cause families：本批未触碰。
