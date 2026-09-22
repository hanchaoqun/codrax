# 等待状态标签固定双例人工审计

- date: 2026-09-22T01:31:03Z
- sweep_start_ts: 20260921-183101
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_wait_bucket_20260921

冻结源码4c47d56a1，干净构建revision4c47d56a1ec4（2026-09-22T01:30:15Z）。runner8714正式exit0；机器1/2、完整人工0/2。主审阅读完整最终Markdown、schema2旁路、工具事实和最终模型输入，不以系统附录修正正文为PASS。未追跑第三例、未改oracle。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | eval/results/hmc_wait_bucket_20260921/real_trace_h2_dstate_dma_fence_triform-20260921-183103 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 110s | 49 | read=4,repo_map=0,list=0,trace=3,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=1 | fail | 调用点提升资源/进程，未验证Binder零值被当排除证明 |
| 1 | real_trace_g1_english_dstate | PASS | eval/results/hmc_wait_bucket_20260921/real_trace_g1_english_dstate-20260921-183103 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 159s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 原生三段D开段IO，被模型说成S且否认D |

## G1：英文整份工件D/IO清单

最终报告`.codrax/output/20260921-183340.576-13832.md`；对应结果目录下`run-1.logs/codrax-20260921-183106-000-13832.log`。

通过：完整范围34579.450627..34579.595184，三次IO等待0.138/0.147/0.350ms，合计0.635ms，完整调用点和逐段时间都在。系统附录非IO D计数0、IO计数3、S侧IO计数0，明确两层语义。有限问题无强加因果投影，schema2空数组/trace_root_cause_contract_not_active符合权限。接受语言为zh，case不固定答复语言，中文不是本次FAIL依据。

失败：正文断言没有D，且三次IO全部属于S，与原生D开段记录冲突。日志1964/1980已交正确D/IO教学，1990–1993交清单和0/3/0，2064实际模型emit仍误述，不是最终上下文没有数值。真实payload `.codrax/blob/20260921-183106-000-13832/trace-query-result-26a1d09a.json` 中三条timeline.intervals的prev_state_raw均为D（起点90/225/2427行），但account的wait_occurrences没有该字段；后续逐段上下文也只剩io_wait。此载体缺口独立修复，不能据此把模型不遵从改签PASS。

过程另记：analyzer有三次调用（544/576/617）；四次trace探索及系统window_stats补齐287条事实。event_search按文本命中有其它payload PID的blocked_reason，探索过程曾混用主体；行发射者与事件主体的角色呈现另审，不凭本次日志擅改检索语义。Binder独立闭合0.524ms已在最终输入，问题问D/IO，不能无条件要求列全部S等待。

## H2：明确窗口dma_fence D等待

最终报告`.codrax/output/20260921-183251.152-13833.md`；对应结果目录下`run-1.logs/codrax-20260921-183106-000-13833.log`。

通过：CompThread_0-2955显式13762.791708..13763.024898；11闭合D段合计36.757ms，IO为0。状态74.915+1.576+118.586+36.757=231.834ms，未归账1.356ms，总窗233.190ms。12条blocked_reason及Σdelay39.157ms单列，未被系统替成11段或sleep库存；真正sleep库存29段155.343ms。新的非IO D计数标签正确。系统window_stats补齐276条事实，有限问题的空旁路及零投影正确。

机器FAIL仅为`(typed )?内核调用点[ =]dma_fence_default_w`不接受当前“内核调用点/符号=”词面；保持原oracle和FAIL。人工独立FAIL：正文把dma_fence调用点提升为已确定DMA fence信号量，将ELF模块devhost.elf说成进程身份；用独立验证Binder为零断言“不是Binder而是DMA fence机制”。最终输入已限制调用点不证明资源/持有者，未验证不等于排除；日志2099/2111保原计数/调用点边界，2176模型仍越界。正文泄漏verified_wait_union；老系统附录opaque工件ID和内部状态词也留单独词汇债。

## 本次结论与后续

§103系统标签窄片有真实命中，公开红绿/race/全仓齐全，可完成该子片；不能销上述正文错误或整个HMC父项。先贯通已有逐段物理状态事实，不加正文关键词门、不修改模型结论；随后处理§104系统优化潜力措辞、已接受业务实例补齐和容量恢复。父账13/79、66开放及B2–B6不变。本批未跑写模式，前批写模式结果不冒充本批覆盖。
