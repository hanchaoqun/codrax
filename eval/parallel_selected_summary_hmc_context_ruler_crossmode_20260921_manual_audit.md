# 跨查询显示尺修复后的双模式人工审计

- date: 2026-09-21T09:17:43Z
- sweep_start_ts: 20260921-021743
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_context_ruler_crossmode_20260921

固定生产版本cd2e029bce1b，干净构建2026-09-21T09:17:01Z。runner session73512正式exit0，2并行×1、无第三例追绿。机器2/2 PASS，人工功能正确性2/2 PASS；以下独立缺口仍开放，不倒签以前的FAIL。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/hmc_context_ruler_crossmode_20260921/patch_go_typo-20260921-021743 | write_apply,write_patch_oracle,answer_contains | none | 121s | 28 | read=3,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS | 单行修复及TestGreet真实通过；PTO身份教学缺口不代销 |
| 1 | real_trace_e1_dual_window_normalized | PASS | eval/results/hmc_context_ruler_crossmode_20260921/real_trace_e1_dual_window_normalized-20260921-021743 | log_regex,trace_attachment,answer_regex | perf_triage+trace_query | 134s | 42 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=0,inv=3/0,fin_reject=0,unavail=0,prune=0 | PASS | 双窗归一化正确；附录重复/记录数措辞留债，§76未直接live命中 |

## 1. 真实双窗Trace

[正文](results/hmc_context_ruler_crossmode_20260921/real_trace_e1_dual_window_normalized-20260921-021743/run-1.principal.md)和[完整答案](results/hmc_context_ruler_crossmode_20260921/real_trace_e1_dual_window_normalized-20260921-021743/run-1.answer-transcript.md)核对原生数据通过：A保留34579.472865–34579.475857，2.992ms = running0 + runnable0.014 + sleep2.978，CPU占比0%；B保留34579.475857–34579.505857，30ms = running3.414 + runnable0.780 + sleep25.806，CPU占比11.38%。CPU1/2/3/5分别1.971/0.702/0.245/0.496ms，与原生CPU明细相等；正文只说分布于四核，没有把单线程迁移说成并行运行。

“无D/IO等待记录”结合末段“机制不明”和系统明细的scheduler/S-IO限定，不表示已经排除Binder、完成唤醒或其它IO机制；状态归账完整没有升级成原因已经查明。此请求是bounded_fact_set事实比较，无因果树符合范围，不判投影丢失，也不算§76背景/邻近尺的live命中。必选schema2旁路实际存在：`.codrax/output/20260921-021955.059-22719.root-causes.json`，root_causes为空、unavailable、trace_root_cause_contract_not_active，没有凭背景造根因。最终md/answer-surfaces哈希绑定核验通过。

过程日志`run-1.logs/codrax-20260921-021746-000-22719.log`第545行首轮拒绝同时给scalar与time_windows且漏fact_families；308行已有互斥教学，572–577行成功保留两窗并补事实族，不是系统矛盾合同。9次模型TraceQuery加2次系统按请求窗补采，未读仓库源码；原生A/B值在1178–1204及最终上下文均可用。探索摘要1846/1854曾将B的CPU3 0.245ms借给A、误说多核并行，但最终typed CPU事实卡2827–2835和答案未采纳，不把中间错误倒灌成最终FAIL。成文一次，零成文拒绝。请求日志仍为首响应10分钟/静默5分钟；本轮无短墙时强制降级，但134秒成功不是10分钟边界实测。

### P2：来源记录数被说成不同范围数（未关闭）

完整答案39/41先列A/B，43/45又列A；47行称“另有7条独立范围”。实际最终上下文2767–2777共11条来源记录（A5/B6，只有两个窗口），7是len(states)-4而非不同范围数。代码在`internal/tool/answer_document_mutation_runtime_wait_coverage.go:364`；范围权威保留EvidenceID/sourceKey是为了避免跨结果借量，不应在权威层按窗口盲合并。

影响是附录重复占用四行预算及错误暗示额外不同范围，不污染本次正文数值。后续先把遗漏计数表达为状态观测记录数；若做紧凑合并，只能在显示层对相同内容保来源地折叠，异值、异范围、不同限制不能合并。不用本次PASS销此债。

## 2. Go单行修改

[计划](results/hmc_context_ruler_crossmode_20260921/patch_go_typo-20260921-021743/run-1.plan.json)和[实际报告](results/hmc_context_ruler_crossmode_20260921/patch_go_typo-20260921-021743/plan-1789982345307972000-22769.report.json)核验：真实差异仅main.go:25的retrun→return；未改测试、go.mod或README。fixture、主checkout、worktree与交付树main_test.go的SHA256均为7d92240e4daa7c9cbe0ad95b89e1d350e1dfe25eb891091c956f71a39265dfdf。

worktree内真实go test -json .退出0（996ms），runner收到assertion级TestGreet通过，原测试顺序覆盖空串、双空格、codrax三项。这是一个测试内的三个表项，不是三条独立runner收据；没有独立go build执行收据。原checkout仍为seed576f1697…，保留worktree和恢复引用指向acd9ac93…，未合并主分支。

首轮planner已收到“不用probe包装编译命令”的准确教学，仍以Go os/exec包装go build，1142–1144行被精确拒绝；随后1207–1233行读取原测试，1267行使用既存测试声明恢复。没有将测试文件塞入源码改动集合，最终输出诚实说明保留worktree、1条验证通过，且自然语言清单不等于逐条独立证明。

### 原生测试身份教学缺口（未关闭）

plan第40行声明PTO suite=main，实际report第9行suite=codrax.example/patch_typo_fixture，精确匹配不成立，报告没有伪造project_test_contract_ref_observed。三个behavior contracts全部planning-only，最终controller输入required=0，所以功能通过不等于遗漏required证明后冒签。

emit_change_plan.go:268和emit_plan_skeleton.go:137只教通用suite/class/module/file及Python例，未明确Go test JSON的Package是import path而非源码package main；模型日志1253行正因package main选错。run_tests_parsers.go:588–593按真实Package/ImportPath发布，write_behavior_contract_retirement.go:261–275维持精确匹配，后者不应放宽。后续应统一各runner可填写身份与模型可见原生证据，不能用Go个例字符串替换硬补；B2–B6仍开放。
