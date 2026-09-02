# r1020 人工审计：根因旁路恢复，原生测试的行为证明未闭合

- date: 2026-09-02T08:11:05Z
- sweep_start_ts: 20260902-011104
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

基线 `5cd7368a4f7b`，从干净构建启动、严格 2 路；已读模型载荷、输出 Markdown/根因 JSON、写计划、应用 diff、项目测试代码与最终证明工件。机器 PASS 不代表模型正文无误。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_fmt_tm_year_overflow_symptom | FAIL | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260902-011105 | write_apply,answer_regex | none | 174s | 27 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | 实现及原测试通过；required 行为证明 1/0，诚实 unverified；B1561 原生证明能力/回补路径待补 |
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260902-011105 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 206s | 47 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=1 | partial | B1558/B1559 live 正证；5 项旁路及投影保留，正文仍有越口径判断；B1562 候选限定旁路缺口 |

## Trace 修复验收

- `.codrax/output/20260902-011428.993-90871.root-causes.json` 为 available，5 项与模型原始选择数量/顺序一致，不是系统补选。无 candidate_id 泄漏；供给 58.320ms、IO 12.658ms、两项低优先级依赖方 7.405/4.710ms、调度 3.956ms 均在。
- 第 1 项证据分为 194/65 字符两条，其他项 199–203 字符，完整保留两个引用与供给下界/频率覆盖/默认算力比说明。B1558 获得生产正证，未再发生超长拒绝；B1559 新 IO 投影/完整家族及无旧伪重叠 oracle 通过。
- 长答案 `.codrax/output/20260902-011428.993-90871.md` 中显式窗、Trace 因果投影、占用/可消除双轴、业务 span 族、IO completion-closed 家族、小量链上项和邻近/背景隔离均在；6 次 trace_query，首次成文零硬拒、零 patch。分析阶段 sub_topics 字符串数组被精确解包，未删除内容；一次工具批次 5→4 后继续补查，未丢最终投影。
- 正文仍有模型误读：用 D/IO 状态计数为零否定外部阻塞，却又列出 completion-closed 的 S 态 IO；自行合计 12.658+3.602=16.260ms 后又说没有合计权限；将 IO 响应阻塞墙钟称为折算；12.115 与 12.658 优先序不一致；出现内部 token。上下文已有对应精确限制，不增全文扫描硬门、不改写模型答案，不将机器通过涂成人工全绿。
- **B1562 / P1 / 待设计**：旁路 `priority_inversion_candidate` 映射到现有 v2 `priority_inversion` 后，仅保留金额和引用；长答案上下文原有 `measured_lower_priority_dependency_supply_candidate` / `priority_inversion_occurrence` 未获授权的限定没有带入，且复合的 runnable 全额 + running 折算组成没有在旁路解释。修复必须保存 source-specific 限定及组成，不改模型排序，不改造同一机制为系统选择的多个根因。公开 category/summary 兼容性与新增 certainty/caliber 载体需先设计，不能只扫字段名改词。

## C++ 写模式

- 应用提交 `70db425c64a49da4d8e96c0337399abdacb23927` 只改 `include/tmfmt.hpp` 两行：先将 year_offset 转为 long long 再加 1900，render_year 参数同步拓宽。测试文件/期望未变；不影响主仓 HEAD。
- `make check` 两次执行均通过（一次项目命令汇总结果，不冒充每条断言）；人工从 applied-tree 另行编译并运行原 `test_tmfmt.cpp` 也通过，产物 `.codrax/tmp/r1020-fmt-boundary-check`。极值 `INT_MAX → 2147485547`、普通 `121 → 2021` 均由原程序检查。
- 首个计划携带 cpp inline probe；当前 schema 仅支持 python/javascript/ruby/java/go，因此被准确拒绝，随后模型改用 project_test_observations。另一次多行 old_text 漏 end_line，被精确行范围提示修好；未发现新的 JSON 合同冲突。
- 最终计划把 `main / expect_year` 声明为项目断言并绑定 extreme_year_contract，但原生测试只打印整体成功，不能给出两个 expect_year 调用各自的执行身份。`projectTestObservationExecuted` 要求同路径/同执行/同 assertion 身份，不能从 make exit=0 铸造这些证据。
- 最终工件 completion=unverified / verification_proof_incomplete；proof.status=weak；project_test_assertion_not_observed 与 behavior_contract_observation_missing；required=1、covered=0。代码已落地不等于证明闭合，最终披露诚实。收尾把同一个聚合测试再跑一遍也无法新增断言身份。
- **B1561 / P1 / 能力与恢复缺口确认**：原生编译型代码在 inline runtime 不支持、项目测试只给 aggregate receipt 时，已有完整测试也无法自动闭合 required 行为证明。优先补 producer 的证明能力披露和恢复选择，再以通用原生 probe / 结构化测试报告路线落地。保留 aggregate≠assertion 红线，不特判 make check、expect_year、fmt 或“测试通过”文本；不把 native 缺能力泛化成所有项目测试都失败。

## 继承项复核与下一批

- B1560 进一步核实：图修补 schema 已有 `from_node_visible_label/to_node_visible_label`，适配器尊重模型标签、缺省才显示模型提交的 node id。本轮没有证明新硬合同矛盾；保留 P2 教学/显示体验观察，不强制模型填标签、不代写业务名称。
- B1554 隔离 C++ 邻行模板原型后，真实 `build<Item>()` 也没有 call row 并被误报 token 不在。9 语言探针中 C++/Rust/Swift/Cangjie 失败，Go/TS/ArkTS/Java/Kotlin 正向通过，所有 wrongTarget 负例仍拒。下一批需 AST callee 归一化 + anchor 诊断同源，变更提取语义必须同步版本；不能把有 token 或泛型括号本身当调用证明。记录 `.codrax/tmp/b1554-matrix-isolated-20260902.txt`。
- 按 ROI：B1562 旁路证据限定 → B1561 原生证明能力与可恢复路径 → B1554 跨语言调用矩阵 → B1560 显示/教学。先复现和设计，再分别提交；不为 prose 波动反复重跑同一 Trace。
- 本批 27%/47% 上下文，无预算耗尽；流持续活跃，没有固定 4ms/4min 无可见正文降级。全仓测试中的 keep-alive first-byte/stall/caller-cancel 回归也已通过。
