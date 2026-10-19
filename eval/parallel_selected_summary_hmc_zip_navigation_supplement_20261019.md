# HMC264：命名ZIP候选导航与成功成员补齐

2026-10-19；基线 `bb3c321f9`；79 唯一任务＝24 已交付＋55 开放，重复 0。本批完整能力新增 0，已交付子能力 2 组，剩余验收 2 组含于55；自然入口未闭环，旧采样解释越权保留。代码 `864f4922bfbb7f5f4b5bd43e507bfd70e82f7dc4` 已正常推送，文档另笔结算。

## 实现与参考

参考仓 `server.py::get_hitrace_path` 的候选/数量发现层与 `core/hiperf_converter.py` 的共享准备层。本仓复用已有ZIP安全检查、压缩输入预算、解码器和来源验证，不复制目录取首项、后缀授权或从时间包络推采集覆盖。

- 候选到上下文：精确成员名、完整候选数和预览完整性；核心最多64名/16KiB名字字节，classifier/Explorer分别32KiB encoded JSON、整记录省略。当前窄工具面不教缺席工具，原始域诊断不泄露自由字段或改变路由。
- 成功成员到补齐：当前run的私有成功查询材料，固定大小账本抗历史裁剪，区分无选择与歧义/失效；后者不回退挑另一原件。仍校验当前命名范围、完整材料代次；请求窗口、链上根因和业务线索不改。

## 固定两例，各一次并行

| 例 | 实际入口 | 机器 | 人工 |
| --- | --- | --- | --- |
| A | 普通命名ZIP中的明确成员，无预选附件；真实CLI | FAIL | FAIL：8次工具不可用，0原生查询，未交付热点/栈 |
| B | 自然只读登记，真实父/planner/独立恢复子 | PASS | 限定PASS：当前/历史执行、只读边界、版本链接与最终说明一致 |

A wrapper退出0不代表评测成功。源PERFILE2为参考实样30,062,585字节、SHA `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`；ZIP包含同字节的两个普通命名运输成员，是一份独立采集，不是新增设备ZIP采集。独立oracle为12000完整记录、窗内1393、目标164/权重259，未输入模型。当前例未取得原生事实，旧采样解释问题记未触发并保留历史FAIL。

A原始结构分类为artifact/optional，但required_outcomes误带source_explanation；归一化为mixed/required，接受的analyzer缺ExternalObservationPolicy，安全命名导航因此未发布到Explorer。这是义务与输入能力交接的系统缺口，不是解码器、成员参数或仓外原件读取权限失败。54个Explorer迭代、12次完成拒绝及75,290最大上下文估计量是本次失败过程，不是新的能力验收门。

实际成文上下文又在0次原生查询时标combined_ready，并带入调度等待事实卡；已读文件被概括成不可访问。共享就绪状态、事实域与失败原因需下一批一并贯通，归已有16.4/18.3，不新增重复编号。

B由操作者准备初始实现及旧结果，`full_cli_run=false`。当前1条断言结果/2条命令；累计2贡献报告/2结果/4命令；准备1＋流程3共4真实invocation，不与7工件数混用。父子各7原件、5 canonical及2条精确final→report链接复算通过，旧未验证attempt和旧in_progress说明未被改写。

## 冻结与测试

冻结时间UTC08:25:06.913456，7128输入。清单SHA `565762df5c86c78545a36840db9d2f3e2200862d45db545f205d22e919033065`；CLI SHA `24b0d49e72344d35d11a3553ad6c74f42d0546d6a246b18037cddc500ac7c37a`；B实际测试binary SHA `10876915fe2fb841fd0dab9c6a2f52468fbb3d62b405237ebfb7c7890bc13d7e`。

- 核心race：33根/136含子通过事件。
- 上层race：25根/80事件。
- 补齐race：135根/236事件。
- 既有窗口/链上IO业务/L1/selector/schema镜像保护通过。

以上是确定性验证，不是模型次数；scripted classifier只证接线。原始RED、测试编译/fixture修正、真实失败均保留。A UTC08:25:13.855547–08:30:35.925976；B UTC08:25:14.857165–08:26:45.741586，两例区间真实重叠。没有第三例。

同冻结全仓 PID30549，UTC08:25:16.070029–08:40:40.336610，正式exit0：96包PASS/13无测试包SKIP、零失败/非JSON行，18个HMC264新根同次PASS。日志SHA `dc27c6f57d80240a5418c4846079900cb487bc2424db1d38a8571d8b1eeed9f5`；未提前把工具包PASS当完整进程退出。

封存保54份既有原件不变，232件逐字节复制，含93份过程凭证、实际CLI、正式ZIP、3份真正最终交付与134份运行blob。286文件SHA索引 `8a076cd64c07bf7f9a9f1a1ac5c9f4ab82f755a14f37635e3b33aa16182ac041`；归档收据SHA `87a56d6df0b4e6517a7dfea6e3b64ee428389f7615fddfcff098cb5178c0c55c`。代码push正式exit0，UTC08:41:15.672248–08:41:26.499686；文档及远端clean核对的独立发布凭证见原件根 `publication-receipts-v1/publication-v1.json`，不修改封存索引。

## 下一优先级

1. 安全原生读取导航与独立源码义务正交；保真正源码要求，避免因缺artifact-only分类而吞掉已命名材料入口，不将所有命名源码fixture强制准备。
2. 旧采样解释越权保留原账，先复用已受信原生事实、降低冲突教学；不加关键词硬门或追跑单句求绿。
3. 更多实机格式/版本、外部TraceStreamer及原生平台继续归17.6，按可得真实收据排序。

详见[人工审计](parallel_selected_summary_hmc_zip_navigation_supplement_20261019_manual_audit.md)及[统一任务账](../docs/design/hmosperf_implementation_tasks_20260917.md)。原件根：`eval/results/hmc_zip_navigation_supplement_20261019`。
