# r1069 人工审计：IO 口径与 C++ 双头修复、来源交接提示冲突

- date: 2026-09-14T08:17:33Z
- sweep_start_ts: 20260914-011733
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

清洁基线 `45e0ad488247`，独立二进制 `.codrax/tmp/codrax-selected-20260914-011733`。243例库存（215读、25 apply、3 plan；Trace29为交叉维度）按影响/权限风险/异构语言/最近覆盖/本机可执行性选H3显式窗IO（上次r1052）和C++双头写（上次r1059）。原CAP5、两例各一次并行2、外层1200s不变；无第三路/补跑追绿。机审0/2，但写补丁独立行为后验通过；读关键三尺正确而完整答案仍有样本范围误述。原机器结果、case、oracle、模型答案和正式验证报告均保持原样。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_nlohmann_long_double_symptom | FAIL | eval/results/github_issue_nlohmann_long_double_symptom-20260914-011733 | write_apply,write_patch_oracle | none | 174s | 29 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | patch pass on current ABI; formal proof incomplete | make aggregate未虚升执行/行为；7056后验不回填proof |
| 1 | real_trace_h3_iofam_one_seat | FAIL | eval/results/real_trace_h3_iofam_one_seat-20260914-011733 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 334s | 49 | read=10,repo_map=0,list=0,trace=8,source_lens=0 | midloop=3,inv=3/0,fin_reject=0,unavail=0,prune=0 | fail in full; core IO calibers pass | 原regex措辞未匹配；另有真实样本范围误述与提示自冲突 |

## 1. H3：关键 IO 尺正确，样本不能当总体

原答案 `.codrax/output/20260914-012304.799-48935.md`、同名HTML/旁路；日志 `run-1.logs/codrax-20260914-011735-000-48935.log`。下列行号指原文件。

- md34的1.347ms是单请求issue→complete墙钟；md49/53–56给出四段S状态完成闭合等待0.782/1.027/1.238/1.337ms，合计≥4.384ms，保容量下界。md26没有用调度标记IO为0否定真实IO等待。1.347与1.337没有混同。
- md62–66把198列为全局请求、返回8条，并说明41.329 request·ms属于未返回请求聚合、非墙钟且不可相加；没有把198当目标线程请求数。机审原正则`(单次请求|设备端).*(墙钟|墙上|wall.clock)`未匹配“单次 IO 请求”标题和“单个请求”说明；保持原FAIL，不改case/oracle消红。
- **真实答案缺口**：md82将已显示的六条目标请求0.865–1.347ms写成没有样本限定的请求范围。原Trace987/1017行同设备12,80、RCVHS、扇区123784216、长度192，目标线程13762.800307发起、13762.800516完成，即0.209ms，证伪总体下界0.865。仅展示范围应明确限定，不能据此称完整答案通过。
- md64将精确未展示190条之和称“估算”，没有写190；md85汇总也弱化未展示范围。md91把返回容量导致的下界描述为“链遍历结果被裁剪”，本次并无wakeup/root/block因果视图查询支撑该具体原因。md41泄漏`request_residence`内部字段名。均见原模型发射log4045，不是系统改写或删答案。
- raw工具文本 `trace_query-44451904.txt:135–143` 已准确提供两墙钟及190条41.329；finalizer上下文log3723–3744、3758–3763、3932–3933继续保留同目标/窗口、四段下界和两个无独立闭合的请求。核心数据供给不缺；不能为上述模型误述加入正文关键词硬门或系统改写结论。
- 首次成文合法、零拒零patch，无图是有限统计问范围正确，不要求强造因果投影或选根因。默认旁路确实生成，`schema_version=2, root_causes=[], status=unavailable, reason_code=trace_root_cause_contract_not_active`；不是有效选择丢失。本例不能代验真正根因问的双轴/因果投影全部能力。

## 2. B1680/P1：系统把外部观测教成源码证据提交

1. H3日志1600/1688/2563三次read-without-emit提示要求把当前事实送`emit_evidence`；其中1689–1691甚至称未提交的事实后续不可见。三个探索路分别照做，1726跳过4项、1794跳过7项、2834跳过10项，合计21项均因外部运行时观测退回；随后三个`emit_investigation_complete`接受。不是JSON格式波动，也不是证据门应放行伪源码。
2. `read_file`真实生产形将外部行范围放在`RuntimeArtifactRead`并使`ReadCoverage=nil`；`runtimeArtifactReadWindowSince`却只读后者。原hint选择另只读analyzer来源合同；大Trace预分析跳过后，即便当前运行时事实已存在，也错走通用“所有事实必须emit”文本。
3. header旧提示还说外部工件有真实行号即可`emit_evidence`，同样与现行外部观测政策冲突。修复必须统一来源明确的软交接说明：当前源码锚走source evidence；日志/Trace/命令/VCS/外部观测走已有reason/aggregate通道。用生产者typed范围识别工件，只改变提示，不借此授source ReadCoverage、不改完成门/工具权限、不扩派生ref读取许可。
4. 当前已移交下一独立修复批B1680，需真实ReadFile→Observe→EmitEvidence公共RED及source/mixed/unknown/trace-json/无扩展/large-read反针。本回放本身不能签已修复，也不假定它是所有模型口径错误的唯一原因。

## 3. C++：双头补丁正确，程序诚实保留验证不足

基线 `a5deba74ef19028624930e664fb13d05eb9bf6d3` → applied `e4075553b457364dc453ec93de531cd8ea5d08b5`（对象在本例`run-1.repo`）。仅两头文件各一行`%.*lg`→`%.*Lg`，签名、精度、buffer和返回值保持，无窄化cast。Makefile、README、原测试未改。

- 正式报告 `plan-1789373976694985000-48941.report.json`只记录真实`make check`一次，exit0、1336ms、aggregate `make-test`一条。两个header路径声明覆盖保留，执行能力均为unknown；未铸两条模型命名的独立断言。原测试实际只main中检查strlen非零，不足以证明完整格式语义。
- 七个合同保持planning_only，规划/应用原日志未晋升硬约束或逐断言证据。模型finish请求all_verified，系统以真实proof_weak降到accept_unverified；没有replan/retest追绿。最终`run-1.out:130–134`说明1条验证结果、未完全验证且不等于代码失败。B1678获自然正证；不应通过再给Make聚合结果虚高能力来消掉此FAIL。
- 独立后验复用格式矩阵，对精确applied物化树检查C locale2352格，德语/法语各2352格（实际decimal_point为逗号），合计7056格零失败；覆盖两header、一般/科学计数、精度、±0、极值/次正规/Inf/NaN、截断与空buffer、返回计数和字节。原测试严格编译执行也通过。源树、formal report及final报告前后SHA相同，独立测试不回填程序proof。
- 收据 `.codrax/tmp/20260914-r1069-cpp-format-audit.log`、`20260914-r1069-cpp-locale-audit.log`，详情目录 `r1059-nlohmann-audit.Zk7wDg/delivery-run.aF6GBS` 和 `20260914-nlohmann-locale-run.V2uiUu`。本机double/long double都为8字节、53位；仅签当前ABI，不冒充扩展精度ABI测试，也不把薄printf包装的locale一致性称完整JSON locale策略。

## 4. 后续优先级与边界

P1先闭B1680提示自冲突，再审尚开放的嵌套schema包装完整性、图操作/关系供给和原生runner目标执行凭证。读样本范围/内部词汇作为模型质量观察，准确上下文已经存在，不单例硬拟合。保持已交付B1677/B1678，不倒签全系统闭环。

读24、写20次请求均attempt1，实际10分钟首响应/5分钟静默配置在日志中可核，无provider超时或固定无正文降级。仍为600/300/600s默认，4ms/旧4分钟无可见正文不是活跃流降级条件；1200s是eval外层预算，不是模型请求超时。无畸形JSON/图修复自然覆盖，不虚报本批全面验证这些能力。
