# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T15:18:38Z
- sweep_start_ts: 20260831-081836
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-081838 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 162s | 38 | read=3,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、typed query、链上根因排序、目标四态账、实际占时/规则可消双账户、背景隔离与系统 Trace 因果投影都完整；但模型自有主回答只讨论调度链，完全漏答用户明确要求的 VerifyClass 工作身份、0.285ms、直接唤醒凭证及“仅关系已证/完成因果未证”边界。系统投影随后正确补充这些事实，但不能替代模型结论。Analyzer 最终只发 causal_contributor_set/evidence_source/count，没有 runtime_work_relation；0 final reject 说明这是完成合同缺口，不是模型被校验拒绝。 |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260831-081838 | typed_inventory_rowset,dimension_substring,answer_contains | none | 283s | 29 | read=10,repo_map=3,list=0,trace=0,source_lens=3 | midloop=5,inv=5/2,fin_reject=1,unavail=12,prune=0 | pass | 最终完整列出 2 个 extend、2 个 foreign func、8 个 public class，共 12 条；名称、文件和 package 与 typed inventory 一致。唯一拒绝是模型首稿把“种类”放在表格第一列，row-id 合同要求成员身份为第一格；交换列后通过，未发现系统权威冲突或事实丢失。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Trace deep audit

- 最终模型自有答案：`.codrax/output/20260831-082118.715-69612.md`。模型只给 NetworkService/CookieMonster/T7 的调度链与排序，没有回答确定性工作关系子问。
- typed/system surface 已有完整证据：`VerifyClass com.baidu.zeus.mml.lac.LacUtils`、宿主 `T7@ZeusThreadPo-61839`、原始时长 `0.285ms`、`34579.496810s` 的直接裸唤醒边，以及“仅关系凭证；语义完成/目标等待绑定未证；规则可消 0.000ms”。因此不是 Trace 数据缺失。
- Analyzer `emit_analysis` 的 `requested_answer_dimensions` 只含两份重复 causal contributor 语义及 proof/count，没有 `runtime_work_relation`。既有 B1474/B1475 软教学连续多次不能保证角色落地，升级为 `B1495-RUNTIMEWORKTYPEDDECLARATION1/P1`：在 `runtime_question_profile` 增加每次必答的 typed 布尔分类位；为真时直接激活模型自有 runtime-work 回答义务，不要求再复制一条展示维度，也不产生系统结论。
- 修复不得扫描请求、thinking、最终 prose 或 Mermaid 标签，不得用系统 Trace 投影覆盖、删除或替换模型回答；投影仍只提供精确事实和边界。

## Cangjie audit

- 最终答案：`.codrax/output/20260831-082318.631-69601.md`。
- 结构化结果为 12/12，`extend=2 / foreign func=2 / public class=8`；没有 modifier 重复计数、装饰显示名重复或跨文件误合。
- 一次表格列序修补属于模型格式波动；既有 row-id 合同正确阻止“种类”占据成员身份列，没有新增 hard gate。
