# HMC264 人工审计：完整结论1/2，55项不变

## A：命名ZIP自然入口 FAIL

原自然问题只给ZIP、成员、窗口、线程和热点/调用栈目标；没有把系统护栏或oracle写入问题。实际CLI从未取得一次原生查询成功：8次trace_query均在工具不可见时被拒，系统自动补齐no_attached_trace。机器verdict明确FAIL，不能将wrapper退出0记为PASS。

本批新增日志证实原始source=artifact、current_source=optional，与required_outcomes里的source_explanation冲突；原确定性义务规则升为mixed/required。接受的analyzer没有ExternalObservationPolicy，现有命名内容准入和发布门因此没有交付本已安全探测的候选导航。公开API及scripted交接通过不能代替自然入口。下一修复应将读取能力与源码义务分开，而非删除真正的源码要求。

最终正文没有热点/调用栈结果，并将仓外路径泛称无法正常访问；实际read_file已有读取，错误在原生工具路线，不是所有文件访问都失败。没有原生采样事实，旧unknown类型被解释为执行状态的历史FAIL仍保留，本例不代签修复。没有图，不扩大成图语义验收。

实际成文上下文中，deterministic_runtime_queries=0仍见combined_ready=true，runtime观察来自分页读取；读者事实卡却带入调度等待维度。正文从符号库存推采样帧也超出已解析事实。这些是本次证实的共享状态/事实交接偏差，归既有16.4/18.3；不能将无原生结果视作热点已就绪，或将文件可读、目录可发现、原生工具可用混成一个状态。

## B：只读登记与独立恢复子终态限定 PASS

真实父控制器、登记planner和独立恢复子重新执行现有测试。父PID30459、子PID36017，当前新invocation与所有历史执行不同。当前1条断言结果/2命令；累计2贡献报告/2结果/4命令，准备1＋流程3共4次真实native调用，与7工件数分开。

父子各7原件逐字节/SHA匹配，5个ChangeReport canonical和2条final精确链接逐项复算。source旧final仍in_progress并指其自身报告，未改指同plan后次结果；旧proof-repair attempt仍unverified。当前登记final complete且精确指本轮报告，当前HEAD、源和测试散列以及工作区clean一致。6个上下文包在独立恢复与两次往返中逻辑身份/内容稳定，不声称跨所有阶段物理ID字面恒等。

保留边界：操作者准备初始实现/旧结果；并非从零full CLI；source-localization weak和apply_checkpoint_missing仍在。此PASS不代销整体18.4或A。

## 原证与口径

独立A/B人工报告、完整两例日志、执行/验证报告、父子typed工件清单和真正最终Markdown/HTML/sidecar已在同批`verification-receipts-v1`逐字节封存。A的134份blob含8份原生查询参数、0份查询成功结果，不把静态能力说明冒充原生数据。54份既有原件前后不变，232件复制、286文件索引SHA `8a076cd64c07bf7f9a9f1a1ac5c9f4ab82f755a14f37635e3b33aa16182ac041`。原始FAIL不改写；完整能力新增0，已交付子能力2组，剩余验收2组含于55，不重复新增任务编号。

独立报告：[A完整FAIL](results/hmc_zip_navigation_supplement_20261019/verification-receipts-v1/codrax-hmc264-sample-manual-audit-v1.md)，SHA `3d0238022be08a00fc2d08fb6c7fc433ac7ff59d913f816fb03dbf285cec5339`；[B限定PASS](results/hmc_zip_navigation_supplement_20261019/verification-receipts-v1/codrax-hmc264-registration-manual-audit-v1.md)，SHA `d4d91d2ebd945ab837a8b900288b0ddc0129f137e49a5aa3b7012a841eda7800`。同冻结全仓96包PASS/13无测试包SKIP、18新根PASS，与模型人工结论分开；没有第三例或改问句追绿。

参见[批次汇总](parallel_selected_summary_hmc_zip_navigation_supplement_20261019.md)与[统一审计账§264](../docs/design/hmosperf_capability_gap_audit_20260917.md)。
