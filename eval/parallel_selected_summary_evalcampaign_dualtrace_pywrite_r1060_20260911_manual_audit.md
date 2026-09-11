# r1060 人工审计：独立双窗 Trace / 跨仓 Python 同步异步写

- date: 2026-09-11T08:17:17Z
- sweep_start_ts: 20260911-011717
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

产品基线为已推送 `main@e622c8264`；提交后 make 通过，二进制 revision=`e622c82641ed`，built=`2026-09-11T08:16:52Z`。08:17:17Z–08:20:14Z 严格两路各一次，原 case/oracle/1200s 不变，未新增第三路、追跑或改预期。产品/测试持续冻结，没有修改生成答案、正式报告或应用补丁。机器 2/2 PASS 不替代人工判断。

库存 243 例（read 215 / apply 25 / plan 3）；按客户影响、来源/范围权威风险、异构覆盖与近期覆盖老化排序，选择上次 r1040 的双窗 Trace 和上次 r1026 的 Python 写，不继续围绕刚跑过的 H9、仓颉、C++ 写和跨语言读追绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_e1_dual_window_normalized | PASS | eval/results/real_trace_e1_dual_window_normalized-20260911-011717 | log_regex,trace_attachment,answer_regex | perf_triage+trace_query | 157s | 37 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | **fail** | 原始数值正确；B1626 请求成员窗身份仍错；模型另有零基数倍数错误 |
| 2 | github_issue_memoclaw_text_search_multirepo_py | PASS | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260911-011717 | log_regex,write_apply,write_patch_oracle | none | 177s | 28 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | **patch pass / verification authority fail** | 补丁及48组独立后验正确；正式探针仅import+源码断言，却获target_behavior |

## 1. Trace：数值已分账，请求窗口身份仍被压成包络

用户只问同一线程在两个独立窗口获得 CPU 的归一化对比，不问帧根因，也不要求工作与目标的关系。最终原文 `.codrax/output/20260911-011952.152-80993.md`（HTML 同 stem）。人工复算：

| 窗口 | 时间范围 s | 窗长 ms | running ms | runnable ms | sleep ms | running / 窗长 |
|---|---|---:|---:|---:|---:|---:|
| A | 34579.472865..34579.475857 | 2.992 | 0.000 | 0.014 | 2.978 | 0.00% |
| B | 34579.475857..34579.505857 | 30.000 | 3.414 | 0.780 | 25.806 | 11.38% |

B 的 CPU1/2/3/5 分别 1.971/0.702/0.245/0.496ms，合计3.414ms。两窗值及分母正确，不能误归为统计值串窗。

### B1626 / P1：第二次同题生产见证，仍未实现

以下Trace日志坐标均为原文件 `run-1.logs/codrax-20260911-011718-000-80993.log` 物理行；合并的 `run-1.logs.all.log` 带额外文件头，不能直接沿用其行号：

1. L634 接受的 analyzer 只有单个 `explicit_time_window`，起止是32.992ms包络；source_quote及子题仍含A/B。
2. L1239–1240模型query使用准确A/B，结果`2b4a90df` / `d6890d7b`分账正确，模型已找到实际窗口。
3. L1798系统补采却用包络；L2440–2452、L2583成文上下文仍将包络视为requested。
4. 最终MD L44标包络为“请求主范围”，L46/48把真实A/B标“探索子范围”。这是系统生成的范围身份错误，不是模型自己合并数值。

B1626已在r1040立案，本批未声称已修。根修须共享typed多窗口成员域，贯穿analyzer/schema/教学、请求关联、自动补采、上下文及显示；包络最多为参考聚合，不能替代请求成员。禁止扫描用户数字补窗、挑其中一窗覆盖另一窗，或只改标签掩盖缺失身份。

### 模型问题：保原文，软引导，不新增算术/答案硬门

- MD L19把0%→11.4%说成“114倍”，L37又写“∞倍”。零基数比值无定义；可比较约11.38个百分点，不能由比例倒数生成倍数。
- L17先称“完全处于可中断睡眠”，又列runnable=0.014ms，“几乎”与“全部”混用。
- L21/50从闭合Binder等待0、标记D/IO0，扩大到未发现内核阻塞；未配对Binder计数A/B为1/10，未评估机制也不能由此排除。系统已有口径说明，模型概括仍超过证据。
- 上述summary出自模型首次emit（L2672），patch（L2728）的summary.text字节不变。无系统替换答案；不恢复退役正文算术评价器，不扫描模型文本强行改结论。

### 合同、上下文及能力边界

- L634的`runtime_work_relation_requested=true`是模型选择，并非系统按关键词强开。L339已教仅独立问工作与目标关系时才选；本题并未问。最终模型在提示后选VerifyClass0.285ms额外关系行（L2728），明确relation_unproven；未提升为链上根因，但无助核心问题。留模型路由/展示观察，未证新“同时必带必拒”。
- 初次analyzer缺必填predicates（L551–552）被拒与schema一致，无JSON教学自冲突证据。模型8次query是6成功+2路径失败（L1297/1300），经attached_trace指路恢复；另有1条系统补采。
- “不分析代码”错放artifact citation字段有提示，但实际代码read/repo_map/list均0，不能混成真正越界读取。
- 有限状态对比无需强套Trace因果投影；默认旁路同stem `.root-causes.json`确有131B，schema_version=2、root_causes=[]、status=unavailable、reason_code=`trace_root_cause_contract_not_active`。不是未生成，也不应造根因填空。
- 最终无Mermaid，图语法/渲染N/A；此例不代替显式根因问题的投影生产验收。无成文拒绝循环和4ms/旧4m活跃流年龄降级；较短运行不能当新长时SSE验收。

## 2. Python写：补丁正确不等于正式行为证明足够

### 交付及独立后验

- 计划`plan-1789114773208931000-81011`，durable commit `98166377b9e4e725562f7a65a6fbf976e9f7cd76`，ref `refs/codrax/applied/plan-1789114773208931000-81011`。
- 工作树`run-1.parent/.codrax/worktrees/trace-1789114773374523000-81216`干净，唯一diff `memoclaw/client.py` +8/-9。
- 仓内`python-sdk/API_REFERENCE.md:5–7`及api-docs要求POST `/v1/search`、JSON query/limit/可选namespace。两方法均已改；原签名、namespace条件、结果透传和真实await保留，删不用的urlencode。
- 独立冷审逐项核原fixture/run parent与交付：测试、Makefile、API及TypeScript源码SHA不变，无跨仓改动、弱化测试或主仓自动合并。
- 根代理独立后验脚本`.codrax/tmp/r1060_python_behavior_audit.py`加载最终client.py，用mock同步transport及真正await的异步transport，覆盖2执行形×2base URL尾斜杠×2query（空/Unicode特殊字符）×2limit（0/10）×3namespace（None/空/Unicode）=48组，**48/48通过**；逐组核恰一次调用、完整method/URL/JSON和返回对象身份，无网络/产品目录写入。
- 收据`.codrax/tmp/r1060-python-behavior-audit.json`明确`independent_post_run_not_product_proof`。后验不倒填正式报告、PTO、case oracle或运行时证明；另一冷审者未运行测试。

### B1575/B1561 / P1：执行粒度能力债新见证，不另造重复工单

初始日志坐标指 `run-1.logs/codrax-20260911-011718-000-81011.log`；应用阶段坐标指 `run-1.logs/codrax-20260911-011933-000-81216.log`，不是合并日志行号。

1. 初始日志L2331纯AST/source探针因目标未耦合拒绝；L2352加入`from memoclaw.client import MemoryClient, AsyncMemoryClient`及变更引用后，其验证仍只是AST/源码检查，没有实例化/text_search调用/transport/await；L2361接受。第二版也将原print+sys.exit失败表达改为AssertionError，不将“只加import”当逐字diff断言；身份≠执行的结论不变。
2. 应用阶段日志L710正式探针50.19ms成功，真实含义是探针进程通过。L712/848明确make为`probe_primary_suite_skipped`，并非make实际通过。
3. `plan-...report.json:104`却为变更路径发`target_behavior`。正式报告正确保留project-test observation missing；不是Make跳过被伪造PTO。最终proof ledger以同ref探针来源协调covered，须修探针授权源头，不误修JUnit scope。
4. `internal/tool/change_plan_validate.go:1167/1248`只核Python import身份Covers；`run_tests_changed_path_coverage.go:353–354`由probe通过+ContractRefs非空即升target_behavior；`run_tests.go:3494`起按同一目标身份耦合把合同refs记covered、发satisfied confidence。身份绑定不等于实际执行，更不等于逐方法行为观测。
5. 最终仍诚实列required_typed_contracts=0、planning_only=9，B1575“整段probe通过不等于逐合同/逐方法独立执行”披露仍在，故不记披露丢失或九硬合同全执行。但说明无法抵销target_behavior及其对完成权威的影响。模型finish还称原`tests/check_search_client.py`断言通过，超过实际执行事实，不能反向补造测试记录。

这是B1575在§123.1642明确未关闭的executor-owned per-ref能力债，兼挂B1561能力感知恢复，不是B1664回归。下一批须区分探针进程成功、目标身份绑定、实际执行/行为观测三种事实，共享决定coverage/confidence/终验；不能拿import、非空ContractRefs或源码关键词当执行凭证，不能只加一句提示便称根修。

## 3. 后续批次与不变量

1. **P1 B1575/B1561：** 先公开RunTests入口复现AST-only import获行为权限，对照同步/真实await正控；盘点执行器实际记录与义务证明。未知来源不升级，但不能全量拒绝合法源码形状测试、模块初始化或旧载体；需要新协议的部分单列设计/施工。
2. **P1 B1626：** 单一请求成员窗身份贯穿全部消费点，同批保护单窗、双窗不相邻/重叠、不同目标/来源、补采与展示。包络不得获成员权限，不改用户请求或模型答案。
3. **现存队列保留：** B1651b Gradle旧XML来源、B1662 Meson原生日志仍待工具链前提/原生验收；B1664未覆盖主体语法不是全语言已证；B1561恢复教学及其它范围/图关系债不被机器2/2销账。
4. **模型波动不追拟：** 零基数倍数、过宽概括、额外业务关系行留原见证；只供精确信息/软教学，不增正文硬门，不代写结论/补图。
5. **Trace及流式红线：** 保显式窗、探索+补齐、已证链上根因及占用/可消除两轴、IO/D/语义/业务线索，邻近/背景只支持；活跃SSE不因4ms/旧4m无最终正文降级，调用者取消/显式deadline/真停滞仍有效。

状态：`machine=2/2-pass`；`human=trace-fail,write-patch-pass-but-verification-authority-fail`；`B1664=pushed/validated/not-production-exercised-by-r1060`；`B1626/B1575-execution-capability=open`。不把审计结论冒称已实施。
