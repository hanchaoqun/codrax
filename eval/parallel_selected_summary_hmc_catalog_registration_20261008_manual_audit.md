# HMC-208 固定双例人工审计

- date: 2026-10-08T09:31:25Z
- sweep_start_ts: 20261008-023123
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_catalog_registration_20261008

冻结 `5e603f29f`，恰好两例并行各一次。机器1/2，完整人工0/2；没有第三次追绿。旧FAIL不改签，post-live端点导航修复不倒签本轮。原始两次调用及payload保留在结果目录与`payloads/`，测试原件位于`validation/`。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_catalog_objects | PASS | eval/results/hmc_catalog_registration_20261008/trace_catalog_objects-20261008-023126 | log_regex | none | 248s | 49 | read=10,repo_map=0,list=3,trace=8,source_lens=0 | midloop=6,inv=2/0,fin_reject=0,unavail=0,prune=2 | FAIL | 发现正确但选错查询；右界、符号、跨源及释放对应关系错误 |
| 2 | native_registration_commandless | FAIL | eval/results/hmc_catalog_registration_20261008/native_registration_commandless-20261008-023126 | write_apply,write_patch_oracle,answer_contains | none | 283s | 29 | read=8,repo_map=3,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | FAIL | 真实登记和新执行已命中，但测试标识未精确对应，终态诚实unverified |

## A：多采集、多线程资源查询

实际CLI约245秒，runner计248秒。目录工具确实发现两份同名文件，工件与源版本独立；没有预登记queries，不能把planned=0当全部对象已完成。8次trace_query全是event_search，没有调用resource_stack；10次read_file均读取运行生成JSON，未发现读取oracle。独立oracle与建库SQL已放在模型fixture外，模型工作仓仅两份采集文件。

[最终答案](results/hmc_catalog_registration_20261008/trace_catalog_objects-20261008-023126/run-1.primary.md)第11–14行编造malloc_usable_size_internal/UJFrame，并纳入10.050右边界事件；第30行把nested的editor业务符号/库污染成gallery；第24/43行把最后观察事件当采集结束（原trace_range为10.100）；第38/44行无生命周期配对证明却断言释放对应某次分配及其它为未释放。正确基准见[独立oracle](cases/trace_catalog_objects.oracle.md)：首文件101=3事件/8帧、202=1/3；nested101=1/3、202=0/0。搜索零匹配不是资源查询空结果，本轮不能替空出口验收。

系统层证据来自主日志`codrax-20261008-023128-000-82372.log`及归档`payloads/20261008-023128-000-82372/trace-query-result-431af4bb.json`：

1. log 616–618将用户明确的时间窗/runtime question因无附件carrier撤成not_applicable；目录内已知采集来源未贯通请求范围域，归16.4/01.3。
2. result JSON第71行有resource_stack query_ready_export、19行导出，证明不是资源数据未解码；第59/65行却仍说native_hook_frame未转换。exporter读了帧/字典但通用coverage未登记对应来源，归17.7/16.4。不能全局掩掉真正unsupported，应按实际成功producer归并真实表覆盖。
3. log 3004完整能力目录含resource_stack，教学不是完全缺失；零匹配指导仍偏向literal/trace_mark/perf，没有消费已验证源能力。下一步应把同源可用视图、结构化对象和查询目的组合为精确导航，不新增用户正文关键词硬门。
4. catalog原先用bool默认值隐含右开边界，与event_search实际含末点合同不一致。post-live改成可选端点语义，未知保持未知，目录参数不代替原生区间合同；实际查询结果未改，此FAIL保持。

## B：完整CLI/controller及只读测试登记

实际CLI约279秒，runner计283秒，CLI exit0但机器FAIL `write_final_verdict:unverified:verification_proof_incomplete`。入口仅`--mode=write --request`，没有导入已有计划。主日志为`codrax-20261008-023128-000-82389.log`：3725 controller授权只读登记；3847完整读取现有测试；4127拒绝登记/probe混用；4158接受纯登记；4514重新运行测试。

[登记计划](results/hmc_catalog_registration_20261008/native_registration_commandless-20261008-023126/run-1.repo/.codrax/plans/plan-1791452134181781000-82389.json)具有native_test_registration、零源码changes，绑定原计划`plan-1791451963682370000-82389`、应用commit `e41bf884a6df1ff51df598b7a1a0b5f1e84a3e20`、原patch及测试文件SHA。[新报告](results/hmc_catalog_registration_20261008/native_registration_commandless-20261008-023126/run-1.repo/.codrax/plans/plan-1791452134181781000-82389.report.json)包含1份ExistingTestExecutionReceipt、3个passed断言和新invocation；没有复用早期成功冒充补证。此真实CLI登记/新执行子能力PASS，不再误列为完全未实现。

最终仍FAIL的直接原因：计划使用裸`test_negative_integers`等ID，实际报告为`python/unittest@packages/widget::test_negative_integers`；suite虽已修正，断言ID未修，三个行为映射仍missing。两个批次保持unverified，最终文本明确区分3条测试通过与整个交付未完全验证。完整来源终态不闭合，18.5不销账。高ROI后续为生产者原生测试身份的结构化选择/校验，减少手抄而不使用后缀模糊匹配。

另有独立教学矛盾：skill/defaults.go中“空changes仅两个例外”与同prompt的native_test_registration第三入口冲突（log 1632/1670/1726），需统一合同，但不能把它直接认定为本次ID错误原因。

隔离核验：scratch HEAD仍seed `fb4953f2e382ac980de8af7dc6d36ccb94165ac7`；应用commit仅改widget.py一行。测试SHA `504b2535…f322`与setup.py SHA `921d85d6…bb7`和fixture一致，worktree干净。主根.gitignore被既有EnsureCodraxGitignore追加.codrax条目且未提交，因此只确认HEAD和业务交付隔离，不虚称主根所有字节不变。

## 计账边界

本批新目录能力的实现验收与模型整答分开：公开确定性正反测试证明目录、代次、来源、查询状态和最终上下文；它不保证模型选择了正确分析工具。两份完整人工FAIL仍归对应16.4/17.7/18.5原ID，五验收父项不销。原本已关闭的限定登记/资源子能力不因新答案失败重写历史，也不因单个子阶段PASS冒充整个业务问题通过。
