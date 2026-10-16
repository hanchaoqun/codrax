# HMC249：原生来源域与资源事实复用双例

日期：2026-10-15。起点`3f362ddfc`；冻结HEAD`49d7ec5c43527668e5a4a402050e8d96ea7eb39d`、6971输入，生产binary SHA256 `da5a7a507c869bd99268f659dd4ed2b0ef5fd3a078a17efb564c18a3656128f3`。沿用§248两道自然问题，各一次并行，没有第三例追绿；准备/oracle不向模型泄露验收答案。

| 案例 | 实际执行 | 能力入口 / 完整人工 |
| --- | --- | --- |
| A：四采集代表对照 | CLI正式exit0；仅discover后直接探查，无representatives/status、无选择快照 | 代表能力NOT_TRIGGERED；整答FAIL：来源错配、右界计入、end_ns空值被解释为仍在使用，未完成用户的代表抽样目标 |
| B：两采集恢复检查 | CLI正式exit0；仅discover及3次普通resource_stack，无resume/ref；root101未本轮执行 | 本轮恢复入口NOT_TRIGGERED；整答FAIL：局部2事件被概括为整个请求、未查/空对象借用其他来源名称、正文和表格擅写调用方向 |

A正式时间01:35:56.426101Z–01:42:12.368205Z，B为01:35:57.579221Z–01:40:23.353931Z（UTC，均属本地2026-10-15）。两例都没有最终选择/恢复状态结果所返回的精确验收路径，不制造快照或用mtime找文件运行结构smoke。CLI退出不等于机器结构或人工通过。

## 本批交付与验收边界

- `126e6a3cb`统一当前实现证据与记录调用位置的定义，修复原位教学/schema歧义；完成拒绝说明typed义务，不放宽源码引用门、不按问答关键词重分类。
- `fd70a49e1`从实际trace_query参数面生成代表计划schema，保类型/枚举/边界，明确无计划先plan；已有计划给实际选择调用。无第二套参数规范、无假查询或自动替选。
- `49d7ec5c4`为精确原生资源查询复用现有runtime_measurement事实引用，正文/表/已许可无边图共用当前source/window/owner/源帧。缺层、同层、未知、原始完整性与省略分开；历史JSON不恢复权限，不授调用边或因果资格。

三组底层修复的公开正反、相关回归、独审及必要race通过，**不代表自然采用成功**。A两次resource_stack带不支持的pattern，实际native unavailable，没有产生本批新固定事实；B三组原生查询的六个summary/members选择项完整进入finalizer、0省略，但emit/patch均未引用它们。不能再将B归因为资源数据没有进入最终上下文，也不能说新事实渲染已在真实答案验收通过。

A实际初始route仍mixed/current_source=required；analyzer已不再把调用位置发成source_location，但检查进度仍被发为function_or_purpose、并产生不必要的关系义务。两次completion降级分别是member_set/grounding问题，非上批citation-floor原样重演；源码waiver已接受。错误视图、时间10..10.06及来源混用早于完成门，不将全部失败归为源码义务。

B历史checkpoint由初始化上下文供给，不是成功read_file读取。读取历史operator结果实际被拒绝，没有旧payload返回或引用碰撞；模型一度把nested101复述为root101，第二次调查纠正为未查，却未补查。最终仍借nested的editor-main标注root101，借root的background标注nested空202。上批11.1有界真实PASS保留，本批未触发如实记录，不用成文错误重开已交付恢复原合同。

## 结算与后续

起点79唯一任务为22已交付/57开放。本批另按原合同核累计原件，09.4无损采样树实现收口，18.3完整输入子链已过但sample-only诚实边界仍FAIL、转待验收。终态为**23已交付、56开放**，34待实施/15部分实施/3待验收/3验收中/1持续执行；完整能力收口1（累计实现验收，非本批从零新增），已交付生产子能力3组，本批剩余验收缺陷2组不与56重复相加。11.2保持部分实施，全部原人工FAIL不改签。两组为自然任务/计划入口和原生事实/状态的默认成文一致性；未执行对象不得借历史或另一来源补全，合法Trace投影/自动补齐仍保留。

下一能力轨推进11.3显式目标的版本证据包/事务报告输出；缺陷轨优先完整预读材料的导航发布、实际查询计划与当前执行集合的结构化交接，减少原生事实和自由综合重复，不继续逐句叠加警告。仅供齐证据后的模型错述不能无限阻塞其它高ROI能力；尚未证明偶发的重复失败也不标为波动。

完整过程审计见[人工审计](parallel_selected_summary_hmc_native_source_resource_20261015_manual_audit.md)，统一记账见[中央账本§249](../docs/design/hmosperf_capability_gap_audit_20260917.md)。本地忽略原件根`eval/results/hmc_native_source_resource_20261015/`保输入、完整日志、全部答案面与失败；大文件不冒称已推送。唯一冻结全仓正式exit0，94包PASS/13SKIP、0失败事件、0非JSON错误行；没有后置Go修补或额外模型追跑。末次6971输入/binary及两例原件、问题、历史checkpoint复核通过；71份逐字节复制、370工件SHA索引完成。最终核账和18个采样闭环根测试的当前PASS见`verification-receipts/codrax-hmc249-final-verification-v1.json`，归档范围见`batch-audit.md`；生产可执行文件仅登记hash、未复制进归档。
