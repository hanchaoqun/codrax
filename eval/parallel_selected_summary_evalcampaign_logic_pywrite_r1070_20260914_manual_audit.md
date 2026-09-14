# r1070 人工审计：参与者接边不等于数据流完整，补丁正确不等于全部原生验证

- 基线：已推送 `13ee5e072d54`，清洁构建 `2026-09-14T09:46:28Z`；不可变快照 `codrax-selected-20260914-024652`。
- 纪律：243 例库存（215 read / 25 apply / 3 plan，Trace 29 为交叉维度），按用户影响、权限风险、关系欠缺、最近覆盖和本机可执行性选本对。并发恰好 2、各 1 次，CAP5 / TIMEOUT1200，没有第三例或重跑追绿。
- 读例上次 r920 有 7 次成文拒绝，旧外层预算 1800s，不能声称同预算比较。单仓不受 CAP 多仓缩减。机器汇总保持原样，人工审计不回填产品证明。

| 用例 | 机器 | 人工 | 关键结论 |
|---|---|---|---|
| qf_logic_view_read_pipeline | PASS，外层666s / case663s | 完整需求未通过 | 图可渲染、参与者6/6，但主数据流缺失且正文两处错误；37探索轮、11成文轮、10拒绝 |
| github_issue_dateutil_relativedelta_float_symptom | PASS，192s | 补丁通过；正式验证有边界 | 原测试未改，独立后验通过；产品只跑动态探针，原生套件策略跳过 |

## 读例：渲染、技术连通和主数据流分别计量

原产物 `.codrax/output/20260914-025756.225-82645.md` / 同名 HTML；原过程 `eval/results/qf_logic_view_read_pipeline-20260914-024653/run-1.logs/codrax-20260914-024654-000-82645.log`。

1. 实际仓内 Mermaid 引擎离线解析/渲染：原 MD 一张 flowchart-v2，SVG17755字节；HTML同源、SVG18475字节、无页面错误。没有改 fence、补图或改原文件，本轮不是语法失败。
2. 最终图第13行匿名n10，21–28行四个空stage分组。六条边是三阶段顺序、一条 `bus.Mutable → AgentContext.Mutable` 共享指针赋值投影、两条提取资格预检 `BuildAgentContext` 的参数传递。后两条输入是 `o.busCtx` 与 `types.AgentExtractor` 枚举，不是Extractor产物输出。参与者接边6/6/0不证明主数据流完整。
3. 真实主链为 orchestrator 构造 AgentContext → 执行 agent → applyStageOutput 合并阶段输出。builder共享Mutable指针并复制EvidenceItems/AnswerChains/AnswerSymbols；Explorer还直接写TurnAArtifacts。四阶段是概念顺序，extract可跳过，不能说每轮固定4调用。本轮取证集中在辅助 `extractStageHasRequiredWork`，缺主链生成/写入/合并/消费关系集合。
4. 正文43把确定性编译写成“非确定性编译”，60把Explorer产生的TurnAArtifacts归给Analyzer。两处在首次成文log5664已存在，最终patch6361只补summary未纠正。原模型错误不由系统删改；也不扫描正文新增硬拒。
5. B1682：log1350以来源/载体/ERM广度称所有证据要求满足，1396已退回关系修复，1452/1548仍用旧就绪latch催完成。首轮六次退回为参与者关系3次、值接收方2次、阶段职责1次。原门拒绝有实际依据，不能全归模型波动。
6. B1683：defaults.go旧411行（本轮log4587）声称角色箭头不受约束，label放file:line给整个label授证；同轮精确关系合同与业务显示教学却要求关系独立证据、定位放citation/typed载体。它是通用教学矛盾，不是单语言问题。
7. 后继审计：log5378的 `proved_through_typed_participant_carrier_join` 只证明技术连通，5380又说跨分量值交接未证，应拆清结构覆盖与产物交接覆盖；log5411/5412同一源码调用用短名/限定名发两条recipe。先公共复现，再从精确operation身份/来源解决，不补模型想要的边或背景根因。

## 写例：补丁和正式证明分开验

1. Plan `plan-1789379371983512000-82658`，seed6869f326b987→applied168258e3e243db1550098b755fa61a621c2fb829；物化tree三个blob逐一匹配，仅构造函数+11行。whole float转int，finite fractional拒绝；`__radd__`、原4测试、README未改。
2. 早期分析误归divmod，正式计划log2232已由模型自纠为date.replace收到float，不把早期错误当最终失败，不加关键词合同。
3. 产品实际一次动态Python探针，仅whole month/year、int month、fraction month四断言；执行log681明确 `probe_primary_suite_skipped` / `suite_skipped`。8条行为合同保持planning_only。required typed义务完成不等于全部自然语言验收逐项运行。
4. 独立后验 `.codrax/tmp/20260914-r1070-dateutil-posthoc.log`：Python3.9.6、原unittest4/4；280常规算术+504月末/闰年/边界+16 finite fractional+6zero/large finite+3原非日期行为=809检查通过。NaN/±Inf两轴6项仅额外观察，不扩成用户承诺，不加入fixture原本没有的月末自动截断语义。
5. 源码、fixture、原测试、本轮全部正式JSON前后SHA一致。后验不回填formal proof；产品原生套件成功凭证本轮N/A，原生runner协议普查债仍开放。

## 按ROI排期

| 顺位 | 项目 | 处置 |
|---|---|---|
| P1 / B1682当前 | 广度就绪提示越权称全部完成，退回仍催结束 | 仅修四面提示和最近实际反馈；不改时机/权限/完成门 |
| P1 / B1683当前 | 旧教学以角色/路径显示名授关系证据 | 退役错误教学，复用当前合同；保模型所有权 |
| P1 / B1684后继 | 技术连通与主数据流取证/覆盖范围混淆 | 公共复现，审跨仓operation/产物供给及精确覆盖披露 |
| P1 / B1685后继 | 同一调用短名/限定名recipe重复 | 精确source-site+唯一身份去重，不并不同调用 |
| 既有P1 | 原生runner能力与逐合同断言覆盖 | 动态探针和目标执行分轴，不用独立后验代替产品证明 |
| P2观察 | 正文职责错误、匿名节点、语言质量 | 留原文与上下文，不系统改答案，不以单例加词门 |

B1681嵌套竞争包装本轮未自然触发，生产覆盖N/A，已有字符串数组兼容不冒充其正证。本对无运行时根因题，不能新签Trace因果投影/自动补齐生产正证。两路持续使用10m首响应/5m真实静默/10m非流式默认，无活跃流因4ms或旧4m无正文降级；eval1200s独立。原case/oracle/fixture/MD/HTML/正式报告不改。
