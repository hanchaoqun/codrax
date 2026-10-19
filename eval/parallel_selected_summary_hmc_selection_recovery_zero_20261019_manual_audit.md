# HMC266 两例独立人工审计

机器2/2 PASS，人工 **1/2 PASS**。不能把命名ZIP查询成功等同整题解释正确，也不把公开确定性重试测试写成真实模型已命中修复。

## A：完整采样答案 FAIL

独立审计：`/tmp/codrax-hmc266-sample-manual-audit-v1.md`，SHA `36fbf0c89220161ef29c7b163d5a42bff03b77d249efe1b633b44abaff5e41c6`。正式用例唯一一次，父PID17830，应用17880。

- PASS：自然命名路径→精确ZIP成员→完整原生查询；3次模型查询成功，另1次系统自动补齐保同成员、线程及显式窗口。
- NOT_TRIGGERED：模型首次即选对，无错误选择后重试；未提交negative_observation，不代签真实未执行→零分支。
- 正确：目标164记录、259权重；CPU0/1各82记录、85/174权重；两热点比例、记录包络、未知采样类型、权重非耗时均保留。
- FAIL：最终MD第19行及HTML/primary/principal由单帧推断“内核中断上下文或回调函数的叶帧位置”。原生仅有IP/单帧，不能证明完整叶函数或执行上下文；正确caveat不抵消该断言。
- 无图要求且实际无图，不冒称图修复通过。根因sidecar未激活，不把采样热点变链上根因。

完整证据包括原问题、独立oracle、真实调用与准备收据、4份原生查询、应用完整prompt、最终4面、整个session19blob、3份准备产物及read-run快照，合计27项额外原件。机器PASS原文件不覆盖；输入只是同一真实录制的2份操作员ZIP副本，不称双侧采集。

最终MD SHA `52e416c2370747b40da868723da75bf47db610a3295adb56a420ac754ed919df`；HTML SHA `21dbf9d3be1b8cb708725956a67737aa2618c2758f64a9b2a331f944e71643d5`。实际文件stem：`.codrax/output/20261019-055236.474-17880`。侧文件摘要与实际全文核对一致，完整审查报告说明每一层范围。

## B：只读登记/独立恢复终态限定 PASS

独立审计：`/tmp/codrax-hmc266-registration-manual-audit-v1.md`，SHA `8b43f87a0d45c19d772aaf42d93ecc558c02f1dec4f63a783a5711e30b32c3d9`。父PID17943、恢复子23962，恢复子属于同一用例，非第三例追跑。

真实planner读取实现和原测试、登记原断言；恢复子实际重新执行后才完成。当前1 assertion/2 commands，累计2 contributing reports/2 assertions/4 commands；实际4 distinct invocation包含operator旧准备1次。父/子各7件工件不是执行次数，兼容副本不重复计数。

逐字节/代际核对父子原件、5个canonical、3个attempt、2条精确final链接。旧source final仍in_progress并指向旧精确报告，旧proof-repair attempt仍unverified；终态只签当前正确代次。持久上下文6包在恢复及往返保持对应内容，源码/测试字节与Git clean一致。历史临时路径可能已清理，归档仍保精确原件映射。

仅签18.4相关保留测试只读登记、重新执行、恢复与报告交付子能力。初始代码/应用/旧执行为operator准备，`full_cli_run=false`；不冒称从零开发、全部CLI或整项交付，弱源码定位与缺原应用凭证仍披露。

## 总账与后续

79唯一＝24已交付＋55开放。完整稳定任务新增0；本批2组系统子能力与人工FAIL分开记录。旧采样解释继续留账，不因入口修复销账；来源/窗口/因果边界及自动补齐保持，真实坏源终止保留。下一优先二进制剩余自然入口与共用事实展示/预算，不按这句失败追加硬门。正式全仓与封存索引见[批汇总](parallel_selected_summary_hmc_selection_recovery_zero_20261019.md)及主账本§266。
