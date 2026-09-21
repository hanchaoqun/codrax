# 5ad 固定双例人工审计：IO显示边界与业务响应

- date: 2026-09-21T06:08:21Z
- sweep_start_ts: 20260920-230820
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_io_group_scope_20260920

固定干净 `5adad718155c`，2并行×1，runner正式exit0；机器1/2、人工0/2。未改case/oracle，未追跑同版第三例。源码全仓87测试包/13无测试包/零FAIL不代签答案正确；本轮均未出现新“同线程IO证据组”组头，不冒称分组公开分支live命中。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/hmc_io_group_scope_20260920/trace_query_wakeup_causal_io_chain-20260920-230822 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 169s | 45 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 窗/量改善；S机制越权、opaque caller升级页面IO因果，漏irq最上游，wakee等待归属错 |
| 1 | trace_query_business_marker_io_chain | FAIL | eval/results/hmc_io_group_scope_20260920/trace_query_business_marker_io_chain-20260920-230822 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 340s | 43 | read=0,repo_map=0,list=0,trace=13,source_lens=0 | midloop=2,inv=2/0,fin_reject=1,unavail=0,prune=0 | FAIL | 缺35ms、业务/查询口径混用及臆造残差；错误scope被称用户指定；未证扩成不存在 |

## 1. 业务响应：FAIL

以下日志行号指该case的`run-1.logs.all.log`，正文指`run-1.primary.md`。原生fixture第4/19行OpenDocument为1..1.05=50ms（状态5/1/44）；第8/14行子业务40ms（8/1/31）；第9/11行请求35ms，第10/12行等待31ms，worker和app各1ms调度，backup第6/18行47ms仅背景。

- **仍错/遗漏**：正文5–7、32以查询0.999..1.051的运行7ms回答“请求本身”，未给业务自身5ms；全文漏35ms。正文29说不可相加，33又把31+2+7=40和剩余12ms归成worker9.5ms及切换间隙，既无完整互斥分解也无该残差归属凭证。35ms虽可从1.005/1.040相减，不等于已用毫秒回答用户要求，不能放宽oracle。25/43把未形成/未记录backup唤醒说成没有唤醒和没有因果关联；将背景排除出链上根因本身正确，过强否定仍不正确。
- **范围权威错误**：原请求2748无数值窗。analyzer792起主动声明`explicit_time_window`、0.999..1.051、quote="OpenDocument 这次操作的完整响应窗口"，856最后稿仍保留，858–859接受。系统2806/3229/3295/3352继而称“用户指定”，正文3照抄。这不是默认capture/query自动回填，而是模型字段语义错误经仅形状/quote存在校验后取得权威。另有bounded+数值被parser自动升级的独立问题，本次未走该分支，两者不可混账。
- **实际供给完整**：最后输入3124/3125给50ms的5/1/44及40ms的8/1/31；3138/3140/3142给35ms请求与31ms闭合等待两个区间；3296–3298给查询52ms的7/1/44。2937–2944禁止残差编造并明确允许两个独立调度段2ms小计。3137/3139/3141明确false不等于已证未唤醒；2821–2826链线程census仅能支持该窗未记录对应唤醒。数字没被截断，不能继续重复同义教学或新增原文关键词门；上下文重复负担另留P2。
- **分类/JSON**：3次analyzer发射、2次拒绝，前两轮分别纠正原文target引用和generic/diagnostic冲突，因果/work宽度最终保留；本例没有独立target_effect_verdict，不能声称混合两角色分支又命中。首次成文3433–3436缺因果强度字段，3491精确patch后恢复，不是畸形JSON/空答案；后续软work patch仅选OpenDocument原生ID，21行的50ms业务回执保留，不冒称LoadDocumentIndex回执命中。没有活跃SSE截断或备用降级。
- **系统图/旁路**：MD82起保Trace因果投影，两个text围栏完整；31ms根因在链上，47ms背景隔离，50/40业务线索未丢。schema2 available，根因3条31/1/1ms；取值和链资格保留，但旁路/系统图也继承误标“用户指定”范围，不能称整个sidecar无误。模型正文与系统补充分别审计，35ms没有因补充区的存在就算正文已答。

输出：`.codrax/output/20260920-231359.123-80619.{md,html,root-causes.json,answer-surfaces.json}`（日志3934–3937）。Markdown SHA `311c93bcfaadeb652015db0dd2a2a4071739ae968e5b3c57d4382e0206bb9a08`与receipt一致；primary文件SHA `1d4c711f1f797fd7dace6bb5c9caa96bc1a9b52d9f6c0b6c76c0f7906cc851b4`。不同口径的answer hash不冒充primary文件hash。

## 2. 明确20ms窗口：FAIL

- primary第1段保2.000000..2.020000、app20ms睡眠、2.020唤醒，未混成窗外2.020020的sched-in；20/17/14/11ms各量保住，未再说11比14/17更长。正文没有为cookie/network补造错误单段端点；系统定位范围保持中性，不当作连续状态证明。
- 但第1段断言“并非主动睡眠、被动等待响应”，S及唤醒关系本身无此机理证明。主要原因段把`fscache_page_wait_on_page_bit`这个原始调用点升级为“页面缓存IO完成、导致整条链”，原始记录只支持IO等待及调用点，不支持特定文件/缓存对象或工作完成协议。
- 最终链漏`irq-2 → threadpool-400`；`pre_wakeup_wait`是被唤醒线程等待，正文却称上游链路等待。依赖图的唤醒边也不等于每跳完成业务工作。PIC低优先级候选20/CFS对52/RT的大方向保留，但“与app同属低优先级依赖方”仍不严谨，候选状态不能升级成已证锁依赖。
- 最终MD有系统因果投影，mandatory schema2 available。本轮模型仅选择threadpool一条11ms根因，描述也含上述页面IO推断；不能用系统证据字段中保留opaque caller就认为模型description被纠正。未出现新IO分组组头，不作§58 live命中。无空答案/活跃流截断/硬成文重试。

输出：`.codrax/output/20260920-231107.654-80601.*`。对照原14行输入、native结果、最后实际成文提示和模型primary独立审计；准确时间/状态的改善不抵销机制与关系错误，也不倒签此前同case的FAIL。

独立复核补记：前置triager日志219误写wake=2.019、blocked_reason CPU2、52为CFS、11ms最长及caller机理；最终输入1863/1868–1875/2005/2398已分别给opaque caller、wakee所有权及irq上游、S机制未提供、链席不等于工作完成证明。最终错误来自模型2491/2545的发射参数，不是renderer新造。系统MD57–60仍有cookie/network状态镜像行、203–204保旧“链上发生段”词面，留显示债但未认定重复计价。MD SHA `381824f09923f67c0442f542c18a48cbe7feb74048bb376cd7d854058f5a4746`；root JSON `1e144ca791bac847c5a2cb6bff838f2b1ebafdce31e8ec6ceba7492eea3de018`；primary/principal相同，SHA `a80dca8f5e544613a0fa8a400e5a55420c3fd751ca5166d3e40208283b1cb5ae`。runner墙钟169秒与进程`run-1.wall`165秒分口径，不互相覆盖。

## 3. 后续处理

先收住§57/58确定性修复及全仓收据；XERR公开带洞/混合状态反例确认后单独修复；随后移除bounded选择器的确定性越权晋升，保护真正explicit与业务自动补齐。模型直接误声明explicit、正文错尺/未证过强推断、上下文精简、原始占用镜像去重和跨模式补证仍开放；不以定向绿销账父项，HMC仍13/79交付、66开放。
