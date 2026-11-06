# HMC358 — 原件、源码义务与二进制分支槽位

`main`，基线 `47d42d7467163d19051f30eb683b3656ff60af63`。79 唯一任务：29 完整、50 开放（17 部分／29 待实施／2 待验收／1 验收中／1 持续）。**完整能力新增 0，已交付生产子能力 0**；两组候选和真实子验收有证据，但冻结全仓失败，未发布、未销账。

## 公开修复与参考取舍

- [旧码公开 RED](results/hmc_source_route_branch_census_20261105_v1/sealed-v1/0289-codrax-hmc358-public-red-v1.log) 复现已接收运行时请求在探索计划强制源码、派生清单冒原件；[端点 RED](results/hmc_source_route_branch_census_20261105_v1/sealed-v1/0355-codrax-hmc358-window-public-red-v1.log) 复现表头/表身不一致。候选统一来源角色与当前路由，并复用原生窗口凭证，不把行选、业务实例和查询区间互换。
- 参考 `core/extensions/hiperf_binary_parser.py` 先按记录保序，再由 `hiperf_extender.py` 做符号和分支统计；参考扩展表会丢弃无效或周期未知条目，不能拿其行数当原始总体。原件属性字在偏移56/72是0/11，记录余尾16/24/32/40 B，不能由文件名、命令行或尾部长度断言硬件布局。只接已有受支持样本的分支计数，保确证0、缺测及独立事件权重，摘要前置精确标量、表格保查询总体而非热点子集；不解分支边、周期、调用或因果。
- [独立原件 oracle](results/hmc_source_route_branch_census_20261105_v1/sealed-v1/0258-codrax-hmc358-binary-oracle-v2.log)：真实30,062,585 B，完整 12000 记录／11978 非空／369677 槽位；[486489.4,486489.6) s、TID1609 是164／163／5216，权重259。定向9包246顶层/160子、race4包186/48，以及完整网易云82.8 MB与元宝63.7 MB实际query→Finalizer无模型回归通过。保留显式窗、原四根因/闭合睡眠、背景边界、合法重试与自动补齐。

## 同代验收

[冻结](results/hmc_source_route_branch_census_20261105_v1/sealed-v1/0275-codrax-hmc358-frozen-v1.json) 7660输入、90外部、24快照；唯一全仓 UTC 04:10:12.898579–04:24:26.016597，源码前后一致。96包PASS、2 FAIL、12无测试，28786顶层/34641子通过，3顶层/8子失败，268858纯JSON。[全仓原终态](results/hmc_source_route_branch_census_20261105_v1/sealed-v1/0276-codrax-hmc358-full-stats-v1.json)：`TestHMC348*` 证明端点权限不应绑生产者族，`TestEmitAnalysis_CurrentSourceExplanationSoftensConflictingExternalExclude` 证明旧混合许可回归；不放宽原断言，也不当环境失败。

两例各一次，机器2/2、完整人工0/2。[实际请求配对](results/hmc_source_route_branch_census_20261105_v1/sealed-v1/0250-codrax-hmc358-actual-request-overlap-v1.json) A16/B4唯一匹配，7歧义和未完日志单独记，7对已证重叠、最大11.794 s。

| 真实问题 | 有限通过 | 人工未通过／未触发 |
|---|---|---|
| [网易云完整 ftrace](results/hmc_source_route_branch_census_20261105_v1/sealed-v1/0207-codrax-hmc358-A-audit-v1.md) | 原业务实例精确窗、15行总/自身耗时、627.645 ms 状态分区，实际成文来源可选 | 51.801/200.474 ms 并非“大部分”；包含/同窗非唤醒关系。并发表未自然选择；3735行默认时序和重复表仍在 |
| [二进制完整原件](results/hmc_source_route_branch_census_20261105_v1/sealed-v1/0228-codrax-hmc358-B-audit-v1.md) | 164有计数、163非空、1实录0、0未知、5216槽位、259权重到终稿；不推执行，原件、清单与成员分开 | 派生采样成员行2064/2065/2066共用40076，终稿误称 producer ID 可定位单行。总体限定还按热点重复 |

[封存索引](results/hmc_source_route_branch_census_20261105_v1/sealed-index-v1.json)：357件，SHA `2bd60fcfbaa374384b1246e2247df16b2690f66a4239e10cae0111c2ed2bed70`；[收据](results/hmc_source_route_branch_census_20261105_v1/sealed-receipt-v1.json)。旧失败和输入不覆盖。不在本代加跑第三例。

## 50 个开放项的执行顺序

| 顺序 | 数量 | 任务（省略 HMC-） | 下一步 |
|---|---:|---|---|
| P0 共享 | 7 | 16.4、01.3、02.4、16.5、01.2、18.4、12.5 | 端点精确权限、混合许可先补冻结门；生产者ID与物理行分权、输出整组预算及跨表图的结构证据 |
| P1 实材 | 5 | 08.3、18.5、03.2、14.1、18.2 | 有新证据交接再复验旧IO；轮换真正只读登记终态与实材 |
| P2 复用 | 11 | 04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2 | 统一实例、帧、资源、时钟和生命周期；活跃WAL继续禁入生产 |
| P3 协议 | 21 | 09.3、04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1 | 09.3有真实原件子范围前置；未证布局、记录序与有效性不能跳到IPC/SPE |
| P4 新域 | 6 | 07.1、07.2、07.3、07.4、15.2、15.3 | 新领域保持隔离、审批和风险边界 |
