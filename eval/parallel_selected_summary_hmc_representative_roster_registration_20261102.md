# HMC335：完整代表名册交接与只读登记终态轮换

日期：2026-11-02（America/Los_Angeles；终态11-03 UTC）。在main开发；代码`4e8927f4521792197474611e86470e5b12683b02`，父`0c9a98e34395aea97faf071e8de5c90bcfa84979`。相邻五文档、冻结输入、全仓与远端main由[publication](results/hmc_representative_roster_registration_20261102_v1/publication-v1.json)独立核验，不为模型答案签PASS。

## 1. 核账和分账

稳定79条/79唯一：28完整、51开放，18部分实施/29待实施/2待验收/1验收中/1持续执行；本批不改任何勾选。[root裁定](results/hmc_representative_roster_registration_20261102_v1/sealed-v1/0140-codrax-hmc335-acceptance-v1.json)不关闭11.2，18.5仍持续执行；上批已闭18.3与旧FAIL均不改。

| 分账 | 本批结果 | 不扩签的边界 |
| --- | --- | --- |
| 完整能力交付数 | **新增0，累计28** | 不是把自然登记复验或共享子片算新父能力 |
| 已交付子能力 | **1组：有界完整名册无损交接** | 公开RED→GREEN及actual INIT自然命中；正文解释仍独立验收 |
| 剩余验收缺陷 | **本批3组继续开放** | 原选择规格、状态/表面覆盖、调查文字与原生属性冲突；旧欠项不被合并销掉 |
| 原A完整能力验收 | **结构FAIL、原有限FAIL、整答FAIL** | 后补资源查询不能代替原资源规格选择 |
| 原B只读终态轮换 | **机器及有界完整人工PASS** | 真实父/子controller＋planner，不是完整CLI、自然代码创作或全部框架 |

## 2. 公开反例与泛化修复

§334实际INIT2只展示3/4未选成员、1显式省略，已在原提交手审纠正其原raw审计过宽表述。新公开回归通过普通discover→代表选择→真实原生重复查询→三阶段context构造，复现8成员只剩7；[RED](results/hmc_representative_roster_registration_20261102_v1/sealed-v1/0476-codrax-hmc335-roster-red-v1.log)原件未改，不以私有灌结果构造假阳性。

旧完整自包含行能全部放下时保持原样；仅在组/选中/未选行确有省略，且**整个精确字典名册能放进原剩余预算**时，改用一份完整JSON。共享完整来源元组与组规格，每个成员仍保queryID、源代次/状态、选择原因、执行结果/次数、完整latest attempt及全部尝试的结果分布；稳定原顺序、零起始局部索引明示。来源用完整可比较元组，不按名称、字节、TID或相似度合并；复用原`RepresentativeNavigation`同代精确join，缺join/重复ID/规格原始表示不一致/计数不符不发布完整名册，退回原完整单元预览及显式省略。

预算仍24KiB，并保选择外/后来查询预约；长合法参数、巨大原错误或整份字典放不下时，不裁字段、不删事实、不改计划/结果/重试权限。完整性只针对这一选择的成员人口，不宣称未匹配材料、全部目录、测量或诊断完整。增加一次软教学：任务组是精确目标/窗/过滤规格，内容相似性可另述但不替换组和计数。没有扫描问答原文、自动改view、自动改结论或新增语义硬门。

对照参考`core/batch/closure/select.py`的manifest所有权/纯选择函数、`package.py`的选择后独立材料打包，保留“清单决定成员、源身份独立、优先级不是物理/完成结论”的设计意图。没有照搬固定5/50作用户语义、缺值补0或宽松文本匹配授权。本项目直接在共享context生产点解决重复元数据挤占，不为resource_stack单type另造路径。4个Go文件、2生产/2测试、352新增；旧断言、问句、operator/oracle不变，Trace因果投影、原4候选、闭合睡眠、链上业务与自动补齐保留。

## 3. 相关验证、完整实采与冻结

最终相关4个实际测试包PASS（另operator包当次无匹配不算套件通过）；原operator随后另跑全套7顶层/40子PASS。共享context/tracecatalog race×2通过；agent/tool/context因果/JSON/旧合同相关套件通过。新增公开三阶段逐项还原8成员；同路径不同源代次、陈旧状态、缺join/重复ID/规格字节/错误计数、原预算和巨大原错误负控通过。旧HMC314部分完整行预算断言未放宽。

认真使用用户的完整实采：105版网易云82838676B/559592行，SHA12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4；原测试真实1次native query/0模型，TID36434、[5969.994941,5970.622586)、原77708–152774行bindApplication的627.645ms inclusive/194.581ms self，以及实际Extract/Finalize原生交接均通过。[原收据](results/hmc_representative_roster_registration_20261102_v1/sealed-v1/0199-receipt.json)不是第三例模型或重新全审旧独立oracle。

[冻结](results/hmc_representative_roster_registration_20261102_v1/sealed-v1/0191-codrax-hmc335-frozen-v1.json)7585输入/251外部/15仓外快照，SHA4efa93555b15b55b80a03de8580d291d8812c317dfc26adfbb54d0e5074dc1e8；之后代码/测试/教学/case/helper/oracle停写。一个标准CLI、一个原orchestrator测试二进制、一个原operator分别构建，启动预检0模型。冻前Ruby生成草稿语法错误未启动评测，拒绝稿留作txt，不冒产品RED。

唯一全仓`go test -json ./... -count=1 -timeout=30m`实际OS0、前后源true：98包PASS/12无测试，28712顶层PASS/45SKIP、34295子PASS/14SKIP；266998 GoJSON、FAIL/非JSON/重复/未终态均0。[统计](results/hmc_representative_roster_registration_20261102_v1/sealed-v1/0211-codrax-hmc335-full-stats-v1.json)，log SHAae9ef2299065245e1a642fd0faf4ddc40e5ca11ce4c3964b8a2c7614e95e2446。默认实采/live opt-in SKIP不冒再次调用模型。

## 4. 恰好两例并行、各一次

| 项目 | A 原代表自然题 | B 原只读登记自然控制器终态 |
| --- | --- | --- |
| 入口 | 原50steps CLI、4完整构造SQLite | 原planner12/default controller/20m harness，非完整CLI |
| 进程/源 | OS0、前后true | OS0、前后true；父68636/子73558 |
| 原smoke / 完整人工 | FAIL / FAIL | PASS / 原有界入口PASS |
| 实际结果 | 8 event_search选择计划、2组/4选/4未选；资源取数在9extra | 来源/补证/整轮verified，新invocation、原件/只读/恢复通过 |
| 新共享名册 | actual INIT4源/2组/8成员精确还原PASS | 无运行时代表名册，不补签此分支 |

原日志36/9个模型request、response同数。精确唯一配对A14/B9，另2个A歧义组完整保留并排除；未解请求0，最大可证重叠17.523秒，见[配对原件](results/hmc_representative_roster_registration_20261102_v1/sealed-v1/0141-codrax-hmc335-actual-request-overlap-v1.json)。不是只有23请求，也不是barrier/进程重叠推断。A一次成文/3条INIT，B恢复子属于同一问题而非第三例；没有第三次追绿。

A原operator实际1：选择view由模型显式写event_search，后9个resource_stack规格/13次真实尝试不能代原8个资源选择计划。总17记录＝8声明+9extra，15记录被尝试、23attempt＝21success+2empty；4selected各2次成功，4unselected为2未执行/2empty。新字典在实际INIT2第17行完整投递，8/8成员/源/规格/attempt分布逐项等于同代index/selection；2组/4选/4未选皆0省略，extra另6展示/3省略不改变原总体。旧缺尾共享缺陷闭环，但不为错view或最终误述免责。

9个资源JSON16事件/45帧原始数值字段逐项核冻结SQLite oracle一致，[独立只读核对](results/hmc_representative_roster_registration_20261102_v1/sealed-v1/0130-codrax-hmc335-A-native-crosscheck-v1.json)不改原contract。正文现在保线程×窗而不是内容重分组，但仍把2未执行＋2empty写成4未执行＋2empty；又声称四张事件表，模型只提交3张。实际5表＝3原生事件＋跨源对齐＋定位状态；渲染没有丢第四张。调查actual INIT3只传1/6单元，该unit0将原非null end_ns写null；终稿未重复该字段，已投上游冲突另留。帧已有非有序链声明仍用方向式列法，记关系歧义，不拿箭头字符作硬门。正文/表格/原生Notes范围和状态消费仍FAIL。

B真实父controller4次verify发射、1次plan发射，planner实际读实现/既存测试后精确登记test_value.ValueTest/test_increment→increment-result；空变更计划经默认持久化，在独立子进程真实新执行。旧source report、两代来源验证和新补证报告分别留原；中间unverified attempt未改写，仅以verified followup说明补齐。最终source/proof/run分别verified，native:11f56:18e466664c0eed20:1区别所有旧/父调用，原source plan/commit归属正确。父子两份归档各7原件/3verify引用及全部final→版本report链接实际匹配；最终计数1测试/1通过/2命令不混用，Git/HEAD/文件只读且授权撤销。四个上下文快照各6唯一逻辑ID、内容一致且落盘轮转幂等。不冒完整CLI/源码创作/负向终态/所有框架的新PASS。

## 5. 原件、留账与51项ROI

508件[index](results/hmc_representative_roster_registration_20261102_v1/sealed-index-v1.json)，SHA7d871d85ad3a3731fd11d08e48abeae3e39a10a9f237d6eceb707e3706df00e9；[原A手审](results/hmc_representative_roster_registration_20261102_v1/sealed-v1/0109-codrax-hmc335-A-audit-v1.md)、[原B手审](results/hmc_representative_roster_registration_20261102_v1/sealed-v1/0132-codrax-hmc335-B-audit-v1.md)及[人工汇总](parallel_selected_summary_hmc_representative_roster_registration_20261102_manual_audit.md)。旧36代14781件按各原ROOT保留，只物理重核最新334的553件及更早链头；37代按代累计15289件，不称全局内容去重或全历史payload重hash。大原件只引用，不冒全部原件已上传Git；providers不读/哈希/复制。

剩余3组：①任务事实族到选择view及后补执行归属；②当前状态/正文计数与实际表面覆盖一致；③原生属性/量尺与调查单元、引用适用域及预算。旧IO集合/硬件解释、采样状态/权重、权限覆盖、图/patch、重复展示/全局预算、h2/h3/lower-cost-full及更多平台材料继续留账。新名册自然命中已证；本批无图、sample-only/其它二进制、缺来源登记负控及新容器形状反馈自然覆盖，未运行/未触发不签PASS。

以下51个开放ID恰好一次。高ROI先补已证共性交接，不让充分事实后的模型误述无限占用完整能力轨；下批完整轨轮换IO原合同或具备真实材料的剩余二进制，代表自然题本批不再追跑。

| ROI层 | 数量 | 稳定ID（层内顺序） |
| --- | ---: | --- |
| P0 共享事实、schema、预算与补齐 | 7 | HMC-16.4、HMC-01.3、HMC-02.4、HMC-16.5、HMC-01.2、HMC-18.4、HMC-12.5 |
| P1 有材料原合同、登记和持续轮换 | 6 | HMC-11.2、HMC-18.5、HMC-03.2、HMC-14.1、HMC-08.3、HMC-18.2 |
| P2 原生复用与较成熟能力 | 11 | HMC-04.3、HMC-04.5、HMC-08.1、HMC-08.5、HMC-12.2、HMC-12.3、HMC-17.6、HMC-17.7、HMC-05.2、HMC-10.2、HMC-14.2 |
| P3 来源/协议与安全前置 | 21 | HMC-04.4、HMC-04.6、HMC-05.3、HMC-06.2、HMC-06.3、HMC-08.6、HMC-08.7、HMC-08.8、HMC-09.1、HMC-09.2、HMC-09.3、HMC-10.3、HMC-12.4、HMC-12.6、HMC-13.1、HMC-13.2、HMC-13.3、HMC-14.3、HMC-14.4、HMC-14.5、HMC-15.1 |
| P4 新领域/材料依赖高 | 6 | HMC-07.1、HMC-07.2、HMC-07.3、HMC-07.4、HMC-15.2、HMC-15.3 |

WAL零副作用未证前生产NO-GO；Trace因果投影、自动补齐、原4候选、闭合睡眠、链上根因和业务线索不变。完整原件基线继续用于跨批保护，不用邻近背景补主因。
