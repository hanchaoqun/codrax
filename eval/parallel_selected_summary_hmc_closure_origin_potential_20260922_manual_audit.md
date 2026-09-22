# aff63 固定双例完整人工审计（2026-09-22）

- date: 2026-09-22T09:35:21Z
- sweep_start_ts: 20260922-023520
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_closure_origin_potential_20260922

冻结生产 revision `aff63de7b9a1`，构建 `2026-09-22T09:34:53Z`；runner 61080 正式 exit 0，严格 2 并行 × 各 1 次，无追加追跑。后续旧测试词针迁移未改生产。机器结果保持 2/2 PASS；完整人工 **0/2 PASS、2/2 FAIL**，不修改历史机器收据。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | nested_python_increment | PASS | eval/results/hmc_closure_origin_potential_20260922/nested_python_increment-20260922-023521 | write_apply,write_patch_oracle,answer_contains | none | 146s | 28 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | FAIL | 实现正确，但已有测试未运行 |
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/hmc_closure_origin_potential_20260922/trace_query_frame_semantic_span_optimization-20260922-023521 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 195s | 43 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 正文完成/唤醒顺序错误及无凭证独立性断言 |

## Trace：计量与正文分别验收

完整阅读305行 answer-transcript、24行 primary/principal、原始附件、最终模型输入及 emit/patch，并核验 `.codrax/output/20260922-023832.878-27889.root-causes.json`；独立审计同判 FAIL。

- app-100在5.000–5.007秒内sleep5.000ms、runnable0.800ms、running1.200ms；worker于5.005秒唤醒目标。VerifyClass区间5.0004–5.0054，原始5.000ms、边前相交4.600ms、边后0.400ms。宿主全窗running4.600ms与依赖窗内running4.000ms分开。
- 因果投影、估算潜力4.600/0.800、缺频不可折算、邻近sleep1.400ms与背景0.800cpu·ms均保留；7ms窗口不证明丢帧。树为专用text围栏，不是本例Mermaid语法错误。本例不承担IO或源码拒绝接缝的live覆盖。
- primary:7称“完成类校验工作后，于5.005s…sched_wakeup”，真实完成却在5.0054；:13又说不证明完成后才唤醒，是实质时序/因果矛盾。:23“两类候选相互独立”亦无干预独立性凭证；区间不相交不等于修复效果独立。后置caveat不抵销前述错误。
- 日志2049/2056明确边前候选且完成/阻塞权威未提供，2156给原始与相交区间。原始Trace和原生查询足以回答，不能归因材料缺失、未经验证称随机波动或新增正文关键词门。另有前置模型观察错误，见主账本§124；不得把原生正确等同所有阶段上下文均正确。
- 系统占用表仍有“现规则可消/可消除榜”等旧词，平台事实和部分标签仍混内部英语，留展示子债，不宣称全域词汇闭环。
- 首次emit未选旁路，系统一次可选提示后，日志2522的patch明确提交`{schema_version:2,root_causes:[]}`且保五块正文。最终sidecar为`schema_version=2,root_causes=[],status=available`（72字节），不是导出失败或系统删候选。模型以无帧因果为不选原因，不能因此强制填满候选清单。

## 写模式：正确补丁不等于完整验证

完整阅读输出、计划、执行报告、交付目录差异及两段planning/apply日志。只改widget.py的`return value`为`return value + 1`，测试/setup.py/tests/__init__.py字节不变。外层146秒，案例`run-1.wall`与summary为143秒，分层保留。

- 交付/执行commit `0f2287370b41d430beb089f065f162c5363f03f2`；实现SHA256 `833556db828032d49efc4c58a380411c53465c917da89bf613a607a10cee2ba6`与回执一致，mapping_complete=true、changed_lines=[2]、executed_lines=[2]，不是仅导入或借其它代码PASS。
- 最终report只有verify_increment_fix一条probe（-5、-1、0、1、7）。原生unittest明确source=probe_primary_suite_skipped、outcome=suite_skipped；3个已有测试及±2**64子例没有执行收据。用户要求运行已有测试，故完整任务FAIL，不能以target_execution或aggregate PASS替代。
- 最终project_test_observations为空，6个行为合同均为planning-only非required。run_tests已发现root及packages/widget两个suite，probe PASS却提前返回；controller收到suite_not_run仍all_verified。下批从精确suite/变更归属和执行策略修复，不扫描原始问题创建硬义务，不把自然语言checklist全升硬门。
- 首轮dry-run的sys.path.insert加from widget已成功导入并触发原bug的AssertionError；随后三次静态coupling拒绝不识别该路径修改，不能误报ImportError。最后cwd=packages/widget对齐已有模块规则后通过。静态保守门与运行证明关系另审，不因一次执行成功放开任意源码绑定。
- seed/交付test_widget.py SHA256均`504b2535413cafa883be2802d92a698036735de8883384530caec4fa8375f322`，setup.py均`921d85d69b7d4ccfd986e6495d1d87025e2af058943e6bfc6f0daa4c9679abb7`。没有事后补跑已有测试倒签live。

## 记账

父任务仍79个唯一ID=13已交付+66开放，不销原答案FAIL或B2–B6。下一优先：已有测试被probe提前收口；§120最终caller/源码边界；§94局部业务补齐/容量和原生配对。模型时序与因果越权独立留账，不用原文扫描或个例禁词修复。
