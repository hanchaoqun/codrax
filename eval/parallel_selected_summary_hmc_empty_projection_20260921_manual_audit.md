# Selected Eval Manual Audit — empty projection authority

- date: 2026-09-22T02:36:15Z
- sweep_start_ts: 20260921-193612
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_empty_projection_20260921

Frozen revision: `c5514cc56787`, buildTime `2026-09-22T02:35:35Z`. Runner2083正式exit0；2并行×1，无第三次追绿，无oracle改写。主审逐读两份完整答案、过程日志及G1旁路，另有独立只读复核。机器1/2、完整人工1/2；只验当前有限问题，不代销旧人工FAIL或父任务。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_g1_english_dstate | PASS | eval/results/hmc_empty_projection_20260921/real_trace_g1_english_dstate-20260921-193615 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 85s | 35 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 空投影真实命中、三段D+IO/.635ms正确；调用点/模块被解释成已证机制，另误释非IO D，完整FAIL保留 |
| 2 | hilog_mixed_arkts_cangjie | FAIL | eval/results/hmc_empty_projection_20260921/hilog_mixed_arkts_cangjie-20260921-193615 | log_attachment,answer_contains | log_triage | 207s | 28 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 四帧/来源/行号/越界信息正确；机器英文contains碎片失败保留。非空投影控制，不冒称新修复命中；前段重试与标签来源债另记 |

## G1：计量/空投影通过，机制叙述未通过

答案：`.codrax/output/20260921-193736.407-55547.md`；旁路同名前缀`.root-causes.json`。日志：上表结果目录下`run-1.logs/codrax-20260921-193617-000-55547.log`。runner85秒，native wall81秒，不混用。

- 整份工件34579.450627..34579.595184；0.138/0.147/0.350ms三段raw D且IO等待，合计0.635ms正确。两次trace查询、没有源码查询；typed自动补window_stats完整覆盖287观察，未偷换窄窗。有限问题没有根因选择合同，schema2空旁路及`trace_root_cause_contract_not_active`合理，不强开因果投影。
- explorer977行提交“两次D”的错误复述集合，漏第三段；与另一个IO原因复述一起被typed projection排空。最终输入没有StructuredAggregateFacts重播，实际emit没有旧“每一聚合必须展示”软建议，answer_contract_advisories=0。确定性公开测试另覆盖cached/direct/emit/patch、源码与日志异构全空、混合独立source、不变的原始审计账。
- 完整人工FAIL：模型把sync_buffer_read_wi说成已证“同步缓冲区读取等待例程/触发”，把sysmgr.elf说成系统管理器，又把iowait=0的D称“纯磁盘/文件类阻塞”。单一内核符号/模块不能证明这些机制，非IO D为零也不能排除全部存储IO。
- 独立追源：explorer977/985中的机制自由总结未进入最终1239–1825输入，1722明确closure省略；1565给opaque callsite边界、1301/1788给非IO D零值边界。错误译文由最终1855发射引入，没有证据表明被过滤的自由总结又成为权威。通用system规则17（1272）仍无条件要求summary解释机制/跨文件，存在教学过宽，未证明为此错误唯一原因，不加关键词硬门、不代写模型答案。

## HiLog：有限帧问题通过，机器词面失败与过程成本保留

答案：`.codrax/output/20260921-193939.890-55538.md`；日志：上表结果目录下`run-1.logs/codrax-20260921-193617-000-55538.log`。runner207秒，native wall205秒。非Trace附件，不要求Trace旁路。

- ArkTS首帧NativeBridge.invokeOhSum / NativeBridge.ets:33及调用帧HomePage.computeTotal / Home.ets:54；Cangjie首帧demo.bridge.ohSum / Bridge.cj:18及checkout / Bridge.cj:42；三路径四帧均准确，index5/size3与中文数组下标越界正确。外部工件来源明确，没有把它们当当前仓读取证明，无源码工具、无虚构图/跨栈确定根因。
- 机器FAIL为`of`、`bounds`英文碎片未保留；不改写原结果。问题要求各语言第一帧，不要求英文逐字输出，主审和独立复核均判本次有限答案PASS。“各自独立”比“未证明关系”略强，仍记措辞债；不能据此说全部上下文/叙述无问题。
- explorer1743/2145分别交付成员集；最终2905–2906保留两个supporting runtime成员集、清除member_notes，2840附近绑定历史外部工件，2978–2990限定peer因果未证。此例是合法非空投影保留对照，不算非Trace全空live命中。
- triager202首轮errors字符串中的坏JSON不可安全修复，203拒绝；226第二轮有非逐字合并evidence及普通错误消息冒充cause_relation.marker，228拒绝合理；273第三轮保2条peer事件/4帧成功。`frames4→0`是当前仓可解析帧数量，原始附件帧701–711仍在，不是丢栈。
- 分析首轮54362上下文token/218380bytes，活跃输出102.088秒后重复熔断（19:38:38.925，580B周期/4096窗口，38328→34040bytes），第二轮成功；不是静默超时或4ms降级。30/60/90秒有活跃进展，最大55882token/28%。最终一次成文、零拒绝/patch/聚合软建议，前段成本不能忽略。
- 新P2来源债：原始六行异常类型仅Error/panic，NativeBridgeError/CangjiePanic是triager派生标签，最终2957却要求逐字提及，读者可能误认原始异常类名。应分离原始类型和派生描述；不新增原文扫描硬门。旧case注释允许无marker传播单链亦留eval维护债，不在本轮改oracle追绿。

## 收口与仍开放

当前代码c551两处存在性修复有公开红绿、race、末版全仓87包（83670正式exit0）、独立审计及G1真实命中。§104未验证优化收益措辞、通用机制教学、日志标签来源、JSON修补/分析重复成本、容量恢复、业务实例局部补齐及B2–B6继续留账。父账13/79交付、66开放不变；600/300/600秒默认值与活跃流保护不变。
