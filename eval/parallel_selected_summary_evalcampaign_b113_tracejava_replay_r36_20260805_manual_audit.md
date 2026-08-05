# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T13:51:56Z
- sweep_start_ts: 20260805-065155
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260805-065156 | primary_answer | none | 138s | 21 | read=5,repo_map=5,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 调用边、容量守卫、Mermaid 方向与五条 invocation anchor 正确，B107 类 participant+qualified operation 修复获得 production 正证；但已读 `AuditLog.record` 的实现仅为 `System.out.println`，答案仍两次声称“写入磁盘/审计日志表”，属于第二次复现的末端名称推断越证。Analyzer 还先以 subtopic symbol unresolved 拒掉已由 relation_map 找到的 `VisitRepository.insert`，白耗一次重分析。 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260805-065156 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 208s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、114.940ms 状态账、双轴、根因排序、唤醒链、自动补齐和系统投影均保留；但模型在 typed `causal_conclusion=unproven`/`frame_evidence_status=absent` 下仍写“直接根因”，把不同 ThreadPool 行的 D-state/IO 数值合成 10.433ms，并把有向唤醒路径升级为“双向强耦合”。上下文已有正确 typed 限界，问题是尾部显著性和后置通用合同冲突；另见同一物理 span 的 original/attached alias 重复行。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## B113 人工审计结论

### Java：图边已复绿，但末端实现语义仍被名称偷换

- 初稿没有 `edge_anchors`，成文门正确拒绝五条 invocation edge；patch 补齐
  `VisitController.create -> VisitService.schedule`、`VisitService.schedule -> ClinicConfig.resolveMaxVisits`、
  `VisitService.schedule -> VisitRepository.countOpenVisits`、`VisitService.schedule -> VisitRepository.insert`、
  `VisitRepository.insert -> AuditLog.record` 后通过。说明 B107 的 owner-qualified operation/class participant 修复在真实 finalizer 路径生效，
  这次重试是模型漏 carrier，不是矛盾合同。
- Explorer 已读取 `AuditLog.java:5-6`，并发射 definition/mechanism 证据：实现是
  `System.out.println("[audit] ...")`。最终正文却写成“写入磁盘”“写入审计日志表”，且只引用调用点
  `VisitRepository.java:23`。调用点只证明调用，不证明 callee 的存储介质；`EVAL-B107-JAVACLAIM1` 从一次波动升级为重复生产 witness。
- Analyzer 首轮 relation_map 已解析 `VisitRepository.insert`，后续 subtopic coherence 却声称 overview 未解析该符号并要求降格到类名。
  这是 `EVAL-B113-ANASYM1`：关系图已证 symbol 与质量门 symbol census 不同源。当前答案仍恢复正确，先列 P1 watch，不用 Java 名称做特例。

### Trace：系统事实完整，模型综合越过 typed ceiling

- 正向：选窗 `34579.472865..34579.587805`、目标状态五分账、实际占用轴、既有规则可消除轴、排名席位、唤醒路径、
  representative windows、覆盖/枚举边界和系统自动补齐全部仍在；系统投影明确写出帧因果未证且未替换模型正文。
- 反向：模型把优先级反转候选直接写成丢帧根因，把不同 row 的 `7.386ms`、`3.033ms` 与
  `d_state=10.433ms,io_wait=0` 交叉解释/相加，并把有向 wakeup/dependency path 写成“双向强耦合”。这些都不是数值缺失，
  而是 model synthesis 没有守住关系/因果 caliber。
- Finalizer 上下文其实已有 `Trace Decision Inputs`、`causal_conclusion=unproven`、`frame_evidence_status=absent`、
  `cross_row_additivity=not_authorized_without_exact_pair_carrier` 和 wakeup 语义；但后面仍有 Submission Checklist、multi-topic 等通用结构段，
  “最终主值”又只重复数值而不重复因果/关系边界。另一个明确冲突是 runtime-only disposition 要求不用 repo citations，multi-topic 尾部却无条件要求
  “Provide citations for each section”。

### B114 泛化修复（本回放后施工）

1. 在整个 finalizer 动态上下文末尾追加 typed `Final Trace Decision Boundary`：只重申最终 causal/frame ceiling、双轴用途、
   exact relation 才能跨行加法、有向路径/holder/overlap 的权限边界；明确结论、优先级与优化方向仍由模型决定。
2. 对 QFCallChain 在尾部追加语言无关的末端实现边界：call-site 只证明 edge，callee 的 body/副作用/存储介质/同步语义必须来自独立 grounded
   definition/mechanism 证据；类名、方法名、注释、层名和请求措辞均不铸造行为。
3. runtime-only multi-topic 改为 artifact provenance per section；只有独立 typed current-source anchor 才要求 repo citation，消除同 prompt 自相矛盾。
4. 只有 typed trace source 才发 Trace decision handoff。纯日志 observation ledger 不再收到空 Trace 合同。

全部是 typed-input soft guidance；不扫描用户请求、模型思考或最终答案，不 hard-reject/normalise/替写结论，也不修改显式时间窗、Trace 查询、
系统自动补齐、因果投影或双轴计算。

### 开放项

- `EVAL-B113-TRACEALIASROW1`（P1，next）：系统投影已按 artifact family 合并，但实际占用/证据行仍能同时出现 original path 与 attached alias 的同一
  物理 span，需审计 row identity 去重是否在某个载体层又混入 path/line identity；不能按文件名字符串删除。
- `EVAL-B113-ANASYM1`（P1 watch）：relation_map 已证方法符号却被 analyzer subtopic census 判 unresolved；再跨语言复现后按同一 typed symbol authority 收敛。
- `EVAL-B107-JAVACLAIM1`：B114 soft boundary 已施工，需异构 call-chain replay 验证，不新增答案关键词 gate。
