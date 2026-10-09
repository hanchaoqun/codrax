# 多框架导航与原生测试证明：完整人工审计（§211）

- date: 2026-10-09T01:35:21Z
- sweep_start_ts: 20261008-183520
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_rendering_native_proof_20261008

固定两例并行、各一次，使用 `f57ff390a6cf-dirty` 冻结构建（当时仅文档未提交）。机器1/2、完整人工0/2。以下判定基于终稿、实际模型上下文、工具载荷和真实应用/执行产物；后续修复与确定性回归不能倒签本次原始失败。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | native_registration_commandless | FAIL | eval/results/hmc_rendering_native_proof_20261008/native_registration_commandless-20261008-183521 | write_apply,write_patch_oracle,answer_contains | none | 98s | 29 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | FAIL | 实现修复正确、既有3断言和精确文件收据有效；子目录图被当仓库图消费，相关测试义务路径缺前缀，最终诚实unverified。新只读登记分支未触发。 |
| 1 | trace_rendering_candidates | PASS | eval/results/hmc_rendering_native_proof_20261008/trace_rendering_candidates-20261008-183521 | log_regex,trace_attachment | perf_triage+trace_query | 240s | 41 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=4/4,fin_reject=0,unavail=0,prune=0 | FAIL | analyzer错写请求窗、查询右界越出用户窗；正确候选被范围冲突挡在专属交接外，终稿混框架/跨进程关系并凭名称断言优先级。 |

## 读例：新查询命中，不等于答案正确

目录：`results/hmc_rendering_native_proof_20261008/trace_rendering_candidates-20261008-183521/`。日志行号指 `run-1.logs.all.log`。

- 自然问题仅要求1.000到1.050秒的框架/线程线索与后续排查入口，未在问题中提示单位、边界或因果守护答案；oracle单独保存。没有源码读取，四次Trace查询，两次实际命中新view。
- 日志950的成功analyzer提交将请求窗写为 `[1.001,1.051)`，实际finalizer日志2916再将它称为用户范围。四查询（1396、1397、1947、1948）用 `[1,1.051)`；终稿第1、3行沿用错窗，第20、32行纳入用户右界TID901。
- 两份候选payload正确保留9个查询范围内组合、PID600的Flutter/Web共存与PID700的缺定义状态；这个9不是用户窗口内的8。实际finalizer未收到专属候选段，只剩2858的截断generic note：错误请求起点1.001与查询起点1冲突，完整范围过滤丢弃了载荷。不能描述为“完整正确上下文已投递后的偶发误述”。
- `run-1.primary.md:11–16`把Web归为Flutter、KMP/Compose Recomposer归为ArkUI；第24行把PID200/300/600称为同一个应用的不同线程，跨owner关系没有来源依据。第24/25行声称7进程/10线程，却分别列9/11；“进程名”列实际抄线程comm。
- 第29–34行从名称推导确定性机制、高低优先级、跨进程Vsync链和Unity/普通图形与掉帧关联较弱。这些数据仅是导航线索，无调度、帧关系或因果链证明；末尾限定不能抵消正文越权。表格语法与列宽正常，无Mermaid，不替旧图关系/量尺失败销账。
- emit首稿被接受，随后patch只补三个facet（3024、3074、3080），没有修正文。日志1494的9个(owner,framework)成员在1501被系统归一为6，已确证：`answer_aggregate_fact.go`剥去括号限定后把不同PID的同名Flutter/Web/RN视为同一成员，并自动缩count。原始producer仍9项/遗漏0；这是独立的通用身份缺陷，不是本例所有错误的单一原因，也不是模型波动。

## 写例：真实执行已证明，系统路径坐标仍阻断闭环

目录：`results/hmc_rendering_native_proof_20261008/native_registration_commandless-20261008-183521/`。计划 `plan-1791509784426868000-9291`，工作流 `wf-1791509757337491000-9291`；原始报告在 `run-1.repo/.codrax/plans/`，应用树在 `run-1.applied-tree/`。

- 与fixture逐字节比较仅 `packages/widget/widget.py:2` 从 `return value` 改为 `return value+1`；现有测试及setup.py不改。测试覆盖负数、零、正数和大整数，真实三assertion通过。
- 当前report第64–78行是精确unittest文件命令；115–140行收据绑定当前来源计划、源码交付、测试文件SHA和唯一invocation，`test_path=packages/widget/tests/test_widget.py`；第35–46行根目录zero_tests仍诚实保留。
- 计划590–607行的相关测试义务却为 `tests/test_widget.py`，缺 `packages/widget/`。不能让完整文件收据靠后缀或cwd猜测替这个路径签收。最终报告保留两个unverified义务，completion原因 `impact_targets_unverified`。
- 实际日志480–481：`repo_map(path=packages/widget,view=source_inventory)`产生4文件子目录图，producer将它发布为全局SearchGraph；`internal/writeflow/impact/repomap_adapter.go`再把相对Graph.Root的索引路径直接交给仓库级影响分析。这是已定位的系统根坐标混淆，不是模型少写路径的随机失误。
- 日志2350模型尝试finish/all_verified，2361系统归一化为accept_unverified；最终输出第66–72行准确显示3测试通过但整体验证不完整，未伪成功。
- 仅一次emit_change_plan，没有新只读登记planner复入或assertion_ref选择。原计划自填pytest/短assertion_id仍是planning-only；本例不代签18.5/16.4只读登记的来源终态、多框架或多来源能力。

## 保留的任务与收据边界

18.5/16.4：后续`5c13c1993`已修代码图全局/局部根坐标和三个影响图消费入口，公共全流程RED→GREEN、独立负控/race及末版统一全仓89测试包通过，代码已推送；本次live FAIL仍保留。01.3/16.4：请求时间窗与采集包络必须有独立权威，候选身份/窗口应准确交接到表格和业务解释；不能通过扫问题或答案原文设置硬门。只读登记未命中、旧HiSys/图/CPU失败及五个验收父项继续开放。不追加第三例或只改自然问题求绿。
