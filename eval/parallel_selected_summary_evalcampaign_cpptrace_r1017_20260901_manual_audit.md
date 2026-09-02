# r1017 人工审计：显式窗有限事实与 C++ 工厂/虚调用（2026-09-01）

- date: 2026-09-02T04:22:42Z
- sweep_start_ts: 20260901-212242
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

基线 `main@50d8d7db6`，固定二进制，恰好 2 路并行。runner 2/2 PASS，但下面人工判定独立于字符串 oracle。
已阅读两份最终 Markdown、探索/成文实际上下文和工具结果，并对照 C++ 源码；未改 eval 期望、模型回答或固定运行预算。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | PASS | eval/results/real_trace_h4_supply_thermal_witness-20260901-212242 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 93s | 33 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail（主值正确，附加归因/窗别有越界） | 四态账、完整 8 CPU 清单、上限存在与影响未证分离正确；S 被称主动，同窗被称同帧，桶代表频率在结尾被升级为目标观测 |
| 2 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260901-212242 | answer_regex,answer_contains | none | 154s | 30 | read=8,repo_map=0,list=0,trace=0,source_lens=0 | midloop=5,inv=3/0,fin_reject=3,unavail=0,prune=0 | fail | 主干和工厂选择在场，但列表显示哈希节点 ID；时间戳/flush 作用被模型凭空补充；锚点修复反馈误导 |

## 1. Trace：精确信息已到模型，主要残余不应以硬门拟合

- 最终工件：`.codrax/output/20260901-212413.555-41742.md`；日志：`run-1.logs/codrax-20260901-212244-000-41742.log`。
- 实际最终上下文 1824 行附近提供完整 8 CPU 运行清单；1830–1853 附近自然语言事实卡给出 157.248/5.604/70.338/0.000ms、合计 233.190ms，
  D/IO 的窄口径、独立 IO 完成闭合尺 4 次/4.384ms（覆盖未知）、CPU0/CPU4 策略记录、每 CPU 桶代表频率与切片绑定未证。
- 模型探索中曾把总和算错、把 558000kHz 写成 558kHz，只看 CPU12/CPU4 两行；最终答案纠正为正确总账和 8 CPU 清单，未把两条 CPU 桶相加当总运行。
- 成文仍将 S 说成“主动进入”，将“同窗”写成“同帧”；结尾说“目标线程在 CPU4 上观察到…558000kHz”，强于桶代表值提供的目标绑定权限。
  事实卡明确不支持这些升级，归为模型事实理解/措辞残余，不能扫描答案词语后删改结论或再加拒绝合同。
- 本次 analyzer 声明 `bounded_fact_set`，仅请求状态/持续时间/频率维度。没有主根因/因果链请求，不强制加因果投影是正常分工，
  不是有时间窗就必须套根因合同。本次不作为“显式根因投影正向回放通过”的证据。
- 默认 `.root-causes.json` 已生成 131 字节合法 JSON：`root_causes=[]; status=unavailable; reason_code=trace_root_cause_contract_not_active`，
  正确区分未启用根因合同与查明无根因；没有零字节伪结果。
- 4 次 trace_query 中 event_search 将 pid 与 CPU 全局频率事件合用而被拒绝一次：线程过滤匹配的是事件发出者，不是 CPU 状态所有者，
  不能用它制造 matched_events=0。工具明确提示保留时间窗、移除 pid/thread；event_types 数组本身受支持。已有精确频率证据在其他结果中，
  未出现持续工具不可达或强制补采；这是既有精确口径防线正常生效，不新立硬门。

## 2. C++：两处系统输入/反馈 gap 与模型残余分开

- 最终工件：`.codrax/output/20260901-212515.064-41744.md`；日志：`run-1.logs/codrax-20260901-212244-000-41744.log`。
- 工厂 create(kind) → console 分支 → make_unique<ConsoleSink>、Logger 构造注入、Logger.log → Sink.write 静态前缀、ConsoleSink.write 的
  fputs/fputc 实现均已读到并以 typed 证据交给模型。模型可用列表回答，未要求必须画 Mermaid；本次没有图，不记“图被系统删除”。
- **B1553/P1 确认**：首稿列表已选 call 证据但缺 edge_anchors；producer 给列表也发了 Mermaid-safe `from_node_ids/to_node_ids`。
  日志 3540 行附近模型明确称应从这些数组选择 node；最终 6 条关系行显示 `LoggerLog_188cb2a568e4047d` 等。
  `preEmitStandaloneRelationClaimRepairDeltaJSON` 调用 `bindDiagramRelationRepairCandidateTechnicalNodeIDs`；
  `exactLocalStandaloneRelationMetadataAddBranch` 又把此操作描述为 hidden anchor，字段缺少“将直接显示给读者”的解释。
  执行器/renderer忠实保留模型值，问题源头是把 Mermaid 语法身份与列表读者标签当成同一合同，不应用输出替换补救。
- 同批修复方向：producer 按 typed carrier 区分 diagram node id 与 list/table reader label；非图分支不发无关的语法哈希候选，schema/错误示例明确
  `edge:{from_node,to_node,visible_label}` 的嵌套和读者含义。图分支的已有节点复用/语法身份仍保留；关系、证据和可见文字继续由模型选择。
- 3 次成文拒绝：首稿缺列表锚；首次 patch 将节点标签放在错误层级且缺 edge；第二次仍把 from_node 放在 edit 顶层；第三次嵌套正确后成功。
  当前 strict decoder 最终能给正确嵌套路径；首个 atomic error 只说“edge with from_node…”而非具体 JSON 路径，应与 schema 同源减少心智。
- **B1554/P2 确认（需扩大语言矩阵验证后施工）**：同一已读 `return std::make_unique<ConsoleSink>();` 上轮换限定名/裸名/完整调用锚仍失败。
  `looksLikeCallSyntax` 只尝试 callee 紧邻 `(`，graph target 比较未统一实例化参数形；最终 `explainUngrounded` 无条件归到“whole-word token not found”，
  即使失败实际位于 call-shape/目标校验。不能把“符号出现”直接升级成调用证明，也不能靠名称白名单放行；应复用解析器 typed call 表达式/精确目标，
  并拆分“词法存在、位置可读、callee/表达式不匹配”的诊断。要同时覆盖 C++/Rust 泛型调用与项目其它支持语言，保留错误 callee 的 fail-closed。
- 模型独立残余：声称 log 拼接时间戳（实际只有 level_label、空格、message）；声称 console flush 强制刷出（ConsoleSink 没覆盖 flush，基类实现为空）；
  用类定义行支持具体分支解释。均不应通过系统改写答案修正。工厂和构造函数存在不等于已找到实际入口连接，两阶段描述应保留这一来源边界。

## 3. 本批收账与后续优先级

- B1552 修复 `50d8d7db6` 已推送；全仓测试通过（tool 221.604s、types 37.251s、tracequery 91.422s、tracediag 14.376s），make 通过。
- 本次没有提交无效 receipt，不能据此声称 B1552 已取得真实模型“减少重试”的对照结果；其同轮反馈/补丁修复已有确定性先红后绿证据。
- 下一开发批先 B1553（跨图/列表/表格载体语义与 JSON 修补教学），再 B1554（泛型调用证据和真实失败诊断）。不按 C++ 符号名/模型原文做硬门。
- 后续 eval 继续恰好 2 路：显式因果根因 Trace + 写模式异构场景；本轮机器 PASS 但人工不全对的答案原样保留，不降低 oracle 或延长重试凑绿。
