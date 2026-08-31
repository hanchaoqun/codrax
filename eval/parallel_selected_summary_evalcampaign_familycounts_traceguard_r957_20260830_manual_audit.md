# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T05:35:21Z
- sweep_start_ts: 20260830-223519
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-223521 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 163s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 10ms 窗、4 次 typed query、最终 Trace 因果投影、链上第一席、类校验业务线索、双账户与背景隔离完整；无固定时限降级。 |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260830-223521 | typed_inventory_rowset,dimension_substring,answer_contains | none | 264s | 28 | read=3,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=3/0,fin_reject=1,unavail=9,prune=0 | fail | 主计数已改为正确 2/2/8、12 行与 package 也正确；但 summary 把 public abstract/sealed 作为“另有”1+1，且自行声称 public class 分布 5 文件（实际 6），仍有派生计数歧义。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual Findings

1. Trace 人工通过。显式 `34579.490..34579.500s` 窗、4 次 typed query、最终 `Trace 因果投影`、NetworkService 5.951ms 链上第一席、
   目标四态账、实际占时/规则可消双账户、`VerifyClass` 类校验业务线索与邻近/背景隔离均完整；非链 D/IO 未升主因，无 4ms/4m/活动流降级。
2. Cangjie 的 B1470 第一层有效：finalizer prompt 明确收到 `extend=2 / foreign func=2 / public class=8`，summary 的三个主家族计数不再漂移；
   12 条表格、路径、符号、package 与 typed row id 也全部正确。说明“把精确家族计数交给模型”方向成立。
3. 但人工仍判 fail。helper 同时展开一行的全部独立 SurfaceTerms，额外发布 `public abstract class=1`、`public sealed class=1`；模型写成
   “public class 共 8，另有 abstract 1 和 sealed 1”，容易被读成 10 个不同 class，而 Animal/Service 本来已包含在 8 行内。模型还自行写
   “分布在 5 个文件中”，实际是 6 个文件；该 distinct-file 数没有 typed 上游事实，违反既有“不要自行派生计数”软教学。
4. 新 P1 `B1471-CANONICALFAMILYPARTITION1` 根修为与逐行 renderer 单源：顶层计数只调用同一个
   `SourceInventorySurfaceFamilyKey` 单值 selector，因此每条 principal row 恰属一个 canonical bucket，Cangjie 只发布 2/2/8；
   sealed/abstract 等更细修饰仍保留在对应行说明，不再铸成额外顶层成员。canonical counts 之和等于 family coverage；无 typed family 的行只让
   coverage 不完整，不猜分类。
5. 同一软教学明确：该载体只授权这些 row counts；如果没有另一个 typed fact，就省略每家族 distinct-file count 或 modifier total。
   这不扫描或拒绝模型摘要，不修改可见答案，也不删除语言细节，只减少模型需要自行做的集合运算和未授权派生数值。
6. 完整 `internal/agent` 套件通过。B1471 仍需下一批 production replay；runner 的 rowset oracle 对此类自由摘要派生计数仍盲，人工审计必须保留。
