# Selected Eval Manual Audit — 原查询范围保护后的跨模式回放

- date: 2026-09-22T15:47:03Z
- sweep_start_ts: 20260922-084703
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_aux_scope_20260922

机器2/2 PASS；完整交付人工0/2，主审与两个分题独立复审一致。统计正确、Mermaid语法/局部关系通过，均不等于整份答案正确。runner42840正式exit0，恰好2并行×1，不重跑原背景题追绿；原题、输入及oracle不变。Trace沿case内stub仓，源码题用当前真实仓，未全局覆写FIXTURE。

构建57274正式exit0，revision=`97407a6baa33-dirty`、buildTime=`2026-09-22T15:46:40Z`；dirty仅文档。两片公开范围修复已分别提交a762b50e2/97407a6ba，末版全仓48562正式exit0（87测试包、13无测试包），代码54428正式推送至97407a6ba。本固定双例不直接覆盖宽窗根因排行/self-running旁路分支；其证明来自真实公开入口回归，不能以本次机器PASS代签命中。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_zero_origin_wait_account | PASS | eval/results/hmc_aux_scope_20260922/trace_query_zero_origin_wait_account-20260922-084703 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 144s | 34 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | FAIL（计量事实PASS） | 模型scope被系统提升成伪第二工件；完整有限统计被微窗提示贬低 |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/hmc_aux_scope_20260922/read_combo_pipeline_sequence_table-20260922-084703 | answer_regex,answer_contains | none | 632s | 50 | read=33,repo_map=4,list=0,trace=0,source_lens=0 | midloop=15,inv=4/1,fin_reject=7,unavail=0,prune=3 | FAIL | 最终阶段边被追加到调度全部return之后；正文dispatch所有者倒置，必填补证与供给存在噪声 |

## 1. Trace零起点：计量正确，系统伪工件关系导致整份答案失败

材料：`eval/results/hmc_aux_scope_20260922/trace_query_zero_origin_wait_account-20260922-084703/`；日志`run-1.logs/codrax-20260922-084705-000-78560.log`；终稿`.codrax/output/20260922-084925.375-78560.md`、同名answer-surfaces/root-causes.json。外层summary144s，案例metrics/wall142s，两个口径不混写。

- 正确：原始Trace0..0.010s完整覆盖；running两段2+5=7ms，runnable[.004,.005)=1ms，原始D且iowait=1的唯一等待[.002,.004)=2ms，一次、20%；非IO D0不否认原始D。终稿21–36数表和53–54系统等待明细正确，没有把caller .004001当等待端点或发明设备/资源/持有者。精确marker关联遵循原唯一性及微小闭端容差，未延长等待。
- 过程分账：log282预提取误发3ms D/错R+语义，analyzer仍曾复述；原生thread_timeline/window_stats及成文前系统补采保正确7/1/2。finalizer1992明确原生替代9条候选，最终量未被污染。首analyzer拒因缺fact_families，非数值硬门。无源码读取、无因果榜；schema2空root-causes、reason=`trace_root_cause_contract_not_active`对有限事实题合法。
- 确定性系统FAIL：终稿49的`client-41_pid=41 ↔ attached_trace-a30e2883.txt`不是两份工件。log1258模型aggregate的scope被`observation_ledger.go:4359`回退成ArtifactID；其Producer照录model provenance的`trace_query:`前缀，ClaimAuthority仍model_inference。`runtime_artifact_pair_relation_authority.go:50–51`仅查origin和producer，不核已有事实权威；无path时:224–239接受伪ID，renderer显示虚构pair。native查询没有制造第二capture，不能归为模型波动或真实时钟缺失。
- 教学接缝：终稿56将完整10ms有限统计称micro-window/仅局部证据。原`traceWindowStrategyCaveats`只按view/时长产生建议，log1870/1880确实交给finalizer；原文限制更宽jank/stall断言，但模型泛化到本题。应按已声明问题/已证覆盖区分适用性，不能强制扩窗或更改原窗计量。
- 可选patch实际被拒：log2342 field_not_published facet_ids，旧已接受正文保留；不冒称patch成功，也不是流式超时降级。成员表在summary Markdown内而结构化维度要求list/table，提示和可发布操作的具体承载另留审计，不能单由拒绝次数下结论。两张IO表重复属表达负担，非计量加算。

后续共享身份入口公开反例：单真实capture+模型scope/provenance不得生成pair；两个真实capture、派生payload唯一/歧义owner、旧数据和独立typed凭据正负控。修pair端点与derived-carrier ownership的同一事实资格，不删模型事实、不用词面扫描或路径猜测代替来源证明。归HMC-01.3/16.4/18.4。

## 2. 源码读模式：可解析的最终图仍有错误时序

材料：`eval/results/hmc_aux_scope_20260922/read_combo_pipeline_sequence_table-20260922-084703/`；日志`run-1.logs/codrax-20260922-084705-000-78543.log`（含NUL，全文审读不能止于普通grep的binary提示）；终稿`.codrax/output/20260922-085733.197-78543.md`及answer-surfaces。外层632s、案例630s；analyzer5轮、explorer两次派发共34轮、finalizer8轮/7次patch，33次read和4次repo_map，最大上下文50%。并非模型静默超时，不中断活跃生成，也未追加第三例。

- 正确保留：Mermaid sequenceDiagram和四阶段输入/输出/状态表均在，主入口、共享BusContext/Mutable及AnalysisIR等关键载体出现。先前无证的dispatch/返回边确实经过精确校验/修补；最终不再有首稿`StageOutput(AnswerDocumentV2)`返回边，不能沿用已删除错误判终稿FAIL。引用多数已用实际源码替换原意译；当前终稿路径和行号可以审计。
- 最终图FAIL：终稿34–41先Run→analyze返回→task→taskGraph→readSchedulerLoop后全部return；45–47再追加An→Ex→ET→FF阶段先后边。源码`runReadSchedulerLoop`在extract/finalize完成前不会作图示整体返回；图还留未连接DC/FV、正文13倒称agent执行dispatchStageCore，而后者属于Orchestrator且负责调用agent。局部precedence顺序正确不等于整个sequence时间顺序正确，不能用机器PASS替代图义。
- 共享状态描述同样未通过：终稿17称AnalysisIR在analyze后只读，但`orchestrator.go:6788–6804`的`drainHypothesisVerdicts`在消费后续verdict时实际调用`AnalysisIR.MarkHypothesis`更新假设状态。主审已逐行确认独立复审发现；应区分分析骨架与后续假设状态更新，不把整个载体概括为不可变，也不凭这一处误述扩大为所有状态说明均错。
- 确定性修补载体gap：`answer_document_diagram_edge_patch.go:2000`的action=add统一在body尾追加边，且1979不接受occurrence/body_occurrence；本次局部租约禁止whole-block replacement。log6524模型选择删除11边、添加3条precedence，执行器将新边安到旧return/note后，最终MD仍然如此。应该提供精确当前图位置/分支的可选插入凭据或能保持时序的局部操作，由模型选择有据位置，不能系统猜业务时间或授予新源码关系；也不能对所有图家族只验边集合而忽略载体顺序。公开正反需覆盖sequence return、alt/loop分支及已有flow/class语义不受影响，归HMC-16.5/18.4。
- 调查供给审计：第一27轮后再次探索不是无界重复同一次dispatch；log3222–3240记录current probe完成后仍有5个required子题，打开第二调查窗。analyzer目标曾写Phase1=analyze+explore、Analyzer产EvidenceItems、AnalysisIR写Mutable等不准确内容，作为调查目标反复供给；但incident参与者已规范化，finalizer5741为analyze/finalizer、context_only空，不能误称参与者门没修。
- 独立必填补证gap：`emit_evidence.go:7045–7082`对已知stage enum作为参数只看阶段触及，不限定接收API属于真实调度交接。log4439把StageAnalyze→softTransportRetryHintForStage(:2410)与→string(:2436)这两条重试UI/审计用边作为completion阻断，且称schema-invalid。源码真实但非本题阶段交接；修复应区分真实调用参数事实与用户所问交接义务，不放松实际交接证明、不扫问题原文关键词。
- 上下文已给正确四阶段precedence（5747），但5800–5938亦带大量重试/日志/time.Now/string辅助边及83条溢出，所问dispatch.Execute/applyStageOutput主干仍欠对应证据。需要按已声明关系职责优化软排序与供给，避免靠噪声边制造额外必填债。
- 7次finalizer拒绝分开审：选错已声明FF/FV端点、越权整块替换、缺显式dependent carrier选择等是有效结构门，不能仅因次数多要求放宽。模型最终按发布操作修补；仍需减少互相不一致的操作教学和局部语义缺失，而不是去掉验证。用户明确问内部代码载体，本题保留相关技术名词合理，不应机械脱敏。

## 3. 状态与后续ROI

两个历史/本次FAIL都保留，66父开放项不冲减。已推送的范围修复由公开红绿、race及末版全仓独立验收；本次未命中其根因展示分支。下一P1集中于共享身份防伪工件、sequence局部修补的时序位置、无关阶段参数强补证；各先公开复现，再独立修复/提交。原生只读断言补绑定/B2–B6、caller双轴、已接受业务补齐/容量及§140.3同类旁栏范围清单继续在原队列，不因新问题丢账。
