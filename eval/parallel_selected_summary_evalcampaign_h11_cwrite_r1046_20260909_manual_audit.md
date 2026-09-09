# r1046 人工审计：明确窗 Trace 根因 / C 实际写入

- date: 2026-09-09T08:06:57Z
- sweep_start_ts: 20260909-010657
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

基线为已推送 `8ddcee9b6`，clean binary `0.1.20260909`，built `2026-09-09T08:06:26Z`。严格两路各一次，不改case/oracle。机器2/2通过不等于答案质量全过：C实际交付/行为通过，Trace正文人工失败。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_c_typo | PASS | eval/results/patch_c_typo-20260909-010657 | write_apply,write_patch_oracle,answer_contains | none | 108s | 28 | read=4,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass（交付/原生行为） | 仅main.c一行；正式证明保aggregate，计划摘要反写为模型措辞残余 |
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260909-010657 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 202s | 58 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail（正文） | 两轴/投影/目标频率通道正向；正文单位/小计/主体/独立性错误，不能以机评分销账 |

## 1. 选择依据及过程

当前243例（215 read、25 apply、3 plan）按影响/风险、覆盖老化、异构语言与可执行性轮换。H11是同Donghu捕获的明确窗因果/修向题，不是再刷H4频率状态题，也不冒称新数据集；C单行apply用原生工具链作写控制，不代表复杂跨计划proof债已闭环。保持read15步、apply24步及每例1200s外层预算，没有第三路LLM。

Trace原log `run-1.logs/codrax-20260909-010659-000-21290.log`；完整4次trace_query均同13762.791708..13763.024898，frame bundle/root rank/wakeup/window stats。没有源码读取；分析因同时声明causal scope与fact_families被已存在合同拒绝1次，纠正后通过。一次成文+一次模型自选summary结构patch，0成文拒绝；patch对字符串包裹的额外`root_causes="[]"`作数组形恢复，然后将错误外层`schema_version="2"/root_causes`隔离，无重试、无图格式修复。上下文估计116530/200000（58%），未触预算耗尽。模型原文未被系统替换。

## 2. Trace：保留能力与真实残余分开

最终工件 `.codrax/output/20260909-011017.152-21290.md`（121401B）及HTML（3266467B）。明确233.190ms窗；四态157.248/5.604/70.338/0ms、运行真实占时与规则折算58.320ms分别成账。投影1份，依赖链、优先级反转/runnable/供给、D/IO、业务span族与JIT关系未证边界均在。目标自身47段、12.658ms为完成闭合的响应阻塞等待，不是请求驻留或调度器iowait标记；状态标记IO=0不否定另一口径的这47段。邻近/背景在系统投影中未升级主因。

**B1633a生产正证（交接，不是模型完整利用）**：4个真实query文本均发布目标CPU7 11.030ms/1280000kHz，以及1/2/4/8/12/13各自正值；CPU3无值仍未知。完整目标账户不从Top8/Top2借行。最终上下文包含新专用代表频率、原始口径及同来源配对；本题未要求逐核完整复述，正文没有列CPU7不判再次掉数据。跨捕获隔离、双榜外与无policy臂有实际入口回归，不冒称本轮单捕获live覆盖那些负例。

**正文不能签绿**（均见最终第15–70行，不改写原产物）：

- 无精确关系却反复写“各方向相互独立”，甚至推出提升一个不削减另一个。实际finalizer log2700–2714已给`unlisted_pairs/physical_relation=unresolved/independence=not_authorized`及自然语言解释；缺关系不是独立性证明。
- `12.115ms`只是CompThread/JankManager两席精确小计，正文多次写整个锁/优先级方向合计；另两席仍有未解关系。模型把目标两项IO 12.658+3.602并为16.26、把跨线程值相加，无对应合计凭证。log2703–2706完整列headline_member_refs、额外成员及小计权限，不能判为系统未给归属。
- `3.956+1.648+1.193`算成5.797（逐值算术应为6.797，但**这不授权把6.797发成收益总和**）；keva/binder多处3.387/3.367及主体借用也不一致。精确主体与原值已在rank/目标账中。
- 策略`558000–2100000 kHz`写成`558–2100 kHz`，同时又写558MHz；把代表运行段起点值写成较强的“当时刻/该运行段频率”。实际新carrier及policy上下文值/单位正确，且明确不得升全程/驻留/目标绑定。
- 可见`leader`、`completion_closed`、`issuer`、`typed overlap`等内部词；链上业务span虽在系统两轴表中，模型自身的业务修向总结仍弱。

本轮能定位为模型首稿已经出现且最终保留的成文错误；**不足以证明随机性频率，也未确认新的供给缺失或系统合同自冲突**。已有精确信息/小计边界/单位教学入模，不追加同题关键词门、不由系统修总和/改结论。保人工失败与后续异构观察，避免为单例再堆重复提示。

默认 `.root-causes.json` 139B存在：`status=unavailable/reason_code=valid_model_root_cause_selection_unavailable/root_causes=[]`。本轮模型未提交有效选择；错误外层数组也只是空数组，并没有被系统丢掉的有效candidate选择。正确嵌套原生JSON对象示例已在log1917–1918。不能把旁路空读成没有根因；投影与默认必选旁路均保留，系统没有代选/代填。模型summary补充了一个观测JIT的关系未证判断，原summary文本逐字保留、另五块未改，没有将该JIT升根因。

## 3. C：真实交付、正式证明、人工补验

正式保留worktree为 `run-1.repo/.codrax/worktrees/trace-1788941293315538000-21412`。只将`main.c:19`的`retrun buf;`改为`return buf;`，1删1增；Makefile未改，主fixture与运行repo基线HEAD/两个文件均未改。规划前3次有字符串包JSON、臆造Makefile附带编辑、Python包装C探针，分别被精确结构/old_text/语言门拒绝，第四次正确计划成功；已有JSON与C原生验证教学明确，不据这些错误认定系统自冲突。

系统实际只跑一次`make test`，1034ms/exit0：原生编译+无参/指定名两次退出成功，报告保持aggregate，没有伪称每个自然语言条款有独立行为证明。最终UI也披露该边界。人工在保留树另跑`make -B test`及无参、`codrax`、单空串、`Alice '' Bob`四组，输出逐字/换行正确、stderr空、exit0；这部分仅工具返回实测，未另存日志或反写正式report，不能算系统原生proof增量。

已接受plan summary反写“retrun被误写为return”，但request/rationale/实际patch/最终目标正确，记P2模型措辞残余。运行repo有早期`.gitignore`，worktree有编译输出`main`，后者正式UI已披露未纳入提交/未自动删除；不宣称全部目录洁净。

主fixture SHA256：main.c `ec5feea61b49d32cf1bdb657f30016edeb4fa0fbd825906fd33d8da9c72edd8d`，Makefile `94569e0763292d58ff6f745f3ee9062bd6ad40bd7cd7504e00d7e194db5fef26`；交付main.c `57704f8f2071b8d1ae4eeac7c6ae53159e264c35438d634074217f599c81e65d`。证据规划log1174–1277/1562、应用log648–654/767–769/881–900及`plan-1788941292591973000-21295.{report,final}.json`。

## 4. 收账与后续

本轮小于4分钟，不作为超过4分钟活跃流实测；4ms/旧总时长帽与真实停滞/取消/deadline已有本批全仓和专项回归。**B1633b没有最终可见live正证**：CPU4/12符合既有纯选择器，但前置对账/拓扑/线程项占满原8项预算，频率2项在“另有2项”中省略；工程公开入口红绿保持，不能扩大cap为了固定case命中。B1632中性覆盖附注本轮未命中，仍待异构观察。

下一确定性高ROI仍是B1624b纯导航谱系：派生grep/read结果只保存原query回程提示，不授予读取/grounding/streaming豁免；失败、冲突、跨代旧来源保持未知。B1629b范围、B1626多请求窗、B1622物理次数/统计组及B1616b/B1561写验证债继续在统一账本，未混号/销账。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
