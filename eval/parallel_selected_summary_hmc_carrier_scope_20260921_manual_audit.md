# 2c0362e6295b 固定双例人工审计

- date: 2026-09-22T03:51:30Z
- sweep_start_ts: 20260921-205128
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_carrier_scope_20260921

冻结revision=2c0362e6295b，buildTime=2026-09-22T03:50:29Z；runner47633正式exit0。机器2/2、完整人工0/2；主审与两名独立只读审计者均阅读完整最终答案、相关输入/工具/修补日志及旁路。无第三例，无oracle修改。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_g1_english_dstate | PASS | eval/results/hmc_carrier_scope_20260921/real_trace_g1_english_dstate-20260921-205130 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 78s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 原始数值正确；调用点机理越权，另证关系摘要将数字成员拆伪关系 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/hmc_carrier_scope_20260921/trace_query_wakeup_causal_io_chain-20260921-205130 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 168s | 46 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 窗口/链/主要值全保；不同状态分量混比和直接/完全机制越权；§104未修 |

## G1：完整工件等待清单

- 日志：结果目录`run-1.logs/codrax-20260921-205133-000-89886.log`；答案`.codrax/output/20260921-205246.165-89886.md`及同名前缀root-causes.json。
- 工件范围34579.450627..34579.595184；三段0.138/0.147/0.350ms、合计0.635ms；原始D+IO、窗口、次数、正文/附录数字正确。真实原生数据日志1727–1730/1832–1835；自动补window_stats保完整287观察。
- 新summary、scalar/decision、全局语气进入system（1279/1453/1456–1457/1503），opaque调用点/范围边界也在1776/1806/1824。模型仍在正文19/23行推出“同步缓冲区读取完成”机理、17行混“纯D”，人工FAIL，不能说因缺事实。
- model聚合2.737ms被已修pre-emit/最终事实账排除；但另一独立Relation Dossier日志1585读原始成员，将时间戳拆成34579→451840等。仅证advisory污染，不声称它导致上述机理推断或已改图。记录§111新任务，不外推§108已覆盖所有consumer。
- 一次final、拒绝0、patch0；有限事实schema2空旁路及trace_root_cause_contract_not_active正确。无超时降级。

## Causal：明确20ms唤醒依赖窗

- 日志：结果目录`run-1.logs/codrax-20260921-205133-000-89875.log`；完整406行答案`.codrax/output/20260921-205416.284-89875.md`及同名前缀root-causes.json。
- 原始工件`.codrax/blob/20260921-205133-000-89875/attached_trace-5d309add.txt`。app在2.000..2.020窗S20ms；窗外2.020020 Running不计。threadpool→network→cookie→app，唤醒2.016/2.018/2.020、CPU4→3→2→1；IO2.003..2.014=11ms，三段runnable各1ms。全部保留且未相加为总阻塞，Trace因果投影未丢。
- 旁路available，模型合法选择一项rank1 IO、impact0.011；不是漏生成，不强填全部候选。末次patch description“直接阻塞源”为模型文本，当前三层唤醒路径不能授权这一直接关系。
- 正文15/339行将cookie17ms sleep与另一个1ms runnable归因比较，推“大部分睡眠未计入app责任”；不同分量不能推出该结论。日志2486–2488的state_value_authority同排却未明确归因分量，属可核实输入精度gap，不单归模型波动，亦不证明必然因果。
- summary“完全通过依赖链间接传导”也过度，已有2489–2512边界并未授权完整原因或直接阻塞。调用点未知资源/后端边界本轮有改善，继续排查文件系统缓存等待本身不判错。没有由thread名臆造业务角色；低优先级调度候选和Harmony优先级口径保留。
- 系统附录55/82/84/187/188行沿用“已证最大可消/11ms可消”等，既有§104仍OPEN；原始占用cookie/network重复行亦未修。原生投影正确不抵销系统显示和模型错误。
- 新教学system1610–1617/1796–1800/1846–1847真实到场。分析器774–775曾合理拒绝bounded_fact_set/root_cause互斥后接受；runnerana=1不是零分析重试。final2614首次5块接受，2615仅忽略漏schema_version的可选旁路；2634软补、2663–2669只补旁路，5块零增删改。硬成文拒绝0/patch1；整例168秒（原生wall166秒）。日志4ms为完成后环境构建，非响应deadline，无超时降级。

## 结论与未销账项

§110载体/来源教学按独立RED/GREEN/全仓及实际到场窄验收；不能据机器2/2宣布用户问题已闭环。下一优先关系摘要投影与数字语法，其次计量分量身份、§104收益口径及既有业务局部补齐/容量恢复。保留旧人审FAIL、父账13/79交付与66开放；本轮没有写模式live。
