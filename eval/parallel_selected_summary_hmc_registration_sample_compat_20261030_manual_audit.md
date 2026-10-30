# HMC324 两例独立人工审计

原问题、预算、断言和附件均未改，各只运行一次。两例进程exit0，原机器均PASS；人工A限定PASS、B完整FAIL。实际请求A3/B13唯一配对、无歧义，最大重叠3.490秒。机器、人工、完整能力及全仓终态分别记账。

## A：来源没有验证终态的只读登记——限定PASS

原 `TestNativeRegistrationLiveRestoredMissingTerminalReceipt` 使用真实planner和脚本控制器；独立子进程恢复并执行生产公开RunTests。中文目标保留在objective，但两份实际INIT没有逐字传它，planner用typed行为合同/只读补证框架。因此不能写成自然问句路由、自然controller或完整CLI通过。

两次公开read_file完整读取2行value.py、4行test_value.py；首次project_test_observations错误字符串数组被拒，第二次模型提交合法对象，精确绑定suite/id及increment-result合同，零changes/零verification_probes受理。原失败保留；assertion_ref分支未成功，不算覆盖。

历史invocation `native:117f2:18e34d0219f4bbe0:1` 与当前 `native:1186c:18e34d04c0c08dd0:1` 分开。当前仅1断言、1ExistingTestExecution、2命令（syntax检查和unittest），不能把命令、历史结果、归档副本累计为本轮多次测试。独立子进程PID71788真实执行；同代断言确认授权用后撤销，源/测试/HEAD不变。

proof批verified；source仅apply，无verify_ref/Completion；整体虽run complete，完成结论仍 `unverified / missing_terminal_verify_verdict`。脚本finish的reason不覆盖结构化结论，不制造来源成功终态。

真正生产 `.final.json` 为26342B，SHA `cb3b6f6cd82a8da801284ad9b7e3354b76621da2e878f02c51990008f60209a8`，精确指向7606B不可变内容寻址报告 `5381cf4228e9f2fc25459d9ffca264604b7c9bf865e0db9764636c875c029fde`。父/子归档各4原件逐字相等、hash匹配，历史来源报告未被覆盖；root final-report.json是ChangeReport快照，不冒真正WriteFinalReport。

范围继续限定：context-pack耐久未验证，保留原persistence_degraded/no_durable_store及materialization unavailable/apply_checkpoint_missing；源码只读不承诺临时文件/Python缓存零写入，不等于WAL验收。无Markdown/HTML视觉或其它测试框架覆盖。原件及详细逐行审计见本批封存的A-audit报告。

## B：完整sample-only二进制自然入口——完整FAIL

原件 `/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/tests/fixtures/hiperf_brbe_001/test_brbe.data`，30062585B，SHA `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`。本轮完整新副本附给标准CLI，原题15steps/n1；不把旧准备材料、标尺或答案注入模型。七份历史oracle只用于独立核验。

实际产出10份原生查询payload、3份成文INIT及一份HTML。目标线程PID1514/TID1609，查询窗[486489.4,486489.6)，完整总体164条/权重259。两个热点分别82条/174权重和82条/85权重；CPU1/CPU0为记录字段，不是调度执行区间。首末记录包络与全量数据一致，分母、单位和源身份正确。工具参数pid=1609在实际target_scope=thread合同下选线程，不误记为PID混用。

确定失败是正文肯定“on-CPU周期采样”“只有线程在CPU上运行时才会产生”，并把未符号化内核地址解释为实际内核态运行；同一答案的原生表却明确sample_kind未知、执行状态未确立。后文承认未知和不能解释停顿，不能抵销前面的肯定断言。旧采样解释越权继续开放，不据表格正确或smoke PASS销账。

已逐一核对3份实际INIT：关键unknown、单位、正确完整包络和统计边界均在；保留的1/5完整调查单元明确不得断言on-/off-CPU。其余被省略单元中错误.469586扫描终点、宽窗链路及相应reason原文未重传，最终也未采用。未发现关键事实丢失，110571tokens与历史裁剪本身不判失败；本例应记证据充分后的模型误述，而非臆定摘要污染。10次查询均成功，另有1次shell拒绝和1次调查confidence缺失拒绝，后续恢复原样保留；只一次答案受理。trace_supplement实际families_present跳过，不计隐藏查询。

两完整热点包络端点相减得到约122.5ms记录空隙，属于合法有限运算，不将该算术本身误判为跨层相加；但记录空隙不证明off-CPU或阻塞。深度0的两个记录节点不制造调用者；完整样本帧与unwind是否完整也分开。不得因此扩大为新硬件、BRBE/SPE或调度采集通过。

最终Markdown21927B，SHA `5bc6deda065629457d888eee76ab4bd39496a8f3f5bdee0d22fe12e821f7ec44`；HTML2801307B，SHA `c860efdccf300ce38e8da1778da467607ccb506c12bc9b3c4eae5c41411e8f0b`。只审HTML源结构/表格，不冒浏览器像素QA。没有模型图和局部patch成功覆盖；原始日志的非UTF-8截断字节保留，未改写原件求解析。

完整能力新增0，稳定开放仍52。§323真实业务账户丢失不同于本例已有未知状态提示后的模型误述；分别留账，不用统一“模型波动”掩盖已证系统缺口。独立完整原报告：[A限定负控](results/hmc_registration_sample_compat_20261030_v1/sealed-v1/0185-codrax-hmc324-A-audit-v1.md)、[B完整失败](results/hmc_registration_sample_compat_20261030_v1/sealed-v1/0199-codrax-hmc324-B-audit-v1.md)。全仓终态、397件封存及提交见[批汇总](parallel_selected_summary_hmc_registration_sample_compat_20261030.md)，不以全仓通过回签语义失败。
