# 只读测试登记／IO活动：固定双例人工审计

- date: 2026-09-24T08:08:45Z
- sweep_start_ts: 20260924-010842
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

机器结果与人工结果独立。构建revision为`2125104ae88c`，25171正式exit0；恰好2并行×1，无第三例。后续`60fd2cbbd`只改旧负控测试，`3ab190d77`修复软链接规范化路径的含锁上下文复制及教学pins；这两次修改未追加live，不能倒签成末版模型回放。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | empty_python_module_apply | PASS | eval/results/empty_python_module_apply-20260924-010845 | write_apply,write_patch_oracle,answer_contains | none | 234s | 28 | read=8,repo_map=0,list=3,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS | 真实命中只读登记及新执行，源码/测试边界和最终状态正确；旧seed/probe角色冲突留账 |
| 2 | trace_query_io_activity | FAIL | eval/results/trace_query_io_activity-20260924-010845 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 461s | 35 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 事件枚举未进入window_stats；RQ总体/桶/比例错、MMC请求字节错、F2FS缺测补零、BIO桶误作请求时长 |

## 1. 写模式：旧失败分支本次实际闭合

完整读`run-1.out`、plan/两份report、最终交付与`run-1.logs/codrax-20260924-011020-000-97661.log`。机器PASS不等于全部合同证明，人工确认以下实际链路：

- 初始实现只改`totals.py`为`return sum(values)`，支持任意精度整数、空输入及单遍生成器；既有测试/配置/依赖不变。初始4条测试绿仍因PTO身份错误不能覆盖必要合同，不被系统提前判已验证。
- apply日志2124当前授权指引；2218–2255两文件真实完整read。2323混合probe与PTO被精确拒绝，2346–2349去probe后无源码登记accepted；不是直接伪造模型已读取凭证。
- `run-1.plan.json:50–134,300`为`native_test_registration`，绑定源plan`plan-1790237420601693000-89887`、提交`25f47b5682db9e527961869bd398c6f9ef5a7700`及测试SHA`95143baf97adf98c6343795287cf6cab334533ddc5b34bea7e1887067c1e5556`，没有新源码changes。
- 2731真实重新run_tests；新报告invocation=`native:17d7d:18d8338fe8a82248:3`，旧源报告为`native:17d7d:18d8337dd6c6d298:2`。`plan-1790237526468555000-97661.report.json:110–137`的exact source/test/command/assertion收据有4断言、0失败。2939当前必要合同1/1，3050正常all_verified；不是复用旧PASS。
- 最终正文`run-1.out:147–169`正确报告4条验证通过与已验证，并明确自然语言验收列表不等于每条有独立执行证明。本例无Trace图/根因旁路要求。

非阻断系统gap保留：2112–2119仍给旧初始IR的next_batch/ready_for_change_plan，紧接当前只读验证任务；2216模型据此误读阶段职责。scheduler的旧probe事件/目标也把真实登记误称探针。下一批改当前typed阶段投影与角色命名，不叠加提示词，不凭本例PASS删除该缺陷。初始错误suite、混probe声明已拒绝并纠正，不记为权限泄漏。

## 2. IO活动：机器与人工均FAIL

完整答案`run-1.primary.md`与24行原始fixture、`eval/fixtures/hmosperf_io_activity/expected.json`逐项比对：

| 项目 | 真实观察窗的预期 | 最终答案 | 判定 |
| --- | --- | --- | --- |
| RQ发起 | 7次、37888B、28次/s、151552B/s | 6次全读、23680B、24次/s、94720B/s | 错总体、漏写/其他操作 |
| RQ完成 | 6次、28672B、24次/s | 4次、20480B、16次/s | 错总体/字节 |
| RQ读写 | 读5/写1/其他1；读写次数5/6与1/6、字节21/37与16/37 | 读6、写0、其他0 | 错，未保各自分母 |
| 100ms桶 | 4/1/2次，尾桶50ms、16384B | 6/0/0，零尾桶说速率无意义 | 错，不把零桶当缺测 |
| MMC | 起点2048B，完成报告1024B | 两端均1024B | 把请求量与完成量混同 |
| F2FS | 两端各1个大小未知，字节/字节率不可用 | 未知0、字节0、速率0 | 缺测补零 |
| BIO | 完整250ms分母，三桶含空桶 | 单桶时长=请求20ms，不存在并发 | 桶与生命周期混同，越出事件统计权限 |

额外错误：2.000 issue的sector100实际在2.010完成，却称找不到；大小表臆造2.185事件、把1024/显式0都当4096。右边界排除和250ms窗口表述正确，但不能抵消主要数值错误。无独立因果图，无已知图解析失败；核了Markdown表格及HTML载体，未声称浏览器视觉验收。

8次trace_query全部event_search（日志1302、1355–1357、1449–1452），没有window_stats。因此上一批新公平数值预览并未被本例真正调用，不能称“完整原生计量已经交付但模型仍乱算”。原始预览完整且多次原始查询返回相关事件，模型仍错误手算；工具选择/类型名导航、前阶段未经计量的错误摘要与下游供给需分层审计，不能把全部错误笼统归为模型波动，也不能用原文数字扫描作硬门。

最终消息精确复核：explorer初始1045已含window_stats.io_activity教学，1194–1218全预览包含所有事件；finalizer2147不重投全预览，2311移除26条预检导航。最终2340–2343的四份清单JSON完整投递BIO/MMC/F2FS，包含MMC blocks=2、block_size=1024及done1024，所以这部分不是字段遗漏；首批RQ清单却被最近4查询规则挤出（2339），一般ledger只保首9行及top5，2.200写、2.220刷新未完整交接。计量入口未采用与最终RQ缺页是系统改进项，不能混同数值已齐仍误述。

event_types按归一化类型而非统一原始名称过滤：BIO原名被归到RQ类，MMC/F2FS原名各0、家族标签或字面pattern才命中；1357的BIO查询实际重放RQ+BIO宽集合。原始payload的16行是8 RQ发起（含2.250右缘）、6RQ完成及2BIO；查找容差把匹配窗扩到2.2505，不可当半开统计总数。高ROI改进应统一事件类别/原始名称合同、真实事件族的就近软测量导航以及多查询范围的预算保留，不靠逐名alias或一例强制查询。

最终模型首调用约96s，第二次修表约54s；日志保留活跃流语义输出，未因短时间无最终答案降级。系统在requested_dimensions软建议后接受手填表，没有因此取得原生数值绑定或因果资格。必选旁路实际生成：`schema_version=2, root_causes=[], status=unavailable, reason_code=trace_root_cause_contract_not_active`，纯测量问题无根因合同；文件缺失问题未出现，JSON合法不等于正文正确。

## 3. 原始证据摘要（不重写FAIL）

| 工件 | SHA-256 |
| --- | --- |
| 写例run-1.out | `7ff4c2cbd0dbbed872055cee15ceff6f044749dd75712fc720029c2f8fb8a55a` |
| 写例run-1.plan.json | `80d295fa1c5eb659ad1d80d9ff0552dd92d53a02f1d978f81f99c5c4668b118a` |
| 写例新登记report | `74fc4b48035b545eedc3eacd462901456b7a256786ab3d2ade42d71a13faa2e5` |
| IO完整日志 | `eb61530a5a7f27b05d115f9fece28ba2a1a379a7cf063cbf5a02f0e13fe43b05` |
| IO主答案 | `7e5cdd06c896f36dc021ee0fcf697186e8580a25524d41e4d014439e235dc836` |
| `.codrax/output/20260924-011624.316-89858.md` | `89bca2323a37b7f81d33d618f17206246b320c09fdd3b9ed0a5e41b5a03b7ea7` |
| 同名HTML | `e84f4672e1a6cb45c81e2bf43f30bcf71dbfdc419d55d5eab850fa08e9ba8d06` |
| 同名root-causes.json | `5f8063f5882d6f6c5da9ef59d8ba0282f0fb2ef8c728bdcc77ef0249d2287e39` |

本批机器/人工均1/2。只读登记子能力取得实际分支验收；18.5持续父项不销。IO FAIL继续归01.3/16.4/18.4/18.5，旧08.3/04.2失败也不改签；后续优先17.7安全SQLite与当前阶段上下文统一，不围绕单例继续堆提示或追跑。
