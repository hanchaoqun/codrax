# r1061 人工审计：Python 跨仓写与 H7 显式窗 Trace

- date: 2026-09-12T07:00:30Z
- sweep_start_ts: 20260912-000029
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

本轮在 `7e74a205c` 提交推送、冻结全仓 86 包及前后置构建通过后运行。二进制 revision=`7e74a205c635`，built=`2026-09-12T06:59:57Z`；严格并行 2、每例一次，07:00:30Z–07:06:19Z 结束，无第三例/追跑。原 case、oracle、1200s 预算及模型答案未改。机器 PASS 只代表声明的 oracle 通过，不代替人工正确性。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_tokenizers_newline_run_multirepo_py | PASS | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260912-000030 | log_regex,write_apply,answer_regex,answer_contains | none | 242s | 28 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 单个换行也被折叠；真实探针失败但反例未充分传递，另复核 impact 逐合同升权旁路 |
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260912-000030 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 348s | 58 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=1/0,fin_reject=3,unavail=1,prune=0 | fail | 显式窗/两轴/投影/旁路保留；模型擅自加总并过称反转，早期 raw 单位与代码索引附注另有供给缺口 |

## 1. Python：补丁、执行事实与验证权限分别判定

1. 需求只允许修改 Python 回退实现：有 `(10,10,rank)` 时，长度至少 2 的连续换行先折叠，再继续原 BPE；单个换行和非换行行为不变。不得删除原五换行测试或修改 native 分支。
2. 正式计划 `plan-1789196629309416000-53974`，实际隔离提交 `b2f668722cb50fb791bcf94d337f4e8b26f65e88`。`run-1.applied-tree/fastlex/tokenizer.py:24–54` 缺少 run 长度判断，每个单独的 byte 10 也执行 `folded.append(rank)`。最终双份计划/快照同一补丁，后续未修复这个边界；不存在测试或 Makefile 被删改来追绿。
3. 原保存的模型探针确实执行了三个检查，第三个 `a\nb` 报 `Expected [97, 10, 98], got [97, 300, 98]`。原 apply 日志 `run-1.logs/codrax-20260912-000349-000-54403.log:712` 附近的进程 exit=1。新的 B1575 观察器记录当前完整源码/commit/owner/实际执行行，但因外层失败不授 target_execution 或 behavior；不能把本轮错误说成“观察器改失败为成功”。
4. 后续 Make check 成功，pytest 环境缺失后回退原生 unittest，实际原有两个测试通过。最终 report 的 3 条通过结果来自 Make 聚合与 2 个原生断言，`VerificationConfidence` 空；通用 `probe_comparator_authority / model_authored_probe_comparator_unverified / observed_failure` 警告仍在持久化报告中。旧策略允许未核实的模型比较器失败后继续项目 suite，这个权限边界本身正确：模型可能写错期望值，不能一律硬判产品缺陷。
5. **非权威失败信息传递缺口确认。** controller 实际输入原日志 L853–858 是 3 passed / 0 failed、当前 proof state=verified、probe 命令裸 exit=1；缺少具体 AssertionID 与失败详情。优先上下文 L868–890 多为审批/规划项，另 41 项被省略，通用警告不在可见部分。源码 `modelProbeComparatorDiagnostics` 只保通用解释、不保原 `TestResult.FailureDetail`；pre-suite 消费后失败行不并入成功 suite 的 TestResults。不能说“毫无失败证据”，也不能把已有准确反例的丢失全归模型波动。下一片应持久化并有界前置该次探针的身份、失败内容、原始引用及比较器尚未核实边界，供模型判断修代码还是修检查；不篡改 suite 终态、不硬化模型断言、不替模型做结论。
6. **逐合同权限另案复核，不与前项混淆。** L854 明确 required typed=0、planning-only=9，不是漏算九条硬合同；但 `.final.json` 的 impact 行仍把 source=verification_probe 的 c1/c2/c3 标成 verified/covered。`impactCoverageForTarget` / `patchReviewCoverageForFinding` 的文件级 target_behavior 加“没有记录 missing”可授逐合同 verified，缺少对应 ref 的正证要求。即使规划项不构成硬义务，也不能由“未设硬门”推出“合同已实测”；正在以公开入口区分规划 advisory、原生路径覆盖和 exact assertion 正证。此项是 B1575/B1561 未闭消费域，不冒称首片已彻底关闭所有行为权限。
7. 独立后验 `.codrax/tmp/20260912-r1061-python-posthoc.log` 在已交付树重跑原保存探针及 54 个边界输入，49 通过、5 失败，失败集中于 singleton 和 singleton/run 混合；rank=0/300 都命中。Python 3.9.6、native=False，正式代码/测试/Makefile 前后 SHA 不变。日志明确 `not_product_proof`，不能倒填正式项目证明，不能修改评测原产物或将该业务补丁合入主仓。
8. JSON/规划过程：先有越界 end_line，再有 insert_after 放错方法域/重复替换；最终合法 patch 被接受。未发现同一结构同时“必带/必拒”的系统合同矛盾，本轮不新增针对换行文本的 JSON 教学或硬规则。最终交付“已验证”不能替代人工失败结论。

## 2. H7：正确数值/投影保留，模型总结仍失败

1. 原始日志 `run-1.logs/codrax-20260912-000032-000-53939.log`；最终 `.codrax/output/20260912-000616.380-53939.md`（以下 MD 坐标均指此文件）。只分析 trace，不读取客户代码，5 次 trace 查询；目标 CompThread_0-2955、13762.791708–13763.024898 秒、233.190ms 保持一致。
2. MD L60–89 实际占时/业务 span，L91 起 Trace 因果投影及可消除榜均存在；running=74.915ms、供给折算=65.912ms、D-state=36.757ms。D 共 11 段、最长 3.853ms；不能误把另一面按 CPU 汇总的 6.495ms 当单次最长。`dma_fence_default_w` 仅为内核等待调用点，不是资源对象/持有者身份。状态墙钟 running74.915/runnable1.576/sleep118.586/nonIO-D36.757/io_wait0 分账保留。
3. 链上/背景未被投影混加：例如 logd.writer 总 49.656ms 拆为锚定 0.033ms 与背景 49.623ms（MD L353）；背景另列，业务 span 给排查线索，不冒充已证可消除总量。默认 `.root-causes.json` schema2/status=available，有模型选择的 12 项及精确窗；非系统自选补根因。
4. **模型总结不正确。** MD L15 自造“同方向其余 6.0ms”和“反转候选约12.4ms（7线程）”；最终上下文 L2588–2609 已准确给出同方向不可任意相加、只有 #4/#5 互斥小计 5.324ms、其余席单列。七项直接数值相加也只是 11.577ms，更不代表可授权合计。MD L24–27 同时写“反转等待全额”和“候选不证明发生反转”，措辞自相矛盾。精确信息在成文前已提供；此类错误保留模型质量观察，不恢复算术评价器/关键词拒绝/系统改写答案。
5. 成文有 3 次拒绝、1 次 patch、1 次 unavailable：table 块混入 summary 专属字段；调用尚未开放的 patch 工具；末轮新 emit 缺必需 summary。schema 与拒绝一致，上一版已接受的结构化正文/投影没有因此被清空。未发现相互冲突合同，但结构教学仍按通用最小负担方向审计，不能为本次名单另设硬门。
6. 本题未产出 Mermaid，仅使用文本因果树和表格，Mermaid 验收 **N/A**。不能以机器 PASS 或文本树存在声称图解析/render 全链已验收。

## 3. 上下文与显示残余（归并既有工单）

- **B1598 单位供给扩展，P2。** event_search 的 raw `blocked_delay=3213` 为鸿蒙微秒字段，代码已有 /1000 换算。原 payload blob `1aa63d8a` 只给 timestamp 的 seconds 与 trace 方言，没有该 raw 字段单位；explorer L956/L967 将它误读成 3213ms。后续 root rank/成文上下文纠正，总答案未沿用错误，但探索输入确有单位 gap。应提供 raw 字段单位/换算口径元数据而不修改内核原始文本，不按单个数值拟合。
- **B1402 附注适用域扩展，P2。** MD L1119–1121 系统“枚举标签核对”把真实线程 RSUniRenderThre/daily_control 送去仓库声明索引核验，造成不相关警告。`repair_caveat_materializer.go` 的代码索引附注需绑定实际证据类型/目标域；不能按线程名字白名单抑制，也不能把所有枚举附注删除。范围缺口已确认，具体发射资格闭包另批。
- **B1263 表格规划残余，advisory。** MD L19–21 等表只有“项目/列2…”且 #1 重复。接受的 t1 无 columns、row.label 与 cells[0] 重复；t2/t3 把表头塞成 item。系统使用中性 fallback，没有删关系或改模型结论。沿用既有通用表格结构教学/元数据问题，不造新病例硬规则。

## 4. 后续顺序和不可回退边界

1. P1：先核闭 B1575/B1561 impact/patch-review/最终 ledger 的 per-contract 正证来源，规划-only 不升级 covered，真实 native assertion 正控必须保持；公共先红后绿，不能只补标签。
2. P1：失败探针的非权威具体反例供给；与上述权限修复分开验收，禁止把模型 comparator 默认升级 hard failure。
3. P1：B1626 请求多成员时间窗入口→补采→账户→投影闭包仍未施工；单窗 H7 通过投影不代验双窗。
4. P2：B1598 raw 单位、B1402 Trace-only 索引附注范围；B1263 和已给准信息后的总结波动不抢占以上代码来源问题。Gradle/Meson 的真实原生环境待验债不虚签关闭。
5. 当前 active-stream 专项已通过（52.453s、count3），首响应600s/中途真实静默300s/非流600s保持。没有固定4ms/旧4分钟活跃连接总年龄降级门；独立 caller 取消/预算与真实静默超时仍有效。本轮 live 非十分钟客户等待验收。

审计结论：机器 2/2 PASS；人工 2/2 FAIL，各自失败轴与保留能力如上。原答案/补丁/机器报告未改，独立后验不回填产品证明，无第三路追绿。
