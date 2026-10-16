# HMC250 完整人工审计

日期：2026-10-15（本地）。两例各一次并行真实普通read pipeline；CLI正式退出均0，当前index结构审计2/2，**完整人工0/2**。没有第三例或操作员重跑追绿，旧FAIL不回写。

原始证据根`eval/results/hmc_recovery_export_20261015/`（本地忽略），A=`catalog-resume`，B=`evidence-export`。逐行独立报告为`verification-receipts/codrax-hmc250-{A,B}-manual-audit-v1.md`；主代理另读实际过程、原参数/载荷、最终上下文、MD/HTML/answer-surfaces/root-causes交叉核验。不是仅审摘要或CLI退出。

## 输入和独立oracle

沿用两份构造SQLite：root SHA256 `a938230f8ec8bceaafd8038d16058057f11407e8ce1846ee43f4e307f6d42bf5`，nested `f912c23433825e0f25188ffef226c296d3515a2550d6a9be56cbc87880e4f03e`。operator每例零模型，公开准备四计划并真实执行一条历史查询，形成1success/3not_executed检查点；不冒称实机新采集或自然模型中断。oracle在模型workspace之外。

A原问题不变：继续checkpoint，逐份检查captures中10.000到10.050秒、线程101和202的分配/释放/调用位置及本次完成情况，只分析采集文件。B只增加业务目标“并将本次检查的证据包导出为capture-review”，没有工具名/schema/护栏答案。

半开窗`[10,10.05)`中root101=3事件/8帧，root202=1/3，nested101=1/3，nested202=0/0且无观测名称。root101释放事件深度0、2；最后分配事件深度0、0、1。资源size只有数值，无声明单位；源帧已保全不等于真实展开完整，深度不授调用/执行方向。14为逐事件展开帧数，不是跨事件物理去重帧数。

## A：恢复与默认原生展示通过，整答FAIL

PID10806，UTC 2026-10-16 02:25:45.220916–02:27:39.607910，正式exit0。日志`catalog-resume/logs/codrax-20261015-192545-000-10806.log`；最终stem `catalog-resume/workspace/.codrax/output/20261015-192739.525-10806`。

1. Explorer INIT只给checkpoint导航候选，没有完整自动预读。实际read_file读取137行，共享导航含完整字节hash、历史4计划/1尝试及当前未恢复标识；随后实际resume，再执行四个ref-only查询。旧root101也真实重查，不用历史结果抵本轮。新增自动预读自然NOT_TRIGGERED，公开实链回归另计。
2. 四条当前attempt各一次，最终3success+1empty。实际resume返回的精确saved_index为`workspace/.codrax/blob/20261015-192545-000-10806/trace-catalog/catalog_1b9c09a5ec85a12d27f61f649edb9c2652f291dd861612189ae7b216f9619d00.json`；基于它运行audit-index正式exit0。没有mtime寻址或伪造快照。
3. 调查草稿曾对调来源、称root101来自历史；它仍以advisory进入最终器，但终稿已纠正3+1/1+0正向归属。不能用中间草稿误判终稿仍倒置。当前执行4计划/4尝试、全部5事件14帧、summary/members选择项0省略均实际到场。
4. 模型未选runtime_measurement，系统默认四表六行仍完整正确落盘：五事件和一条空状态。显式members和图未触发，不代签。两张手填表重复同一事实且含错项，不等于系统重复注入原生表。
5. 终稿MD第26行给nested空202借background；13/15/32–36行加B；13/34行把两个depth0已知符号串为malloc→buffer_cleanup；23行称仅一异常事件而实际缺层/重复分属两事件；24–25行“调用栈完整”超过unknown展开边界。HTML及primary/principal同错，附表正确不免责。
6. 末事件编号5可为答案内编号，不单凭它判源rowid伪造；事件表按事件号排列，也不能在未声称时序时判倒序。无Mermaid，错误箭头在正文/表格，不虚报图渲染失败。root-causes为空，没有新链上根因或CPU/等待断言，保留这项正确边界。

## B：包物理导出通过，交付及整答FAIL

PID10805，UTC 2026-10-16 02:25:45.220883–02:31:12.958241，正式exit0。日志`evidence-export/logs/codrax-20261015-192545-000-10805.log`；最终stem `evidence-export/workspace/.codrax/output/20261015-193112.822-10805`。

1. 实际同样完整read_file→resume，不是自动预读。四个探索分支分别恢复/查询；20次trace_query提交中4次混用catalog_query_ref和自由view/window被精确拒绝，16次形成四计划各四attempt。不是20次成功，也不是旧查询复活；失败参数分支随后用合法ref-only真实重查。
2. 实际resume返回相同精确saved_index：`workspace/.codrax/blob/20261015-192545-000-10805/trace-catalog/catalog_e97060c31b775a03334523a06c416a7696d0e0c08e3181727a5fc5dbb931e4ee.json`；audit-index正式exit0，合并末态4计划/16attempt、3success+1empty，历史独立。各导出manifest是fork-local四attempt快照，不能与合并16次混算。
3. 以下五包在同一run的`workspace/.codrax/blob/20261015-192545-000-10805/evidence-exports/`真实存在。完整manifest、每个文件SHA/长度与对应query/attempt/source/window逐项核验，不以目录存在代通过。

| 包名 | 材料完整 | 引用状态 | 范围 |
| --- | --- | --- | --- |
| capture-review | true | 8 copied，4唯一原结果+report | 用户所求目标；当前四查询快照 |
| capture-review-v2 | true | 8 copied，4唯一原结果+report | 分支另名输出，不代替用户目标 |
| capture-review-10150 | true | 8 copied，4唯一原结果+report | 同上 |
| capture-review-20261015 | false | 8 stale，只有report | 保守披露失效，不冒称材料齐全 |
| capture-review-v3 | false | 8 stale，只有report | 同上 |

4. 所有包均保持native_availability=unknown、analysis_completion=not_asserted；包内report是确定性执行报告，不是客户诊断报告回写。两份不完整包不是复制后内容破坏，而是复制前旧generation见证已失效。生产保存同字节结果仍改generation，export强校验正确拒旧引用；应修保存边界，不放松代次门。重复分支/另名输出是未闭环的交付协调问题。
5. 指定完整包真实落盘，但最终MD/HTML/primary/principal没有交付export路径或完成/失败说明。capture-review仅出现在用户问题，不算模型交付。正文/手填表仍借空对象名称、加B、串重复深度/错深度和过度完整性；原生四表六行正确不能抵销。完整原生5事件14帧确实已供齐，不再误归为上下文漏行。
6. 无Mermaid，未验图自动修复；没有客户报告回写、外部授权撤销、取消或坏原件的自然测试。当前有限导出通过不为这些能力代签。root-causes为空，Trace因果权限没有由本包新造。

## 已证系统缺口与结算

现有状态交接和默认原生表解决了本批漏查/事实落地子问题，尚未统一自由成文与原生事实。同来源未知字段、单位和帧关系仍被重述覆盖，冲突调查摘要仍有噪音；归16.4/16.5/18.4，不按关键词拦用户/模型原文，也不把重复失败轻称偶发。

能力轨11.3优先不可变结果材料、单一当前交付意图/父侧合并集合和精确交付收据，再补显式报告版本更新；同字节不能串源/授历史查询资格。有限原子更新与包发布分轴，失败不称全局事务完成。原生结果不变量、来源/窗/关系/Trace因果投影与自动补齐保持。

79稳定任务仍23已交付/56开放；本批新增完整能力0、交付子能力3组、剩余验收缺陷2组，不重复增ID。11.3部分实施，09.4/11.1既有交付不重开；17.7活动快照、11.2代表自然入口、sample-only诚实边界和只读登记轮换保持原账。本批末版全仓、冻结与归档收据统一见[摘要](parallel_selected_summary_hmc_recovery_export_20261015.md)及中央§250。
