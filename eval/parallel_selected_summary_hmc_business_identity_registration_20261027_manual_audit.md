# HMC302 双例人工审计

日期：2026-10-27，运行收据UTC为10月28日。**A完整FAIL；B有限正向PASS；完整能力新增0，54项仍开放。** 原问题、输入、预算与退出条件不因本次结果改写；机器2/2不是完整人工2/2。

## 1. 审计范围和真实并行

A原`trace_business_io_sequence`，构造ftrace业务夹具，15步、一次；不冒称实机采集。审计4份真实query payload、3份实际Finalizer INIT、提交/补丁、最终Markdown/HTML/answer-surfaces及root-causes旁路。B原`TestNativeRegistrationLiveNaturalControllerTerminal`，20分钟上限、一次；实际父controller、planner和独立child模型调用，源码初始准备由harness控制，不冒称完整CLI或自然源码创作。

正式进程收据A/B exit0，冻结输入前后均true。原始logger逐唯一request/response匹配：A11、B9，无未闭合或歧义配对；最大实际请求重叠16.296秒：A日志260–261为PDT18:35:40.029–18:36:09.895，B父日志637–638为18:35:53.599–18:36:10.425。原冻结overlap helper因非法UTF-8退出1，未修补/重跑；根侧独立字节级检查只作审计，保失败原结论，不用屏障或进程重叠代替请求证据。

## 2. A：业务Trace时序题——FAIL

### 2.1 原生事实及新共享通路

查询总体为0.999–1.051秒/52ms，主线程运行7ms、可运行1ms、睡眠44ms。业务OpenDocument为1.000–1.050秒/50ms，其内部运行5ms、可运行1ms、睡眠44ms；LoadDocumentIndex为1.0045–1.0445秒/40ms，运行8ms、可运行1ms、睡眠31ms。worker闭合睡眠实际`[1.009010,1.040010)`31ms，主线程睡眠`[1.001,1.045)`44ms；读请求1.005–1.040驻留35ms，不等于设备驻留或全部线程等待。

以上不同范围/量尺的事实都已到实际成文上下文。新字典实际触发：INIT03有23个内容body、46个note定义、2个独立query receipt、26个唯一`(observation_id,view)`导航；3个body由window_stats/wakeup_chain各引用一次。缺定义、行数账不符均为0；另有43条导航明确整条省略，不能称所有原生表已全部投递。预算省略不被计为完整发布。

本次确有已投递的调查冲突，不只是猜测：`emit_investigation_complete`的`/native_fact_handoff/interpretation_units/0/text`及`/aggregate_facts/0/member_notes/0`已把50ms写成7/1/44；错误解释进入INIT03 L620，与L483正确原生业务表并存。最终emit同样错配，说明共享交接同时供给了冲突事实解释；仍不能仅凭相似文字断言这是唯一成因。worker宽窗9.5ms的另一条错误member note未被最终正确的业务8ms段落继承，分别留证，不泛称全部摘要错误均传播。

最终原始emit的4个blocks没有`runtime_measurement`选择器，调查handoff虽选择4组原生事实，不能据此宣称终稿原生表绑定自然验收通过。新共享预览进入上下文与最终答案正确复用是两个验收层次。

### 2.2 正文和图的可核失败

最终原始报告`.codrax/output/20261027-183928.038-51389.md`：

| 位置 | 已到场事实 | 实际输出 | 判定 |
|---|---|---|---|
| L15摘要；L55图Note | 50ms业务内运行5、可运行1、睡眠44；52ms查询窗才是运行7 | 50ms里写7+1+44，合计52 | 错范围，FAIL |
| L20/26/43/46与L23 | 实测闭合睡眠31ms、独立优化估计也为31ms | 前后承认实测31ms，L23却无范围限定地称“不是实测等待长度” | 量尺解释冲突，FAIL |
| L35 | 业务50/5/1/44及worker40/8/1/31正确 | 与摘要/图矛盾 | 正确段落不能抵消前述失败 |
| L46 | 请求驻留35ms与阻塞31ms分别展示 | 正确区分且未相加 | 此限定通过，不倒签旧IO解释失败 |

最终图只有3lane，不是旧6主体/3孤立角色形态。原生实例锚点和两条wakeup边仍保留；可见alias改为“存储完成/文档工作线程/主线程”，原生线程名被隐藏。`runtime_participants_v1`有3绑定、原生edge anchors有2条，无`runtime_notes_v1`，所以图Note的业务范围数字仍是自由文本，未同源绑定。末尾仍保三项unproven关系边界。**不能因此断言已取得新的业务关系权限，也不能称图身份问题已闭环**：已证native边与业务名称映射未证/边界文字之间仍有展示语义冲突。未新增关键词扫描、未放宽精确身份门；安全边界公开测试和自然图结果各自留账。

31/44ms睡眠、35ms请求驻留和两次精确wake保留，背景backup请求未替代链上根因。Trace投影/业务线索存在不等于整答正确；不以原生表或旁路正确盖掉正文错误。

### 2.3 产物与上下文成本

最终MD SHA `318d620ee0f4b4fa86c51bd117b768bc8cddcda8535e012b515fe62a997875f0`；HTML SHA `569f4d32ae05c7dcd60889438a7a537a3a670a2d9bf5009f6c1487460a4a3d6a`。源码级HTML/Markdown四表单元格一致，1个Mermaid fence/element，surface索引的Markdown摘要匹配；这不是浏览器像素渲染验收。图的错范围与原生身份隐藏不因格式可解析而通过。

A原始log SHA `4230d507dcaf7838017c985a2f39baa6ae807b877e557a23a66ced165302438d`，整体包含非法UTF-8，原字节保留；3份INIT按实际日志切出，不声称看到了未记录provider wire。机器指标为4次成文拒绝、峰值161703估计tokens/200000窗口约81%、250秒墙钟；不是付费token。公共夹具2.18%局部交接减量不外推此真实请求下降；整体预算/重复块债继续16.5。

## 3. B：只读登记和独立恢复——有限PASS

真实请求为父controller5、planner2、child controller2，共9。原始只读登记计划`changes=[]`、1个existing-test observation、无probe；完整读源码与测试后才登记。独立child PID57765不同于父51344，恢复后产生新的native invocation，与旧/source invocation区分，未把历史通过直接当本轮执行。git HEAD未变、diff/status空、source/test字节SHA不变，readonly_checked/readonly均true。

当前ChangeReport是1条原生assertion结果、2命令（syntax preflight＋unittest），最终`verification`同为1/1/2；累计`proof`为2报告、2结果记录、4命令，不是2个独立测试。`probe_count=3`是confidence记录数量，不是3次新probe；计划没有probe。进度正文明确source=1、本轮=1，未知中间结果数未补零。

两目录分别保同7份原件，各7/7的bytes/SHA核齐，不是14次执行；3个attempt引用保留旧失败报告，2个实际`links`分别连source final与本轮final到同代原报告。最终说明把源码owner与后置validation-only计划分开，不把验证计划说成代码修改来源。原始临时路径已清理时，交付路径采用保留副本archive_path，不谎称原路径仍在。

旧attempt仍为unverified/verification_proof_incomplete，由后继精确proof dependency闭合，不回写旧attempt为PASS。五态上下文包各6个逻辑ID、无ID碰撞或引用累积，content SHA一致；持久化ID规范化与内容一致分别表述，不谎称所有artifact ID都相同。

仍未覆盖：完整CLI、自然源码定位、多语言、多断言、缺来源/失败来源自然负向；最终仍披露source localization weak和apply_checkpoint_missing，不代签这两项。本批确定性缺来源负向保护通过与自然正向结果分栏。

实际planner上下文有`verification_probe_required=true`与允许只读现有测试登记且不混probe的教学冲突。本次正确选择无probe登记并完成，留共享教学债但不无限阻塞其它ROI。B采集器派生诊断误用`final_report_links`字段，真实`links`原件和同代路径已独立核齐；冻结后不改helper，不把派生null说成生产丢失。

B父日志SHA `9ea847b611bed9239516daf18df4937d173c577004b35f3c39ce3db80a065987`；child日志SHA `9728cfbcebb1b002dacac16f93893194bf1e0cf83e4f307d4ca8e13bc2c52b36`。独立B审计含逐请求UTC/行号、每份原件SHA及JSON指针，随本批封存。

## 4. 留账与下一步

本批明确保留四组欠项：①范围/量尺的正文与图解释冲突；②业务角色可见名、原生线程身份和关系边界交接；③整体上下文重复/预算，及登记probe教学冲突；④审计helper的UTF-8/主机Ruby兼容与派生links字段。旧采样、IO、栈关系、代表和权限未触发仍归原稳定项，不能以本次PASS清账。A未使用最终原生表选择器、B缺来源负向未自然触发，分别记录，不隐藏为PASS。

唯一全仓97包PASS/12无测试，28379顶层/33322子例PASS、零FAIL；冻结前后输入不变，候选不进main。后续按主体×范围×量尺的结构化证据绑定修系统接缝，不扫自由正文做硬门。完整能力轨提升已有真实PCAP材料的只读流式接入可行性，跨源校准前提不取消；同源采样包装不算新格式。活跃WAL未达零副作用，保持生产NO-GO。

正式封存和两笔候选发布收据见[批汇总](parallel_selected_summary_hmc_business_identity_registration_20261027.md)；独立审计、原始失败和NOT_TRIGGERED不随后置文档改签。
