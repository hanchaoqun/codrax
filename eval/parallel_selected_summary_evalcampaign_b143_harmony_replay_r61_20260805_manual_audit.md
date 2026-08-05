# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T23:43:18Z
- sweep_start_ts: 20260805-164315
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260805-164318 | typed_inventory_rowset,dimension_substring,answer_contains | none | 99s | 20 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 2 extend、2 foreign func、8 public class 均完整；Cart extend 恢复，三类各自保留文件与 package，Finalizer 1 轮、0 reject。 |
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260805-164318 | typed_inventory_rowset,answer_contains | none | 139s | 20 | read=5,repo_map=3,list=1,trace=0,source_lens=3 | midloop=3,inv=4/2,fin_reject=0,unavail=0,prune=0 | pass | 最终 4 个 @Entry + 2 个 @Builder 完整，文件/行号列均在；但 Analyzer/Explorer 被矛盾的 file-role refinement 带入伪 absence，Explorer 11 轮、completion 2 reject、emit_evidence carrier 2 reject，见下方 GAP。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case audit

- `EVAL-B142-TABLEROWSHAPE1`：production replay closed。ArkTS 的多列表格保留函数名、文件路径和行号，不再静默压掉文件列。
- `EVAL-B142-CJFAMILYCARRY1`：production replay closed。Cangjie 的三个 typed family 均独立成表，`Cart @ Cart.cj:30` 未再被标题派生的错误 family scope 删除。
- `EVAL-B143-SILENSREFINE1`（P1）：真实系统合同矛盾。`repo_map(source_inventory, roles=[file])` 的拒绝建议在 Analyzer 中要求
  `list_files recursive=true`，同一阶段随后又硬拒递归 list_files；Explorer 的 scheduler-owned 工具面也没有 list_files。模型遂把 overview 的
  “no exact production hit”错误升级为全仓 absence，并连续两次提交不成立的 `negative_observation`。根修应从 typed request role / exact
  attribute role 生成同工具 `repo_map(source_inventory)` refinement，只有没有语义角色且工具可用的真正文件族发现才建议 list_files；overview
  caution 只能是导航提示，永远不能证明全仓 absence。
- `EVAL-B143-EVIDJSONMIND1`（P1 efficiency）：Explorer 第一次把 `items` 发成包含语法破损的 JSON string，不能安全自动恢复；第二次 native
  array 已合法，但把每行相同的 `source:line` 又放进 item-local `support_refs`，烧掉一轮。通用方案是：教学在工具入口一次说明 native-array
  与字段所有权；运行时仅当所有 support_refs 精确重复该 item 的 typed source:line 时无损折叠，任何跨行/额外关系仍 fail-loud。不能从任意
  畸形字符串抽词拼证据，也不能把 support_refs 静默移到语义不同的 aggregate fact。
- 两份最终答案均为模型一次成文，系统只补引用摘录披露，没有替换结论。Trace 显式时间窗、因果投影、自动补齐、两维根因均未触碰。
