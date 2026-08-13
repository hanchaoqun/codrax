# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T01:00:30Z
- sweep_start_ts: 20260812-180029
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260812-180030 | write_apply,answer_regex | none | 144s | 23 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial / honest-unverified | 生产实现、false/0/空串测试、existing-default 负例与 durable applied tree 均正确；`make check` 是 fixture 明示的 Python source-shape 检查，主机又无 Node/npm，因此 `production_verification_source_static_only` 是正确 fail-closed。r412 的探针权威修复已生效：静态 suite 没有再把行为缺证洗成 verified，runner 也不再误报 durable apply ref 丢失。不得为自动 PASS 降低验证口径。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260812-180030 | answer_regex,answer_contains | none | 181s | 29 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=4/1,fin_reject=0,unavail=0,prune=0 | fail | Mermaid 语法合法、源码引用正确，也诚实说明 buildAnalysisIR 不调用 gate.Run；但最终把同一 buildAnalysisIR 函数体内按行排列的 8 个 sibling calls 称为“8 条独立路径汇聚”，混淆了 direct call edge、callee→next-caller 链段、同 caller 的源码有序阶段。探索为 no_directed_path/endpoint 定义共 15 轮，也显示 call reachability 合同与用户所问执行顺序没有分层。自动 oracle 只查图与端点，故假 PASS。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

1. B685/B686/B687 获生产复验：Zod 不再发生“探针全不可用、静态检查却签行为绿”，最终
   applied tree 也能从前序唯一 durable ref 正确物化。当前 FAIL 是能力边界，不是代码交付失败：
   fixture 的 `make check` 明确只做 Python 源码形状校验，环境没有 TypeScript runtime/provider。
   保持 `unverified` 才符合验证红线；本批不修改产品门，也不把静态检查包装成行为测试。
2. 新确认 B688（P1）：call-chain support lane 把每个 `ClaimCallEdge` 都渲染成
   `entry_role=directed_hop`，但没有说明“一条 direct edge”和“可首尾衔接的 path segment”的
   区别。同一 caller 的多个 callsite 因源码行排序，看起来像执行阶段，却被模型解释成多条路径
   在最终 callee 汇聚。typed endpoint capsule 对 `buildAnalysisIR -> RunWith <- gate.Run` 的方向
   本身正确；错误来自 sibling call rows 的关系口径缺失，不是 Mermaid parser 或源码图错误。
3. B688 采用语言中立的软承载修复：模型上下文现只从结构化 Subject/Object/Source/Line 计算
   direct-edge count、callee→next-caller transition count，以及同 caller+同文件 sibling callsite
   group。只有前一 callee 精确等价于后一 caller 才形成连续链；同 caller 多边仅可按源码行展示
   为阶段/callsite，行序不证明所有分支执行、值传递或 callee 之间的路径。跨文件同名 owner
   fail-open，覆盖 Java 多实现、C/C++ 多翻译单元、ArkTS/Cangjie 等同名函数。
4. 该 carrier 不读取 raw request、thinking、答案 prose 或 Mermaid body，不新增 reject，不替模型
   画图或写结论。图中优先使用仓库/业务语言、减少 `typed lane/entry_role/evidence capsule` 等内部
   编排术语只作为软教学；显式 endpoint boundary 继续诚实披露而不伪造到达关系。
5. B689（P1，留档）：本轮 exact no-directed-path closure 为 endpoint existence/body 和集合计数
   消耗 15 explorer rounds。边界查清是必要的，但 endpoint 已 read 后的 definition/call handoff
   仍不够一次完成；后续应从 read closure + parser-owned definition/call carriers减少重复 emit，不能
   放宽 no-path 证据门，也不能用用户/模型措辞硬判。
6. 本批未修改 Trace 查询、因果投影或自动补齐。链上-only 主因、实际耗时/业务线索与规则计价
   可消除量双轴、邻近/背景 support-only 均保持。活跃流只要持续收到字节，4ms 未形成完整答案
   也绝不降级；Reader-level liveness pin 已跨过 stall threshold。合法恢复条件仍只有 caller
   cancel/deadline、no-first-byte、真实 byte-stall、transport/decode failure。
