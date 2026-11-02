# HMC333：原生引用导航、原IO有限复验与只读负控

日期：2026-11-02 UTC（本地20261101）。main基线`3e074c941df7b6435a1c30439ae94414ede391d3`，实现提交`aafa997e514dd63659424631ea39d4729cb937a9`。这是当前冻结代的源码/测试/人工结果汇总；最终文档、远端和完整性以[实际publication凭证](results/hmc_reference_context_io_registration_20261101_v1/publication-v1.json)为准，不从进程退出推断验收。

## 1. 三本账与剩余任务数

重新解析79条稳定任务，79唯一/重复0，27完整/52开放；开放为18部分、29待实施、3待验收、1验收中、1持续。原勾选不动，root明确`closure=none`。

| 口径 | 本批结果 |
| --- | --- |
| 完整能力交付 | 累计27，本批新增0；原HMC-08.3有限FAIL，不能按数表正确销账 |
| 已交付子能力 | 本批2组：当前原生引用的同源导航交接（含原预算保整项）；报告/JSON结果格式导航 |
| 剩余本批验收缺陷 | 5组：IO硬件解释、跨窗集合解释、selector/JSON形状重试、描述自然投递覆盖、结果导航自然覆盖 |
| 两例真实评测 | 各1次并行；A原smoke PASS/有限及整答FAIL，B原负控harness和限定人工PASS |
| 全仓与源交付 | 唯一full OS0/98包/28704顶层/34287子例PASS；与能力验收分账 |

不把两个公共测试族、预算修复或B负控局部PASS分别算完整父能力。旧采样误述、IO/权限未覆盖、图/patch、代表规格、其它二进制、全局预算/去重、h2/h3、lower-cost-full仍留账；WAL未满足零副作用前生产NO-GO。

## 2. 选择两条工作轨道与参考取舍

完整能力轨复验原IO在途自然问题，不修改原问题、断言、15steps或n=1；轮换原只读缺来源终态负控作为另一且唯一另一例。共享缺陷轨针对§332 B已证的错主体引用和结果格式猜测：地址指向H:getframe，文字描述bindApplication；TXT中找JSON字段花12次读取。前者不应靠扫描解释文字硬门，后者不应让模型猜结果介质。

参考仓`/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main`的AGENTS、GOLDEN_RULES、architecture及相关实际实现已对照。`core/preprocess/context_index.py`保自包含材料/步骤/路径元数据，适合精确导航；`marker_ops.py:355`以itid物理线程实例绑定业务段，`:517/551`处理子阶段及状态交集，说明量尺必须跟原实例走。本项目取其意图，保私有来源/查询/表绑定、子区间并集、未知不补零；不移植松散文本匹配、空值归零或直子时长直接相减。此前`io_ops`桶内相交数/发起桶统计不替代本项目瞬时/时间加权深度，不借静态参考作客户因果证据。

## 3. 两组源码子能力及边界

1. **当前原生引用同源导航。** 新共享描述从私有BoundTable取得label/columns/原始来源/窗/选择/角色，复用于Explorer引用schema的description和当前受理解释单元旁的Extract/Finalizer消息。原精确observation/view/row地址不变，不增加模型字段或新事实/因果权。当前但错主体的地址仍可能配错文字；本批不声称验证任意文字、算术、业务关系、图注或已发表状态。
2. **结果格式导航。** 既有payload指针说明raw_ref是描述报告、payload_ref是结构化JSON，查JSON键用后者；两者均非原始采集。没有改变reader、路径权限、原件行号、结果分页坐标、query路由或自动重试规则。

描述仍受原整分支schema容量限制，不能保证所有表都入一次schema菜单。解释预览仍900/1800B：先预约所有原本可容纳的完整单元和边界，再用余量整项追加full/compact描述；压缩只保完整label/columns/reference_index并标省略，不能截字符串或挤掉后续业务线索。无足够余量时描述可全部省略并披露，不扩大预算凑自然命中。当前completion、来源、查询及表重新绑定；JSON恢复/陈旧/换源不能造当前描述。Trace因果投影、原4候选、闭合睡眠、链上业务线索、自动补齐未改。

源码9个Go文件，6生产/3新测试，577增8删；旧测试、cases和oracle零修改。完整实现和冻结输入由publication再次对提交blob核验，不以统计行数代替行为证明。

## 4. 公开证据、真实ftrace与唯一全仓

| 检查 | 当前实际结果 |
| --- | --- |
| 初始新公共RED | 3顶层/2子FAIL；实际query/completion/schema/read路径缺导航元数据/格式说明 |
| green-v1 | 两旧HMC315子例真回归：元数据挤出原本可容纳的单元；原FAIL保留 |
| 最终related-v2 | OS0，4包、146顶层/460子PASS，8顶层SKIP |
| race-v1 | OS0，2包、21顶层/30子PASS，2顶层SKIP |
| 完整网易云模型外v2 | 原生query与实际Extract/Finalizer消息PASS，1顶层；不启动模型 |
| 唯一全仓 | OS0，98包PASS/12无测试包，28704顶层PASS/45SKIP，34287子PASS/14SKIP |
| 全仓终态普查 | 266932 GoJSON，FAIL/非JSON/重复终态/未终态均0 |

新测试初始要求复制完整来源标题/版本字段，冻前按紧凑描述设计修正新表示断言，旧精确地址和私有来源验证不变。实采v1查询成功，但新测试错误要求inclusive-only状态表含self；修为该表inclusive627.645＋精确BusinessTree节点total627.645/self194.581双断言，未放宽self保真。两个旧HMC315失败是真候选回归，由原预算先预约修好，不写成夹具错误。helper无效语法草稿未执行，零模型/构建/full；原稿及失败保留。[独立证据更正](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0262-codrax-hmc333-public-evidence-review-v1.md)逐项限定归因，不回签原FAIL。

本批认真使用`/Users/han/opt/donghu/基线数据/105版本/网易云音乐/应用后台销毁后冷启动/3、LOG_ftrace_20261019_17.55.02/ftrace-20261019_17.55.02.ftrace`完整82,838,676B/559,592行原件，SHA`12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4`；旧独立整数µs oracle不改、不进模型。精确TID36434/77708–152774行/查询窗[5969.994941,5970.622586)的原生事实到当前schema、completion、实际Extract/Finalizer adapter贯通，Extract compact/Finalizer full描述可见。v1/v2各1native query/0模型，两次是模型外公共尝试，不冒第三/第四个真实模型case；不重复宣称本代又对§332全部16账户/3735时序逐项手审。

唯一普通CLI SHA`98511729ae0e7b562e4655472cef0568861757c9cb77507c2b40a67b25a8c0ff`，原orchestrator harness一次构建SHA`0d344b9c0f0679bd3037ccbb50ee5cc762df24f7e38a1de63bd79f8534d87589`。冻结7580输入/178外部/19仓外快照，SHA`e5a4815447c2a1947b66d2ef9283ef77d5ce8aee2ec501509305613d436e821c`；之后源码/测试/教学/helper/case/oracle停写。full唯一`go test -json ./... -count=1 -timeout=30m`，06:51:22.252730Z至07:04:46.626297Z，OS0且源前后true，原日志SHA`4f3c92a5f25c5b8b635cb65708221e6726d3be328fe2bb13a952c90e61b37d42`。新增可选实采测试在普通全仓SKIP，不冒全仓另跑了实采。

## 5. 恰好两例真实评测及人工结果

### A：原IO自然题，有限与整答FAIL

普通CLI15steps/n1，原问题要求[1,1.01)秒按RQ/BIO×设备×读写给峰、均、busy、面积和发起，并解释意义。输入是原1839B/23行/21事件构造文本，不冒设备实采。1次成功trace_query，1份105001B原生JSON；4组24数值、四成员完整端点/裁窗驻留、深度和两activity表同源正确。最终8张原生表保全部7个已选展示；附加RQ读summary未丢。HTML共9表含附件，21数据行，MD/HTML单元格一致，无图，无浏览器像素QA。

原smoke PASS、OS0不抵销两个原合同失败：正文用低配对深度排除硬件瓶颈/存储队列；又将4相交配对当6窗内发起的子集，称1.012完成的端点不计入窗内数字，而同答原生成员明确贡献2ms。正确完整6/4/4/4ms与裁窗4/4/4/2ms都在实际上下文，不能改原生迎合文字。

实际7次completion：前5结构拒绝，第6降级缺member_set，第7resolved。1次full答结构拒绝，5次patch中4拒1受理；最后拒绝不覆盖accepted稿。错误对象形状数组及选择器重试成本留账。实际Finalizer1次初始化3条消息，只投解释[0,1,2]共3/9、披露5851B省略；坏unit5/6/8未转发，不冒摘要污染。三个optional描述在既有预算全部省略，新描述自然正向NOT_TRIGGERED；A无结果导航读取，格式导航自然效果也未覆盖。原native表/端点/量尺/覆盖已充足，不造OOM或源事实整体丢失原因，不追第三次求绿。

### B：原只读缺来源终态负控，限定PASS

原`TestNativeRegistrationLiveRestoredMissingTerminalReceipt`，真实planner＋脚本恢复controller＋新进程/public RunTests，非自然controller/fullCLI。模型完整读取2行源码、4行测试，发`changes:[]`及1项existing-test登记；真实执行exact increment(4)==5，1断言/2命令，旧新invocation分代。补证批verified/tests_passed；来源批仅apply、无VerifyRef/Completion，整体unverified/missing_terminal_verify_verdict，正确阻止误完成。

actual final与内容寻址report及父/子归档四个链接字段同代；git HEAD/diff/status与来源计划不变。context_pack_durability_verified=false，不能代签完整上下文包或完整18.5；没有模型最终MD/图要求。原负控全部限定人工PASS，不扩展成任意输入测试或多框架证明。

两进程06:51:21Z附近并行启动，各一次/源前后true；实际唯一请求区间A18/B2、歧义0/未配对0，最大可证重叠5.915s。A首请求为路由，B两条INIT是同次planner的system/user，不是两case或两次初始化。详见[人工审计](parallel_selected_summary_hmc_reference_context_io_registration_20261101_manual_audit.md)。

## 6. 原件与同代交付

本代根`eval/results/hmc_reference_context_io_registration_20261101_v1`：347件原样封存，[index](results/hmc_reference_context_io_registration_20261101_v1/sealed-index-v1.json) SHA`4cdf055474e841aee4590a949ec2a5f2b9500e262e749abdf606d34397dc4493`，[封存凭证](results/hmc_reference_context_io_registration_20261101_v1/sealed-receipt-v1.json)。source/receipt/实际INIT/全部失败与受理/完整MDHTML/真实报告及全仓68MB原日志均保原，不按提示预算裁档。大ftrace只保SHA和路径，不复制进Git；封存本地留存不冒上传远端。

| 证据 | 当前封存路径 |
| --- | --- |
| A根人工审计 | [0173-A-audit](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0173-codrax-hmc333-A-audit-v1.md) |
| B根人工审计 | [0195-B-audit](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0195-codrax-hmc333-B-audit-v1.md) |
| root明确不销账 | [0203-acceptance](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0203-codrax-hmc333-acceptance-v1.json) |
| 实际请求重叠 | [0204-overlap](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0204-codrax-hmc333-actual-request-overlap-v1.json) |
| 冻结清单 | [0218-freeze](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0218-codrax-hmc333-frozen-v1.json) |
| 全仓完整终态 | [0230-stats](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0230-codrax-hmc333-full-stats-v1.json) |
| 原始计划 | [0260-plan](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0260-codrax-hmc333-plan-v1.md) |
| 完整实采当前描述及实际消息 | [0264-context](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0264-current-reference-context.json)、[0265-extract](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0265-extract.txt)、[0266-finalize](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0266-finalize.txt) |

旧34代13881件保各原ROOT；最新§332的365件本批物理重核，其前33代13516件仅核链头，不宣称全历史payload重hash。既有33代导航和失败快照见[上一汇总](parallel_selected_summary_hmc_scope_zero_netease_20261101.md)及其历史链接；不递归复制历史档进当前sealed。旧§321/328等失败无后置publication回签。当前publication独立检查相邻源码/恰好5文档、main干净/远端同代、每个冻结输入与两提交blob、全部347件原件及所有真实终态；closure只允许明确none。

## 7. 52项按ROI排序与下一双轨

以下每个稳定ID恰好一次，与[任务原勾选](../docs/design/hmosperf_implementation_tasks_20260917.md)对应。层级按影响面、可复用性、已有反例/材料和验收成本综合排序，不以名次授执行/根因权限；持续18.5跨层轮换。

| ROI层 | 数量 | 稳定ID（层内由前至后） |
| --- | ---: | --- |
| P0 共享事实、schema、成本与补齐 | 7 | HMC-01.3、HMC-02.4、HMC-16.4、HMC-16.5、HMC-01.2、HMC-18.4、HMC-12.5 |
| P1 有材料/近期原合同和持续轮换 | 7 | HMC-18.3、HMC-18.5、HMC-11.2、HMC-03.2、HMC-14.1、HMC-08.3、HMC-18.2 |
| P2 原生量测复用与较成熟能力 | 11 | HMC-04.3、HMC-04.5、HMC-08.1、HMC-08.5、HMC-12.2、HMC-12.3、HMC-17.6、HMC-17.7、HMC-05.2、HMC-10.2、HMC-14.2 |
| P3 来源/协议扩展与安全前置 | 21 | HMC-04.4、HMC-04.6、HMC-05.3、HMC-06.2、HMC-06.3、HMC-08.6、HMC-08.7、HMC-08.8、HMC-09.1、HMC-09.2、HMC-09.3、HMC-10.3、HMC-12.4、HMC-12.6、HMC-13.1、HMC-13.2、HMC-13.3、HMC-14.3、HMC-14.4、HMC-14.5、HMC-15.1 |
| P4 新领域/材料依赖高 | 6 | HMC-07.1、HMC-07.2、HMC-07.3、HMC-07.4、HMC-15.2、HMC-15.3 |

下一完整能力轨优先原代表规格的线程/窗口/每组预算→真实执行与同代终态，二进制原有限合同在已有材料上轮换；共享轨优先实际工具schema、合法原生选择器与执行状态的简洁结构化交接。不要再从自由label/当前地址猜物理关系，别将预览作为全集，不扩大语义硬门；更广整组预算/去重依然开放。已充足事实后的IO/采样偶发误述保原失败，不无限追同一道题。WAL零副作用证明前不接生产。
