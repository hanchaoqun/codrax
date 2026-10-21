# HMC273：原生优先级关系、叙述角色交接与活跃WAL安全设计

本地日期2026-10-20；基线`442844d9ff5b55a7d8439fb2ff4da29c98a3c312`。施工前重核79唯一稳定编号/0重复＝24已交付＋55开放（33待实施、15部分实施、3待验收、3验收中、1持续执行）。本批不把共享接缝修复计为完整能力交付，不倒签§272及旧采样解释失败。

## 实现及参考设计

缺陷轨沿上批已保存的实际成文消息修复两类系统接缝：原生关系已产生却在紧凑摘要丢失；原生表正确到场后，调查复述又带入冲突值。不是修改正确统计器来拟合正文，也不扫描问答关键词裁决真假。

1. 原生优先级关系、证明口径与两端工件来源一起进入角色摘要及冲突指纹。复用producer已验证的OHOS/generic口径，不根据两个数字重算。弱/未知关系仍保端点及口径，不造平级、反转或根因；原8行摘要预算、来源/目标/窗口约束、原ledger及Trace投影不变。
2. `interpretation_role`将纯原生复述与独立/混合分析分开。当前动态schema显式选择角色，旧wire缺省兼容；纯复述原文留审计但不再进入Extract、Finalize和恢复advisory，其完全同字节notes副本同样隔离。近似文字不删，独立推导量、负发现、源码/链上业务线索继续交接。角色不是原文真实性校验；私有接受代次、fresh绑定、发布完整性和预算分别保持，不能借省略解释复活旧reason。
3. 修正JSON patch教学：完整block只复制当前schema允许的模型字段，原生选择器标题由系统持有；静态说明、实际dispatch说明及add/replace数组同源。普通标题不变，没有增加硬门或自动剥离模型字段。

参考`core/preprocess/sleep_ops.py`阻塞树连同主体/进程/优先级保留，其缺失标问号的意图适用；首条sched优先级和平台区间不能照搬为本项目唤醒时点关系。参考`core/query_engine.py`保查询参数、原始行与缩减元数据，`core/batch/closure/package.py`分开领域材料与当前输出合同，启发复用原生视图、减少重复摘要及教学竞争；不移植其字符串段落处理作为事实硬门。

能力轨核对`core/db_pool.py`：连接缓存与自动索引的目标是降低查询成本，实际会在原库创建索引/commit，path+mtime缓存不是严格只读/强代次证据。可在本项目自有快照上优化查询，不能照搬对用户原件写索引。

## 活跃WAL：明确未交付

现有稳定main/WAL、checkpoint、缺失WAL路径保持。新增公开安全回归在`PrepareExistingTraceDB`快照阶段后注入真实WAL-only commit，断言`source_generation_changed/pre_commit`、无发布、准备器不额外改变提交后的源bytes/代次/旁件集合。该测试通过只证明正确拒绝，不是活跃写入快照成功。

当前modernc Backup能支持事务一致性，但直接`mode=ro`或`readonly_shm=1`不能证明所有权限环境零源副作用：Darwin Unix实现可能复用同进程可写SHM节点，root路径仍有fchown；SQLite按路径打开的对象也未与本项目held descriptor逐一绑定。下一步应先做有限的只读handle-bound WAL捕获/VFS可行性包，再设计独立捕获凭证和共享准备入口。不放宽通用SameVersion，不顺序复制活跃main/WAL，不用immutable/nolock伪装一致。

设计、实际依赖源码行号、SQLite官方来源和未覆盖平台记录在归档中的`process/codrax-hmc273-wal-plan-v1.md`。17.7仍开放；没有运行在线Backup/root/跨平台成功实验，不冒称支持。

## 确定性验证与冻结

先公开RED再修复。优先级6场景的真实TraceQuery→实际Finalizer证明原生字段已在而摘要遗漏；patch实际schema/DescriptionFor证明旧说明与可执行标题合同冲突。叙述角色前置RED是新wire字段未支持，旧冲突见§272原件；不将schema RED冒称新的事实计算错误。

施工期的测试构造错误、宽测试选择器混入其它并行RED全部保原；日志名green不决定PASS。最终精确结果：

- 叙述三包相关回归正式exit0，包含实际提取/成文/恢复、旧兼容、近似笔记、空解释、未知枚举、动态schema、因果投影和自动补齐；日志SHA `acf09b2f6a637cc3a78479e0f76fb21118f8f35f4dcaca9f2c430dea4b16ab1a`。另有三根实际消费者独立审查PASS。
- 优先级8根/2包、旧消费者15根/4包及合并23根/4包race正式PASS；race日志SHA `0c5ac100ab9e66550798c4862f75b5958f9d747ac74e4685df0a2c9a7d95a1df`。
- 标题教学及真实patch事务窄race正式PASS，SHA `e6eb53a887821281058381110dbc5f68b2b41cb4725e0bc42cc1ea5e261ccac5`。
- WAL公开拒绝和稳定/损坏/未提交保护PASS；checkpoint/missing五项另有精确PASS。未把未触发的错误共享入口测试记为能力RED。
- make正式exit0，实际CLI SHA `fd157e3872f4f558f461223a6d6308186ea4587242d32ddfababd9baf2bf626c`。7209输入冻结索引SHA `c2ec6c97128850a9d6056a92d0ad822633f1c38ec3d2f51a37f36a51758ca6a4`。

v1冻结期间不改源码或测试；一次全仓和两例各一次并行，原始失败与模型材料全部保留。v1全仓已发现字段注册契约遗漏：新展示消费者读取两端artifact来源，但registry仍为display_only。同一reader新增读取的priority_relation也应统一登记。待v1正式结束、源码/CLI/两例/全部失败完整封存后，只同步常量与soft_consumer登记及golden，另建v2验证，不隐藏扫描器、不升级因果权限、不把v2测试倒签为v1通过。

## 自然评测与人工审计

结果根`eval/results/hmc_priority_narrative_wal_20261020`。A复用17行构造调度Trace，问题只问[1,1.01)的峰值/均值/3ms变化/持续分布及主要线程活动；独立oracle不放进问题，不冒称设备采集。B用新命名ZIP路径包装同一份54,122,686字节完整设备文本，文本SHA `f9d1e3a77441e38eaf659272d7430f5227cb1434ef4251c2425b213d3069b572`，ZIP SHA `5add62b8d2f61ff303cdca8e4d1f58b74e193a576b5c25cf8ff4af4f38bc6bfe`。仅问SaInit0/TID3051的100ms状态和唤醒者，不塞系统护栏。

两例均正式exit0/机器PASS；完整人工0/2。A为04:17:41.259646–04:24:34.999681Z，B为04:17:41.259863–04:19:38.566518Z，各一次且时间重叠。A内部wall 411s、上下文估算115,778 token/58%；B内部wall 113s、95,111/48%。这些是实际成本线索，不据此断言错误根因。

- A：原生8张表及最终表面全部峰值/均值/分布/3ms四桶/成员正确；running实际1ms峰值未再写成桶忙时2ms，upload-103没有再错绑decode-102，104未进入闭合总体。但正文从有限观测并行度直接推断“CPU资源充裕”，整答FAIL。上批错时长/成员在本次未复现，不回签旧原件。
- B：100ms状态账、33段、8次唤醒、三线程两进程及窗口边界正确。6条选中边的高/同优先级关系、caliber、两端来源均进入实际成文上下文，交接限定PASS；正文没有比较优先级，关系复述NOT_TRIGGERED。正文将1.088ms全runnable解释成唤醒后等待，但其中0.533ms来自4个R+切出后再调度区间；8个真实唤醒至首次运行仅0.555ms。独立物理行及原生分段均证实，整答FAIL。
- 两例新restatement角色均没有自然采用，不以“无旧错误句”代签角色功能；公开生产路径正反例单独验收。A最终撤去typed handoff，legacy reason又把错误4/3峰值和“待window_stats”送入实际成文，属于继续留账的共享退路；B既有bounded-runtime分支先隔离自由调查摘要，本次错误发生在之后的成文解释。
- A还证实了结构化路由义务冲突：原始artifact/optional声明同时带source_explanation outcome，被规范化为mixed/required；随后bounded_fact_set与mechanism/flow、stage_or_workflow、all scope组合又延续源码/操作转移义务。实际两次flow降级及后续mixed-origin阻断、三项source-inventory pending保原；不把全部4次complete reject都归flow，不依据think文字定因。下一批优先审计这些typed声明之间的一致性，而不是关键词禁止源码或扩大必查任务。

完整审计见[人工审计汇总](parallel_selected_summary_hmc_priority_narrative_wal_20261020_manual_audit.md)及归档中的两份逐项报告。没有实际图，不强加图验收条件；旧采样越权、根因/代表选择未触发及其它旧FAIL不回签。不追第三次模型求绿。

## 计数与后续ROI

本批完整能力新增0（累计24/79），共享生产子能力3组；活跃WAL只有安全负控与设计，不计生产子能力。两组整答人工欠项包含在55开放中，不与55相加。08.4保留开放；原生统计器无新证据故不重写。

下一缺陷轨优先结构化任务领域/源码义务一致性与legacy摘要退路，再统一runnable状态转换来源及量尺交接；能力轨轮换03.2地址/资源子类→08.3真实IO在途深度，避免第三轮紧盯同一并行度题。17.7并行推进有界handle-bound只读WAL安全原型，满足零源副作用/事务一致/取消无发布才接产品。18.4/18.5只读登记终态与sample-only按槽轮换；17.6其余真实格式/外部工具/原生平台前置仍留账，不重复普通ZIP/gzip运输。

## v1正式终态与原件封存

全仓PID56420，UTC2026-10-21T04:17:41.262613Z–04:33:15.521484Z，正式exit1：95测试包PASS、13无测试包SKIP、1包FAIL；唯一失败测试为types的`TestInfoContractNoteKeyCarrierTruth`。零非JSON行，日志SHA `42d1d88d4fb4462097e1749a5b6634b48a4c22c6efb3e39c5246dbd20e3a06a3`。不能用窄race或两例机器PASS抵消这个登记错误。

首次归档因审计清单包含已冻结的仓内case/fixture而保守拒绝；partial原目录完整移到`archive-attempt-v1-failed`，原件未删除。v1b只允许精确冻结路径且hash相等的输入，未放宽任意路径准入。UTC04:34:01.641725完成封存194既有原件/160新增复制、17源码快照/实际CLI/118个审计路径；354文件索引SHA `a5bc88448c24dde4eafb1559791a31ba18857359c26f7e821d06d2b7b5af446b`，归档收据SHA `5d0646cc9727fe729b659bd5b782a965af2a32ee98899cc68080842cdd308ecf`。包含首次归档失败记录及全部真实评测失败，不含providers/.git。

后修只同步展示消费者登记，不改v1索引或原始答案。三键登记为soft_consumer，新priority_relation常量贯穿已有producer/reader，测试继续独立钉原wire字节；golden及新pin同步，scanner、因果算法/权威和旧relation别名不变。相关三包正式exit0（PID52397，UTC04:35:52.730776–04:36:27.583245），日志SHA `3eb1c354929c99b754924107a79d5d15fdbab06a21a965d8f1aca353bcebc828`。

v2 build PID55935，UTC04:36:49.067891–04:37:00.518190，正式exit0；CLI SHA `fd7d21849193e9c533a551961c4ac46c45c324a9eeb458cb710d4b9ab299dbd9`。仍7209输入，较v1仅9个NKR相关文件变化，冻结SHA `3f47f6e89b7e5313ec4a71186bd95eab8dd5f37ce920b7efc8136103d8d4f754`；独立v2全仓正式结果见下，零新增模型调用，不倒签两个v1 live。

NKR相关实跑37个顶层测试/3包PASS，窄race 14个顶层测试/3包PASS、无SKIP/FAIL/race（日志SHA `bd7463d53dd3806fb78516fa9c2ea115cb4356e4777fe5197903411358ecbc34`）。实际覆盖registry/emit/consumer pin、六场景公开Finalizer、弱/跨源/错窗/冲突、弱证据不升硬反转、链上主因/链外背景。独立只读复核逐一对照v1封存源码或v1哈希验证的基线，确认9文件均匹配v2冻结，未改变wire值/分支/因果资格；新role代码在两冻结间哈希相同。代码复核不替代自然问答和正式全仓收据。

## v2最终验证与发布

全仓PID57338，UTC2026-10-21T04:37:09.131824Z–04:52:58.326859Z，正式exit0：96测试包PASS、13无测试包SKIP，零FAIL/非JSON行；日志SHA `7fc1911819f1cdebe9cb2ca6b4edcc41f6a6f0a24992723723f700217a38c964`。首轮types契约FAIL已修，未修改扫描器；普通全仓通过不代签真实答案。

UTC04:53:15.674670再次核验v1全部原件不变，独立封存v2的37文件（22源码快照及实际CLI/新收据/审查）；v2索引SHA `683e34f7f2cf4356e298271c31a5fc60007ef4d776a501c83f9eddd4da6b6407`，归档收据SHA `949241c5224b647936f9e2ce831b52cd20d67b7022797746217c3c93971cf2d7`。v1首轮失败、两份模型输出与全部失败保持原样；归档原始资料留本地，不混入Git源码提交。

代码`92323ba4fd4d11c5cefdd46a79b5094fa344bf77`单独提交/普通推送，文档另笔。最终两次commit/push收据、文档完整SHA、实际远端main与clean状态保存在`eval/results/hmc_priority_narrative_wal_20261020/publication-receipts-v1/publication-v1.json`，独立于两个已封存索引，避免为登记后发生的发布修改原证据。末次逐checkbox核对仍79唯一/0重复＝24交付＋55开放；完整能力本批0、共享子能力3组、两组整答人工欠项不销账。
