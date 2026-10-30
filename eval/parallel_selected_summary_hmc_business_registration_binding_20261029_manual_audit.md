# HMC319 人工审计：业务实例与只读登记

原题、EXPECT、退出条件未改。两例各一次；完整审计原件在本批443件[封存索引](results/hmc_business_registration_binding_20261029_v1/sealed-index-v1.json)中按canonical source与SHA检索。A原件/private/tmp/codrax-hmc319-A-audit-v1.md（SHA31aa2f26968aedab1eb741a85f941ab60944c01c624940a0261d768deda2e22c），B原件/private/tmp/codrax-hmc319-B-audit-v1.md；根代理已复读原报告及关键原生/最终材料。

| 侧 | 进程 / 机器 | 人工裁定 | 不能扩张为 |
| --- | --- | --- | --- |
| A | exit0 / PASS | 原业务树完整FAIL；新身份图桥自然有限PASS | 04.2销账、所有图/局部修补/像素均通过 |
| B | exit0 / PASS | 原缺来源终态负向合同PASS | 自然controller、完整CLI、通用持久化或全部父项通过 |

## A：正确核心事实与已证图桥

精确run为results/hmc_business_registration_binding_20261029_v1/A/trace_query_business_tree-20261029-235921。最终MD为/Users/han/opt/codrax/.codrax/output/20261030-000234.470-3524.md，19944B，SHA2857dd8dfab77cfc72c92cf72dbdef5defe511c843edf11db44e8ad6ba5a2c7a；同stem HTML为2799589B，SHA84a649be44cef3430a763399bb16a5e4315a191f7b55199c77a8e996d641273f。

原始16行1481B文本给出TID700/TGID600的OpenDocument→LoadIndex→DecodeRecord，同步包含而非唤醒/调用因果。inclusive10/5/1ms，self5/4/1ms；self分别为[5.001,5.002)+[5.007,5.011)、[5.002,5.004)+[5.005,5.007)、[5.004,5.005)。self运行5/3.5/1，等待调度0/.2/0，睡眠0/.3/0。整12ms窗线程运行11.5、就绪.2、睡眠.3。正文与目标表这些值正确。

AsyncPrefetch在700的S/F两端5.0025/5.010、cookie41、7.5ms，非同步子层；background-900同名OpenDocument是另一个5.5ms实例，调度状态未知≠零。正确身份及非因果边界保住，不把完整FAIL说成所有取数失败。

实际9模型查询＋1系统补采；complete两次一拒一受理，emit一次受理，patch零。INIT03:217–234的requested_participant_binding，经提交d1三个原生实例及两条contain anchors，到原日志3721/3722受理、MD45–52和HTML同三节点二边。业务名为可读标签，长ID只作源锚；旧三个额外unproven角色和“所请求关系未证明”边界未复现。此例自然覆盖完整提交，不冒局部patch所有分支。

## A：三组完整验收欠项

### 1. 发布记录计数升格为物理实例（系统接缝）

INIT03:200–208是同三个marker实例各三份查询发布，记录来源为236dc…、2b160…、3957c…，每次原库存node_count=4，另一个节点是900。INIT03:209却称“独立展示9个实例”，MD151据此写“本次观察到9个业务实例”。不是原件9实例，也不是三查询各4节点的总体。

生产定位internal/agent/answer_document_business_tree_handoff.go:28–49：按完整ObservationRecord JSON去重，保不同查询身份正确，accepted/shown却被命名为实例数。修复应分发布记录/物理实例量尺，不能删掉合法查询来源或按名字跨源归并。归16.4/16.5/18.2，冻结后没有改码。

### 2. self墙钟已知与状态未知混在一起（字段交接）

三份原生business_tree中background marker:900:9的direct_child_count=0、self.duration_ms=5.5；只有self.states.coverage=unavailable。最终MD67及HTML手写表把self也写未知。

归因不能跳层：实际INIT03业务树段只保700的self；background仅在:332/:384的trace_business_span表保inclusive5.5和状态未知，未带self。故不是“完整self已到场后纯模型误述”，而是原件→上下文字段丢失叠加最终unknown外扩；原始查询值没有损坏。不新增背景完整图等退出要求。

### 3. 合法self状态小计的正文规则冲突

MD31/80在self说明中一概说跨层状态数值不可相加，随后又明确本次三层self分段已证互斥且允许合计。失败不是未输出新小计，而是禁止与许可同口径冲突。self运行5+3.5+1合法，但不强制答案做此加法。

新教学已实际进入INIT03:199/213/327，系统表MD145也正确限定inclusive不能直接相加、已证互斥self可加；原模型提交仍产生矛盾，不能说新教学未到场，也不以末尾正确句抵消前文。继续16.4/18.2留账，不加原文关键词门。

Async段“该线程没有确认成对同名”指代有歧义：若指700则矛盾，若指标题里的另一线程900则合理。明示700/7.5及表owner正确，因此仅记歧义，不冒确定错归或新增退出门。

HTML源四表与MD一致（含表头行数2/6/3/14），但F2在两表面共同存在；附件1481B与预览1571B区分正确。未做浏览器像素QA，不签布局、裁切或字形。

## B：真实登记、独立恢复及负向终态

原输出/private/tmp/codrax-hmc319-registration-v1。真实planner4请求/4响应；5工具声明=2次完整read_file＋3次emit_change_plan，2拒1受理。参数在内联日志，blob数0不是零调用。最后计划changes数量0、具体project_test_observation1，不修改源码/测试/配置/依赖。

| 报告代次 | assertion结果 | 报告命令 | existing_test_execution |
| --- | ---: | ---: | ---: |
| 历史source | 1 | 2 | 0 |
| 本轮proof | 1 | 2 | 1 |

历史invocation native:d98:18e33c789a7dff00:1与当前native:e0e:18e33c7b520bb3e0:1不同。当前2命令是py_compile和实际unittest；1登记凭证绑定command_index1及原测试SHA。多份当前报告别名不累加为执行次数。

父3480、独立子3598真实恢复后通过公开RunTests执行。只读审计确认value.py43B、test_value.py144B、HEAD/diff/status未变；“只读”不等于整个夹具、日志和报告零写入。source先前真实apply但无Completion/verify；proof verified，整体仍unverified/missing_terminal_verify_verdict，没有拿当前测试绿替source补终态。

真正WriteFinalReport位于history-report-artifacts/originals和restart-child-delivery-artifacts/originals下的plan-1793343576664620000-3480.final.json；均26519B、SHA2c3404ad3b5938d37bb80ae31a2a036130f1a6636e9af4da9e7f32635c3316dd。两目录各4原件/1verify attempt/1final→report link，逐件bytes/hash及两链接独立核对通过。report_id/report_path均指向本轮d7f426…不可变报告，不是历史报告；根目录final-report.json只是ChangeReport别名，未冒真正最终说明。

controller为scripted_verify_then_finish，full_cli_run=false；context_pack_durability_verified=false及no_durable_store保留。父子workflow的正式完成/报告一致，context-pack artifact ID不全相同，不签全部持久化。

## B：错误形状反馈债，不扩大负控退出

首次真实changes={}被兼容层转[{}]，不是模型所称“发了[]”。该形状跳过emit_change_plan.go:386的零变更登记分支，在:447以registration=false归一化；native_test_registration_selection.go:19–40因此不加载choices，先报assertion_ref not current。该ref与INIT逐字一致，真正空path校验到:464才执行。二次改用suite/id仍{}，报空path；三次省略changes后受理。

授权边界没有绕过，但错误形状诱发身份错误反馈和不必要的身份改写。归16.4/18.5；保两次拒绝和模型错误自述，不能用最终PASS抹去，也不能仅据第一错误说ref已陈旧。未冻后修改或追跑。

## 统一结论

完整能力新增0，累计26，53开放不变；产品子能力2组。A三组欠项、B一组反馈欠项，另保Async歧义和旧自然未触发，不混成稳定任务数。先修已证共享计数/字段与IO端点接缝，再轮换11.2原代表合同；旧充分事实下误述不无限阻塞其它高ROI能力。WAL未零副作用不入生产。

全仓唯一一次exit0、98包通过与原43/14跳过保留；独立回归/图桥有限PASS不能代签04.2完整人工通过。[批汇总](parallel_selected_summary_hmc_business_registration_binding_20261029.md)记录冻结、失败保真、main提交与publication收据。
