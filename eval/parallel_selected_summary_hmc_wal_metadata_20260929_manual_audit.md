# HMC §193 双例人工审计

固定85921正式exit0，恰好2并行×1，没有第三例追绿。20260929-024850冻结构建revision `1c5f23818e1f`；机器2/2、完整人工1/2。WAL接入通过不等于最终答案通过；本批没有真实命中metadata patch，该能力由实际finalizer工具调用路径的确定性回归验证，不能倒签§192旧FAIL。

| case | 机器 | 人工 | 时间 | 实际验证 / 未通过部分 |
| --- | --- | --- | ---: | --- |
| trace_sqlite_wal_records | PASS | FAIL | 106s | 默认发现WAL、已提交更新与新启动记录正确；系统事件未知域/未解析名称被编造，时序重叠解释错误。 |
| nested_python_increment | PASS | PASS | 119s | 仅目标函数一行变化；嵌套项目3条原生测试真实执行，控制器完整收尾verified。 |

## 1. WAL准备与查询

自然问句只要求整理1.040–1.080秒的启动记录及同期系统事件。仅附加主文件，不把生成SQL、README/oracle或WAL路径列为用户提示。默认准备实际发现旁邻WAL，无SHM也可读。main=61,440字节、WAL=8,272字节、CommitFrame=2；原主文件末端1.045s被WAL提交更新为1.048s，新增RestoreTabs 1.064–1.072s。两次trace_query都用精确1.04–1.08窗，未读源码；finalizer收到8个去重行（两个启动区间的4端点及4条系统事件）。

准备收据 `.codrax/trace-input-66182cd643bd0b4677d0850405fcf433/preparation.json` 分别保留原main、原WAL及私有规范化镜像身份。main SHA=`40b91431aec185ab73c9fe853779a80f84da7d2b68a1b848955856703b67c23e`，WAL SHA=`278e9be77900e1bd1ffc40accb22af334042b458e71b18d7458b4001989ab731`，镜像SHA=`3d7e3c502044604e79e2381017bea090110fc51a7674dafd1561a54a6ceca16a`。SQL保真库存没有private_uncommitted表。本次Python生成器虽然保持未提交事务，实存WAL仅2个已提交帧，没有成功制造未提交磁盘尾；不能拿这例冒称覆盖尾帧隔离。该负控由真实SQLite spill且断言tail>CommitFrame的公开Go测试独立验证。

本能力仅接受准备期间源代次稳定的main/WAL组合；运行中的连接可存在，但持续写入/检查点导致文件变化会失败，绝不把它宣称为在线持续写入的一致快照。

## 2. 完整答案与实际模型上下文

正确部分：LoadPreferences 1.040–1.048s和RestoreTabs 1.064–1.072s均8ms，进程owner PID27599正确，明确不是执行线程。4条系统事件时间和contents准确；表头与s单位完整。初次emit直接使用summary内Markdown列表/表格，最终无patch、无图，不能据此证明真实模型已选择新无损元数据操作。

人工FAIL证据：

1. 1.050s原始domain_id为NULL，最终却写“STAGE_READY（内部）”；1.052s event_name_id=50重复引用无法唯一解析，最终却写成“APP_LAUNCH”。这两种未知身份在日志1961行的真实finalizer display_rows中明确为 `unavailable/null_reference`、`unavailable/unresolved_reference`，并未被截断；不属WAL丢数据，也不能仅凭本例宣称已证模型随机波动。
2. 正文“1.048s结束后…1.046s已发出”前后矛盾；RestoreTabs 1.064s才开始，晚于所列最后一个系统事件1.058s，却称与此前序列“有一定重叠”。不能把这种时间邻近解释当作因果链或阶段关系。
3. 系统事件全部归PID的泛化没有逐行证明。完整解析的HiSys行仍走旧print表示，向模型投递合成cpu0、emitter_tgid=tid；未知身份行则走sql_hisysevent、保source.tid及cpu=-1。源hisys_all_event只有tid，无CPU/TGID字段，当前两个出口的角色语义不统一是系统缺口，不能把本例全部归为模型误述。挂05.1/17.7/16.4，优先复用源记录表示统一来源角色，保普通可见性与不双计。

finalizer估算58,850 tokens，全链最大84,607/200,000=42%；源字段已完整到场，与前例问句不同，不声称token收益。原始上下文/输出保留；已精准供应的未知身份误述列答案验收，不再增加某事件名专用提示或扫描问答原文的硬门。真正的角色供给缺口单独优先处理。

根因旁路 `.codrax/output/20260929-025035.932-91858.root-causes.json` 为schema2、空根因、trace_root_cause_contract_not_active；未把邻近信息晋升根因。模型请求实际配置timeout/首响应10分钟、中途静默5分钟，没有活跃流降级。Meta时长/信号来源旧缺口仍留账，本片未涉及。

## 3. 嵌套Python写模式

真实流程plan→apply→verify_batch→finish，最终complete/verified/all_batches_verified。交付树与fixture仅 `packages/widget/widget.py` 第2行从return value变为return value + 1；测试、setup.py及其他源码逐字节不变。负数/零/正数三个unittest断言真实执行，invocation=`native:bca:18d9c1d28336a808:2`，suite=`python/unittest@packages/widget::tests.test_widget.IncrementTest`。

根目录discover为zero_tests、语法检查为syntax_preflight，两者都没有伪装成断言PASS；系统继续在packages/widget目录运行discover，3测试全部通过。报告worktree_audit=clean，真实控制器据新执行完成收尾，用户结果明确“3条验证结果通过、最终已验证”。这属于普通apply相邻回归，不代签只读登记/跨进程恢复；只读登记完整CLI已在§192通过。

seed主仓HEAD仍 `ccfdaf9f6fa4fae3c516764ea7be58aeb38a35e3`，无自动merge；运行器仅在主仓.gitignore追加.codrax/，不是生产源改动，不能说整个运行仓逐字节不变。原测试与交付树测试SHA均 `504b2535413cafa883be2802d92a698036735de8883384530caec4fa8375f322`。内部ChangePlan/worktree/结构化义务术语仍有呈现债。

## 4. 原始凭证

| 工件 | SHA-256 |
| --- | --- |
| Trace完整日志 | `7d3d7831b7d02604ee306f6f520e899c4194df8e505687ff2c06e054f2a68079` |
| Trace最终正文 | `e1a6ba4393f57f811b4113790c63fff2f4f65efa05747fecc4fdda457fb7b9ed` |
| Write完整日志 | `7674d4d8318e3f9d030b2e81d75e3ab3b8773e68d15b3f9d09ace224a84a0126` |
| Write原生执行report | `8cb7df8e13ce565da1800f8427e0c3e4723221607ce4bf7d132a48168cd5d908` |
| Write完整final报告 | `7d77d450b9b31d108a405c617ef99edaaabe418f52ce9453bf8a36ade03308e4` |

机器原判、正文及关键执行/完成报告随批登记，大日志与运行仓保存在对应结果目录。全仓及分片推送收据见统一账本§193。旧机器/人工FAIL不覆盖；剩余稳定任务仍63项，5个稳定验收父项与本次1份完整答案FAIL分口径报告。
