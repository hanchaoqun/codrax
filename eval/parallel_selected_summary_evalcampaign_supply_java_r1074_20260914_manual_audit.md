# r1074 人工审计：供给证据回查与源码成员资格（2026-09-15）

- date: 2026-09-15T07:09:27Z
- sweep_start_ts: 20260915-000927
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

机器结果保持原样；人工检查实际请求、模型输入、工具过程、源码/Trace 原文及最终答案。机器通过不代表业务结论正确。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_handler_impls | PASS | eval/results/sr_java_handler_impls-20260915-000927 | typed_inventory_rowset,answer_regex,answer_contains | none | 100s | 29 | read=5,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass（核心）；引用质量 P2 | 三组类/路径正确；聚合两处资格一致；注解未入可引用池，多行注释仅展示开符 |
| 1 | real_trace_h4_supply_thermal_witness | PASS | eval/results/real_trace_h4_supply_thermal_witness-20260915-000927 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 340s | 51 | read=3,repo_map=0,list=0,trace=24,source_lens=0 | midloop=1,inv=3/0,fin_reject=0,unavail=0,prune=3 | fail | 状态/占时正确，频率单位及事件种类错误，Binder 与独立 IO 口径混淆；另有系统源行回查误拒 |

## 运行与覆盖范围

- 已推送代码 `de15d5c7848b`，清洁构建 `0.1.20260915 / 2026-09-15T07:08:58Z`；`CAP=5 PARALLEL=2 TIMEOUT=1200`，上述两个 case 各一次，无第三路或失败追绿。
- 当前 243 cases（215 read、25 apply、3 plan），按用户价值、确定性风险、新鲜度与来源跨度选 H4（最近 r1045/09-09）和 Java（最近 r1043/09-08）。前批已跑 C write，本批两种 read 来源不冒称新增写模式验收。
- 两路均 full 后一次成功局部 patch、零成文拒绝。Trace 机器 `exp=0` 不是没有探索：实际 3 路子调查、24 次 trace_query；指标不能替代过程审计。
- 无必需图，两例无图不判关系丢失，Mermaid/全语言图谱生产覆盖 N/A。

## Java：核心通过，引用未完全闭合

工件：`.codrax/output/20260915-001105.137-2334.md`；日志：`eval/results/sr_java_handler_impls-20260915-000927/run-1.logs/codrax-20260915-000929-000-2334.log`。

1. 最终三组映射 `EchoHandler → /echo`、`UpperHandler → /upper`、`StatsHandler → /stats` 均与真实 fixture 一致；对应注解行 7/9/13、定义行 8/10/14。接口、实现关系、方法效果均正确，没有把 String.length() 称作字节数。HTTP 字样比当前路径路由代码证明范围更具体，属轻度措辞过度。
2. B1695 获得实际负向保护证据：日志1245的模型聚合只用定义行，不带系统关系成员标记。1977 ledger 为 `model_inference`，2069聚合供给为 `advisory_model_inference/not_authorized`，2082明确未形成权威关系集；不再一处独立已证、一处未授权。合法系统 marker 正控本轮未自然触发，仍由本批公共测试证明，不能虚记自然正控。
3. 引用不是逐项闭合：最终22–27行只有定义和文档注释，没有三条注解。探索1114/1176两次 emit_evidence 都选择定义行；finalizer 可引用池不含注解，路径仅随模型候选聚合/调查总结交接，不能单独归责 finalizer 漏选。
4. **B1698-DOCQUOTERANGE1/P2，实际佐证、待公共 RED**：Stats 完整多行说明在自动 companion 中被提取为摘要，`autoPairRoleDescriptionEvidence` 却只保存首行，不保 LineEnd/Snippet；最后引用退化为 `/**`（MD26）。后续共享原有注释提取器的真实首尾，保已读原文范围，不从摘要推行号，不自动补注解或替模型选答案。
5. JSON 教学没有新增矛盾：2308 full 接受；2327局部展示提示；2355 patch 只补 member_set facet，2364接受，正文与引用继承。无畸形 JSON、全答重写或失败补丁循环。

## H4：人工不通过；区分模型错误与系统缺口

工件：`.codrax/output/20260915-001504.755-2331.md` 及同名 HTML/root-causes.json；日志：`eval/results/real_trace_h4_supply_thermal_witness-20260915-000927/run-1.logs/codrax-20260915-000929-000-2331.log`。

### 正确事实及不能误判的部分

- 精确窗 `13762.791708..13763.024898`，总长233.190ms；Running157.248、Runnable5.604、Sleep70.338、D0ms，和为窗口总长，最终数值正确。旧 case 注释的1.994/73.410不是本次真值，不改 oracle 追绿。
- 8个CPU累计运行：12=96.081、4=35.960、7=11.030、1=7.155、8=4.479、3=2.221、13=.184、2=.138ms，最终没有丢CPU。
- CPU4 `cpu_frequency_limits` witness17113，最小558000/最大2100000kHz、同窗有效记录28；CPU0 witness8048，418000/1530000kHz、16条。它们是策略记录，不是已证切片性能影响或热限频根因；CPU0不是目标运行CPU，只能背景。
- **CPU3 不能倒填频率**：本次5份 target_window_states 原始结果均无该桶 representative_frequency，finalizer3433/3442忠实 absent。目标CPU3运行段在13762.791708..13762.793929；附件CPU3首频率样本直到690行13762.798126=840000kHz，920000更晚在819行13762.798984。不能用未来样本反证此处“无频率采样”。初审怀疑已否证，不记缺失数据 gap。
- 本题是有限状态/供给事实题，`bounded_fact_set` 且不要求帧因果/链上根因。因此没有强制根因投影是正确范围；必选旁路实际存在：schema_version=2、空 root_causes、status=unavailable、reason=`trace_root_cause_contract_not_active`。这不是 root selection 出错或必须投影退化。

### 实际错误及产生位置

1. **频率单位1000倍错误**：最终CPU4写558kHz、CPU1/2写920kHz；实际finalizer3431/3432/3434提供920000/558000kHz。错误已在模型首次提交3575的 cells 中，非 renderer 转换错误。应分别为558/920MHz；不能系统扫描正文换数。
2. 策略事件误名：最终52行说28行 `clock_set_rate`，原始证据是 `cpu_frequency_limits`；28是该查询有效记录数，不证明28条均有同一 min/max。最终使用“最后采样值”缩短了供给中的“运行段起点所见的最后正频率”口径，不能冒称该窗口所有原始采样的最新值。
3. **IO/Binder混账**：finalizer3321清楚提供 Binder 5次/3.094ms，3526单独提供目标独立IO闭合至少4次/4.384ms（有S状态区间）；最终caveat却写“独立IO完成闭合口径至少5次Binder等待”，把两源混合。四状态表又把 D-State/IO-Wait作为同一状态名。不是生产测量把Binder铸作block IO，不新增针对这些词的正文硬门。
4. 内部枚举直接外露、CPU表无 columns：3575原始模型JSON省略CPU表列名，最终显示“列2/列3/列4/列5”；3575也已含 comparison authority 两个内部枚举。仅一次s2状态表patch补columns与facet，s3未修。不靠替换输出接管答案，后续从初始/修补同源简化展示教学。
5. analyzer在517/527把用户未请求的 `drm/hwcomposer/legacy` 子题带入探索；后续关键词零命中不能代表全类限频不存在。暂没有系统prompt硬编码该三词的证据，按本次模型分类偏移记观察，不因单例新造关键词排除门。

### B1697-TRACESOURCEFOLLOWUP1/P1：原始工件回查误当源码

- 1988/1989模型依据工具发布的 witness_line，以同一source/时间窗/行范围回查，没有指定pid/thread/event_types。1994/2011有效参数被继承目标17267，实际策略事件emitter4776被过滤，命中1/0条。
- 2056/2057随后 `read_file` 直接读当前附件的8045/17110行附近、limit10/6，被2058–2068的runtime-evidence source fallback门拒绝；接着又回到相同受过滤查询。附件17113的原始 `cpu_frequency_limits` 行确实存在；wrapper与fixture相差一行，不是工具witness索引错误。
- 两个gate只有当前query发布的 payload_ref/raw_ref 可读逃逸；原始source坐标不在同一读能力中。它是运行时证据核对，不是切回客户源码；此处属于系统供给/执行能力不一致。
- **并非完全死锁**：已存在精确CPU-global event_types查询车道，后续可不继承目标；不能称所有替代都被堵死。本片不删除目标继承、不扩大时间窗、不根据event pattern猜全局范围。
- 最小修向：成功原生查询的精确原始source身份与有界只读能力同源，保runtime分类；已发布结果、原始capture、派生阅读文件严格分开。新能力不可由summary、basename、模型正文或任意路径推得，保turn/fork/终止admission边界。先公共RED+反例再施工；是否改善本例模型单位/结论仍需以后真实回放，不能把二者因果相连宣称已证明。

## 收据、优先级与红线

- 机器汇总不改；`.codrax/tmp/20260914-r1074-case-fixtures-before.sha` 的15项在回放前后全部一致。审计结果与答案快照为 `.codrax/tmp/20260915-r1074-results-audit.sha`、`20260915-r1074-answer-audit.sha`，原答案/日志/指标/verdict保留。
- 先收 B1695 已交付与本轮审计，再处理 B1697 公共源回查；B1698 注释range是独立小片P2。B1693/B1694身份物化与合并、B1319/B1561写验证、B1696展示教学、完整图关系/时序/逻辑库存仍按原账OPEN，不以两题或无图代销。
- 模型选择/答案/关系/结论不由系统替代；Trace邻近背景不铸主因，链上占时/可消双轴与业务线索保护不变。本轮不是根因型Trace，也没有新的图/写链生产正证。
- 实际请求日志为first_byte_timeout=10m、stall_timeout=5m、timeout=10m。600/300/600s默认不变；活跃heartbeat/reasoning/tool字节不因4ms/旧4m无可见正文降级。340s整题不等于单次活跃长连接专项。
