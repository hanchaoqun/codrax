# HMC276 两例人工审计

同一冻结版本恰好两例，各运行一次，UTC区间实际重叠。机器 **2/2 PASS**；人工 **1/2 PASS**：A 数据正确但整答解释 FAIL，B 限定只读登记终态 PASS。进程 exit0 不等于人工通过；未追跑第三例。

## A：IO自然问题 — 机器PASS，人工FAIL

原问题及构造 ftrace 未改，不把测试夹具冒称新设备采集。正式 PID58085，UTC2026-10-21 09:10:13.902994–09:17:21.519093。运行目录 `results/hmc_fact_provenance_atomic_wal_20261021/trace_io_inflight_overview-20261021-021013`，实际 CLI PID58138。全文由主任务审，另由子审计者独立核实际成文上下文。

已读实际工具/成文消息、完整MD、HTML源和root-causes旁路。完整MD：`.codrax/output/20261021-021719.257-58138.md`，SHA `588598b519edd292cb56a0a4879394629de2a05fb53ef6c4d96f37a96bd4bf1d`；HTML SHA `02c0d8e9a99f00d6e570cbe20916652a60ddca760f593ffeedf5e92527659338`。HTML保留末尾时段，也实际保留错误跨层求和及设备解释，不能用MD正确局部替它签PASS。HTML中的Mermaid错误模板不是实际生成图；本例无图请求/图块，不追加图退出条件。

### 正确的事实部分

独立按 `eval/fixtures/hmosperf_io_inflight/expected.json` 和原始事件核对：

| 分组 | 峰值 | 全窗均值 | 忙时ms | 请求·ms | 窗内发起 |
|---|---:|---:|---:|---:|---:|
| RQ / 8,0 / 读 | 2 | 1.4 | 10 | 14 | 6 |
| RQ / 8,0 / 写 | 1 | 0.6 | 6 | 6 | 2 |
| BIO / 8,0 / 读 | 1 | 1 | 10 | 10 | 1 |
| RQ / 8,1 / 读 | 1 | 0.4 | 4 | 4 | 1 |

答案四组全部正确。请求成员实际起终点、窗外起点、窗外终点裁剪、RQ写最后 `[1.009,1.010)` 零深度段、全部13段时序也正确；RQ读相邻同深度段合并不改变积分。partial配对、1未配对起点、1歧义cohort及捕获完整性未知最终有披露。当前没有证据要求继续重写这些正确算法。

### 保留的人工FAIL

1. 将“完整配对人口每组峰值不超过2”升为全设备无严重争用、IO利用率较高但不过载、没有造成争用。原件只证明部分可配对请求的在途事实，不能证明硬件占用或排队/拥塞状态，且未配对/歧义请求仍存在。
2. 断言BIO和RQ是同一物理IO不同阶段，猜多个RQ共享一个BIO/合并写入或超集；原件没有跨层映射，BIO对象与RQ对象的匹配资格不能由相邻时间/数量推出。“可能”措辞不补足其前后的事实断言。
3. 一边说跨层不能相加，一边写设备8,0共9次＝6读+2写+1BIO，并以此与8,1对比；分组口径冲突。末尾免责说明不能修复正文错误。
4. 发起人口定义一处写开区间 `(1.000,1.010)`，后文与实际值又使用半开 `[1.000,1.010)`；BIO恰在左边界，正文口径自相矛盾。数值正确不抵消定义错误。
5. “两次读请求至少间隔2ms”“4ms单次停留偏长”等缺比较基准，且峰值时段间隔不是所有发起间隔。没有把这些句子当合法链上根因或通过关键词补硬门。

根因旁路131B，`status=unavailable / trace_root_cause_contract_not_active / root_causes=[]`，不存在偷偷发布的已验证根因；但旁路空不替正文越权免责。

### 新实现实际触发范围

- Finalizer Known Facts实际收到3条trace_query预览，带精确evidence_ref、recorded_tool_output、typed_records=1/39/39、runtime_artifact及interpretation_unverified，不再附类别confidence0.80。两次同原生结果仍保不同实际查询时间，不假称独立新人口。
- 本轮未调用exec_command，四次冲突awk分支 **NOT_TRIGGERED**；其系统缺口由公开确定性RED→GREEN证明，不借本轮自然题倒签。
- Explorer自然生成6项、后续7项interpretation_units。首调用漏旧必填interpretation键被拒，模型补齐后接受；失败原文保留。新增units虽可选，但“双载体存在性仍要求旧键”造成额外调用，登记为共享JSON教学/存在性合同待收敛，不把过程重试当最终失败或把它漏记。
- 既有runtime-only叙述抑制使这些模型解释没有进入实际Finalizer，INIT消息不存在独立解释单元/省略预算区。故 **units生成受理触发；成文原子预算及超限分支NOT_TRIGGERED**。不能为证明新能力而关闭已有抑制门重传错误解释。
- 实际Finalizer原生四组统计、完整成员、时序和限定已到场；最终7块都是summary/section/caveat自由成文，没有以原生展示块复用数表。解释越权发生在充分数据之后，不能全部归为缺数据或摘要截断。下一步审共享事实/解释的结构化消费合同，不增加IO专用词表门，也不重跑同题无限追绿。

## B：只读测试登记自然终态 — 机器PASS，人工PASS

独立审计原件 `/tmp/codrax-hmc276-B-manual-audit-v1.md`。PID58083，UTC09:10:13.902988–09:12:18.447108，恢复子PID71307。真实父controller、planner、独立恢复子controller；源码修复、合同和旧执行是operator公共夹具，实际执行由公开RunTests负责，明确 `full_cli_run=false`。

父模型一次plan_batch漏batch，工具拒绝后自然补齐；planner实际整文件读取源码/测试，按具体assertion_ref登记changes:[]；登记不假称测试通过。恢复子重新执行后才finish。原件/HEAD/测试不变，当前授权撤销，当前登记验证有不同于旧执行的新invocation。

当前ChangeReport与实际WriteFinalReport均为1条assertion、1pass、2commands；累计有效证明2reports/2results/4commands。实际本轮runner尝试3次（来源verify、中间缺合同试验、恢复登记verify），加旧operator共4次；中间失败attempt留unverified，不能将7个归档文件当7次执行。前置syntax command不是另一条assertion。

父/子分别7份原件SHA重核一致，3个verify引用、2条final链接全部matched；当前实际final.json不是兼容别名final-report.json。历史source final的weak缺合同状态未覆盖，当前后续见证完成累计闭环。5份context-pack快照内容与身份映射稳定，持久化与重复保存不累积引用。

夹具缺apply checkpoint导致materialization unavailable、source localization weak仍如实保留；不扩签从零源码交付或报告回写。B无read Finalizer/用户MDHTML/图，不覆盖新Known Facts/units/Trace/WAL/sample-only。只读登记有限轮换PASS，不新增完整能力计数，也不替A及历史FAIL销账。

## 留账与退出

本批整答剩余验收缺陷 **1组（A）**；旧sample-only等历史失败继续留原，不因B轮换而销账。生产来源交接/原子预算的确定性通过、WAL安全反证、自然触发与最终人工结论分别登记。55项父账仍开放；不将单个文字错误拆成新的父项，不追加第三次模型调用。
