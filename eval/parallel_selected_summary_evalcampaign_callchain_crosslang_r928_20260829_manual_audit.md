# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T06:50:28Z
- sweep_start_ts: 20260828-235027
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260828-235028 | answer_regex | none | 172s | 27 | read=6,repo_map=5,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | `main -> run -> collect_files/walk -> index_file -> Matcher.is_match` 与两种实现均有源码证据；walker 的目录遍历/过滤/递归/文件收集职责准确。sequenceDiagram 关系完整；首稿因关系锚精度被拒后仅校准锚，不丢主链。B1441 的 `discover_terminal` 与跨语言逐跳操作边界均进入 Explorer/Finalizer，未压缩 Rust 关系。 |
| 1 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260828-235028 | primary_answer | none | 173s | 29 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=3/0,fin_reject=1,unavail=0,prune=0 | fail | 六条 typed call、容量 guard、`AuditLog.record -> System.out.println` 与 Mermaid 都完整；但终稿仍把仅执行 `rows.add` 的 `VisitRepository.insert` 称为“持久化”，也未明确标准输出不等于数据库/持久化。B1441 已正确归一 endpoint mode 且教学三次到达，残余由 Finalizer 内 `member_notes`“候选/证据上限”与“保留细节”自冲突稳定触发，确认 B1442。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Audit Findings

1. 本轮使用已推送 `d80072e1e` 重建的程序严格并发恰好 2 路。Rust runner/human 均 PASS；Java runner/human 均 FAIL。两路均正常等待活跃模型流，不存在固定 4ms、4m、首字节或上下文比例降级，也没有旧稿恢复、系统代写答案或系统创建关系。
2. B1441 获得跨语言生产正证：Java 与 Rust 的一源空终点都由 typed `runtime_selection_profile=false` 归一为 `discover_terminal`；共享 observed-operation/effect 边界进入两种语言的 Explorer 与 Finalizer。Rust 的完整关系和业务角色没有因此丢失。
3. Java 失败不是 endpoint、parser、关系、Mermaid 或证据缺失。最终证据池已有 `rows.add(...)` 和 `System.out.println(...)`，六条调用边及容量检查位置完整；答案仍把内存追加称为“持久化”，并沿用“审计落库”的请求概念而没有披露标准输出边界。
4. 新 P1 `B1442-AGGREGATENOTEAUTHORITYCONFLICT1` 是精确合同自冲突：同一 Finalizer 上下文先声明 aggregate facts/`member_notes` 是 `advisory_model_inference`，且 `member_note_support_authority` 是行为主张上限；随后 source-operation contract 又无条件要求保留 `member_notes` 的逐成员细节。后一条把未经函数主体证明的模型描述重新抬升，压过了 B1441 的逐跳真实操作教学。
5. 最优根修是统一 typed 权限而非新增词面门：成员 identity 与每行 support location 必须保留；`member_notes` 只作候选描述，每个行为/效果必须服从该行 `member_note_support_authority` 与已接纳 grounded operation。若证据只支持更窄操作，模型应陈述真实操作或披露边界。系统不得扫描请求、thinking、备注或终稿寻找“落库/持久化”等词，也不得删除备注、替写结论或改图。
6. Rust 首稿出现一次关系锚校验失败，但补丁只校准已有关系元数据，最终主链和 walker 角色完整，不构成新的结构性 GAP；继续观察跨图族的重试率，但不为本样例增加 case-specific 放宽。

状态：

`r928=runner-1/2-pass+human-rust-pass+java-fail`；
`B1441=production-positive/core-closed`；
`B1442=confirmed+implemented/pending-full-suite-and-replay`；
`member_identity+support_location=preserve`；`member_note_effect=typed-support-ceiling`；
`request/model/final-prose-hard-gate=forbidden/none`；
`system-answer/conclusion/relation/node/label/layout-authorship=none`；
`Trace explicit-window/causal projection/auto-supplement=unchanged`；
Trace root=`typed-on-chain-only`；adjacent/background=`support-only`；
`active-stream-4ms-or-4m-degrade=forbidden/unchanged`。
