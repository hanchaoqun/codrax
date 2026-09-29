# HMC §192 双例人工审计

- date: 2026-09-29T09:13:06Z
- sweep_start_ts: 20260929-021259
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

固定两例并行×1，46820正式exit0。二进制revision `7320af656b27`，先提交Go/build/测试再make并快照；后续仅文档更新。机器1/2，完整人工1/2；未追加第三例，不改写旧FAIL。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_sqlite_startup_subject | FAIL | eval/results/trace_sqlite_startup_subject-20260929-021306 | basic_output | perf_triage+trace_query | 160s | 41 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 四段时长/进程与未知执行主体正确，元数据修补丢表头；正文PID错写、合成名及HiSys线程角色错误。 |
| 2 | empty_python_module_apply | PASS | eval/results/empty_python_module_apply-20260929-021306 | write_apply,write_patch_oracle,answer_contains | none | 285s | 28 | read=10,repo_map=1,list=2,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS | 完整CLI真实命中只读登记→新执行→controller finish，默认持久run complete/verified；仅totals.py改动、测试及主仓HEAD不变。 |

## 1. 启动来源与答案

新问句只表达业务目标、窗口和所需列；没有给用户塞入防护协议。复用原SQLite fixture和shell oracle，静态声明发现仍标basic_output的历史盲点保留。两次完整窗查询保留各自收据，最终去重展示13行=4条启动源记录的8端点+5条系统事件，metadata omission=0，未查源码。

实际finalizer有source.row_id 2/3/4/5、source.owner_ipid 1、source.owner_pid 27599、完整纳秒区间及8/4/4/5ms时长；source.subject_role为process_owned_interval，执行TID/CPU均not_recorded。初次表中所有数值和缺失执行主体正确，没有复述cold/ability_init或把两端算两次启动。源名称NULL/未解析也在输入，但表仍用合成AppStartup:startup当名称，未清楚区分真实业务名与显示标签。

全文另有错误：正文一次把27599写成27559；把HiSys全部归同进程、全部无发射线程以及CPU0断言泛化，未遵循逐行源角色；没有process名称不等于源明确“未命名进程”。这些均不允许用数值表正确掩盖。

### 可证的通用修补接缝

日志2215行首次emit两个table都有columns，含时刻和ms单位。2223行post_emit_advisory只要求requested_dimensions展示归属；2272行模型用replace_blocks替换两表，复制所有行但漏columns；2279行被接受，末版自动渲染成“列1…列7”。这不是renderer凭空丢列，更不是单纯旧单位正则误报：最终单位确实丢失，机器缺8ms/5ms保持FAIL。源数据/行数保留不等于可用表格。

代码`emit_answer_document_patch.go:1604–1682`显示已有无损add_facet_id通道只接**一个**member_set维度且**唯一**已绑定enumeration_item、含item evidence的载体；本例多维/多表不满足其公开条件，普通整块替换承载了纯元数据修订。下一修复需审typed维度身份、可选目标集合与模型自行选择的无损操作，扩大通用无损编辑的安全表达面；不按表标题/用户关键词选目标，不强继承用户明确删除字段，不自动代写列名。挂01.3/16.4/18.4高影响待修，不能说§191数量修补已经覆盖该问题。

### 上下文与权威边界

finalizer初始system 85,816字节、估算62,198 tokens，第二轮73,270；全链最大82,895/200,000=41%。13/32展示行，所有源字段完整；与旧问句维度不同，不把token变化归因优化收益。工具等待仍timeout/first-byte=10m、stall=5m，持续语义流没有降级。

本次pretriage没有发Startup，所以**不能声称live命中Startup权威拒绝**；该分支由公开发射→实际消息正反矩阵证明。它仍在Meta发duration_ms=0.08、signals=cold-start-slow，实际explorer/finalizer展示为Duration0.1ms和Signals cold-start-slow。Meta.Summary被挡住，但其他meta标量/信号尚未验证；这是新具证的共享摘要来源缺口，挂同父项而非冒称全部预提取已修。最终答案没复述该数值也不能销系统缺口。

根因旁路`.codrax/output/20260929-021543.581-41459.root-causes.json`为schema2、空数组、trace_root_cause_contract_not_active，无邻近证据晋升根因。原完整日志、报告、初始emit及patch保留，不追跑。

## 2. 写模式只读登记完整收尾

初次plan尝试create既有空文件被精确拒绝后改insert_at_eof；只交付totals.py的6行实现，最终`return sum(values)`满足空输入、负数、大整数、单遍生成器。初始PTO误用`tests.test_totals::TotalTest`，实际unittest suite是`tests.test_totals.TotalTest`。4测试PASS不被当成empty-input-zero合同已闭合，控制器创建只读verification_proof_followup。

真实planner读取已经交付的源码和完整旧测试，使用`changes:[]`与4个精确project_test_observations登记，未改测试/配置/依赖，未新建probe。公开系统凭证绑定原source_plan `plan-1790673318064898000-41478`、实际commit `a2e8d190e51462612b867361c1718b4102568388`、测试SHA及原合同。新的verify再次执行4条断言，旧invocation=`native:ecea:18d9bff36d4e8780:2`，新=`native:ecea:18d9c005525f5e10:3`，不是复用旧PASS。

报告existing_test_executions记录4断言/0失败、精确测试文件及已交付来源；worktree_audit=clean。真实controller输出verify_batch后finish/all_verified，3批均complete；默认持久run `run-1.repo/.codrax/plans/workflows/wf-1790673318851150000-60650.json`和最终报告均complete/verified/all_batches_verified，不是§191的no_durable_store阶段结果。

人工核对run-1.applied-tree实现正确，原测试与交付树测试逐字节一致，SHA均`95143baf97adf98c6343795287cf6cab334533ddc5b34bea7e1887067c1e5556`；主仓HEAD仍seed `4d75460579ebd74b264a4b247b8c7bdaaba55136`，实际改动在隔离交付ref。最终用户结果明确4测试PASS、交付已验证；用语仍暴露ChangePlan/contract/PTO等内部术语，作为呈现债保留，不伪称整体产品体验完成。

本次可以关闭**只读登记的完整CLI/controller/默认持久存储收尾验收退出条件**。不代签跨进程重启恢复整轮、不覆盖所有语言/多batch组合；§191公开持久文件恢复+真实planner验证仍只算其阶段证据。18.5为持续验收父项，依旧开放。

## 3. 工件指纹

| 工件 | SHA-256 |
| --- | --- |
| Trace完整日志 | `1a76a6cef67a48e6016a2488b011d761229c23003bb1603b0ac7f4ec3f0dc3b8` |
| Trace最终正文 | `e3b65a2597470ddfe6110bbd44a4bc634474396c86d71d671824f1402a632b73` |
| Write完整日志 | `66e84a2e0a6c3d0e0842c8aa8cf58ac17c5117a5a361d739be32a18c232573b3` |
| 只读登记计划run-1.plan.json | `2d3619b57124c4e5c4701a2031db1d61d400c25e32ed15ca0f303d8851142bcf` |
| 新执行report | `eb10ef65039d73e74169cf9a31dee374898f585188798980dd5fb101d54169a6` |
| 完整final报告 | `9640b6eb00efaf37cca83bb3e5977471b44ac44c0ec1422cb65de5f8ff13cea1` |
| 默认持久workflow | `c806ab31395859557664841dc29f750a2c54b89f363a41fc0b85ae56085c3ba8` |

大原始日志/运行仓库留在对应结果目录，机器verdict/metrics和关键计划/执行/完成报告随批登记；旧FAIL不覆盖。完整回归与推送收据见统一账本§192。
