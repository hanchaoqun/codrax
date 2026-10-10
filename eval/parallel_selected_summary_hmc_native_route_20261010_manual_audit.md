# §221 自然入口与原生事实展示：完整人工审计

日期2026-10-10；冻结`93a2506d3`，snapshot `codrax-selected-20261010-001233`。并行2、各1次，未改自然问题或追跑第三例。机器1/2、完整人工0/2；runner正式exit0仅代表批次完成。§220原失败不覆盖。

| 用例 | 机器 | 完整人工 | 时间 | 结论 |
| --- | --- | --- | ---: | --- |
| trace_dual_measurement_records | FAIL | FAIL | 228s | 再次进入data而非原生查询；流程错误终止，没有量测答案 |
| log_shared_sources | PASS | FAIL | 201s | 完整报告自动保留全部9条原生记录；模型摘要仍有单位、标识和时间错误 |

结果目录为`eval/results/hmc_native_route_20261010/{trace_dual_measurement_records,log_shared_sources}-20261010-001234`。主代理及两位独立审查者读完整答案、实际过程和结构化结果。下文日志行号指`run-1.logs.all.log`；`primary.md`是principal投影，不是完整客户报告。

## 双侧量测

`run-1.verdict:1`为read_exit1及无trace_query；metrics记failed、data3轮/repair6轮、answer_len=0、consumed=0。日志46–47首次分类选择data_aggregation/confidence0.95，9秒正常返回；没有原生查询，不能对13/8条量测、同名序列、窗口及未知值签验收。

本轮日志只有messages=2/body_bytes=43983（43），没有current_named_inputs等原始消息。真实adapter公开回归证实注入路径可达；模型提及原问题未写出的SQLite仅为间接线索，不能声称live逐字证实profile到场。静态复核发现system及route/needs_data_access schema把结构化过滤、计数、聚合广泛归data，主要只排除“log/trace root-cause diagnosis”；非因果原值/区间职责与新增user导航重叠，实际data reader又无SQLite原生读取能力。属系统歧义，不能直接称模型波动。

独立审查确认同一工作流状态矛盾：1114–1124将cover_required_materials/inspect_material标ready，1284写下一阶段cover_required_materials，1493–1523却以prepare_contribution_inputs拒绝该动作。不能为本例放松状态机，需统一状态交接。原绝对路径被workspace gate拒绝、改写相对路径后ENOENT分别在归档terminal JSON 35/89；原fixture存在且SHA未变，不能说源文件缺失，也不能借此放开工作区边界。

终态JSON `20261010-001619-235008-96830-terminal.json:2–8`和stdout163–168诚实报failed、无答案；不是活跃流被降级。本轮consumed=0，没有复现上一轮unknown inspect假消费，也没有partial出口，不能据此宣称partial传播已live命中。相关历史失败继续留案。

## 多源日志

实际log_query先分别取app/kernel，再按rx17筛5条。首次emit及patch接受summary,caveat,table,table,table（3042–3048、3088–3093）。完整`answer-transcript.md:35–76`保app6+kernel3共9条，78–100再附过滤5条；14行但只有9个独立记录，不相加。NULL身份、malformed/orphan/unknown、精确boot_ns及来源代次均保留，不能再记“默认展示没命中”。`internal/render/answerdoc.go`设计上从primary排除system-owned内容，不能只用primary评判原生表缺失。

完整答案仍FAIL：transcript25将原始秒值9007199.254740993称ns；26把8ns差写8μs；27虚构observer共享rx17；29把99-09无效月份说成年份并泛称日志时间单调。主文16手写7条排除两条未知/孤立，虽全报告有9条仍存在摘要口径不完整。21/26/31还将TimestampError、StorageLookup当原日志错误类型，源中没有这些字面标签。

存在系统诱导，不能只归模型：log_triage在286自造标签/observer关联；2964–2965的系统教学又要求summary逐字保三种标签，2991–2992称Error occurrence；`answer_document_evaluator.go`缺原始literal见证便将LogBundleErrorTypes升权。另2802/2805要求模型成员list/table，2866又说原生表自动提供，3051–3062在3张表已存在后仍要求成员维度修订，模型仅给原错误prose贴member_set。2808/2811对日志先后又给源码关系路径教学。这些挂16.4/18.4共享来源与维度交接，不以扫描正文关键词修补。

跨来源时钟不可直接排序、共同标识不证明因果的边界保留；最终匿名kernel没有被补PID/TID。无图，不把表格检查当图形视觉验收。子查询重叠展示5条是噪音接缝，后继须凭source/generation/recordID/字段覆盖审计，不能抹查询条件直接合表。

## 处置与原件

两份完整人工FAIL保留，79=20+59不变。原生默认展示、完整publication冲突及私有生命周期的确定性修复与答案验收分账；下一批优先统一路由职责及原生日志事实/模型标签的来源层，成员维度复用、data状态矛盾、消费凭证和独立输入准入/逐源窗全部挂原ID，不漏债、不同时无限扩轨。

原始日志、机器verdict、answer-surfaces、完整transcript保留；工具blob、data审计材料及相关验证收据已按原字节归档并cmp/diff。全仓和推送收据另记统一审计账本§221，不用早期GREEN代签。
