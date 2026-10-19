# HMC259 双例人工审计

日期：2026-10-18（本地；收据UTC）。与[批摘要](parallel_selected_summary_hmc_binary_native_handoff_20261018.md)共用冻结版本、原问题和原始输入。每例一次且并行，没有追加第三例求绿。自动verdict、有限入口和完整人工结论分别记录；原文件不纠正覆盖。

## A：真实sample-only二进制

**机器PASS，完整人工FAIL；完整原件→准备→原生取数→原生表有限PASS。** PID67536，UTC01:03:15.364767–01:06:08.763364，正式exit0。普通可列举目录中的30,062,585B原件，不是HMC259新增search-only权限分支的live验收，也不证明BRBE解码。

独立参考解析、离线生产oracle与实际公开查询一致：全源12,000、窗口1393、目标tid1609为164记录/259权重；CPU0回调82/85，CPU1未知内核82/174。记录比例均50%，权重比例32.82%/67.18%。采样类型全未知，记录首末点及1ms桶不证明连续运行、停顿、CPU时间或调度状态。

| 交付表面 | 人工结论 |
| --- | --- |
| 原生热点和总体质量表 | PASS：来源、CPU、分母、权重、完整符号、未知状态与非耗时边界到场；总体质量明确不是单热点质量。 |
| 正文、primary/principal、持久化Markdown/HTML正文 | FAIL：将两组采样桶写成4ms On-CPU，其余196ms停顿；122ms没有记录被说成强烈暗示未运行；业务函数名被用于排除停顿位置。 |
| root-causes旁路 | 缺证边界保持：unavailable、空数组，没有发布确定链上主因。 |
| Trace投影及自动补齐 | 保留：投影仅缺口，自动查询确实执行，无调度证据不补造因果；本例没有Mermaid，未签视觉布局。 |

不是事实没传到模型：finalizer两summary均无预览省略，总预览28/128行、3组；首轮66,069/200,000tokens。错误最早在explorer的`native_fact_handoff.interpretation`及`reason`出现；实际finalizer日志1887–1906把独立解释与旧调查正文重复交接，正确表同时存在。最终又扩写4ms/196ms，五条advisory、零严格违例不等于语义通过。未证明偶发，不销18.3，不以多加提示或关键词删文修复。

独立审计原件：`/tmp/codrax-hmc259-sample-manual-audit-v1.md`，SHA`d6b18711db35262c711ea457a5e2d48ed161eff1624d6f961d5eabda4f605534`。完整报告列原始纳秒范围、实际上下文、所有表面和4份发布文件、18份完整工具载荷、3份准备材料的逐项归档清单。原生时间表示微秒精度已披露，不将未承诺的纳秒尾数算成漏值。

## B：只读登记终态

**固定合同范围SCOPED PASS。** 正式运行PID67534，UTC01:03:15.364753–01:05:04.713433，exit0；恢复子进程PID76710。真实父controller五次决策、planner两轮、子controller两次决策。父模型三次过早accept_unverified由精确typed义务推进为验证/探索/登记，不是一次直达或脚本指定成功；planner完整读取2行源码/4行测试后才登记，子流程真实重执行并finish/all_verified。

初始源码、合同、Git和历史执行由operator准备，验证器为公开真实RunTests；受控重启，不是完整CLI、从零自然修复、崩溃恢复或全部框架通验。

独立断言只有`test_value.ValueTest::test_increment`一个。原始日志有4次实际测试执行＝历史1次＋自然工作流3次（父来源验证、父补证验证、独立子登记验证），不能按3份report文件计成3次，也不能把编译预检当断言。最终report仅含子进程当前1个断言与1条新执行收据，当前invocation为`native:12ba6:18dfc8bdfb75cbd0:1`，旧invocation为`native:107ce:18dfc8a72184f128:1`；当前证明没有复用旧执行。

最终workflow complete/verified/all_batches_verified；source有自己的验证终态，早先补证attempt仍保unverified，最终由明确的新补证依赖完成。源码、测试、HEAD不变，git diff/status为空，执行授权用后撤销。六份上下文包在五份快照中逻辑内容不变；持久化原始/加载/两次往返的引用与工件SHA一致、无累积。child内存一份artifact_id在Save规范化副本时重算，不能声称所有内存ID也字节一致。

**新发现：生产历史报告覆盖，留账但不否定本次新执行。** `saveChangeReport`按同一`<plan>.report.json`重写，第2次全局执行的完整报告/invocation已被后次覆盖；source-verify-report只保存最新第3次。旧attempt仍有绑定SHA和1次结果快照，日志可核真实命令/时刻/exit0，但不能据此重造旧完整载荷。不是仅helper展示问题，按既有18.4/18.5及共享执行交接提高优先级；下一片应按attempt/内容身份不可变留存、当前索引另写，保原终态与授权，不根据文字猜历史结果。本次child的新完整报告和来源验证终态仍有独立证据，限定PASS成立。

独立审计原件：`/tmp/codrax-hmc259-registration-manual-audit-v1.md`，SHA`0d2440fe56807c510d00c7a8bacba4b052333f1fabd1011ddd381aef911404e3`。原始失败和未覆盖范围不重签，18.5持续执行。

## 账本与后续归属

79稳定唯一任务，24已交付、55开放不变。完整能力新增0、两组生产子能力；本批剩余验收缺陷2组：A事实/解释/生命周期及同源量尺冲突，B历史报告不可变留存。两组归既有16.4/16.5/18.3–18.5，不与55重复相加。原11.2自然代表入口未触发、11.3完成说明复验失败、旧图失败均保留，不以本批两例代签。正式全仓、提交和归档收据以批摘要及统一账本§259为准。
