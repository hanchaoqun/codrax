# HMC 背景原生计时：固定双例人工审计

- date: 2026-09-20T15:42:11Z
- sweep_start_ts: 20260920-084211
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_native_duration_20260920

固定代码00be02cac73e，原生构建2026-09-20T15:41:34Z；2并行×1，各1800秒上限，没有第三例追绿。机器2/2，人工0/2。自动断言未覆盖全部叙述与窗口一致性，不能据机器PASS销旧FAIL。此时末版全仓另有两条结构登记失败，收据见主账本§34，不冒称此版本全仓绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/hmc_native_duration_20260920/trace_query_wakeup_causal_io_chain-20260920-084212 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 171s | 45 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 20ms窗/11ms链上IO保留；把14/17ms睡眠与1ms就绪等待混解释，窗末唤醒误述立即运行 |
| 1 | trace_query_business_marker_io_chain | PASS | eval/results/hmc_native_duration_20260920/trace_query_business_marker_io_chain-20260920-084212 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 384s | 41 | read=0,repo_map=0,list=0,trace=10,source_lens=0 | midloop=1,inv=2/0,fin_reject=1,unavail=0,prune=0 | FAIL | 背景47ms正文/图/表恢复且不入链；业务50ms仍套51ms账，31ms又被误称请求耗时，控制枚举泄漏 |

## 业务例：改善项与仍失败的边界

报告`.codrax/output/20260920-084834.210-33537.md`及同名root-causes.json；日志为结果目录下`run-1.logs/codrax-20260920-084214-000-33537.log`。原fixture为`eval/fixtures/hmosperf_business_io_chain/events.systrace`。

已命中本批修复：独立backup请求47ms在模型正文、系统总览/树/关键指标及背景观测保持47，而非18.55/17.85ms；背景未进入根因JSON三项。IO请求35ms、S睡眠完成闭合阻塞31ms、主线程及依赖线程各1ms就绪等待都已到达模型，正文这次也出现35ms。该确定性发布缺口取得live正针，不代表整份答案通过。

人工FAIL：

1. OpenDocument完整B/E为1.000–1.050，50ms；模型明知该窗却查询1.000–1.051，把Running6+Runnable1+Sleep44=51称可直接对应业务窗。原50ms业务账应为Running5+Runnable1+Sleep44。系统投影及JSON如实保51ms查询窗，未自选业务实例。两次completion均未提交实例选择，补齐skip no_typed_target。不能从截断的日志摘要搜不到token，就断言工具未给模型引用；同参数真实消息回放见下。
2. 第3维先说“请求发出后31ms才完成”，同段后面又正确说请求35ms/睡眠阻塞31ms；同一答案自相矛盾。原始issue1.005、complete1.040、睡眠1.009010、唤醒1.040010均已提供，不是证据缺失。
3. 第2维把互斥Sleep/Runnable说成不能相加为总等待，并称44ms睡眠全部由单个IO链解释；系统自己的等待账仍保31ms已归因及14ms未归因（另有非可加链上行）。单次唤醒关系不能独证整段睡眠机理，也不能据可消量保证删去IO后整个睡眠“不被触发”。
4. 可见正文泄漏`typed_pairwise_disjoint_section`、席位/授权等内部控制词，且“不等于两张1ms直接相加”与精确互斥合计2ms表述别扭。既有json-only教学已到达，未发现schema要求把该枚举写进正文；不通过扫描/替换模型散文掩盖。

过程：10次trace（3搜索、4统计、2唤醒链、1根因榜），两次explorer；analyzer实际4次emit/3拒绝，第四次保留完整根因问题及背景IO判断，尚不能定为无解合同。finalizer一拒绝是缺summary及其限定字段，随后add summary成功；另纯有序根因数组安全包装为schema2容器，最终旁路available三项，答案未丢失。上下文41%，原始数值及口径都有供给。预阶段曾把业务span当帧、35/31混淆及使用错误平台优先级含义；后续精确工具纠正了部分内容，仍归既有预阶段权威观察项，不称全部污染已根修。

引用供给独立核验：临时overlay用真实Explorer.Execute、生产TraceQuery、完整物理附件绑定和同四组统计参数，只以capture adapter替代网络模型，捕获下一次LLM调用完整messages。除临时路径长度后字节数与live日志1474/1490/2120/2136精确相同：28158、27709、28227、27778；每条含2个本run当前可解析引用，OpenDocument为TID100/1..1.05/行4..19，focus仍None。`/tmp/codrax-33537-ref-audit.OIiApc/actual-message-normalized.log` PASS0.839s。这是同参数确定性消息回放，不是原live完整消息的历史抓包；不能把旧358仅rank受视图白名单的漏发原因套本轮。生产agent发送完整Summary，日志仅打印前2000字节，引用在约26KB之后。

## 明确窗口例

报告`.codrax/output/20260920-084500.986-33526.md`及同名root-causes.json；四次查询保2.000–2.020/TID100。20ms目标sleep、0running/0runnable、11ms链上D/IO及400→300→200→100链保持，schema2 available四项。

正文第30–31行把network/cookie的14/17ms睡眠称为折算后1ms优先级候选；实际1ms是各依赖线程自身就绪等待，不是睡眠折扣。第31行把2.020000唤醒说成立即运行，但真实切入2.020020已在用户窗外。第41行由内核等待调用位置推缓存页读取及预加载修向，与第25行自己的限制冲突；第42行把相邻唤醒事件约2ms的间隔当调度延迟探索入口，不能替代实际wake-to-run 1ms，也不能只凭跨CPU推出同核修复收益。

旧跨CPU概括错误本例已改善：第17/37行明确只指400→300→200→100三跳，4→3、3→2、2→1确实跨核；不能再拿旧报告包括irq首跳的断言判本例。当前列表使用label+text，没有cells，不冒称列表新分支已live命中。关键模型上下文已含实际状态/归因分尺、优先级候选不证反转及调用点限制；继续分析共享上下文如何更清楚，而不是据一次错答增加散文硬门。

## 保护边界与下一步

两例均有完整答案、Trace投影、mandatory旁路；零超时、零系统unavailable。传输日志保stream=true、非流式/首响应10分钟及静默5分钟；阶段预算日志的3分钟不套到活跃流，原实现及保护测试已核。没有4ms无答案降级。

先补末版结构登记并验全仓；再修已确定的“不同查询视图携同一原生完整实例但导航引用不等价”接缝，但它不能替本轮已有引用而未选择销账。另一行级上下文候选在审：末尾候选事实卡是否把睡眠占用和就绪归因紧邻并称“对应”，而没有复用前文已有的精确值构成。旧业务/明确窗人工FAIL及跨模式B2–B6保留，13/79交付、66开放不变；不重跑本版本第三例、不改case/oracle追绿。
