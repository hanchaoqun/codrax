# §366：修补请求预算与只读验证终态交接

`main` 源码 `80918bdb7a75e4404c88e9b2abff14e6d5a1cc92`，基线 `c135ac179ce704d19dd77f4dc2d093165ceb88b8`。共 79 项：29 完整、50 开放（18 部分、28 待实施、2 待验收、1 验收中、1 持续）。本批完整新增 **0**，交付子能力 **2**；两例机器 **2/2**，整例人工 **0/2**。未触发与失败不销账。

## 两条工作轨道

- **必要成文修补（16.5/12.5）。** §365 蜂鸟同轮 patch 777,916 B、重派 826,610 B 均超过 700,000 B，未发送。参考仓 `core/skill_executor.py` 将结构教程与具体语义/材料分层，`core/query_engine.py` 集中展示重复说明并保留单位、空值状态、截断及原因。本项目在已有 patch-only 路由、无已受理答案、默认技能且实际请求超额时，换成一份短修补教程。保留动态事实、工具参数结构、原始请求及反馈、约束、语言、接受权限和原预算。首稿、自定义技能、可选修补、强制全量路径不变；仍超额则不发送。不读取用户/模型原文作硬门。真实适配器入口 [RED](results/hmc_repair_terminal_20261108_v1/sealed-v1/0215-codrax-hmc366-repair-red-v1.log) → [GREEN](results/hmc_repair_terminal_20261108_v1/sealed-v1/0214-codrax-hmc366-repair-green-v1.log)。§365 [原始失败](results/hmc_tree_depth_restore_20261108_v1/sealed-v1/0051-codrax-20261108-173921-000-11238.log) 保留，重派仍独立开放。
- **只读登记恢复（18.5）。** 生命周期 `complete` 与终态验证分轴。控制器逐批展示验证状态和原因；聚合与控制器共用同一事实，在名册裁剪前计数。当前证明只证明自己的范围，不补签来源，也不更改验证和审批。新 [公开回归](results/hmc_repair_terminal_20261108_v1/sealed-v1/0180-codrax-hmc366-controller-green-v1.log) 和 [§365 原始工作流重放](results/hmc_repair_terminal_20261108_v1/sealed-v1/0217-codrax-hmc366-restore-archive-v1-receipt.json) 已通过。

## 两例真实评测与人工审计

原问句、两例各一次，实际请求唯一配对 A10/B4，共 5 对重叠、最长 12.009 s、无歧义。[重叠证据](results/hmc_repair_terminal_20261108_v1/sealed-v1/0165-codrax-hmc366-actual-request-overlap-v1.json)。

- [A 蜂鸟整件](results/hmc_repair_terminal_20261108_v1/sealed-v1/0130-codrax-hmc366-A-audit-v1.md)：[原答案](results/hmc_repair_terminal_20261108_v1/sealed-v1/0042-run-1.answer-transcript.md) 和 [旁路](results/hmc_repair_terminal_20261108_v1/sealed-v1/0051-run-1.root-causes.json) 均生成。窗 `[11343.248,11343.310)`、TID 64156 的 10 段原生分区与原件一致：运行 15.218、就绪 0.090、休眠 46.692 ms。时序只有一条已证唤醒边，唤醒和上 CPU 相隔 13 μs；场景端和 GC 只是背景，类校验和 `PostFork`、`ZygoteInit` 保留。真实首稿直接受理，超限修补未触发。
- A 的失败：原件复算唤醒方在主线程休眠期四段累计就绪 45.378 ms、四段运行 1.314 ms（CPU1/4/7）。正文与旁路把累计写成持续，正文和图把唤醒点同 CPU4 扩成整段同核；模型实际已收到状态反证和点位权限。13 μs 是 64156 就绪、64176 同核运行的重叠，原生含两方关系；旁路却把同一 64156 的别名当第二人。`priority_inversion_candidate` 和 `priority_inversion_runnable_wait` 在成文解释被套成同一“低优先级依赖方”角色。应传真实两方身份与见证，保留合法 13 μs 及链上 36.311 ms 估算候选，不取 `nearest_chain_thread` 代替竞争者。参考 `core/preprocess/game_render_ops/bottleneck_scanner.py` 按显式唤醒关系和线程实例认定来源，而非相邻或名称。另留 JIT 初始化→编译、调度器 IO 零→物理机制排除、运行子账未量测→零及等值重复展示；不在本批扩大修复。[角色对照](results/hmc_repair_terminal_20261108_v1/sealed-v1/0153-codrax-hmc366-A-roles-v1.log)、[原件逐段复算](results/hmc_repair_terminal_20261108_v1/sealed-v1/0193-codrax-hmc366-honeybird-waker-oracle-v1.log)。
- [B 只读缺来源终态](results/hmc_repair_terminal_20261108_v1/sealed-v1/0156-codrax-hmc366-B-audit-v1.md)：真实规划器与独立恢复子控制器。现行证明 1 次既有测试、1 断言、0 失败，历史执行独立，三份报告同字节，源文件/测试/Git 不变。来源无验证终态，本批 `verified/tests_passed`，整次 `unverified/missing_terminal_verify_verdict`。模型已看到差异、不再报全部验证，仍从工具表选了不适用的第二次 `verify_batch`。执行态拦下并转 `finish`，没有额外测试。需统一投影与状态核准、保留合法下一批和恢复，不给旧/新整例人工转 PASS。[实际两次决策](results/hmc_repair_terminal_20261108_v1/sealed-v1/0157-codrax-hmc366-B-context-v1.log)。

## 冻结与二进制后继

- 12 包定向 503 顶层/541 子；6 包 race×2 20 顶层/30 子。75 份用户基线中实跑蜂鸟整件、网易云和元宝整件回归，另复验参考 30 MB 原始二进制。单次全仓 OS 0、98 包通过、12 无测试、28,803 顶层/34,666 子通过；失败、重复、未终态均 0。[冻结收据](results/hmc_repair_terminal_20261108_v1/sealed-v1/0185-codrax-hmc366-frozen-v1.json)、[全仓统计](results/hmc_repair_terminal_20261108_v1/sealed-v1/0186-codrax-hmc366-full-stats-v1.json)。
- [二进制只读证据](results/hmc_repair_terminal_20261108_v1/sealed-v1/0168-codrax-hmc366-binary-attribute-ownership-v1.log)：原件 164 个样本按采样 ID 都唯一回到属性 0；结构为 120 B、文件表项 136 B，+72 为 11，+56 为 0。参考仓读取 +56 并按剩余长度试解；现有 Codrax 读 +72，但尚须验证声明的属性边界、不同记录 ID 和可选索引。公开的 [Linux perf 头](https://github.com/torvalds/linux/blob/master/include/uapi/linux/perf_event.h) 和 [OpenHarmony 样本读写代码](https://github.com/openharmony/developtools_hiperf/blob/71a3a0e7207e4cca8afd940b3658193bfd5c797a/src/perf_event_record.cpp#L632) 支持分层处理的方向，不证明该采集的厂商标志、地址或周期语义。仍不宣称 BRBE/SPE 生产支持。活跃 WAL 未满足零副作用前不接生产。
- [封存索引](results/hmc_repair_terminal_20261108_v1/sealed-index-v1.json) 250 件，SHA-256 `675ef9250e8dd7ff7e27680ffbf90a9573564d74d95a204ea5d722d1dc803e0b`；[封存收据](results/hmc_repair_terminal_20261108_v1/sealed-receipt-v1.json) SHA-256 `ec43a50176e66960c9f7777aea123e7b166a678babb0ee2731aada6ddce1da9b`。独立物理核对 §365 的 237 件；原采集和密钥均不入 Git。

## 50 项开放项（按 ROI）

1. **P0 共享 8：** 16.4、18.5、02.4、12.5、16.5、01.3、01.2、18.4。先把优先级两方及时间范围交到正文/图/旁路、完成态交到控制器实际动作；并列修 JIT 操作归类及未知/零的分离，再复验真实修补和跨图族。旧模型误述及未触发分支单列，不用新硬门追第三例。
2. **P1 有实材 5：** 09.3、03.2、14.1、18.2、08.3。先验真实属性选择、分支布局和字段有效性，继续资源申请/释放、硬件状态与 IO 在途的真实验收，保源、单位、窗及部分覆盖边界。
3. **P2 跨域 11：** 17.6、04.3、04.5、08.1、08.5、12.2、12.3、05.2、10.2、14.2、17.7。继续用异构整份基线，统一业务区间与图、选择和自动补齐；保原总体，WAL 继续隔离。
4. **P3 协议 20：** 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1。有真实可核材料后分批，不用邻近或名称签证。
5. **P4 新域 6：** 07.1、07.2、07.3、07.4、15.2、15.3。依赖新实材，保只读和审批边界。
