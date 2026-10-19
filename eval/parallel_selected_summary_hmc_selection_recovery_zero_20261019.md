# HMC266：ZIP 选择恢复与零结果交接验收

日期：2026-10-19。基线 `587855ba3`。总账 **79 唯一 / 24 已交付 / 55 开放**，无重复。代码两轨：可修复成员选择与物理坏源分型；模型负声明与真实空查询分层。旧采样语义失败不销账，不以确定性测试代签真实答案。

## 冻结与运行

一次冻结、两例并行、各一次；没有第三例追绿。冻结7144输入，SHA `2871a98fcd8de8cc77d0ce6c7dc9897b10fef21b333db4a1ef37abfee1bbfef0`；实际CLI SHA `f9673e77fc4fc521268642892f4c6d440810285760db33bf29184e3eb39f55db`，B测试二进制SHA `cf8d57f0b861971ab2af4418ebeb7ecf5a92a4d0c6a5a703c305cfbbac58cc67`。构建成功后冻结，生产/测试未再修改。

| 例 | 正式父PID、UTC起止 | 机器 | 独立人工 |
|---|---|---|---|
| A 自然命名ZIP指定成员采样 | 17830；12:50:29.640282–12:52:39.126438 | PASS，exit0 | **整题FAIL**；入口/完整准备/同窗自动补齐PASS |
| B 保留测试只读登记与独立恢复 | 17943；12:50:30.705354–12:52:03.301955 | PASS，exit0 | 限定合同PASS，不是从零创建或完整CLI |

正式结果根：`eval/results/hmc_selection_recovery_zero_20261019`。A目录 `trace_zip_selected_member-20261019-055029`，B目录 `native-registration-natural`。问句保留自然对象与窗口，不注入oracle/防护要求；完整问句、脚本、冻结、原件和正式终态收据独立保存。A父日志SHA `d33f9101aa711563abc6c4263aea5731f5b2fad16e24b4d1f82741c64ffe3a85`，B父日志SHA `deb763b66d34979b2b63a7153b293099ad223926405d1cc0ce33a04a3bec3172`。

## A：入口通过，解释越权继续留账

自然问题为：请看指定ZIP里的 `captures/device capture.data`，在486489.4到486489.6秒内poll_report线程（tid1609）的采样热点和调用栈主要集中在哪里。无 `--htrace` 预选。运输ZIP含同一真实PERFILE2的两份命名副本，只有一份独立录制，不冒称两设备或实机ZIP。原件30,062,585字节、SHA `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`；ZIP及原件在审计后保持原字节/文件身份。

模型首轮即精确选择成员，3次原生查询（window_stats/perf_stats/process_profile）全部成功，完整索引12,000条。系统另实调1次window_stats补齐，复用相同成员与[486489.4,486489.6)显式窗、目标线程，不用预览代全集。perf_stats伴生完整采样树已成功，不能因未单独调用tree view就说没有栈。**选错→正确重试、未执行→误写零两分支本例均NOT_TRIGGERED**；其公开正常Run/emit/patch反例另有确定性证据。

独立参考解析器oracle与原生载荷一致：全窗1393条、目标164条、权重259；CPU0/1各82条、权重85/174。最终正文和表格正确保留这些分母、比例、CPU及记录时刻包络，未把权重换算耗时。本次没有旧“全在CPU0”错误，不沿用旧指控。

真正失败在最终MD第19行、HTML及primary/principal：把单帧depth=0解释为“表明采样时…正处于内核中断上下文或回调函数的叶帧位置”。实际只记录PERF_SAMPLE_IP单帧、sample_kind未知，不能证明中断上下文、执行状态或完整展开后的真正叶帧。末尾正确caveat不能抵消正文的确定性越权。最终上下文已有精确采样事实及边界，不以本例证明模型只是偶发，也不加句子关键词硬门。

本题未要求图，实际无图；根因sidecar为unavailable/trace_root_cause_contract_not_active，未把热点提升为因果链根因。峰值上下文估计90,280/200,000 token（45%），属效率警示，不证明它导致上述误述；同源多查询/补齐的预算与复用问题留既有01.3/16.4系统轨。

## B：只读终态和历史代次

真实父控制器、登记planner与独立恢复子进程运行；初始实现、应用与旧执行由operator fixture准备，原生RunTests实际执行而非模型自报。源码/测试不改，恢复子Git clean。当前验证 **1条断言、2条命令**；累计证明 **2份贡献报告、2条断言、4条命令**。实际4个distinct native invocation＝operator旧准备1＋流程3，不能按7件工件或兼容副本计执行。

父/子各保7个原件，3个verify attempt、2条最终说明→精确报告链接。逐字节SHA、5个ChangeReport canonical及链接均独立复算匹配。历史source final仍in_progress、旧proof-repair attempt仍unverified；后续成功没有改写历史原判。真正最终说明与当前ChangeReport分开，旧临时路径清理后以归档映射取原件，不承诺临时路径永久可下载。源码定位weak/apply_checkpoint_missing边界保留，不外推完整18.4交付。

## 确定性验证与剩余范围

选择恢复：正常Run真实工具6种成功恢复、2种CRC终止反例；109根旧消费者/race通过，4个环境/opt-in skip明示。零交接：公开emit/patch6子RED、编译origin/marker4子RED、独立claim-binding3子RED均修后通过。types273根、tool720根、agent44根的直接旧消费者race通过；3个旧纯模型fixture不再期待系统制造零证明，原失败与原声明保留。

独立公共阶段4根/13子覆盖真实completion→ctxbuilder→Extractor/Finalizer、实际event_search空查询、日志/MCP原件。早期测试夹具缓存/编译失败没有冒称产品RED；Extractor既存MCP正文展示不足单独留01.3，未扩大本批协议。显式窗、链上因果、自动补齐和L1保护通过，真正坏源终止未松动。

本批完整稳定任务交付新增0、已交付子能力2组；人工未通过1例，另有2条未触发自然分支，不重复增加稳定任务数。55项保持开放，更多二进制格式/平台与旧采样解释继续原ID。

## 全仓、封存与发布

同冻结全仓PID17746，UTC12:50:28.352010–13:05:49.224282，正式exit0：96包PASS/13无测试包SKIP，零fail/非JSON行，15个HMC266新增根同次PASS。JSONL SHA `9483acaba76427e95343007d506529c27e0043c69f3fb5471b8eebbade381521`；未用包中途摘要预签进程终态，无第二轮全仓或第三例模型。

UTC13:06:13.807717封存完成：54份既有原件不变，90份过程材料、实际CLI/ZIP及27份最终/运行原件，共119份逐字节复制。173文件SHA索引 `b40f22c6a0b2efcf4602eaf32360cb8cee8b7681149cab34984e07fa9e01ed9c`，归档收据SHA `fd7b6b436201aaed4c270495629d5db1e7a82e645c1614124ee762f72dfc611c` 另存避免自循环。全部RED、夹具失败、审计及原始机器/人工不同结论保留；未读或复制provider凭据。

代码 `0313994d220c96d6482644b6a7bf03871b81e0f3` 已正常推送；push PID5579、UTC13:06:34.796879–13:06:43.085274、exit0，日志SHA `baa95c37c8d92033e15a64ee2a5e94d6db903ad64bebb56a67340766d952b6a4`，推后0/0。文档分笔发布；最终远端HEAD/clean与封存不变复算单列于结果根 `publication-receipts-v1/publication-v1.json`。

下一两轨：17.6 普通命名gzip的有界导航复用已有完整解码；01.3/16.4 同源展示身份与查询凭证分开、共享跨块预算。后者不按数值相等合并独立分母，也不将90280总上下文全归因采样树；初始固定schema/教学成本另留账。只读设计已随原证封存，未当作新增实现。

详见[人工审计汇总](parallel_selected_summary_hmc_selection_recovery_zero_20261019_manual_audit.md)和[主账本§266](../docs/design/hmosperf_capability_gap_audit_20260917.md)。
