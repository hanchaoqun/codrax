# HMC292 人工审计：代表同代交付与只读登记

结论：**A完整FAIL；B限定只读登记链路PASS。** 两例各一次并行，未改自然问题或补跑模型求绿。HMC-11.2及其余父项不销账，54开放、25累计完整交付保持；本批交付2组共享子能力，完整能力新增0。实现和验证见[批汇总](parallel_selected_summary_hmc_snapshot_budget_registration_20261025.md)。

## A：原选择与执行正确，终态分支未触发，正文仍失败

原题为按线程和窗口归类、每类挑两份代表，对比10.000到10.050秒内线程101/202的分配、释放及调用位置、检查进度。没有工具名/护栏答案提示。4个输入文件是两个已有SQLite的复制，不是4台设备或新格式样本。题目235字节SHA `51343bc029c3241ce53dffaa5318cf3c642f6d344f749a080afa2ae8f461b96a`。

实际标准CLI PID91789，exec UTC03:50:15.678241，退出收据03:53:41.937680、exit0；这只证明程序正常退出，没有独立机器语义PASS。完整诊断日志、9条实际INIT（3条Finalizer）、18份工具参数、4份完整原生载荷、两份catalog回执及输出原件已读并保留。实际消息来自诊断日志，不声称完整provider wire。

| 原件/线程 | 选择与执行 | 原事件/物理帧 | 独立统计组及流量 |
| --- | --- | --- | --- |
| a/101 | selected/success | 3/8 | 2组；allocate2/768 bytes，release1/512 bytes |
| a/202 | selected/success | 1/3 | 1组；allocate1/128 bytes |
| b/101 | selected/success | 1/3 | 1组；allocate1/512 bytes |
| b/202 | selected/empty | 0/0 | 0组；真实查询零匹配 |
| c、d/101、202 | not_selected/not_executed | 未查询 | 不以operator oracle替代执行，不宣布正常/零/失败 |

真实request_plan形成8计划/2规格组/4选4未选，4次普通原生查询后3success/1empty。5事件/14帧与独立oracle一致；窗口[10,10.05)，端点外事件未混入。100ms原件声明范围与50ms查询窗分开；unknown符号、signed -1、缺深度及重复深度保原。无原生漏数/算错证据。

初始discover同时返回index SHA `22a820cbb27701044f577b6488a1834b1abe8c69338e1c4d43f2295a3db43958`及selection SHA `ca9ed481b181cdbd696151d1ce6062f235683c33c29a54da70ed8db1155eb426`，快照绑定同次索引。之后4查询没有改写它们；普通status返回当前index SHA `4ad13345458d435cc2b5300e46ae2bcf865f8e3ee3c3d5baad9d47b259bf3835`及正确执行集合。初始pair是pending代次，不能拿它与最终index拼接；本轮没有representative_status，离线助手执行0次，目标终态三元组 **NOT_TRIGGERED**。初始pair自然命中不等于代表选择未触发，也不等于最终同代收据已经验收。

239行MD、完整HTML正文/12表、两个sidecar已审；表格cell一致，UTF-8有效，没有实际Mermaid/SVG/正文图片，图N/A，未做像素QA。MD SHA `68086c68aec0ba4b51d4bfa81f0a234015cc92817de414cd3a79f60c6c069544`，HTML SHA `e5a8dadcb0660915798efe215add1673d1a09558ff3062e0c7caf173a15f17da`。

完整FAIL的三类直接证据：

1. 开头将malloc、ImageCache::reserve、UIFrame::render的深度记录连箭头，后文称“上层”；原生只保证记录深度，未授真实调用方向/完整展开。最终调查member_notes已经带箭头，与原生限制竞争；本轮无独立interpretation_units字段，不能套用前批字段或断言唯一生成原因。
2. 进度段称a/b线程101各1独立组，而a101原生及最终原生表均为2组。实际Finalizer已收到“3事件/2组/已展示2组”，属于规格组/统计组的自由解释混淆，不是漏供事实或旧索引覆盖。
3. b202段“不是已测得0次分配”过度否定查询域真实0。应同时保留成功查询零匹配与未知采集完备性，不能互相替代。把Background进程名叫进程实例也保留为小项，实际实例为2，原生身份表准确。

文末正确限制不抵消正文；根因sidecar为unavailable/trace_root_cause_contract_not_active，没有新CPU/等待时长/泄漏/响应根因结论。本轮未自然覆盖跨分支并发合并、peer-only导航、失败重试及未选实际执行；它们仅由公开定向/race另证，不限制这些合法行为换绿。

## B：自然规划、独立恢复和新测试凭证限定通过

原题：“实现已修好，请运行保留的现有测试，确认返回值正确。不要再修改源码、测试、配置或依赖。” 不含内部工具护栏。PID91790实际exec UTC03:50:15.709494，测试152.95秒、外层03:52:49.544534 exit0；自然父controller5次、planner2次、独立子PID7997 controller2次，共9个真实模型请求，仍是1个场景。

既有实现和历史测试由operator公开emit/apply/Git夹具准备；verify执行使用真实RunTests/verifyPostHook，非模型源码创作/模型verifier，**不是完整CLI**。父source测试通过但无精确合同见证；中间补证再次真实测试仍缺证，attempt保持unverified。planner实际read_file源码2/2行与测试4/4行后，登记test_path/assertion_ref/contract_ref，无文件修改或新增probe。新进程恢复后重新执行，不能用旧PASS代替。

| 执行 | invocation | 断言/命令 | 归属 |
| --- | --- | --- | --- |
| 准备历史 | native:1668e:18e1f7d3da4de068:1 | 1/2 | 非本轮新验证 |
| 本轮source验证 | native:1668e:18e1f7d634266ea0:2 | 1/2 | 来源通过，仍无登记见证 |
| 中间补证 | native:1668e:18e1f7e2ca48b440:3 | 1/2 | 测试passed但proof incomplete |
| 子进程新登记后执行 | native:1f3d:18e1f7f587359908:1 | 1/2 | 精确原件/断言/合同/来源commit绑定 |

当前登记digest `46242dee86c1961d1cdbaef971f39f2055ef91e7758c2790401327b89305dfac`；source计划`plan-1792986615821349000-91790`，当前登记计划`plan-1792986748762448000-91790`。source commit `49d8ede429e9e4185d366e9252ada3d458c4f8fc`与实际HEAD相符，diff/status空，源码/测试SHA前后相同；本轮未改源码、测试、配置或依赖。

父/子分别捕获7个报告原件、3verify refs、2final→report链接，逐bytes/SHA一致，不等于14次执行。旧source final仍in_progress、保原引用；当前WriteFinalReport明确validation_only与source_plan_with_later_validation_followup。当前1测试结果/2命令，累计2报告/2结果/4命令不表示两个独立测试。中间attempt仍unverified/verification_proof_incomplete，完成字段单独记superseded_by_verified_proof_followup，直接依赖当前精确见证，未覆写历史失败。

缺省恢复初始预算1/5，后续2/5→3/5；最终三个唯一批次complete/verified，批次数源来自宿主。五份快照各6逻辑包/6唯一ID，逻辑内容SHA相同；child_memory的active merged ID保存后规范化改变，后四份持久ID/引用稳定，不能说五份所有ID一致。临时文件由harness清理前oracle核对，独立人审复算留存快照，不宣称事后又读取已清理临时文件。

限定PASS不覆盖：自然显式cap耗尽拒绝（定向/race另证）、多语言、多来源、完整CLI/自然源码创作、全部交付展示。当前source定位weak/observed_only、apply_checkpoint_missing诚实保留；旧exploration rounds 1/0仍可见，本批只统一批数预算，不称全部循环预算完成。没有本轮MD/HTML/图产物，图QA N/A。

## 证据与后续

独立报告：封存内`tmp/codrax-hmc292-A-manual-v1.md` SHA `8e6c70a3c9807c5e0bd9a1f3c07fb22a6586396357ae9c74b6774a54b20c6244`；B同名SHA `864195ac78432989ea160c0cc67b18b56039a20c384756d14a796063c6ee717a`。B零模型复算v3通过；此前审计脚本对持久化refs及null数组的两次失败同样保原，不是产品回归或额外模型。完整路径/原件索引及发布终态见批汇总；本地载荷按项目规则忽略，不推送凭证/日志。

残余3组：代表最终同代三元组自然覆盖；原生事实、调查和正文一致性；只读登记更宽交付/探索预算展示边界，均在既有54项内。旧采样、IO和权限覆盖债不倒签，WAL未达零副作用不接生产。
