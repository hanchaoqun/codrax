# HMC299 人工审计：sample-only 与普通命名 gzip

两例使用原自然问题、各一次真实模型运行，无追加防错问题或追绿。两者是同一真实 PERFILE2 采集的原件与既有 gzip 包装，不能作为两个独立设备／新格式覆盖。全仓、冻结、机器 verdict 和人工内容 verdict 独立登记。

## 输入和独立核算

A：`/private/tmp/codrax-hmc299-sampling-v1/input/test_brbe.data`，30,062,585B，SHA `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`。参考源 `tests/fixtures/hiperf_brbe_001/test_brbe.data`。B：`/private/tmp/codrax-hmc299-gzip-v1/input/capture.gz`，3,141,140B，SHA `8b81184667832f2e8e46373de75313d4cd8792a789a5727aa823b80479e99591`；完整解压 CRC／EOF 通过并得到相同 A 字节。没有为模型预置准备结果或查询目录。

独立 oracle 是既有 HMC296 对相同 SHA 原件的参考标量 parser 结果，本批未重跑 parser、不称新 BRBE／SPE 解码：全量 12000 记录、55 个进程／线程对、权重 43037682；指定窗口全线程 1393／2455、11 对；目标 tid1609／pid1514 为 **164／259**。CPU0 为 82／85，CPU1 为 82／174。该 oracle 独立核时间、PID/TID、CPU及权重，不独立验证符号化或完整展开；热点的 OnDataEvent_ 完整符号／libsensor_stub_3.0.z.so 与 `0xffffff800b829d24`／`[kernel.kallsyms]` 来自本轮完整原生结果，不扩大 oracle 证明范围。

两记录时刻包络为 `[486489464088122,486489465612497]ns` 与 `[486489588095622,486489589609372]ns`，只支持所选记录的首末时间，不是连续运行或阻塞时长。每个热点仅一个 IP 栈帧；depth0 不能提供调用方向或完整栈。官方适配器不可用后的 raw fallback 成功及降级边界均需保留。

## A：机器 PASS，完整人工 FAIL

原目录 `results/hmc_sampling_gzip_entry_20261026_v1/A/trace_binary_sample_only-20261026-233714/`。真实完整准备和取数成功，目标 164／259、热点 82／174 与 82／85、10ms 分桶等限定事实正确。正确原生表和实际 Finalizer 上下文已经携带 unknown、权重单位、记录包络及非执行状态边界，不能归因为全部事实缺失。

最终主答案仍有明确反例：

- 将两包络跨度约 3.037ms 与 200ms 窗相减，称余约 197ms“几乎没在 CPU 上跑”，又以 122.484ms 记录空白定位停顿。未采到不等于未运行，记录包络不等于 CPU 耗时。
- 断言“几乎没有消耗 CPU”“符合不可运行／阻塞、被调度器摘走”，同时又说明样本类型未知；正文和原生事实自相矛盾。
- CPU0／CPU1 记录位置被写成已经发生调度迁移，没有调度关系证据。
- 将 486000–487000 秒写成“一秒”，实际跨度是 1000 秒。原生数值不因此改写，错误仍留原件。

表格正确不能抵消正文。MD／HTML 的完整主答案均需按同一事实边界审计；无实际 Mermaid 图，不签图形自然覆盖。上游 emit reason 中的同类错误没有进入实际 Finalizer InvestigationSummary，不能据此把本轮正文错误归咎为该摘要已传播。18.3 原 sample-only 诚实边界仍 FAIL，不以 B 新问题正确回签，也不称重复错误是已证模型偶发。

## B：普通命名 gzip 有限任务 PASS

原目录 `results/hmc_sampling_gzip_entry_20261026_v1/B/trace_gzip_named_binary-20261026-233714/`。没有 `--htrace`：自然命名路径→安全导航→真实共享准备完整 12000 记录→6 次成功原生查询（1 次 perf_stats，5 次 event_search）。两次 shell 尝试被只读规则拒绝，随后合法原生查询继续；不把被拒 shell 计为成功查询，也不误判真实源坏或查询终态失败。

实际 Finalizer 与最终正文保目标 164／259、CPU0=82／85、CPU1=82／174、unknown、两 depth0 单帧、原始来源／准备成员及正确记录包络。正文区分记录位置和执行状态，权重 cycles 不换算毫秒，单帧不扩成调用关系。HTML 四个主区表与原生事实相符；检索明细 20/164、CPU 分别 1/82 的预览与完整聚合分开，没有用前几行外推完整时域。

调查原文出现过 82／174 混写和“证明内核态执行路径”，但实际 Finalizer INIT 与最终 MD／HTML 未投递或复述；保该上游错误，不谎称已污染本轮终稿。root-causes 为空且 contract_not_active，未授因果权限。本有限问题 PASS 不签旧停顿题、其它格式／设备、跨源或图形 QA。

补齐阶段为 `families_present`，没有额外引擎查询，故缺失事实自动补查自然触发 **NOT_TRIGGERED**。两例均为 sample-only，新增睡眠量尺交接自然分支亦 **NOT_TRIGGERED**。本批不调用第三例补覆盖。

## 交叉复核和结论

两个进程各 exit0，冻结输入前后均一致；实际模型请求重叠 22.348 秒，不仅是 shell 启动屏障。完整人工 1/2，完整能力新增 0，仍 25 交付＋54 开放。公开确定性回归证明新增状态区间可到达实际成文上下文，不代表自然答案已验收，原 IO 请求驻留→设备驻留错误也未在本轮复验。

独立逐阶段审计保留于 `/private/tmp/codrax-hmc299-A-audit-v1.md`、`/private/tmp/codrax-hmc299-B-audit-v1.md` 及各自 extra-artifacts 清单，随本批封存。唯一全仓失败和后继工作见[批汇总](parallel_selected_summary_hmc_sampling_gzip_entry_20261026.md)；不因内容 B PASS 掩盖结构／carrier 登记及闭合睡眠投影回归失败，也不在冻结后修改代码／测试补绿。候选仅保存在独立 codex 分支，未进入 main，不签完整交付。
