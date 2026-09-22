# Finalizer 字段与标签固定双例人工审计

- date: 2026-09-22T15:14:05Z
- sweep_start_ts: 20260922-081403
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_finalizer_reader_display_20260922

机器 **2/2**，完整人工 **0/2**。机器结果见[原始汇总](parallel_selected_summary_hmc_finalizer_reader_display_20260922.md)，不修改机器verdict，不回签历史FAIL。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | PASS | eval/results/hmc_finalizer_reader_display_20260922/real_trace_h4_supply_thermal_witness-20260922-081405 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 185s | 47 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 四态/CPU4策略证据正确；代表频点升级为主要频点/范围，CPU3被补频率，内部枚举外露 |
| 2 | trace_query_wakeup_background_demotion | PASS | eval/results/hmc_finalizer_reader_display_20260922/trace_query_wakeup_background_demotion-20260922-081405 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 208s | 47 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 原窗/链上IO/背景分离与投影保持；系统附录串窗、正文时间/CPU/机理误述 |

## 1. 冻結版本与范围

构建91774正式exit0，revision=`6643304beaee-dirty`、buildTime=`2026-09-22T15:11:35Z`；dirty仅文档，Go/构建输入已提交。runner21438正式exit0，快照`.codrax/tmp/codrax-selected-20260922-081403`，恰好2并行×各1次，无第三例或重跑求绿。案例自身用时H4为183s、background为206s，表内为外层用时。

两例均用已有`FIXTURE=eval/fixtures/stub_repo`隔离源仓。H4本来使用该fixture；background采用运行器已有环境覆盖，问题、Trace及oracle不改。与历史未隔离background运行不是严格相同输入A/B。包含完整system/user原始字段教学及标签/内部顺序分离，不含随后旧测试文案适配或本次发现的旁路范围修复。

## 2. H4：完整答案与输入审计

读取本轮primary/principal/transcript、日志、查询JSON、原Trace及旁路，独立复审一致。以下日志行指`run-1.logs/codrax-20260922-081407-000-49763.log`。

1. 目标17267、13762.791708..13763.024898原窗233.190ms保持；正文running157.248、runnable5.604、sleep70.338、D/标记IO0及未归账0正确，8CPU分量均保留，未混加132.041。零值未扩大为无任何IO，Binder5次3.094ms另计，不与四态相加。
2. CPU4同核直接策略记录max2100000kHz、min558000、28条及见证13762.940114正确；CPU0的1530000kHz未冒充CPU4上限。正文保留“策略上限存在”与“目标实际受限制未证明”两个判断，未把558MHz当上限触发；代表策略记录也不证明全窗不变。
3. 人工不通过：正文把CPU12桶附带2075MHz称“主要运行频点”，并由代表值概括窗口频率范围；把其余CPU聚合行说成“各一条运行记录和频率样本”。query_json20598–20600明确last_positive_running_segment_start，CPU1/7/8的segment_count实际9/7/2（20519/20567/20580），CPU3无频率（20542–20549）。log1911–1928已给逐核数值及非恒定/驻留/均值/效果限定，不能由聚合行推导这些结论。CPU12原CPU窗口驻留另有2750MHz，但它也不能偷换为目标驻留。结论中“CPU12未受…约束”应保留未知，不能由缺配对证明未受限。
4. 正文括号露出`target_effect_unproven_no_slice_binding`等内部控制词。log1401/1407新版完整system、1974原始字段教学、1730–1742标签分离真实到场；允许原始业务字段不授权内部控制码。标题未复制“第N维”，不以一次残余泄漏称全部修复无效或已经完全解决。
5. Explorer六次查询（863/864、935/936、1005、1044）含一次`pattern=cpu=12`无结果后的去pattern查询。1115自由总结有“558kHz”、系统整体低频/CPU4压制的越权推断；最终typed矩阵及1930边界足以纠正，正文没直接复述“明显压制”，但仍误读代表值。不是原生缺字段或可据一次认定纯模型波动。
6. 有限供给事实题不要求因果根因图，普通Markdown状态表格式正常，无Mermaid。`.codrax/output/20260922-081708.807-49763.root-causes.json`正常生成schema2、空根因、`trace_root_cause_contract_not_active`，符合未启用根因合同，不是投影丢失。系统事实对照正确限定附带频率口径，不能用附录正确倒签正文。

## 3. 唤醒链：系统与模型失败分账

读取case内17行Trace、三份原生查询、完整日志/答案/文本投影和schema2旁路，独立复审一致。以下日志行指`run-1.logs/codrax-20260922-081407-000-49764.log`。

- 原窗2..2.020、目标app100的S20ms/running0/runnable0保持。三次模型查询用了2..2.0205宽窗（1304/1305/1383）；系统1521起按原窗补采root_cause_rank和critical_blocking。自动投影transcript69–175存在，主账未使用窗外2.020020起跑。主榜/旁路保threadpool400链上IO11ms #1及三个1ms低优先级依赖候选，runnable/算力缺口分解保持；logger900的D19.5ms不入榜，不直加不同方向潜力。
- **确定性系统串窗一**：transcript118优化旁栏写“app-100窗内running0.480ms”，与58/76/242原窗running0矛盾。0.480来自宽查询self_running_fold_unmeasured，原始起跑2.020020全在用户窗外。类型侧旁路只保Subject/数值并按Subject去重，渲染不带查询域；不是模型错句。
- **确定性系统串窗二**：transcript441单条事实对照写app-100“链上根因排序=#5(有效归因0.020ms)”但无查询身份。值是宽窗2.020..2.020020的runnable；442/443多榜域分支带范围，树173正确标跨窗背景，主榜及sidecar仅4项。问题在单条显示分支丢域，不是核心选举已错误授予主窗资格。两出口按同类“旁路不得丢测量域”修，不改原值/主榜/用户原窗。
- **模型事实错误**：primary22把cookie被唤醒时刻说成2.020，实际为2.018，2.020是它唤醒app；primary34说logger在CPU6，原Trace是CPU5。完整事件及最终上下文足以区分。
- **链资格与机理混淆**：primary1/32把三个供给候选写成“非已确立的因果链”；具体反转机理未证不能抹掉已证链依赖。sidecar cookie描述把1ms写为进入睡眠等待，但其impact_breakdown明确runnable1ms，机读答案也不能整体签收。primary7/36及sidecar首项把caller词面提升为已证明缓存页位等待，log2071/2671只证明调用位置；应保caller排查线索，不赋具体等待对象/持有者/后端机制。
- **关系完整性**：原始4边irq2→pool400→network300→cookie200→app100；native树仅4node/3edge，finalctx2081另有irq→pool的2.014锚，最终未保IRQ却称“四跳”。应区分终端唤醒锚和有计量资格的根因席，补锚不自动给IRQ根因资格。primary20将嵌套等待说成串行，也混淆拓扑顺序与墙钟可加性。
- **内部词泄漏**：primary7的io_dependency、11的sleep_wait/proven_blocking_wall_clock/coverage=complete、13的pre_wakeup_wait不是本次允许保留的原始业务字段。

新版reader/顺序教学log1771/2153–2165/2628到场，本轮未露“第N维”。0成文硬拒、1可选sidecar patch（2738–2778）只补JSON、不改模型块。`.codrax/output/20260922-081731.174-49764.root-causes.json`生成4项及正确原窗，结构/范围正确不抵销模型description错。此题投影是text图、不是Mermaid，语法合法不等于关系/范围完整。

## 4. 处置与未销边界

两处附录丢域优先做通用范围保真公开回归，覆盖单/多行、同/跨窗、缺范围和不同来源；不通过扫描问题/答案增加硬门或系统代写结论。其它充足上下文下的事实/机理误述留HMC-16/18人审债，不追加原题追跑求绿。root-causes必有文件、显式窗、自动补采与链上根因权威保持；600/300/600及活跃流未改。

旧§131/133/136 FAIL不改。原生只读补绑定/新执行代次、caller双轴、已接受业务局部补齐、B2–B6及其它能力继续开放；79唯一任务仍13交付+66开放，不因确定性子修复冲减父项。
