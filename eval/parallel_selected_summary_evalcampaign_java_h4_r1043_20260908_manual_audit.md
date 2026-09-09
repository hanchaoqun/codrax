# r1043 Java 实现/路由 + H4 精确窗供给人工审计

- date: 2026-09-09T02:11:57Z
- sweep_start_ts: 20260908-191157
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

构建基线为已提交推送的 `20507fcdeb1b`，clean build `2026-09-09T02:10:48Z`。243 个现有用例（215 read、25 apply、3 plan）按真实客户影响、证据权限、模式/语言覆盖老化排序；本批补 Java 声明与属性区别、Trace 精确窗状态与供给。上一批 r1042 已跑仓颉读和原生 Python 写。本批严格并行 2，不启动第三个用例；未修改 case、oracle、fixture 或历史答案。机器判分与人工语义审计分开。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_handler_impls | FAIL | eval/results/sr_java_handler_impls-20260908-191157 | typed_inventory_rowset,answer_regex,answer_contains | none | 73s | 28 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial：核心映射通过，额外解释错误 | 逐行文件名 oracle 未满足；模型重复表及字节长度误述；另发现系统证据范围倒置 |
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260908-191157 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 122s | 38 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=3/0,fin_reject=0,unavail=0,prune=0 | partial：核心状态通过，CPU0句自相矛盾 | 两个措辞 regex 假阴性；独立发现上下文跨窗展示和 blocking coverage 域错配 |

## Java：核心答案正确不等于整份答案全绿

工件：`.codrax/output/20260908-191308.495-35773.{md,html}`；过程日志 `eval/results/sr_java_handler_impls-20260908-191157/run-1.logs/codrax-20260908-191159-000-35773.log`。runner 73s，内部 metrics 71s，context 56,419/200,000。

1. 三组真值均正确：EchoHandler → `/echo`，UpperHandler → `/upper`，StatsHandler → `/stats`。源码注解分别在 7/9/13 行，class 在 8/10/14 行；不是依名字猜路由。机器 rowset oracle 还要求每行带 `.java` 文件名，而用户只要求实现和路径；保留 FAIL，不修改 oracle 追绿。
2. 完整答案仍 partial：MD29 把 Java `String.length()` 说成字节长度；两张重复表和该解释都是 log2266 首次模型 payload 原有，不是系统附录代写。第二表未给 columns，现有 renderer 才使用“列1/2/3”。三条引用落在 class 定义行，不单独证明注解路径；源码已完整入模，但引用选择不充分。均不按原文关键词增加拒绝门，不由系统改成期望结论。
3. B1620 获得生产接线正证：log1970 的 ModelNotes 是独立候选，2091–2093 NoteParts 保留来源及 definition 支持上限，不宣称整段解释已证。错误 bytes 不能重新归为已修字段权限混载。
4. 只有一次后续 metadata patch 拒绝：模型提交未发布的 `add_facet_id`，executor `field_not_published`。当前教学明确“若 schema 发布才用，否则完整 replace_blocks”；该稿 source inventory row-ID 表不满足窄原子分支资格，完整替换仍合法，不存在同时必带/必拒的无解合同。记 P2 提示/低负担恢复观察，不为单次波动扩大门。
5. **新 B1629/P1**：log1975–1977 的支持范围是 `8–7 / 14–13 / 10–9`。EmitEvidence 将 line scope 的终点设为起点，GroundItem 随实际源码匹配将起点移到 class，却保留旧 annotation 行终点。这是精确可复现的系统坐标不变量问题；需按 scope/定位凭证修区间，不能简单扩大终点把注解与声明一起授证明。独立小批施工中。

## H4：主账、供给和等待口径分别检查

工件：`.codrax/output/20260908-191357.148-35771.{md,html,root-causes.json}`；过程日志 `eval/results/real_trace_h4_supply_thermal_witness-20260908-191157/run-1.logs/codrax-20260908-191159-000-35771.log`。runner 122s，内部 metrics 120s。

1. 独立按 `donghu.ftrace` 调度事件复算，窗宽 233.190ms，Running 157.248 + Runnable 5.604 + S 70.338 + D 0 = 233.190。目标各 CPU 为 1:7.155、2:0.138、3:2.221、4:35.960、7:11.030、8:4.479、12:96.081、13:0.184ms，合计 157.248，CPU0 无目标运行。MD15 四态正确，首 query 文本及 JSON 完整目标账可用，B1625 获生产正证。旧 case 注释的 sleep73.410/runnable1.994 不作为真值。
2. MD17 明确策略上限存在不等于目标性能受限已证，未把邻近 CPU0 上限或 558MHz 频点当链上根因；但同句“两个 CPU 均有运行切片（CPU4 35.960，CPU0 无运行记录）”自相矛盾。log2896 模型 reasoning 已知道 CPU0 无目标；精确 roster 也已提供，故保留模型表述波动，不改答案或加 prose 门。两个旧 regex 漏接“实际占用 CPU 运行的时间”及“因果关系本身未获证明”，不代表数字或限定缺失；机器 FAIL 原样保留。
3. 独立供给校验：CPU4 共28条 limits，2.27/2.10GHz 各14条，CPU0 共16条，1.72/1.53GHz 各8条。不能把总行数理解为所有记录同一上限。目标 CPU4 35.913ms 在首条 limits 前（策略未知），另 0.047ms 在已观察到2.27GHz的期间，没有可观测2.10GHz期间的目标CPU4运行；不据此排除全系统其他供给问题。已核系统源头：`accumulateCPUFrequencyLimit` 累全部count，min/max/line/ts只保最严格max的一条；`query.go` evidence Summary 又写该min/max “appeared N time(s)”，入模卡称“N条…策略范围”。这是实际代表值/总次数错述，不只是模型自己省略：JSON也没有所有max清单。记 **B1630c/P2** 独立展示口径任务；最小修向明确全部记录数与最严格观测row分离，若以后提供各档次数须由完整源数据统计，不能从现聚合猜造；不以旧oracle的2.10GHz要求硬铸目标受限。
4. MD19 的 Binder5/3.094ms 与 IO completion 至少4/至少4.384ms 分属独立闭合等待，IO 是容量下界；它们处于 S。4条主窗已证issuer17267的completion_closed_issuer_blocked分别为0.782/1.027/1.238/1.337ms，state/wake来源562/602、4038/4161、8195/8322、9759/9939行，互不重叠共4.384ms；190个IO overflow使它是下界。附录0段只覆盖 D、scheduler iowait、带 iowait 标记的 S，不能代表所有层级 IO 为零。模型本次说明了二者不同口径。
5. **新 B1630a/P1**：入模 target CPU identity card 只按subject/CPU/value去重，log2816–2824 把CPU12主窗96.081与子窗94.933并列且没窗口。后方另一张卡正确分窗，属系统自身展示域不一致。修向为现有typed capture/query/window身份分组、未知显式披露，不选择大值或删除子窗。
6. **新 B1630b/P1**：blocking authority 的 `lower_bound_capacity_truncated` 误传只认识状态覆盖枚举的 `StateCoverageWord`，log2776 的已知主窗容量下界到2834变成“覆盖未知”并丢“至少”。修共享调用域，不扩大 StateCoverageWord 去猜其他枚举，不改计时、原数据或模型正文。
7. 本题是 bounded fact set，未请求完整丢帧链/根因调查，不应强制补 Trace 因果投影；sidecar 必选旁路已生成有效 JSON：schema_version=2、root_causes=[]、status=unavailable、reason_code=trace_root_cause_contract_not_active。未把非链背景强升根因，未丢模型答案。3次调查完成不是成文重试。
8. 实际调用进度带 transport/protocol/semantic_seen=true、36353bytes、sse_framing，32.952s正常完成，无retry/degraded。本次没有超过4分钟的调用，不冒充4分钟live证据；本批发布前活跃流及4ms分帧专项count3已通过，仍保真实停滞、调用方取消、显式deadline。

## 批次结论与优先级

机器 **0/2**，人工两例均 **partial**，不把核心维度正确写成整体通过。先独立修 B1629 引用范围一致性，再修 B1630a/b 两个上下文边界；各保有效 RED、泛化正负矩阵、影响包与全仓验证、独立审计、小批提交推送。B1630c 总count/极值口径、B1624b 来源导航、B1626 多请求成员窗等继续队列，不宣称全部闭环。下一live仍按覆盖老化配对含写模式，不反复只跑本题追模型措辞；JSON schema与完整替换后备教学保持一致，系统不创建/删除/改向模型图，不接管业务推理。
