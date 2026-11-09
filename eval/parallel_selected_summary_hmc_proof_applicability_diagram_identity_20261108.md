# §372：原生登记适用域与同稿图端点

基线 `90385dffb8e8e2bc7b6fc7e3394f234e15a9d4f8`；候选 `e17c0cef2fea93370243e613760ba24994be4ece` 留本地 `main`，远端只推审计。79 唯一、29 完整、50 开放；本批完整交付新增 0、生产子能力新增 0。§371 错绑的真实原件和 §368–370 原图/预算失败不改 verdict。

## 双轨交付与真实验收

- **只读登记适用域 `18.5`**：[红测](results/hmc_proof_applicability_diagram_identity_20261108_v1/sealed-v1/0219-codrax-hmc372-anchored-proof-red-v1.log)复现旧执行收据无方法范围却给精确测试行签证明。现从同一次原生执行携带方法坐标；精确登记测试文件、摘要、调用、结果和现行合同行同域后才允许该行的通过或失败归属。位置匹配是必要条件，不是语义等价证明。真实 Python 四组交叉和 JSON 恢复负控保真实运行、原断言、普通验证和合法重试。不按方法名、原问答原文或邻近行猜。控制器的附加规划提示与主要 planner 共用当前批次归属，不再重复主证明条目，原持久义务不变。
- **B 原完整 CLI**：实际只改源实现，测试和配置不动；自然选择 `proof_probe_only`、零变更、零登记，新的登记适用域未触发。普通现有测试 3 断言／0失败，探针执行真实目标行但依协议仅证执行；不把它算原断言或合同证明。持久 `complete/unverified/verification_proof_incomplete`，机器和人工 FAIL。真实最后 `Planning context` 已清理，另一生产者把证明条目复制进探索请求、先裁成 240 字符＋省略号，旧 probe 必需仍漏出。下一步按生产端当前批次去重，以各运行时的真实能力生成证明方式；逗号多行模型引文不当单一方法证明。[人工审计](results/hmc_proof_applicability_diagram_identity_20261108_v1/sealed-v1/0201-codrax-hmc372-B-audit-v1.md)。
- **图端点 `12.5/16.4/16.5`**：[公开红测](results/hmc_proof_applicability_diagram_identity_20261108_v1/sealed-v1/0236-codrax-hmc372-graph-identity-red-v2.log)复现 JSON 有原生选择器却没有既有端点约束，无声明也错误提供绑定。参考仓从 `wakeup_chain_index.py` 保留物理事件与内部/显示线程身份分层；本项目在结构、诊断和执行共用精确当前序列图声明。绑定时选现稿局部节点，不把原生坐标当图节点；未选绑定时仍可合法新建。业务名称、箭头和范围不靠别名推断，不改 Trace 因果投影/自动补齐或相关保护。
- **A 蜂鸟原件与原图问题**：机器 PASS，正文/图/HTML 与根因旁路均落盘，八表和一图的 HTML/Markdown 源内容对应。真实 patch 只修正文，新增图局部原生分支及旧累计超限均未触发。62ms 中原生状态 15.218/0.090/46.692ms 和链上语义保留。人工 FAIL：唤醒不是窗末、13µs 是目标就绪与唤醒者实际运行相交，正文却混写线程角色；状态也不是已证根因。另有系统输出问题：提交只选事实ID和结论，自动工作附注以代表名称报3.928ms，原生同窗单段2.954ms、5条合计3.868ms。不能把不同量尺和身份互借。这一问题优先做共享结构修复，非模型措辞问题。[人工审计](results/hmc_proof_applicability_diagram_identity_20261108_v1/sealed-v1/0180-codrax-hmc372-A-audit-v1.md)。

## 冻结与后续顺序

- 8 包定向350顶层/610子、race×2 5包20/42、蜂鸟/网易云/元宝与二进制整件8/2。冻结 7,695 输入/56外部/15快照，SHA-256 `bb08b3d6e7306673a7cced99965402489d4824ece946a7a819f5a2f1f5428927`。唯一全仓 exit 0：98包通过、12无测试、28,813顶层/34,689子通过，失败/重复/未终态0。[正式统计](results/hmc_proof_applicability_diagram_identity_20261108_v1/sealed-v1/0231-codrax-hmc372-full-stats-v1.json)。
- 两个真实 CLI 各一次，A11/B30 个唯一完成请求，25 对实际重叠、最长18.318秒。[重叠](results/hmc_proof_applicability_diagram_identity_20261108_v1/sealed-v1/0218-codrax-hmc372-actual-request-overlap-v1.json)。封存 268 件，[索引](results/hmc_proof_applicability_diagram_identity_20261108_v1/sealed-index-v1.json) `4945828017496f114340512c614bfb14ab075deb2c52b23c27febba54e6f1246`，[收据](results/hmc_proof_applicability_diagram_identity_20261108_v1/sealed-receipt-v1.json) `45c70091e61d7a9e61515558dff5029a5e423ca4cc66b3bce07fb8559beb3c6e`；前代303件逐份核验。原始用户采集与结果不进 Git。
- **二进制 `09.3`**：参考 `hiperf_extender.py` 以时间＋TID查样本 ID，重复键覆写、`perf_sample_matched` 在查到 ID 之前累加。当前原件 12,000 样本/12 个属性 ID，78 个重复时间戳键，时间＋TID 对本文件唯一，不能推广为跨时钟/跨来源或 SQLite 关联。下一步分别计存在、唯一匹配、冲突与未知，不采信参考的乐观计数；无厂商 flags/映射/周期/SPE 完整验收。[原字节对照](results/hmc_proof_applicability_diagram_identity_20261108_v1/sealed-v1/0221-codrax-hmc372-binary-sample-key-v1.log)。活跃 WAL 保持生产 NO-GO。
- [50 项 ROI 队列](parallel_selected_summary_hmc_state_account_budget_20261108.md#回归封存和剩余顺序)仍按 P0–P4＝8/5/11/20/6。P0 内先收口系统工作附注量尺与证明方式、探索来源，再覆盖窄窗竞争事实、原图的局部绑定及累计预算；旧 IO、采样、栈、代表和权限自然欠项不销。用户 75 份 ftrace 已按应用/版本和冷启动条件分层，下一对照优先用抖音极速版 100/105 同场景，避免对蜂鸟单件过拟合。
