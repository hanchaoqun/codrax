# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T02:09:45Z
- sweep_start_ts: 20260828-190944
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_dateutil_relativedelta_float_symptom | FAIL | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260828-190945 | write_apply,write_patch_oracle | none | 346s | 27 | read=8,repo_map=3,list=0,trace=0,source_lens=0 | midloop=3,inv=0/0,fin_reject=0,unavail=3,prune=0 | fail | Write analyzer/controller/planner 均正常进入；planner 首次错误读取不存在的嵌套路径，工具返回 typed `repo_map` 定位建议，但失败读取与一次成功读取一起耗尽 2 次 synthesis read budget。下一轮 `repo_map` 已被 schema 收窄移除，模型按刚收到的建议调用时连续得到 unavailable，随后只能用陈腐/猜测路径成计划，最终没有安装计划、没有 apply。缺失路径 repair pack 也没有携带已经存在的同 basename typed root candidate。属于预算计数、工具建议和恢复载体三面自冲突，不是单纯模型波动。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-190945 | answer_regex,answer_contains,mermaid_edge_count,typed_diagram_participant_coverage | none | 421s | 51 | read=9,repo_map=4,list=0,trace=0,source_lens=0 | midloop=18,inv=4/0,fin_reject=13,unavail=0,prune=0 | partial | 最终 Mermaid 合法，阶段 precedence 有证据；`Mutable`/`BusContext` 无本轮 exact typed directed relation，保持 disconnected/unproven 是诚实降级，不是关系被系统误删。但成文经历 13 次拒绝：`boundary_participant_not_visible` 要求增加/对齐可见 participant，而 local repair lease 只给 boundary add/remove/dedupe 和 orphan remove/retain，没有“模型选择并补可见 participant”的可执行 capability；模型被迫跨代整块重写后又触发关系门。摘要另泄漏字面 `\\n\\n`，属于用户面 prose 转义归一化缺口。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed follow-up gaps

1. `B1431-FAILEDREADBUDGETCONFLICT1/P0-P1`：planner 的 exact-read 配额把失败执行当成成功取证；同一失败又建议使用随后被 schema 收窄移除的 `repo_map`。修向为“成功取证配额 + 独立失败上限”，并把基础成功读取预算从 2 提至 3；缺失 patch/modify 路径还应从 typed write/explore/context carrier 中发布唯一同 basename、真实存在的 relocation candidate，但不得自动替模型改路径或创建文件。
2. `B1432-PARTICIPANTVISIBILITYCAPABILITY1/P1`：generation-scoped local diagram lease 对 participant identity/visibility mismatch 没有可执行动作。需增加模型选择的 participant visibility capability，只能在指定 block 内声明/对齐 exact participant，不得创建边、关系、结论、业务标签或布局。
3. `B1433-PROSEESCAPEVISIBLE1/P2`：非代码 prose 中字面 `\\n\\n` 被原样发到用户面。应在 renderer 的 prose-only 布局层安全转成段落换行，明确隔离 scalar、代码块和 inline code，不修改模型事实与结论。

以上三项均不读取用户请求、模型 thinking/final prose 或 Mermaid message 作为事实/硬门；Trace 查询、显式窗、因果投影、自动补齐、链上根因选举和活动流时限路径未被本批修改。
