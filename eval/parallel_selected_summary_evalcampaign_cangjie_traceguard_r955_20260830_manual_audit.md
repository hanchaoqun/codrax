# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T05:19:15Z
- sweep_start_ts: 20260830-221914
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-221915 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 146s | 36 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 10ms 窗、typed query、最终 Trace 因果投影、NetworkService 链上第一席、实际占时/规则可消双账户和背景隔离均完整；无固定时限降级。 |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260830-221915 | typed_inventory_rowset,dimension_substring,answer_contains | none | 220s | 28 | read=0,repo_map=5,list=0,trace=0,source_lens=5 | midloop=2,inv=4/2,fin_reject=0,unavail=6,prune=0 | fail | 12 条 typed 行及 package 全部正确，但摘要声称 2 foreign + 3 extend + 9 public class，与表格实际 2+2+8 和总数 12 自相矛盾；runner 只验结构化行集而误判绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual Findings

1. Trace 人工判定通过。目标显式窗为 `34579.490..34579.500s`，最终保留
   `NetworkService-60595 -> CookieMonsterCl-59843 -> com.baidu.tieba-59566` 已证链、5.951ms 第一席、目标状态账、实际占时/规则可消双账户、
   确定性语义工作线索和邻近/背景隔离；没有把非链 IO/D 或邻近压力提升为主因，也没有按 4ms、4m、轮次、上下文比例或活动流年龄降级。
2. Cangjie runner 虽 PASS，人工必须判 fail。typed principal rows 精确包含 2 个 `foreign func`、2 个 `extend`、8 个 `public class`，12 行的
   路径、符号、`package` 与行数均正确；模型却在 summary 写成 2/3/9，同页合计 14 又声称总数 12。表格正文保持 2/2/8，故这是摘要算术漂移，
   不是提取器漏项或 Cangjie 解析错误。
3. 根因是上下文减负边界过度：系统把正确的三个 model-authored aggregate 值降权并隐藏为
   `value_omitted=shadowed_by_authoritative_principal_rows`，只给 finalizer 12 条 typed 行和总数 12。该设计正确阻止错误旁路候选与主清单竞争，
   但也迫使模型自行按 `surface_family` 重数；模型违反既有“不要自行计数”教学。runner 的 typed rowset oracle 只验 12 行，不验自由摘要里的自报计数，
   所以形成“结构绿、用户答案错”的验收盲区。
4. `B1470-INVENTORYFAMILYCOUNTCONTEXT1/P1` 的泛化修向不是扫描或改写最终摘要，而是从每条 principal row 的 typed `SurfaceTerms` 机械计算
   `typed_surface_family_row_counts`，和主清单一起交给 finalizer。独立 family 在单行内去重，允许一行属于多个 family，因此明确“不保证分类数相加等于总数”；
   无 family 的行只降低 `family_coverage`，不猜类别。模型继续决定是否以及如何在答案里陈述计数。
5. 修复不读取请求原文、模型 thinking、最终答案、Markdown 表格或路径关键词，不恢复被降权 aggregate 的数值，不新增正文硬门，也不让系统选择成员、
   分类、措辞或结论。定向 family-count 测试与完整 `internal/agent` 套件通过；需要下一次异构 inventory production replay 才能转正。
