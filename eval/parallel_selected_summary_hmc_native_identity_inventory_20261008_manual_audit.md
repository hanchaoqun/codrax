# 精确登记身份与可读事件交接：固定双例人工审计

- 批次：统一账本 §210，2026-10-08；构建 `bf1be10c27d4-dirty`（构建时仅文档未提交）。
- 恰好2并行、每例1次，timeout 1200s；未改自然问题、未追加第三例追绿。
- 原始目录：`eval/results/hmc_native_identity_inventory_20261008/`；完整工具载荷在 `payloads/<原session>/`，失败/成功回归日志在 `validation/`。
- 机器1/2，完整人工0/2。实现子能力的限定验证与整例验收分开；旧失败不重签。

| 用例 | 机器 | 人工 | 时间 | 限定通过与剩余阻断 |
|---|---|---|---:|---|
| trace_static_initialize | PASS | FAIL | 134s | 解析名称、区间、逐成员CPU可知性及目标清单交接通过；受限查询未命中被写成整个Trace不存在 |
| native_registration_commandless | FAIL | FAIL | 159s | 修复正确、既有3测试通过、终态诚实unverified；普通相关测试的精确文件证明/后续补证未闭环，新登记选择入口未命中 |

## 1. 动态库初始化：关键交接修复命中，负向结论仍越界

结果目录 `trace_static_initialize-20261008-060030`；日志 `run-1.logs/codrax-20261008-060032-000-40457.log`；最终正文 `run-1.primary.md`。

原问题只要求线程101、10.000–10.050秒的动态库初始化明细。构造SQLite协议样本及独立oracle不在模型工作仓；本例不是实机采集验收。

1. 日志1172为 `event_search` 查询 `SoInit:dlopen`，用户目标线程自动继承；1231第二查询显式 `pid=101`，窗口扩至10.060；1276为同扩展窗口的统计。没有读取源码，无完成拒绝/无效工具。
2. actual finalizer 2009–2011收到完整清单，包含 `libimage.so`、`librender.so`、线程101、CPU0已知与CPU未知的区别、两个查询的独立范围及 `prompt_scope_projection`；展示预算遗漏为0。用户窗仍是10.000–10.050，不能把第二次查询窗当原请求。
3. 终稿两行分别为 `[10.002,10.008)` 6ms / CPU0、`[10.030,10.045)` 15ms / CPU未知，与oracle一致。表头/行宽正确，背景线程202与窗外 `liblater.so` 未进入目标清单。未生成Gantt，故不能据此销旧图量尺问题；端点CPU观测也不证明完整区间持续占用该核。
4. 阻断：终稿第7行声称搜索 `libbackground.so` 后“在这段 trace 中未找到任何匹配记录”。原始 `eval/fixtures/hmosperf_static_initialize/capture.sql:22` 及实际导出16–17行明确有线程202的 `[10.010,10.018)`、8ms背景事件。当前查询只搜索线程101，未命中不构成全Trace不存在的证明。
5. 错句已在原始 `emit_answer_document` 入参2157，2161接受；非renderer增删，未发生patch。finalizer 2030省略探索阶段自由文本closure，2052只余背景观察引用、无完整正向背景说明；但2010/2046及投影说明已提供准确的 `pid=101` 约束，因此根因不能再归为本批目标清单被过滤丢失。记录为查询范围到负向结论的边界缺陷，挂16.4/01.3，不扫正文关键词硬拒。
6. 上游perf-triage/analyzer摘要曾把两个CPU均写为0，实际query及本次最终清单纠正；该自由文本摘要不能冒充逐成员坐标权威。终稿暴露 `cpu_known` / `unknown_start_cpu` 属业务语言质量项，继续留案，不给单个词添加硬门。

## 2. Commandless写模式：正常修复通过，证明链仍不完整

结果目录 `native_registration_commandless-20261008-060030`；日志 `run-1.logs/codrax-20261008-060032-000-40472.log`；终稿 `run-1.out`、终态 `run-1.write-apply.json` 及原始计划/报告保留。

1. 用户要求修复 `packages/widget` 的increment，保留既有测试/配置/依赖并运行测试。实际只把 `widget.py` 改为 `return value + 1`；3个负/零/正数测试通过，现有测试和配置没有改写。
2. 只有一个普通计划（1770）和一次 `run_tests`（2457），无只读登记/补证批次。因此本例不验证新增 `assertion_ref` 的live效果；该分支有真实producer→完整读取→登记→新执行的公开确定性测试，不冒称已被模型命中。
3. 日志2605：hard_required=0、soft_required=0、planning_only=10。不能把planning-only升级硬合同，也不能用“缺PTO”解释本例未闭合。
4. 真实命令在 `packages/widget` 执行 `python3 -m unittest discover -s "tests" -v`；报告保留 `python/unittest@packages/widget::test_widget.IncrementTest` 的3个精确断言身份。根目录额外discover为zero_tests，不能把它抹掉，也不能用它否认非根目录3测试已通过。
5. 2609–2612仍有3个未闭合项（test_surface、related_test_surface_unverified、根目录no_tests），同时2616变更源码路径为covered。现有普通unittest impact lane运行目录发现，而非单文件执行收据；`verifyCoverageConfidenceFromEffectiveReport` 消费suite时没有保留WorkingDir/invocation，scoped suite不能当旧模块路径别名。单纯拼接cwd和目录并放行所有相关文件会伪造精确文件证明，不采用。
6. controller 2711尝试finish/all_verified；系统2722改为 `finish/accept_unverified`，最终 `unverified / impact_targets_unverified`。用户终稿明确“未完全验证”，没有虚报完全成功。完整人工仍FAIL，因为用户要求的自动验证闭环未完成，机器FAIL正确。
7. 下一修复挂18.5/16.4：精确相关测试候选→已有单文件观察器/执行收据→当前源与测试字节/真实调用身份→仅关闭对应test_surface；通过后仍未覆盖的项必须保留可执行补证意图，不能被passed run的展示压缩吞掉。此为系统proof消费/续行缺口，不笼统归模型波动，不要求每个普通计划都生成PTO。

## 3. 退出与优先级

本批完整能力新增0、累计18/79，剩余61项及5个稳定验收父项不变；交付范围为当前授权内原生身份选择、有限目标事件可读交接两组子能力。原生来源终态、多框架/多来源、旧图关系/量尺、HiSys行身份成文、目录工具路由等原债继续挂原ID。

下批双轨优先08.6 GPU active×frequency完整纵向能力与18.5精确相关测试证明消费/续行。Trace负向结论保原证据并列入通用查询范围/完整性设计，不靠反复增加单句提示或重跑同例占满能力轨。单位、缺测、窗口、对象及因果边界仍由系统负责，不写回用户自然问题。
