# HMC322：sample-only 与只读登记人工审计

实现 `ab5834066d310208474e16e0c913f9b9c491a680`，main。两个真实问题各一次，冻结后不改代码/输入、无追绿重跑。root完整读取独立审计并复核A完整主文、原生表和实际B合同；源码/全仓通过与整答通过分别裁定。

| 侧 | 原机器 | 人工 | 关闭权限 |
| --- | --- | --- | --- |
| A sample-only | PASS | FAIL | 不关闭采样解释/18.3 |
| B 缺来源终态负控 | PASS | 限定PASS | 不关闭持续18.5或自然controller/fullCLI |

完整能力新增0；共享子能力2组；79＝27＋52不变。当前9组重点遗留和完整52项排序见[批汇总](parallel_selected_summary_hmc_endpoint_sample_registration_20261030.md)，未触发与失败分开，不新增原题退出门。

## A：完整取数通过，解释仍越权

[完整独立审计](results/hmc_endpoint_sample_registration_20261030_v1/sealed-v1/0156-codrax-hmc322-A-audit-v1.md)，SHA `927df2139db60c48e5f820d1bacf1cd1596d54748ea0d23f818cf33f32c4a26e`。原run `A/trace_binary_sample_only-20261030-033029`，原题/机器EXPECT不改，15步/n1。

完整PERFILE2原件30,062,585B，准备产物bundle11394B、采样成员14549669B/12000事件、预览14549907B分别保留。原件及副本SHA相同；历史独立parser oracle只证明时间/线程/CPU/period标量，不冒符号化/BRBE/SPE证明。

| 集合/热点 | 记录数 | 权重 | 核验 |
| --- | ---: | ---: | --- |
| 全文件 | 12000 | 43037682 | 历史独立oracle |
| 请求窗全线程 | 1393 | 2455 | 独立总体，不套到tid1609 |
| poll_report-1609 | 164 | 259 | tgid1514/代次1 |
| CPU0、OnDataEvent_ | 82 | 85 | 完整包络486489.464088–486489.465612 |
| CPU1、未解析内核IP | 82 | 174 | 完整包络486489.588096–486489.589609 |

CPU0零值省略不是未知，perf_cpu_known=true；权重cycles不换毫秒，不当调用次数。两独立深度0记录节点不串caller→callee；原生unknown、IP-only/部分符号化、假定时钟和缺调度明确。完整164条检索与另82匹配/20返回独立集合分开，预览没有替代完整尾点。

13实际提交＝1分析＋9查询＋1shell＋1调查完成＋1答案。9显式查询7成功2拒绝，再有2系统补采，得到9份成功payload；3拒绝不是成功。一次答案受理、0patch，原生事实/Trace投影/补采实际保留；critical_blocking_calls无可用观测不算该能力已补齐。模型图、局部patch和Notes删除复发布未触发。

**FAIL原因是一个既有解释越权家族**：正文多处说“确实在CPU上跑”“在CPU上运行”“直接执行回调”“缺的是off-CPU那一半”，但原生表及末尾明确样本类型unknown、执行状态未确立。离散包络也不是运行区间。未解析被进一步解释为“采样器没拿到对应内核符号表”没有精确失败原因见证，同族留账。没有选择停顿主根因，不能因此忽略前述事实越权。

实际3份INIT均已提供关键unknown/完整数量/CPU/权重和范围；完整错误调查reason未重传，闭环状态只作advisory。explorer峰值122725/200000、finalizer106627/200000、一次153600B历史裁剪都有原日志，但未发现关键事实因裁剪丢失。不能假定本次失败由上下文缺数造成；按用户要求留账，不为这种充分给证后的误述无限追跑。系统重复显示两份同cohort采样总体是去噪观察，未相加成328/518，不加新退出门。

MD SHA `ac95a425000b500ac56a7afc648bda9931d88bec77d5d5e7e107ee87fd715556`，HTML SHA `9194c784870a0de1705e47831898f26dd5ce269ec9c8088b47bbe849d7fe5c1f`；主HTML同字节、5表列结构正确，完整C++名与模板字符保留。HTML源审不是像素QA；原始日志非法UTF8不清洗覆盖。

## B：真实当前补证不倒签来源完成

[完整独立审计](results/hmc_endpoint_sample_registration_20261030_v1/sealed-v1/0177-codrax-hmc322-B-audit-v1.md)，SHA `b3cff9c2e382be4485873afc673bec5f5c2695a882d82a6c4af60914b7f7b588`。原入口 `TestNativeRegistrationLiveRestoredMissingTerminalReceipt`，仅此opt-in，真实planner12轮上限；恢复controller脚本化，不冒自然controller/fullCLI。

真实planner两轮完整读value.py/test_value.py后一次emit，changes=[]、精确server assertion_ref→test_value.ValueTest.test_increment→increment-result。声明不是测试结果。当前登记持久化后，父7246/子7364独立恢复，public RunTests重新执行，旧 `native:1c4e:18e347fe186ed3b8:1` 不复用为新 `native:1cc4:18e347ffa820dd20:1`。原实现/测试/HEAD保持，规划及执行授权均撤销。

本轮1断言/1通过/0失败/1native invocation，报告2命令是syntax preflight＋runner，不是两个测试；历史另1结果不相加。proof.probe_count=2是既有置信条目口径，不是新建两个probe。

source只有apply，无verify_ref/verify attempt/Completion；proof本轮verified，整体run.status=complete但Completion仍unverified/missing_terminal_verify_verdict。finish明确说明source结果数不可用；真正WriteFinalReport保留source无验证及整体unverified，不把本轮通过或旧report.passed回填来源终态。

父子各4原件/1attempt/1link：实际.final.json均26101B，SHA `af8f56836f5d7a34e0dfe3d41c8c6d8588e9671c480debf1053debd1f99d44c2`，精确引用当前内容寻址report SHA `c4fd5f83be8a6960c8ba3887eea17474cc2c2b1268c6cdee0fa4c64f2f6bbfaa`。不是根目录final-report.json兼容alias；全部8原件字节/SHA已核。

父子final plan/report相同，workflow不是字节相同：父后来补一个context-pack artifact_id/引用，执行与终态身份一致。不凭此签context-pack全字节耐久；本次也无实际Markdown、自然controller、其它runner或正向来源终态覆盖，不事后重渲染补证。只读证明限源码/测试/HEAD，不代替WAL操作系统零副作用。

## 发布与保留

唯一冻结全仓已通过，源/输入后置校验通过；[根裁定](results/hmc_endpoint_sample_registration_20261030_v1/sealed-v1/0392-codrax-hmc322-root-acceptance-v1.md)允许发布共享修复，但closure=none。426件本批与旧23档7703件保真；HMC321全仓及双人工失败原件不改。下一共享轨优先业务账户/聚合预览/IO三值/预算，完整能力轨轮换代表选择及具备材料的二进制入口；WAL仍生产NO-GO。
