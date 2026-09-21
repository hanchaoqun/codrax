# HMC 时间线等待清单交付：固定双例人工审计

- date: 2026-09-21T13:29:01Z
- sweep_start_ts: 20260921-062859
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_timeline_wait_delivery_20260921

冻结代码 `22b55f6d9`，构建 revision `22b55f6d9d29` / `2026-09-21T13:28:26Z`；runner 88217 正式 exit 0。2 并行 × 1，无第三次追绿。机器 2/2，完整人工 1/2。新增无窗 `thread_timeline` 路径本批没有现场命中，以公开回归验收，不拿本次 live 代签。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/hmc_timeline_wait_delivery_20260921/patch_go_typo-20260921-062901 | write_apply,write_patch_oracle,answer_contains | none | 86s | 28 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS | 真实单行修复、原测试一条原生断言通过；非 required/PTO 验收，不销 B2 |
| 1 | real_trace_c2_dstate_iowait | PASS | eval/results/hmc_timeline_wait_delivery_20260921/real_trace_c2_dstate_iowait-20260921-062901 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 135s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 三段 D 侧 IO/0.635ms 已正确；主线程误分类无命名目标、全域问题被窄窗代答、事件执行者/时点/调用点语义仍误述 |

## 1. C2：量与原生 D 分类改善，不抵销完整人工失败

结果目录 `eval/results/hmc_timeline_wait_delivery_20260921/real_trace_c2_dstate_iowait-20260921-062901`。以 `run-1.principal.md`、`run-1.logs/codrax-20260921-062904-000-46661.log` 和原始 `donghu_tieba_frame.systrace` 逐项核对。

- 三段开始/结束/时长正确：34579.451701–34579.451839 / 0.138ms，34579.452934–34579.453081 / 0.147ms，34579.471372–34579.471722 / 0.350ms；合计 0.635ms，均原生 D 且 iowait=1，非 IO D 为零。本次不再错误否定 D，也未据低占比声称影响可忽略。
- 实际 4 次 trace_query：日志924行 event_search、970行有界 window_stats、971/972行两次 event_search。没有 thread_timeline，故未现场覆盖本片新入口。没有源码工具或成文拒绝；一次 analyzer 拒绝（553–584行）是模型首版 bounded_fact_set 与 required target_effect_verdict 冲突，后重发一致元组，不能说零重试，也未证系统自相矛盾。
- 用户明确说“com.baidu.tieba 59566 主线程”，accepted 分析却给 no_named_target / 空 runtime_targets，理由为“进程整体……不特指某个子线程”（584行）。1127行补齐因此正确跳过 no_typed_target。这是目标语义误分类，不能用 Entities 或原文扫描越权重授根目标；是否有系统教学诱因另审。
- 用户要求“这份 trace”全工件，模型自行选 34579.450000–34579.595000 的 145ms 窗；原生实际边界 34579.450627–34579.595184。最终 Running 52.294ms 是窄窗值，完整窗应为 52.478ms，尾部 0.184ms 未覆盖；窗口前缀 0.627ms 本无观测。D/IO 三段恰好都在该窄窗，正确的 0.635ms 不能证明全域状态账交付齐全。最终输入有全工件搜索覆盖，不等于局部 window_stats 得到全域计量权限。
- 最终称前两段“由 hilogd.pst 捕获”、第三段“由 SaInit0 捕获”。原始90/225/2427行这些名字来自 sched_switch 行头/next_comm，分别是切入的另一个线程，不证明它们是等待捕获者或唤醒者。原始117/250附近及2532行的真实唤醒事件来自 IRQ，不能拿切换行头替代。
- 最终称“每次进入阻塞时……记录了调用栈”。三个 sched_blocked_reason 实际记录在唤醒附近（118/2533等行），其中一个 caller 只是阻塞调用点，不是完整调用栈。仅 iowait=1 也不能单独确定磁盘/文件系统设备机制；可保 IO 等待，不据函数名或相邻事件补造具体机理。
- 默认旁路 `.codrax/output/20260921-063114.538-46661.root-causes.json` 实际存在，schema_version=2、root_causes=[]、status=unavailable、reason=trace_root_cause_contract_not_active。此有限状态问题没有要求选根，空根因旁路合理，不是落盘失败；本例无需强制因果投影。

下一步先审实际目标/问题范围分类教学与 coherent causal/finite 的公开交接；不放宽已接受 typed 合同、自动恢复完成散文为权威、增加关键词硬门或为本例改答案。

## 2. Go 写模式：独立人工 PASS

结果目录 `eval/results/hmc_timeline_wait_delivery_20260921/patch_go_typo-20260921-062901`。机器用时83s（批次封装86s）、exit0；独立审计计划、实际 diff、执行日志及最终报告。

- 实际仅 main.go:25 的 retrun→return，1增1删，通过 git_apply 落地。fixture、seed、worktree 的 main_test.go SHA256 都为 `7d92240e4daa7c9cbe0ad95b89e1d350e1dfe25eb891091c956f71a39265dfdf`，原测试未改。
- 隔离 worktree 真实执行一次 `go test -json ./...`（apply日志642–645行），exit0。原生 assertion suite=`codrax.example/patch_typo_fixture`、ID=`TestGreet`；同一个测试覆盖空串、空白、codrax三输入，不能数作三个原生断言。
- hard-required=0、soft-required=0，三项 planning-only；无 PTO/probe/source-free。最终 complete/verified 与真实 project runner 一致，并披露自然语言验收不等于逐项独立执行证明。一次计划/apply/run_tests，无 replan 或验证重试；本例不证明 required/PTO 修复，不销 B2。
- seed HEAD 仍 `e52552e27288a4ca1bf817bf8666ffee5496607d`，交付commit `5e28ee116bfdd7635ac8007eb6d065f5cad89dc3` 留隔离树、未自动合主干。seed tracked 文件未改，只有运行期未跟踪 `.gitignore`，不宣称整个目录字节相同。
- 非阻塞文字瑕疵：计划说“其余37行”，文件实际总共37行；实际修复范围无误。

## 3. 状态分账

本片新时间线清单交付以公开 RED/GREEN、race 和末版全仓单独验收；本次机器2/2不替旧人工FAIL重签。C2旧D误述本次改善，其余目标分类、完整计量范围与事件语义仍开放；HMC父账13/79、66开放及B2–B6保持。
