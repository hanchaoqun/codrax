# Selected Eval Manual Audit — complete answer review

- date: 2026-09-23T02:59:44Z
- sweep_start_ts: 20260922-195944
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_existing_edge_empty_read_20260922

机器 1/2，完整人工 0/2。主审与独立审查均阅读完整答案、相关日志；写模式另核两代计划、报告、执行凭证和实际交付树。保留原机器判定，不以功能成功或图可解析代替整例验收。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | empty_python_module_apply | FAIL | eval/results/hmc_existing_edge_empty_read_20260922/empty_python_module_apply-20260922-195944 | write_apply,write_patch_oracle,answer_contains | none | 373s | 28 | read=12,repo_map=0,list=2,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 功能及交付范围正确；4条原生断言通过但必需证明未绑定，补证计划又缺当前执行收据。最终缺环境解释错误，不能销账 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/hmc_existing_edge_empty_read_20260922/read_combo_pipeline_sequence_table-20260922-195944 | answer_regex,answer_contains | none | 963s | 65 | read=23,repo_map=3,list=0,trace=0,source_lens=0 | midloop=16,inv=3/0,fin_reject=8,unavail=0,prune=5 | fail | 图可解析、关系位置正确；逻辑阶段误说必经执行，Objective字段、调度器归属和产物合并路径不准确 |

## 冻结与证据边界

- 两例均为固定 `17c51a21d52f-dirty` 可执行文件，buildTime `2026-09-23T02:58:53Z`；当时构建输入干净，dirty仅文档。恰好2并行，各1次，未追跑第三例或改原材料/oracle。
- runner 73915 于20:15:47正式exit0；读模式外层963秒、案例报告960秒，写模式373秒。记录两个不同计时口径，不把runner exit0当案例全通过。
- 后续职责迁移 `c213fd893` 及原因展示修复 `0caa55175` 不在本次live二进制内；其定向/race/完整回归另记统一账本§149–150，不能回写本次结果。末版完整30138正式exit0，87测试包通过、13无测试包；构建86574通过，代码推送56251正式exit0。
- 两例不属于Trace问题，未触发Trace因果投影；本批不能代签IO/链上根因、业务补齐、显式窗或其它Trace能力验收。

## 写模式：功能正确，但证明和最终披露未闭环

输入是已有0字节 `totals.py`，实现整数可迭代求和、空输入、精确大整数及单次遍历。最终交付仅该文件新增两行 `def total(values): return sum(values)`；对比基线与交付树，测试/README/依赖配置保持。原四条unittest断言实际通过，且源码计划有精确计划/提交/补丁/测试字节及断言身份收据。

物理行修复真实命中：read_file返回空文件0行0字节；`insert_at_eof`从真实空基线生成两行，未借虚构第1行。此处PASS仅覆盖这一能力，不代表整个写任务通过。

失败链：

1. 模型PTO声明 `pytest / test_totals.py::TotalTest::test_empty`，真实结果是 `tests.test_totals.TotalTest / test_empty`，因此hard-required c1未绑定。源码、标识教学和后续完整真实身份已提供，不能称证据缺失，更不能由系统自动猜改声明。
2. source-free补证计划只有来源计划谱系，没有自身AppliedCommitSHA/PatchEffect，当前身份检查不启用原生收据观察器。测试实际执行成功，但最终报告 `existing_test_executions=null`、`required_existing_test_not_executed`；缺口是跨计划交付身份解析，不能靠借用历史收据解决。
3. 后补报告5项通过是1个探针结果＋4条原生断言，不是5条原生测试。完整结果仍 `unavailable / verification_incomplete`，机器FAIL正确；controller没有凭探针通过把交付标成verified。
4. 工具摘要固定写changed_path原因，最终report却是required_existing_test原因；用户输出又称缺运行器/依赖，实际Python/unittest正常。确定性展示接缝已在live之后修复，旧误导保留。
5. 两轮 `create_path_exists` 均建议modify，下一步micro范围又禁止modify；合法EOF patch随后成功。另空文件被要求现有owner而额外重规划。两者分别留统一账本，不用此例改变所有非空定位门。

主要原始证据：`run-1.out:218`起、两份 `plan-1790132542642790000-62414` / `plan-1790132709531874000-70390` 的计划及report/final JSON、两份 `.logs`、`run-1.delivery-seed.json`、实际交付树。未修改这些收据或替产品补签合同。

## 读模式：图修补完成，整份解释仍不准确

最终 `run-1.primary.md` 具有合法sequenceDiagram和完整阶段表。第8轮三个stale-anchor replace使用同一当轮placement_ref，按提交顺序插在选择的Note之前；第9轮只 `remove_if_isolated` 清除Orch后接受。没有隐式末尾追加、重复边或位置漂移；但不是§147限定调用证明桥的现场命中。

人工FAIL依据：

- 第1行把规范四阶段说成每次固定执行；本次日志4400实际跳过extract，源码存在skip/reuse/条件入口。
- 表格写不存在的 `busCtx.Objective`，真实目标来自 `Mutable.Objective()`；准确字段已由上下文提供（日志5365–5373，相关源码1835/4052）。
- 第30行将runReadSchedulerLoop归为explore内部，而该循环调度extract/finalize；全部产出统一由applyStageOutput合并的说法也过宽，AnswerDocumentV2由工具直接进入Mutable，FinalAnswer不在该方法归并。

上下文确有一条需修的系统边界：`answer_document_evaluator.go:25301` 的 `every selected stage is executed` 混淆静态成员归属与实际调度。应讲清“如果执行，由该agent负责”及条件/跳过/复用的证据要求，保原能力和先后关系；其它载体/调度器错误主要为模型在已供准确上下文后的越界，不新增散文扫描硬门。

效率：9轮成文、8次拒绝，23次读取，3次repo_map，峰值129849 tokens。四次同typed关系replace+add冲突，另有越出局部关系、缺placement、孤立参与者收尾。初始说明候选只是许可，首个冲突已明确要求只选一个生产操作；未证系统强制同时replace和add，重复误填记模型波动，不继续单例拟合。格式要求被分类为exhaustive成员、重复读取和重复Note另记观察。

活跃语义流超过4分钟继续等待，没有固定4分钟降级；未实等600秒首响应，不声称覆盖该超时边界。修补使用当轮租约和最终草稿，未将失败修补推理写入最终正文。
