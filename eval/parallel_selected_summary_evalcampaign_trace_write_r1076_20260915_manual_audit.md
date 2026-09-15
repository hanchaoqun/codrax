# r1076 人工审计：真实双窗 Trace 与 TypeScript 多仓写修复

- date: 2026-09-15T10:05:07Z
- sweep_start_ts: 20260915-030507
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

清洁构建 `fefbcfbef374`，构建时间2026-09-15T10:04:39Z；B1699 `0995bf36d`、B1700 `fefbcfbef` 已推送。CAP5/PARALLEL2/TIMEOUT1200，两例各一次，未重跑或增加第三例。runner exit0仅表示两路完成；原机器汇总保持1PASS/1FAIL，不以后验改判。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_e1_dual_window_normalized | PASS | eval/results/real_trace_e1_dual_window_normalized-20260915-030508 | log_regex,trace_attachment,answer_regex | perf_triage+trace_query | 231s | 39 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=4/2,fin_reject=0,unavail=0,prune=0 | fail（测量及主对比正确，解释错误） | 两窗/Running/归一占比正确；零基准∞倍与完全睡眠错误；系统重复状态行及错域源码枚举提示待单独修复 |
| 2 | github_issue_memoclaw_text_search_multirepo_ts | FAIL | eval/results/github_issue_memoclaw_text_search_multirepo_ts-20260915-030508 | log_regex,write_apply,write_patch_oracle | none | 309s | 28 | read=14,repo_map=3,list=1,trace=0,source_lens=0 | midloop=3,inv=0/0,fin_reject=0,unavail=2,prune=0 | 交付后验pass；正式proof未闭 | 真实TS导入6/6后验；两次probe unavailable，第二次合法JS与系统包装变量冲突；准确报错未交接 |

## Trace：正确测量不等于正确解释

- 日志L=`eval/results/real_trace_e1_dual_window_normalized-20260915-030508/run-1.logs/codrax-20260915-030509-000-8831.log`；答案A=`.codrax/output/20260915-030856.319-8831.md`，同名前缀HTML/侧车保留。7次有界、pid筛选的window_stats，未读仓库，首次成文0拒绝/0patch，无JSON降级，无Mermaid（不需要强造图）。机器ana=2对应两次stage分析；全日志实际4次emit_analysis，前次stage内有2次工具拒绝，第三次提交后另因subtopic_coherence质量门失败，第二次stage首提交成功；不能漏读后半日志。
- 原始trace逐sched_switch裁窗独立核算：A=2992µs、Running0；B=30000µs、Running3414µs/19片=11.38%。L1848/1863附近的真实工具结果及最终上下文L3360–3363同值；B CPU1/2/3/5分别1.971/0.702/0.245/0.496ms，未发生用户曾反馈的Running CPU证据缺失。本例不需要据此推断算力不足。
- A21–23/32–34正确给出Running、Runnable、Sleep及分母。A26/52的“完全睡眠”与自己列出的0.014ms Runnable矛盾：原trace3213行34579.475843唤醒，3221行34579.475857切入Running，短窗最后14µs已就绪。A41/46/75以零为基准作“∞倍”不构成有效倍数比较；已有正确窗口占比，可比较增加11.38个百分点，不能将除零当可靠归一化结论。A60把占比约5.5倍简写成延迟5.5倍亦易混淆实际毫秒口径。原探索reason已带∞/完全睡眠倾向，最终模型自行写出，系统未代写这些结论，不增加答案原文扫描硬门。
- A50/75及系统明细保留“调度器标记口径为零不排除其他IO”边界；未把邻近工作或零Running铸链上根因。`root-causes.json`真实产生schema2、空数组、`unavailable/trace_root_cause_contract_not_active`。本题是两个窗口的状态/占比对比，修正后模型scope=bounded_fact_set、frame_causality_requested=false，因此无Trace因果投影不属于丢失；状态补齐存在且两窗分别列明。未调用链/根因视图，不代销全链双轴或因果自动补齐专项。
- JSON/教学审计：L550首拒为artifact_value_profile=true却无value；L577次拒为runtime target无pid/thread及relation_analysis与fact_families冲突。L615自行补pid/thread、改bounded_fact_set，保两个ordered time_windows。L1967完成拒绝来自把持续时间放入member_set的计数值（另一个调用亦因同类载荷拒绝）；不是合法JSON被解析失败或同声明必带必拒。日志schema/提示事先有正确字段要求，单次语义误分类先记模型教学负担观察，不为本case增加硬规则。
- 来源边界及被撤回的中途判断：L615初次stage虽发exclude，但正确原句被放进artifact_citation_quotes，L616–617回到default/optional；该stage随后L656–661因组合实体com.baidu.tieba 59566与拆开的primary entities不共享精确元素而失败。第二次stage L1168正确提交专用current_source_exclusion_quote，L1172明确exclude；最终L3202–3203源码excluded、required=false、source_records=0，因此本轮有真正source-excluded保持正证。不能将首个失败stage的optional误当最终状态；也不能把它扩大为B1699混合源码义务已自然命中的正证。组合/拆分实体的质量门适用性另作观察，尚不宣称第二个必带必拒实例。
- 两个系统追加面的独立问题：A67/69与71/73完全重复同两窗状态，L3360–3363已在模型上下文重复；A85–87把运行时表标签accounted_total当仓库声明做核对，属于错域软提示，不是模型答案原文。本批保存并单独审计，不用字符串后处理删除原报告。

## TypeScript：正式验证与人工后验分开

- 结果根目录 `eval/results/github_issue_memoclaw_text_search_multirepo_ts-20260915-030508`。runner309s，工件内运行耗时305s，口径分开。实际交付HEAD=`bf3e6ea24e4c5caf94a36b5734dfdf14b58d4137`，仅src/client.ts修改11增7删；最终两份计划只补验证，交付归属正确追溯首个源码计划。原12个fixture文件与父checkout一致，交付内API/Makefile/package/原测试4文件未改。
- 独立后验直接用Node24.19.0的`--experimental-transform-types`导入真实交付src/client.ts，在内存中替换fetch，不访问网络。POST精确路径、JSON query、默认10/显式3/显式0 limit、namespace有无、响应对象及fetch/json异常传播6/6通过；原fixture同脚本2/6通过、4条路由失败。脚本 `.codrax/tmp/20260915-memoclaw-ts-native-postcheck.mjs`；结果 `20260915-r1076-memoclaw-ts-delivery-native-postcheck.log`、负基线 `20260915-memoclaw-ts-native-original-red.log`。这是人工补充检查，不是类型检查、不回填正式proof、不更改原机器FAIL。
- 正式3份report、3次make check、2次失败probe；`project_runner_commands=5`含两条suite_continued元数据，不等于五次套件执行。Make只运行Python源码扫描；首计划自报的test_search并不存在，未获真实assertion授权。三代hard=0/soft=0/planning=3，未通过replan删除既有hard义务；早期MemoryClient.textSearch未验证项一直留在累计账。模型提前声称all_verified时，日志L3262纠正为实际验证；最终run-1.out:246明确“未完全验证”，正式unverified/verification_proof_incomplete判定合理。
- **B1701/P1，JS探测包装器词法污染**：按两份正式ProbeExecution的executable/args/原probe.code/原worktree只读重放，第一probe精确错误为Node strip-only不支持TypeScript构造器parameter property；第二probe为`Identifier 'fs' has already been declared`。后者不是模型合法const fs语法错，而是`run_tests_verification_probe.go:131–149`系统顶层fs/vm/resultPath/encoded/writeResult与vm.runInThisContext共用词法域。修向是隔离包装器实现绑定，保cwd/require/argv/真实结果收据，不能禁模型变量名或只给fs改名追case。
- **B1702/P1，probe不可执行的错误上下文丢失**：执行层1125–1142捕获完整stderr，1217–1274形成FailureDetail；但288只为tests_failed保辅助观察。run_tests.go的后续静态套件通过时，仅携带一般diagnostics/commands，未合入unavailable的结果行；短StoreBlob返回的inline被忽略，且passed报告不强制持久化、不渲染失败行。后续模型只见泛化SyntaxError，缺准确原因和可回读引用。须用真实本次plan/probe/execution绑定的非权威诊断保有界原始错误及持久引用，不改变Passed、unavailable分类、assertion/proof资格；B1122已修的native失败lane不因此被冒称回归。
- 两次人工原执行重放收据 `.codrax/tmp/20260915-r1076-typescript-probe-receipt-native-replay.log`。仅RESULT置空以避免写正式收据，其他executable/args/code/cwd相同；后验前后实际源码/正式report不变。第二probe还退化成源码字符串扫描，即使包装器修好也不能冒充业务行为验证，不用系统替模型补造验证结论。

## 优先级及既有债对账

先修B1701（确定性合法代码误拒），再B1702（所有语言的诊断交接）；本对不追绿重跑。Trace两项P2并入现有债：等价pid/thread选择器的同窗状态重复属于B1626 §1782.6显示压缩未闭，须保不同QueryScopeID/MeasurementOrigins的来源隔离，仅压缩已证显示等价项；错域枚举提示并入EVAL-B36-SYSAUTH1，外部A/B两bucket仅授权比较，不该借`HasAnyObligation`取得源码声明核对资格。不能删除整个合法目标状态补齐、不能事后改写模型表。

## 工件保全与验收边界

- 原case/trace/仓库fixture前后SHA均一致：`.codrax/tmp/20260915-r1076-cases-trace-before.sha`、`20260915-r1076-repo-fixtures-before.sha`。
- Trace机汇总/MD/HTML/侧车/原日志/metrics/verdict冻结见 `.codrax/tmp/20260915-r1076-trace-output-machine.sha`。清单只覆盖所列文件，不声称哈希了所有嵌套缓存。
- B1699/B1700公共先红后绿、专项和最终冻结86包全测在统一账本§123.1813–1814；本对不冒充原r1075混合问题已自然复现回放，也不关闭B1694/完整图表达/写证明强度债。
- 默认600s首响应/300s真实字节静默/600s非流式不变。L491等analyzer每阶段3m预算按active-adapter仅作用非流分支；流式字节持续活动不受该旧年龄预算截断（`agent.go:2482–2494`及4ms混合fallback回归），显式调用方cancel/deadline与eval1200s总预算独立。
