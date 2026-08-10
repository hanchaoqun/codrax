# CHATFIX-1:寒暄路由双层修复(2026-08-10)

**客户现象**(logs/codrax-9b0dc2c8,qwen3d6-35b 自托管):REPL 输入「你好」→ turn-policy 分类器 10s 硬超时(请求体 33.5KB,该模型首字节 30s+)→ fail-safe 落管线 → analyzer 判 intent/kind=unknown 但管线照走 → 探索阶段拿问候词满仓 grep 6 轮 → 7m42s 后客户 Ctrl+C。日志定谳:`classifier timed out: context deadline exceeded`,且按设计跳过 legacy 回退。设计注释里"最坏情况=浪费一次 LLM 调用"的代价假设被证伪。

## 系统修 A:管线内寒暄短路(第二道防线,零额外 LLM 调用)

- **typed 枚举**:`ScenarioChitchat`(types/analysis_ir.go)+ 枚举表教学条(skill/analysis_contract.go,经 `renderEnumTable` 自动进 analyzer prompt 与 schema 双面);**LLM 分类、禁系统侧关键词匹配**(红线形)。
- **模型自著回复**:`chitchat_reply` schema 字段(教学:仅 chitchat 时必填,1-3 句,用户语言,禁宣称仓库事实)→ `RequestModel.ChitchatReply`(非 chitchat 场景 emit 层即丢弃,精确配对)。
- **短路**:`orchestrator/chitchat_shortcircuit.go`(新文件,避 LOC ratchet):`runTaskGraph` 入口处,门=scenario==chitchat ∧ reply 非空 ∧ 无附件(log/hitrace 附件=用户要分析,veto);发射=镜像空仓短路形(`SetResultPlain(reply)`+IsTerminal+StageFinalize+PipelineEnd)。**回复 100% 模型文本,系统零词**(系统不可代替红线);空回复退化形 fail-open 回普通管线(最坏=现状,永不悬置)。
- **配套豁免(两处,e2e 亲证)**:①degenerate-classification 门(emit 层):寒暄合法零关键词/实体,豁免键=typed scenario+reply 非空(首轮 e2e 见证:未豁免时模型被迫补占位关键词烧一轮);②**budget.Compute 寒暄 carve-out**:kw=0 使乘法预算跌破 `budget_sanity` 硬地板(5 files/4 iters)→ 整 IR 被拒 → analyzer 重试耗尽 → **降级 IR 丢失 chitchat 字段 → 探索照跑**(第二轮 e2e 实锤的隐蔽击穿链);carve-out 直接铸地板值预算——短路后预算无消费者,仅为保 gate 绿。
- **reconcile 守卫**:`reconcileScenario` 首臂——chitchat 不被剥夺、也无任何臂可铸入(单向:只有 analyzer LLM 的枚举choice可铸)。

## 系统修 B:分类器连续超时退避(B2;B1 瘦身按纪律缓议)

- `turnPolicyTimeoutBackoffThreshold = 2`:同会话连续 2 次 DeadlineExceeded → 本会话跳过分类器(`turnPolicyClassifierAvailable` 谓词,一次性可见提示:路由小模型 / 调大 `repl_turn_policy_timeout_seconds` 两条出路)+ 每轮 debug log;任何一次成功归零 streak。非超时错误不计数(自有 legacy 回退车道)。消除"每轮 +10s 空等"隐性税。
- **B1(prompt 瘦身)缓议记档**:turnPolicySystemPrompt 实测 25.4KB + schema 10.8KB;25KB 全是承重路由规则(逐段可溯源既往裁定批),无行为 harness 下裁剪=盲修(「平台不可验证改动不盲修」红线)。前置条件:先建 turn-policy 路由探针 harness(canned 输入 × 前后路由对照),再议瘦身。B2+A 已完全消除客户痛点,B1 边际收益(10-15s 档模型)不抵回归风险。

## 验证

- 单元:短路四臂(命中/空回复 fail-open/附件 veto/非寒暄场景)+ runTaskGraph 接线正向 pin + 所有权 pin(回复零装饰)+ reconcile 守卫 + normalize(`greeting` 不别名进 chitchat,枚举须显式)+ 预算 carve-out pin(含 reply-less 负臂)+ REPL 退避三态(阈下运行/阈上跳过+单次提示/归零复活)。
- **真机 e2e 三轮迭代至判决绿**:①短路通+degenerate 门摩擦 → ②豁免后 budget_sanity 击穿(降级 IR)→ ③carve-out 后零探索直出模型回复(`grep 探索 =0`)。
- 全仓套件绿;eval 冒烟 qf_architecture/logtri_go/trace_short_runnable **3/3 PASS**(零答案代价,覆盖改动面场景族);五镜头对抗复核(wf_9b0a022c-cf5)。

## 客户侧即时缓解(随版本发布前可先发话术)

providers.yaml `agents.chitchat_classifier` 路由到快的小模型(设计本意,启动日志有提示);或调大 `repl_turn_policy_timeout_seconds`;升级本版后两者皆非必需(退避+短路自动兜底)。

## 对抗复核处置(wf_9b0a022c-cf5,五镜头 18 finding → 去重 7 件全修)

| # | 汇合度 | 问题 | 修 |
|---|---|---|---|
| F-A(高) | **5 镜头** | 预算 carve-out 与短路 veto 键不同信号——附件 veto 路径拿着地板预算(4 iters/5 files)跑真实日志分析,静默饥饿 | **源头对齐**:emit 层附件在场即降级 chitchat→generic 并清 reply(附件=要分析),carve-out/短路对附件不可达;orchestrator veto 保留为纵深 |
| F-B(高) | 4 镜头 | 无回复 chitchat 的重试教学教错方向(degenerate 门教补关键词,永不教 chitchat_reply)——T3-2 教学同步类 | emit 层专用拒绝臂:「chitchat 必须带 chitchat_reply,重发或改判分析场景,禁止补占位关键词」 |
| F-C(中) | 2 镜头 | `reconcileDiagnosticQuestionProfile` 是第二 Scenario 写者,先于守卫跑、把 chitchat 覆写成 root_cause 且 stale reply 随行 | 矛盾 emission 定谳=诊断赢;覆写时同步清 reply(配对不散),carve-out 随之不适用 |
| F-D(中) | 2 镜头 | 短路后 answer reviewer 仍带空 FinalAnswer 派发 LLM(零调用承诺被破) | `runAnswerReviewerOnSuccess` 增 ResultIsPlain 精确跳过 |
| F-E(中) | 2 镜头 | 退避不可逆:2 次瞬态超时=全会话丧失 write/data/local 结构化路由 | 每 5 跳一探针轮;成功全恢复(streak/skip/notified 三清零+日志);提示词改为诚实形(周期重试+显式 /write // /mode 出路) |
| F-F(低) | 2 镜头 | 短路双发 EventPipelineEnd(与空仓先例路径差异) | 删短路内 emit(经 runTaskPhase 返回,Run 尾部恰一次) |
| F-G(低) | 1 镜头 | 渲染面 stripAgentLabels 可能改写模型自著回复 | plain 结果渲染 verbatim(系统 fail-loud prose 无标签,旁路行为中性) |

复核后终验:全套件绿+重建 e2e 判决(零探索直出回复)。
