# r1055 人工审计：类型关系与 Python 持久交付

## 运行与判定边界

- 2026-09-10 13:50:37Z 开始，13:54:57Z 结束；严格两并发、各一次，无第三个 live。
- 冻结代码 `87a1c1af9825`，干净构建 `codrax 0.1.20260910`，built `2026-09-10T13:44:20Z`。
- `PARALLEL=2 TIMEOUT=1200`，原 case/oracle/模型预算未改；243 库存仍为 read 215 / apply 25 / plan 3。
- 选择依据：类型关系距 r1022、Python 行为距 r1042，覆盖图关系与写交付两个模式，均能独立核源码/原生行为；不继续追跑缺报告能力的同一 fmt 环境。
- 机器 summary 保持原样。以下人工审计同时检查作者答案、上下文、修补、原图、持久 ref 和验证报告，不把机器 PASS 当全问题闭环。

| 用例 | 机器 | 人工核心结果 | 尚存边界 | 外层用时 / 上下文 |
|---|---|---|---|---|
| qf_type_relation_loop_controller | PASS | 12 行实现/文件与 12 条方向正确的关系，原图可渲染 | 无关比较附注来自系统；一次真实枚举错误；纯 facet 原子操作不覆盖此类源库存表 | 260s / 39% |
| github_issue_dateutil_relativedelta_float_symptom | PASS | 持久补丁正确；原 4 测及人工独立 17 边界通过 | 产品仅有定向 probe，项目套件策略跳过；计划根因说明错误且预称回归全过 | 211s / 28% |

内部计时分别 258s / 209s，与外层用时不同口径。结果路径：

- `eval/results/qf_type_relation_loop_controller-20260910-065037`
- `eval/results/github_issue_dateutil_relativedelta_float_symptom-20260910-065037`
- 选跑日志 `.codrax/tmp/20260910-r1055-selected.log`。

## 1. 读：图与来源完整，不把附注错误抵销掉

实际终稿 `.codrax/output/20260910-065454.863-50322.md`，同名 HTML。题目接受 Mermaid 类图或类型关系图，因此 flowchart 并非格式错误。作者自称 12 个生产实现，逐个核得：

| 实现 | 文件（internal/agent/） | 类型声明行 | Observe 行 |
|---|---|---:|---:|
| analyzerEvaluator | analyzer.go | 49 | 1205 |
| answerDocumentEvaluator | answer_document_evaluator.go | 50 | 15534 |
| explorerEvaluator | explorer.go | 69 | 13240 |
| extractorEvaluator | extractor.go | 194 | 3008 |
| logTriagerEvaluator | log_triager.go | 102 | 137 |
| multiRepoFocusEvaluator | multi_repo_focus.go | 13 | 65 |
| perfTriagerEvaluator | perf_triager.go | 96 | 119 |
| plannerEvaluator | planner.go | 39 | 1935 |
| subExplorerEvaluator | sub_explorer.go | 198 | 298 |
| verifierEvaluator | verifier.go | 40 | 301 |
| writeAnalyzerEvaluator | write_analyzer.go | 21 | 94 |
| writeControllerEvaluator | write_controller.go | 17 | 72 |

接口是 `agent.go:519` 的 `LoopController`。全部 12 条边为实现类型 → 接口，带 implements；未混入 Agent、coderEvaluator 或测试实现。请求只是“主要实现”，本审计不将本次数量倒铸为产品必须全集的硬门；核全集是因为作者主动如此主张。摘要把各 Observe 一概解释为控制循环终止略粗：有些为 no-op，最终决策还涉及 LoopPolicy；不影响核心成员/关系判断，也不新增句式约束。

原 Mermaid 使用仓库自带 `internal/preview/assets/mermaid.min.js` 离线 parse+render 成功，SVG 26238 字节，收据 `.codrax/tmp/20260910-r1055-type-mermaid-render.json`。未改图文本来通过人工校验。

### 1.1 实际过程与 JSON 教学

审阅 `run-1.logs/codrax-20260910-065039-000-50322.log`：

1. 636 行 analyzer 错把 Agent 分组带入候选；642 行 source_quote 的精确参与者来源门把它移出图参与者权威，仍保探索候选。模型随后读取声明/方法，最后自己纠正到 12 项。
2. 1857–1873 行探索完备性提示要求已选关系读声明来源，2071 行闭合；不是 finalizer 拒绝，也没有把“主要实现”硬改成全集 12 项。
3. 2554 行共享 claim_form 教学列出合法值；2571 行把 type_relation 放在 edge_anchors.relation_kind。3453 行首次实际 emit 的 13 条 claim_uses 却填 `claim_form=type_relation`，3459–3460 行正确拒绝。不是 claim_form 与 facet_ids/component_relation 教学矛盾。模型后称 schema 不支持 claim_uses，也与现有 schema 不符，不能以其 thinking 证明系统漏字段。
4. 第二次完整 emit（3527 行，接受 3544–3545）删掉非法元数据后成功。两次作者图 body 都为 1761 字符、12 锚，body 和 edge_anchors 完全相同。既有 Mermaid 展示 shim 仅把标签换行表示为 `<br/>`，不改关系。
5. 3644 行唯一 patch 只替换表格并补 `member_set`，3652–3653 行成功；图不在 patch 内。该表全部条目带 SourceInventoryRowID，`emit_answer_document_patch.go:1632` 明确排除它们进入 atomic add_facet_id 候选，所以完整替换是当前合法 fallback。不能说模型放弃了已发布的原子操作；本轮没有完整发送 schema 抓包，判断来自保存字段、精确候选谓词和实际参数。

合计 1 次成文拒绝、1 次 patch、3 成文轮；2 次 blocks-string 安全恢复并未绕过非法枚举门。无 unavailable/prune、无跨阶段成文重试；read15、repo_map2、source_lens1、trace0。没有关系丢失或无解双合同证据。

### 1.2 已确认的系统显示残余

MD77 附注“对比问题中各方的证据采样不均衡，某一方的支持强度明显弱于另一方”不是模型写的。本题是成员枚举/类型关系，未要求比较。产点链：

- `internal/types/facet_plan.go:764–773` 仅由至少两个 bucket 选 QFComparison；
- `internal/agent/erm_completeness.go:156–158,726–785` 进入 EntityParity，按 prose bucket anchor 和 EvidenceItem.AnchorSymbol 计数，不读取表格单元格或图关系的完整性；
- `internal/orchestrator/orchestrator.go:5353–5361` 保持 soft violation；
- `internal/types/violation_registry.go:545–547` 将样本差异表述为支持强弱。

归并既有 `EVAL-B54-TIER2PROSE1` / B54-E 的 P2 显示权限残余。该规则仍 soft、不可 strict 提升、没有本轮 retry，不能说以前硬门修复失效或新 P0。建议下一独立小批：统计留内部，已有 typed `comparison_axis` 只用作软展示选择；即便确为比较，也只能准确描述采样范围，不能从计数推出正文缺失/支持弱。保原模型正文，不变全局家族路由，不新增模型字段或硬门。

源库存表纯 facet 原子修补暂记 P2 后序覆盖项，已有 replace 出口；单次枚举误填先作模型精度观察，不继续围绕本题增加教学负担。

## 2. 写：补丁有效与正式证明范围分开

原 fixture 为 `eval/fixtures/github_issues/dateutil_relativedelta_float`。原始四测基线：1 正常、2 TypeError、1 未抛 ValueError（`.codrax/tmp/20260910-r1055-dateutil-baseline.log`）。未修改原测试期望。

计划 `plan-1789048381555114000-50337`，唯一补丁为 relativedelta.py 构造函数 6 加 2 删：两轴 float 非整数先抛 ValueError，存储前转 int。原 tests/README 字节保持，未改运行器或验收条件。

### 2.1 持久交付与原生独立复验

- `run-1.repo` 原 main=`5db30b64d088d2c96cc92428eb5077dd2550e204`，保持不变。
- `refs/codrax/applied/plan-1789048381555114000-50337`=`5ebb957afd9d2c345e7e4686c1b099fba5d76136`。
- 真正保留 worktree 为 `run-1.repo/.codrax/worktrees/trace-1789048382213943000-50553`，另有 `run-1.applied-tree` 导出。
- root 在保留 worktree 运行原始 unittest，4/4 通过；日志 `.codrax/tmp/20260910-r1055-dateutil-durable-native.log`。
- root 独立黑盒 17/17：整数值 float、原整数、负历年/月、正负零、组合跨年 9 个 normalize/apply 分支；years/months 各 `-1.5,-0.5,0.5,1.5` 共 8 个拒绝分支。日志 `.codrax/tmp/20260910-r1055-dateutil-independent.log`。

以上仅该有限输入域的真实独立测试，未声称所有 Python 输入类型兼容；也未把人工补跑写回产品报告或伪成模型执行。

### 2.2 产品本轮真正执行了什么

`plan-1789048381555114000-50337.report.json` 只列一个 `fp_months_whole_value` Python probe 的实际子进程收据，内部含多项 assert，覆盖整数值 float 的两轴转换/日期相加、months=1.5 拒绝及原整数存储。不能把一个 probe 行误称为只验一个行为，也不能把它叫全套原生回归。

同报告列出的 `python3 -m unittest discover -v` 为 `source=probe_primary_suite_skipped, outcome=suite_skipped, exit_code=0`，没有真实项目套件执行；0 为未执行记录零值。`run_tests.go:610–669` 在 pre-suite probe 通过、没有继续项目套件的 typed 条件时主动记录策略跳过并返回。Python 已存在、unittest 已发现，不是缺运行时，亦非 B1650 的具名测试 skipped 状态。

八条自拟无证合同保留 `planning_only_ungrounded`，required=0；final 状态说明结构化必需义务闭合，不表示每条自然语言验收独立证明。B20-W1/W2 的 required 保护与 B1132 降权未见回归，不销 B1561 原生具名结果缺口。产品的通过只覆盖实际 probe/当时必需结构化义务。

### 2.3 上下文可更精准，模型解释仍有错误

apply 日志 `run-1.logs/codrax-20260910-065301-000-50553.log` 680 行已明确策略跳过，799–805 行保 required/planning-only/source；聚合却将 suite_skipped 归 `capability/unavailable`。控制器后来把它猜作环境/预存失败，并额外探索，在客户仓搜系统枚举 suite_skipped。并非 source 丢载，最窄改进是对该已有 typed 来源明确“未执行；0 非测试通过；策略选择而非环境故障”。不要按 acceptance prose 强制跑所有套件、提升 planning-only 或填造 report。

持久 `run-1.plan.json` summary/rationale 仍说“divmod 不接受浮点”，真实情况是 divmod 可接受 float，而日期 `replace` 的年月字段要求整数。summary 还预称“现有回归测试全部通过”，产品本轮未执行该套件。root 后续补验为真也不能倒签这句写于计划阶段的证据。两处按模型解释精度观察，不由系统替换计划正文或添加关键词硬门。

过程一个源码计划、一次 apply/verify，后续一次探索；无修改重试、无 finalizer。一次当前工具表没有的 exec_command 请求在执行前被拒。read7、repo_map2、list1，unavailable1 源自上述跳过口径，context28%；不以无成文重试冒称过程无噪声。

## 3. 施工闭环与后序

- B1649 完整公开 13→8+5 续修针：`961ebb78e` 已推；真实 finalizer 安装器续租、公开 emit/patch、陈腐 ref 反控，原图/摘要不变，不扩 cap。
- B1650 多框架未断言结果不授行为证明：`87a1c1af9` 已推；普通 passed/failed 原行为不改，新增 non_asserting 是系统结果范围，不是模型 JSON 义务。
- 冻结全仓 `.codrax/tmp/20260910-b1650-final-frozen-full.log` exit0，86 个有测试包通过，部分未变包缓存；tool334.051s、agent95.509s、types63.670s、tracequery135.635s、tracediag28.156s、orchestrator26.100s。主动中止的旧全仓不计成功。
- count3/race 及活跃流收据见统一文档 §123.1743–1746。4ms 部分帧、隐藏推理/工具/heartbeat 不因没有可见答案降级；真实 stall 和显式 deadline/cancel 仍保留。本轮没有自然超长流命中，不倒签 live 覆盖。
- **下一最高优先 P1 B1651-JUNITFRESH1**：真实公开反例确认旧 JUnit 可借当前 exit0 授 covered。优先绑定本轮执行与报告来源，保 fresh 正控，旧/错来源不授当前成败，不能删用户报告或仅凭名字/mtime 认领；详见统一文档 §123.1745。
- 之后 P2 依序：B54-TIER2PROSE1 用户附注权限、策略未执行说明、合法源库存表纯 facet 原子元数据操作；B1561 Make 原生报告声明在来源绑定后另批。无解合同未证，不为上述模型精度问题拟合专门硬门。
- 下一 live 保持两并发异构，显式 Trace H1 为高优先保护候选；本轮两个均非 Trace，只能说相关全仓工程通过，不能称本轮生产命中因果投影/自动补采。

统一追踪：`docs/design/eval_priority_campaign_audit_20260730.md` §123.1743–1746。本批无系统代写作者结论、请求/答案关键词硬门、Trace 链资格或显式窗/自动补采修改。
