# HMC 提交合同与窗口口径：固定双例人工审计

- date: 2026-09-21T03:27:29Z
- sweep_start_ts: 20260920-202729
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_submission_window_20260920

固定生产版本 `519c997ef88e`，原生干净构建退出0并核对版本。两个旧FAIL各一次、并行2；不改case/oracle，不跑同版第三次追绿。机器2/2，人工0/2，旧失败不销账。核对最终正文、完整报告、实际finalizer上下文、工具原始记录及fixture；两份归属收据均available，primary/principal已验证，不借系统补充表判模型正文通过。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/hmc_submission_window_20260920/trace_query_wakeup_causal_io_chain-20260920-202729 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 212s | 43 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | FAIL | 把被唤醒线程14/17/20ms等待移给唤醒者；状态机理与跨CPU修向过度推断 |
| 1 | trace_query_business_marker_io_chain | PASS | eval/results/hmc_submission_window_20260920/trace_query_business_marker_io_chain-20260920-202729 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 247s | 44 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 50ms业务窗与51ms查询窗混账；31ms等待误作请求耗时；捏造物理重叠 |

## 1. 业务窗：供给已补齐，正文仍矛盾

正确部分：OpenDocument 1.000000..1.050000=50ms、LoadDocumentIndex 1.004500..1.044500=40ms，storage-irq→document-worker→app-main链正确。根因明细正确区别31ms响应等待/35ms请求驻留，47ms后台IO未加冕。Trace因果投影、自动补齐均保留；模型未提交业务实例引用，系统没有擅自把1.000..1.051查询窗改成业务窗。

人工FAIL证据：

1. 摘要说“这次操作”运行6ms，但50ms业务区间自身是运行5+就绪1+睡眠44；6+1+44属于51ms查询窗。真实finalizer消息日志第2495行已提供marker-local 5/1/44，不能说本次缺证；explorer completion第1583行先将业务50ms与查询运行6ms混写，finalizer沿用。后续审计多来源上下文的窗口归属，不扫描正文硬改。
2. 摘要将“请求发出1.009010→完成唤醒1.040010=31ms”当请求生命周期。fixture实际issue=1.005000、complete=1.040000，35ms；31ms是线程S至唤醒。后文写对不能抵销摘要矛盾。工具上下文第2475行已同时清楚提供两种口径。
3. 声称IO等待和两个1ms调度等待“时间部分重叠而非串联”。实际三段1.009010..1.040010、1.040010..1.041010、1.045000..1.046000互不重叠。禁止跨口径随意求和是边界，但不能捏造物理重叠作理由。
4. 将S进一步解释成“协作性睡眠，不是IO阻塞”，状态本身不能排除下游IO依赖。backup请求fixture明确W却写成读；storage_latency直接发射也是内部术语泄漏。

schema2旁路available，31ms IO、两个1ms调度项保留，不等于正文通过。analyzer 5次尝试4次拒绝后成功，没有再误报5次写入，但profile真实不一致仍多轮修正，未宣称重试问题解决。

## 2. 明确窗：等待归属共因

正确部分：2.000..2.020明确窗不变、目标20ms睡眠；400→300在2.016、300→200在2.018、200→100在2.020；IO11ms及三个各1ms供给候选保留。network递归2.001..2.018已称测量窗口，没有再直接叫睡眠起止。窗口显示补正有命中，但不足以人工通过。

人工FAIL证据：

1. 摘要与唤醒关系把400/300/200自身等待写成14/17/20ms，这三个量实际属于右端network/cookie/app的唤醒前等待。`WakeupEdge.LatencyMs`及finalizer第1988–1991行的pre-wakeup wait未在数值旁写明owner；Observation subject=waker/object=wakee加大歧义。不是`TargetBlockedMs`数值错误，不应改底层账；应消除角色供给歧义，不能全销为波动。
2. 将S说成非阻塞性等待；把缺少业务方法归属扩大为已有调度关键链仍需证明，混淆两层证据。
3. 仅凭跨CPU及唤醒间隔提出调近CPU减延迟，缺竞争/通信代价证据；三条边三个事件只有两段相邻间隔，却写成三个2ms。
4. runtime_work_relation、schema与英文修向直接泄漏。系统占用表cookie17/network14各重复一行，挤占五行额度，仍属独立展示缺陷，不能同名同数字强去重。

schema2旁路available，模型选择IO11ms/cookie1ms；允许部分选择，不把未全选记schema失败。旁路PIC定位仍用“发生”写递归范围，纳入同类窗口显示补正，不改资格或时长。

## 3. 验证边界与后续

该版全仓实际exit1：tool占用位置文字pin、dispatch-sensitive Description byte golden、types的Producer精确相等遗漏run后缀，共三条失败。前两项须审查后更新；后一项是边界缺陷，复用现有producer家族函数，不放宽lint。保留`/tmp/hmc-submission-window-final-full-20260920.log`，不拿前版/定向绿色替代。

优先修累计包络与精确interval教学、唤醒边等待owner；再处理原始占用展示身份及业务窗摘要归属。各片另记版本/退出收据，本次人工0/2永不倒签。HMC总账保持13/79已交付、66开放。
