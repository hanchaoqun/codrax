# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T04:44:06Z
- sweep_start_ts: 20260830-214404
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-214406 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 198s | 40 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=2/2,fin_reject=0,unavail=0,prune=0 | pass | 显式 10ms 窗、3 次 typed trace_query、最终因果投影、链上 NetworkService 第一席、实际占时/规则可消双账户、确定性优化线索与邻近/背景隔离均在；无固定时限降级。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260830-214406 | answer_regex,answer_contains | none | 876s | 74 | read=31,repo_map=2,list=0,trace=0,source_lens=0 | midloop=10,inv=7/0,fin_reject=7,unavail=1,prune=1 | fail | 恢复稿含完整表和可读图，但最终结构化答案未通过，runner 正确拒绝。根因是同一物理 Mermaid 边同时发布 relabel-only 与 remove/replace ref，联合动作集为空，连续 7 次修补后降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual Findings

1. Trace 人工判定通过。目标窗为 `34579.490..34579.500s`，3 次 target-filtered typed query 和 1 个最终投影均在；
   `NetworkService-60595 -> CookieMonsterCl-59843 -> com.baidu.tieba-59566` 为已证链，NetworkService 第一席有效归因 5.951ms，
   目标五态账、实际占时/规则可消双账户、类校验语义线索与邻近/背景隔离完整。系统没有把背景 IO/D 升格为链上根因，也没有按 4ms、4m、
   活动流年龄、重试数或上下文比例降级。
2. read runner 与人工均判失败。恢复稿本身含四阶段表和可解析 sequence diagram，但日志明确显示最终 `answer_document` 未通过结构化校验，
   `degraded_answer_checks_skipped=1`，因此不能把“读起来有内容”当作成功。B1468 的 `multiple declared sequence participants` 错误未复发，
   但本轮图结构不同，没有形成多声明 stage 的自然正证。
3. 新 P1 `B1469-SHAREDBODYCAPABILITYINTERSECTION1` 为确定性系统合同缺口。同一 Mermaid body occurrence 同时收到一个
   `target_carrier=label_pair, allowed_actions=[relabel]` ref 和另一个
   `target_carrier=visible_body_edge, allowed_actions=[remove,replace]` ref；executor 正确禁止两个 ref 对同一 statement 做混合动作。
   两个各自合法的 capability 联合后没有共同动作，模型连续尝试 relabel+remove、只 relabel、把 label ref 当 remove，最终耗尽修补并恢复旧稿。
4. `c21f10e85` 已按物理载体安全交集根修。相同 block/from/to/body occurrence 同时存在 label-pair 与 visible-body failure 时，所有相关 ref
   仅发布共同可执行的 model-selected `remove`；label ref 精确转为 prior-anchor carrier，shared-body executor 删除正文 statement 一次、每个被选
   anchor 一次。系统不选择删除、不自动重建关系；模型仍可使用独立 typed addition 选择重建。混合动作、漏选 target、陈腐 ref 和未授权 action
   继续 fail-closed。`internal/types`（29.173s）与 `internal/tool`（183.219s）整包通过，等待生产回放。
