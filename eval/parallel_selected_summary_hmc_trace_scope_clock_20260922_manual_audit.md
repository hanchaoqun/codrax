# Trace scope / source-clock 固定双例完整人工审计

- date: 2026-09-22T14:01:55Z
- sweep_start_ts: 20260922-070154
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_trace_scope_clock_20260922

runner93854正式exit0；恰好2并行×各1次，机器1/2通过、完整人工0/2。冻结构建483f87e4ae0d-dirty（2026-09-22T14:01:25Z，dirty仅文档）包含新时轴合同和分片实现，但不包含随后d1cdd4b43的安全上下文复制修正。后者未重跑live，其独立定向/race/全仓另记，不替换本次原始版本或答案。主审及两个独立审阅者均完整阅读对应答案、原始字段、过程/最终输入和旁路；没有第三例追绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/hmc_trace_scope_clock_20260922/trace_query_frame_semantic_span_optimization-20260922-070155 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 181s | 43 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 原窗/投影/状态分账保留；正文越权说已证丢帧原因及校验完成触发唤醒，违反已送达的时间与机理边界 |
| 1 | trace_query_jank_field_inventory | FAIL | eval/results/hmc_trace_scope_clock_20260922/trace_query_jank_field_inventory-20260922-070155 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 202s | 32 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 3条、7/4/2、大整数、70/40/20ms均正确；正文仍错误否认同源时间轴，appid无据升级为应用PID |

## 1. 卡顿字段清单

证据目录：[jank产物](results/hmc_trace_scope_clock_20260922/trace_query_jank_field_inventory-20260922-070155)。完整终稿39行，对照run-1.primary.md全部及主日志3083行；外层202秒、案例自身200秒。输出原件 `.codrax/output/20260922-070515.487-74956.md`，必选旁路同stem。

- 最终清单3条，顺序7/4/2、各头报告时间及全部原始纳秒整数均正确；差值70/40/20ms未经过浮点失真。99帧的other前缀反例与invalid数值均未纳入。不是初次宽搜索7条或早期4条草稿的最终答案。
- primary第19行明确声称“时间轴不匹配”“两者并非同一时钟域”，这是实际语义错误，不是仅漏写单位换算。第21行把appid=620解释为发起应用进程PID也超出记录证据；emitter101与marker201倒已分开。source_trace_clock、scope_complete、artifact、on-chain等仍外露，显示债保留。
- 实际finalizer INIT三个消息始于主日志2292/2590/2644；第2763行提供完整共享同轴合同、只除1e9、报告时间≠症状端点、精确整数先作差及三种身份分离。2764–2766三个原生inventory均明确source_trace_clock，source/canonical=trace_seconds且identity；原始字段和物理行完备。2729明确5条模型自由预分诊观察已由同源原生查询取代，2605–2607保留的是60.010ms时间单位事实。未发现系统仍把这些字段标不同时钟或缺映射；旧探索1254的错误“非线程运行态时间轴”未作为终稿事实送达。
- 因此已证parser/typed传递/本次教学修复成立，而最终答案违背充足上下文；未证明随机波动，也未证明某一旧clock提示污染。不能加正文词扫描或自动改写根因来代替后续泛化改进。
- 机器另报7/4与其整数超过表内距离针：最终表后按序列出7/4/2，人工不把正则距离漏针算为数值内容错。同轴正向regex还会命中“并非同一时钟域”，说明机器粗针不承担语义验收；本次不改oracle重签结果。
- 独立上下文噪声留HMC-01/16：分析IR把“原始起止纳秒整数”标为source_location，并把预览行4/10/12当用户引用行；成文2708带无关源码定位提示，2821–2825误说问题指定了这些行。最终没有引用错正例行，不能把此噪声定为时轴错误直接原因。读者末卡2870–2875缺业务字段/时轴解释，应评审typed语义投影，不从正文关键词硬门补课。
- 无需因果图的清单题没有投影合理；旁路schema2/root_causes=[]/trace_root_cause_contract_not_active合理，不是生成失败。一次普通预分诊和emit，未命中新受控分片机制。

## 2. 显式窗口、确定性语义工作与因果投影

证据目录：[semantic产物](results/hmc_trace_scope_clock_20260922/trace_query_frame_semantic_span_optimization-20260922-070155)。完整终稿310行，旁路56行；原件 `.codrax/output/20260922-070454.136-74944.md`。外层181秒，案例自身179秒，分层保留。

- 最终第15行直接称类校验为丢帧及阻塞原因，第19行又承认无frame/deadline证据；第260行明确“类校验的完成触发wakeup”。原始trace第6行5.005000唤醒，第7行5.005400才结束校验，完成晚0.400ms。正文两处均不成立，正确系统附录不能抵销。
- 成文输入主日志1973准确wake/CPU2→CPU1；2216给出原始5ms、有效4.6ms、完整5.0004..5.0054和完成机理未证；2439明确不能用当前凭证断言目标等待该操作、完成触发唤醒或直接阻塞，2452/2458再次按成员封顶。未缺反证，属模型对正确结构化证据越权，不是分片丢失时序；不能仅凭本次进一步认定随机波动。
- 显式5.000..5.007范围、S5/runnable0.8/running1.2ms，完整因果投影、VerifyClass墙钟5/链上计入4.6ms、worker边前running4/full4.6ms都在。没有把两方向潜力相加，没有把跨CPU唤醒当直接竞争证据；背景与邻近不进榜。
- schema2旁路available，两个模型主动选择候选为0.0046/0.0008秒，原窗与frame_unproven准确，证据保留语义完成机理未证。第一项模型描述“持续4.6ms（折算有效归因）”仍混口径，不能因JSON数值正确宣称全文可接受。第二次成文仅补旁路（日志2611–2618：unchanged=5，replace/add/remove=0），未修正正文。
- 1.1KiB inline/普通single-shot，未触发分片来源/取消机制；只证明普通路径保护。三次模型trace_query加一次系统补采，无源码读取；最后旁路补充8.484秒，最大上下文86,241 tokens/43%。首响应/静默600/300秒仍在日志187，未见提前降级。
- 低优先确定性显示残留：背景压力输入2093是cpu·ms，报告95/301简写ms，125虽注明跨线程累计，单位仍不一致。须独立修复，不与模型完成机理越权混作同一故障。

## 3. 本次交付与剩余范围

本批parser协议、教学、受控分片和容量的公开红绿/race按统一账本§132验收；两例未命中新分片，也不把随后安全复制修正倒签到483live。旧人工FAIL原样保留，79唯一任务仍13已交付+66开放。下一优先级保持原生断言只读登记/新执行代次、业务字段与caller语义投影、局部补齐/容量及其余B2–B6。不因两份新模型错句无限增加提示或对单一案例硬约束。
