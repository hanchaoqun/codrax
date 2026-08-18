# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T06:52:41Z
- sweep_start_ts: 20260817-235240
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-235242 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 32 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 精确 1.000..1.010s、worker-200 链上 #1、9.000ms 累计/8.300ms 有效归因、跨 CPU2→CPU1、10.000ms target sleep、背景 supply pressure 不晋升以及实际占用/规则可消双轴均正确，Trace 因果投影和确定性补采完整，无 4ms 降级。正文仍直接复制 `on_chain`、`state_occupancy`、`effective_attribution`、`dominant_state`、`priority_inversion_candidate`、`priority_or_dependency_supply`、`complete` 等内部 token；与客户指出的 `bounded_window_candidate` 同属 typed 值缺统一读者语言投影，不应逐词扫描答案修补。 |
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260817-235242 | answer_regex | none | 218s | 31 | read=1,repo_map=3,list=0,trace=0,source_lens=2 | midloop=4,inv=1/0,fin_reject=3,unavail=0,prune=0 | partial-system | Python guard/native/fallback、PyO3 wrapper→Rust core→best_merge 与精确定义引用均正确；B1048 没有生产触发，因为 Analyzer 未选择新 `branch_behavior` role，但模型仍自行补齐了 import-error 回退。B1049 被旧 patch 语义抵消：第一次 repair 仅提交 `{id,edge_anchors}`，full-replace 语义清空 chain 的 items/kind/claim/role；删除坏图后 orphan normalizer 再删 anchors，连续 3 次拒绝，最终只剩普通列表，精确关系不可见。确认 B1050，不是模型关系证据缺失。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generalized findings and disposition

- `B1050-SPARSERELATIONPATCHCONTENTLOSS1/P1-high`：精确 typed relation repair 与 full-block replacement 的模型心智冲突。根修只吸收严格的 relation-metadata-only shape（`id + nonempty edge_anchors + optional claim_uses/kind`），保留上一稿模型可见内容；任何正文编辑、删除、未知 id 或 fused block 均维持完整替换语义。系统不生成关系、标签或结论。
- `B1051-TRACETYPEDREADERLANGUAGE1/P1-high`：Trace 主答案与系统附注缺统一 reader-facing value projection，内部枚举会被上下文诱导复制。下一独立批从 typed 字段的 context/render 构造点提供读者词面并把 raw token 留在审计索引；禁止扫描或改写模型最终答案。
- `B1048` 本轮标记为 implemented/not-triggered，不因单次 Analyzer 未选择 role 而用请求关键词硬推导。后续异构 call-chain/conditional cases 继续验证 typed role 选择覆盖。
