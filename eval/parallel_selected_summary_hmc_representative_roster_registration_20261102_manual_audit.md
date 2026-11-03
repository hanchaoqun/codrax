# HMC335：逐例人工验收与不销账边界

[完整index](results/hmc_representative_roster_registration_20261102_v1/sealed-index-v1.json)封存508件，SHA7d871d85ad3a3731fd11d08e48abeae3e39a10a9f237d6eceb707e3706df00e9。[A原审计](results/hmc_representative_roster_registration_20261102_v1/sealed-v1/0109-codrax-hmc335-A-audit-v1.md)、[B原审计](results/hmc_representative_roster_registration_20261102_v1/sealed-v1/0132-codrax-hmc335-B-audit-v1.md)保原；本页是提交保护的汇总/精度更正，不回写原件。完整新增0、子能力1、79/28/51，不用测试/进程/部分表格替代原有限或整答审计。

## 1. 终态矩阵

| 项目 | A 原代表自然入口 | B 原只读登记自然控制器终态 |
| --- | --- | --- |
| 原问句/入口/次数 | 原50steps CLI / 1 | 原planner12/default controller/20m / 1 |
| 实际OS及源前后 | 0 / true / true | 0 / true / true |
| 原operator / harness | **FAIL** | **PASS** |
| 原有限/完整人工 | **11.2 FAIL / 整答FAIL** | **原有界入口PASS，非完整CLI答案** |
| 新无损名册实际交接 | **PASS，actual INIT自然触发** | 无该运行时名册，不代签 |
| 图/局部图修补 | 无图/未触发 | 不属于CLI表图验收 |
| 本次二进制/缺来源负控 | NOT_RUN | 本例正向不代负向 |

唯一full OS0，98包/28712顶层/34295子PASS，三层FAIL/非JSON/重复/未终态0。原request/response A36/B9；严格唯一区间A14/B9，A另2歧义组排除，无未解请求，最大重叠17.523秒。实际区间而非barrier证明并行，B父/子是同一原问题恢复，不计第三例。

## 2. A：先看计划与执行，不让正确资源值替换原任务

4完整SQLite是原两夹具各两份复制，来源分开，oracle在模型目录外。910行模型明确request_plan.view=event_search；受理线程/窗/每组2正确编译，没有view字段被系统丢失。唯一production receipt4333行绑定index1747403f…与selection9253c316…，不是模型正文/latest拼接。

| 原选择规格（均event_search） | 选中 | 未选原计划 |
| --- | --- | --- |
| 101、数值窗10–10.05 | d/b，各2次success | a/c，各0次not_executed |
| 202、数值窗10–10.05 | c/a，各2次success | b/d，各1次empty |

17查询记录＝8声明计划＋9资源extra；23实际attempt＝10 event_search（8成功/2空）＋13 resource_stack成功，15唯一JSON＝6事件＋9资源。15记录有尝试/2未尝试、21success/2empty。合法未选查询真实发生，不把unselected等同未执行。原结构operator实际OS1，错误cluster specification or full membership differs：选的是event_search，不是原resource_stack矩阵。附加资源查询不能回写选择归属；原断言不改，11.2不销账。

## 3. A：新名册真正无损，旧缺尾不再冒充模型原因

actual INIT2第17行真实4源/2组/8成员字典。逐项还原所有source tuple、member和group，同代selection的8个成员/2组全相等；8个attempt_outcomes逐个对原index尝试历史一致，query_origin均declared_current_plan。4selected/4unselected/2组显示全部0省略。当前2未执行/2empty也全在场，旧§334只有3/4未选个体投递的问题在本分支已闭环；不是只检查字符串ID出现。

选择外9查询另6展示/3省略，名册总量明确且不改原8人口完整性。新packing不截字段、扩大预算或恢复私有权限。3条INIT节块67464/82525/230278B，非provider wire/tokens；1次finalizer不是3次成文。没有主名册缺失或OOM证据为最后错述免责。

9资源JSON/16事件载体/45帧载体对冻结SQLite原event_frame_rows逐项核事件时间/身份/操作/size/address/end/callchain、帧depth/IP/ID/offset/vaddr均一致，[原核对数据](results/hmc_representative_roster_registration_20261102_v1/sealed-v1/0130-codrax-hmc335-A-native-crosscheck-v1.json)。按载荷统计，不把重复查询或相同字节的不同来源合成一物理事件；半开资源窗排除10.05边界。

## 4. A：成文失败与上游债分开

终稿确实回到线程×窗组，具体4代表正确，且分开同TID不同来源；不再将内容形态替代任务组。仍有原任务内失败：①选择view错误；②先正确逐组列2未执行/2empty，随后误写“四个未执行和两个空”；③承诺四张事件表却只提交3个event_search表。最终HTML5表＝3事件/1跨源对齐/1定位，源数据保真；不是renderer吞第四表。MD/HTML唯一cell诊断不等在定位表code反引号，非数值丢失；source-only，无像素QA。没有图、附件图、patch覆盖，不能补签。

actual INIT3只传unit0/6，591行披露省6455B；其“第一条end_ns=null”与实际10025000000冲突，当前合法native_ref只签地址不签该文字，已转发上游债保留，最终没有重复此字段。未投进度单元还写旧4预算外未执行，不冒本次已投污染；旧地址哨兵/有效性、unknownCPU诊断被误当资源丢行的上游文本也保原，终稿没重复不扩大失败范围。关系表达已有非有序链声明却继续方向式列帧，属于歧义/旧关系债，不仅凭箭头字面判调用，更不设关键词硬门。原生event_search双端包含与资源右开分别保留，不能用统一组标题认证不同查询的端点含义。

最终MD91070B/SHA710c2e758f766a3a9ff5c56921ba66020ed1e0f82e4470f3a95c754dcd6bd860、HTML2850163B/SHA56d064f088e9886c9a5b4f6727ead799f7004148da9cc9a84e1576ca51110c27，surface摘要匹配。57声明全部解析，6次completion中3失败/3受理，原失败保留；1次full answer受理/0patch。原有限/整答FAIL，不重跑第三例。

原A手审精度更正（不改封存）：§1草稿残留“43?不作为计数”不采纳为统计，本代有效计数为57声明、23实际查询、36模型请求；§2“9次资源查询”应精确读为9个资源规格/9唯一JSON、13实际尝试。原数据、原operator FAIL及最终裁定不变。

## 5. B：真实自然控制器、只读登记、恢复与终态

父68636/子73558，evaluation明确real_model_parent_and_restored_child、full_cli_run=false；原源码修复由公开fixture准备，verify执行器是公开RunTests，不冒模型代码创作或模型verifier。10条工具声明：父4verify＋1plan、planner两读＋emit、子verify＋finish；声明不是执行计数。planner真实读取value.py/test_value.py后选当前test_value.ValueTest/test_increment与assertion_ref，绑定原increment-result，changes为空；原计划/授权/测试摘要完整持久化后由生产load/hydrate跨进程恢复。

源码交付plan-1793671040679668000-68636，commit a2260e5cf279b262cd7e6bcc4376db37a773d545；登记plan-1793671130537004000-68636的结果仍准确归属原source/diff，不错归补证自身。旧native:10c1c:18e4664ef49faf00:1不复用；来源验证新:2、中间补证尝试:3及子新native:11f56:18e466664c0eed20:1各自独立。中间unverified/verification_proof_incomplete原attempt保留，最终以superseded_by_verified_proof_followup说明新证补齐，不重写原失败。source/proof/run最终verified，不能用before-restart in_progress代签。

新实际执行1既存断言/0失败，最终1测试/1通过/2命令，source/proof两WriteFinalReport各绑自己的不可变版本report；父子各7原件/3verify引用字节SHA全匹配，全部final→report实际字段/planID/SHA匹配。两份归档不算14次执行。restart-child-audit明确HEAD=expected_head、git_diff/git_status空、源码/test SHA与预期一致；公开原断言确认授权撤销。上下文包四快照各6唯一逻辑ID/6唯一artifact、内容一致，落盘轮转幂等，不以artifact ID换代推断内容丢失。

有界完整人工PASS，不冒完整CLI、自然源码创作、多来源/所有框架或缺来源负控。父日志有NUL但父子UTF8均有效；A日志无效UTF8原字节保留，区间配对不修原日志。无独立CLI最终模型正文/HTML/图，不补这些表面QA。18.5持续开放，本例不是新增完整能力。

## 6. 发布与后继

源码4文件/352新增、公开RED与全部旧失败保原；唯一full及508封存不签A通过。完整实采82838676B/1nativequery/0模型继续保护627.645/194.581账户与实际消息，不冒第三例或重跑旧独立oracle。旧36代14781件保各原ROOT，新37代累计15289件，物理重核最新553件和更早链头，不冒全历史重hash。

3组剩余验收（view/执行归属、状态/表面覆盖、原生/调查范围属性）及旧采样/IO/权限/图/预算平台欠项，见[51项ROI](parallel_selected_summary_hmc_representative_roster_registration_20261102.md)。后继轮换其它有材料完整能力，代表不在同批无限追跑；未覆盖的新容器反馈、图/patch、缺来源负控及二进制分支不销账。WAL零副作用前生产NO-GO，Trace因果投影/自动补齐、原4候选、闭合睡眠和链上业务信息不变。[publication](results/hmc_representative_roster_registration_20261102_v1/publication-v1.json)只核相邻main提交/冻结/全仓与无新增闭项，不冒模型全面通过。
