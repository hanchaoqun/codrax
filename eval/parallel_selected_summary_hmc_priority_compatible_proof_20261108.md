# §371：旧排名兼容与完整只读登记验收

基线 `e76502558013ff546b552cfe8fb703f90ffc2513`，候选 `941414661f11c6bc49557c635a69466d38b9602e` 在本地 `main`，远端只推本批审计。79 项唯一＝29 完整＋50 开放；完整交付新增 0、生产子能力新增 0。原 §368–370 失败/未触发继续留账。

## 双轨结果

- **兼容与窄窗调度事实（02.4/16.4/12.5）**：参考仓 `wakeup_chain_index.py` 保留内部线程身份与物理事件点，不以显示 TID/标签替代；`thread_sched_ops.py` 状态/维度独立。原生就绪／运行两端、CPU、相交范围及源行仍经现行代次进入成文和可选旁路，不放进旧 `RootCauseRankItem` JSON；字段归属登记实际拥有者和消费路径。原非事件结构保护、原 Trace 因果投影/自动补齐、链上根因资格及窗边界不变，不扫用户或成文原文。
- **A 蜂鸟 106,896,120B 整件**：真实有限问题机器 PASS、首稿受理，HTML 与 Markdown 三表一致；人工 FAIL。62ms 窗运行/就绪/睡眠 15.218/0.090/46.692ms，13µs 唤醒至切入与类校验 3.868ms 保留，不把窗外阻塞位置/GC当直接主因。真实 27 个查询结果无根因排名，事件搜索却具 L25220/L25227/L25229；没有同窗、同核的两方竞争见证，正文退成同核放置并否认已有直接竞争证据，还漏列全窗就绪中的醒前21µs。醒前依赖供给36.311ms未进入本例。有限查询不应被强开为全量排名；旁路如实 `unavailable/trace_root_cause_contract_not_active`。未要求图，不替代 §370 原图修补/超限失败。[人工审计](results/hmc_priority_compatible_proof_20261108_v1/sealed-v1/0219-codrax-hmc371-A-audit-v1.md)。
- **只读登记完整 CLI（18.5）**：公开 planner 实际请求 RED 复现同一合同重复3处及跨仓库授权提示。候选只投影当前批次、物理仓库及私有合同授权，主补证段写一次，探索/pack去重复，探针专属不改；无授权、撤销、异仓和恢复历史不借权，不改持久义务、验证或审批。[RED](results/hmc_priority_compatible_proof_20261108_v1/sealed-v1/0279-codrax-hmc371-planner-red-v1.log)。真实第五条独立 `Planning context` 仍从控制器带入旧探针必需，实际入口单测未覆盖该生产者。
- **B 原完整 CLI**：已有测试和配置不动，实际仅修源实现。后续自然提交零变更、零探针的原生登记，实际重新运行原文件 3 断言/0 失败，来源和当前调用/报告同代分离；有限执行通过。但模型用精确选择器将负数、正数合同互绑到对方测试；账本确认两个错绑为覆盖，现有执行身份校验不能证明合同适用。`c_signature` 与 `c_tests_pass` 未覆盖；真实终态 `complete/unverified/verification_proof_incomplete`，机器/人工 FAIL，未误报全验证。下一步从实测方法位置、原文件、调用和现行合同的适用域复用，拒绝把“选中并执行了测试”当语义证明；不按测试名猜。[人工审计](results/hmc_priority_compatible_proof_20261108_v1/sealed-v1/0240-codrax-hmc371-B-audit-v1.md)。

## 回归、证据和顺序

- 定向 8 包 304 顶层/480 子通过（writeflow 单独 214/154），race×2 5包18/36，蜂鸟、网易云、元宝和二进制整件 8/2。冻结 7,695 源输入、56 外部、10 快照，SHA-256 `fb78b655085c1d389a4ca39cf3b946a0bd5cc42028f9569a6ebfde397d289a5d`；唯一全仓 exit 0、98 包通过/12 无测试、28,810 顶层/34,683 子通过，失败/重复/未终态0。[正式统计](results/hmc_priority_compatible_proof_20261108_v1/sealed-v1/0269-codrax-hmc371-full-stats-v1.json)。
- 两真实例同时各1次。完成且唯一配对 A4/B32，排除6歧义组；11对实际重叠、最长7.465秒。[重叠](results/hmc_priority_compatible_proof_20261108_v1/sealed-v1/0257-codrax-hmc371-actual-request-overlap-v1.json)。封存 303 件，[索引](results/hmc_priority_compatible_proof_20261108_v1/sealed-index-v1.json) `aedb29466cc396c9c2e28d95133f046b2abf07d5294f9387c547fed28b5c1c70`，[收据](results/hmc_priority_compatible_proof_20261108_v1/sealed-receipt-v1.json) `5906f3f5a3a83b73709f97bc7203148a763b471ce56982d4693995383b99b4ba`；前代281件逐份物理 SHA 核对。原始记录仅存本地、不入 Git。
- **二进制 09.3**：参考 `brbe_ops.py` 仅将同一采样的相邻槽配对，之后计算块跨度。完整原件 12,000 样本、369,677 槽独立只读核对得到 357,699 对；其中 34 对有零端点、2 对原始地址逆序、35,695 对相等。启发式把逆序直接按最少一条指令聚合会掩盖未证边；不能从原始数值授厂商 ABI、地址代次、有效基本块、周期、IPC 或 SPE。[原始只读核对](results/hmc_priority_compatible_proof_20261108_v1/sealed-v1/0259-codrax-hmc371-binary-adjacency-v1.log)。活跃 WAL 未证明零副作用，保持不上生产。
- [50 项 ROI 分层](parallel_selected_summary_hmc_state_account_budget_20261108.md#回归封存和剩余顺序)保持 P0–P4＝8/5/11/20/6。P0 内先修假覆盖及独立规划提示；并行的下一高影响面为窄窗竞争证据、图局部修补的同稿本地节点名册及累计字节。旧 IO、采样、栈、代表选择和权限覆盖继续留账；没有把本批失败归为偶发波动。
