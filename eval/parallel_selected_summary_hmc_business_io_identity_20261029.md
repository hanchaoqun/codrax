# HMC311：图局部修补身份统一与 IO 在途完整能力复验

日期：2026-10-29（UTC）。候选分支 `codex/hmc299-wait-interval-candidate`，起点 `df90d0a499a8cb635ecd10faa593cdac220b84e5`；main 保持 `788588e365d40a48b30827ffd8db08dae4e2ae9d`。本批恰好两例原自然问题，各一次，标准 read CLI；无第三例、模型回放或冻结后追修。

## 计数与双轨

根与独立代理逐稳定 ID 核对：**79 唯一＝26 已交付＋53 开放**，18 类仍有开放项。开放状态为18部分、29待实施、3待验收、2验收中、1持续；不把本批人工问题另造重复任务。完整能力新增0，候选已交付子能力1组（图完整提交/局部修补共用精确主显示身份），main 新增0。08.3 IO 在途的原生算法和24个分组数值通过，但完整自然答案解释未通过，不减少53；§310已交付06.1不重复计数。

缺陷轨先公开复现§310实际多行 `ensure_visible` 失败：业务首行＋完整原生线程次行在 full Emit 可用、在 atomic patch 前置却被拒绝。修复共用已有 `diagramEvidenceLabelSymbol` / `DiagramPrimaryVisibleIdentity` 的精确首行投影，原整标签及代码身份兼容路径保留。只证明可见身份，不授业务映射、线程归属、关系或根因权限；当前 retry lease、块指纹、唯一节点、端点/anchor/未证边界及合并后完整校验仍执行。

能力轨轮换08.3原自然 IO 在途题，不新增图、所有成员打印、实采来源、指定工具顺序等原合同没有的退出门。11.2代表任务仍需实际选择/执行同代终态；17.6缺新格式/工具/宿主真实材料，旧PERFILE2重新包装不算新覆盖。旧采样、IO、栈关系、权限覆盖和代表未触发继续留账；WAL未满足零副作用仍生产 NO-GO。

## 参考仓与泛化边界

核读参考仓 `core/llm_contract.py` 的共享成员/字段精确校验与恢复意图，以及 `core/preprocess/io_ops.py:114–179` 的20ms桶计数/均值实现。本项目复用精确类型化身份合同，不照搬文本子串、数值容差做身份硬门。现有 `internal/tracequery/io_inflight.go` 采用闭合合格配对、来源/层/族/设备/操作分组、半开区间扫描、完整查询窗分母及独立发起计数；参考桶均值不能替代时间加权并行度，本批不改正确原生算法来迁就错误解释。

保留 Trace 因果投影、自动补齐、原4项根因合同、31ms闭合睡眠与35ms请求驻留、50ms业务/52ms附件查询、链上合法2ms小计及47ms背景边界。不放宽旧断言，不扫描用户或模型原文设关键词硬门，不改 L1 调度循环。

## 公开回归、冻结与唯一全仓

公开路径为 EmitAnalysis→真实 TraceQuery→完整 Emit 拒绝→公开 EmitPatch→完整重验。测试用 helper 依据真实拒绝构造生产类型 retry lease，**不是实际 BaseAgent 模型重试循环**；自然 A 的真实工具记录另外验该入口。原多行 patch RED 保留，修后4顶层/20子例 PASS；反例覆盖第二行冒认、前缀、额外限定、错角色、旧引用、缺锚、错端点、错身份及缺未证边界，原业务名不模糊匹配。

相关四包249顶层/301子例 PASS，相关 race 同数 PASS；投影/睡眠/原4项等保护四包326顶层/335子例 PASS。原日志前缀 `/private/tmp/codrax-hmc311-graph-`，包括 `red-v1.log`、`green-v1.log`、`green-v2.log`、`related-v1.jsonl`、`race-v1.jsonl`、`protection-v1.jsonl`；命令收据 `commands-v1.json`。旧§310扩大 race 的分配 delta8>5/包超时仍留18.4，不被本批普通全仓或窄 race 的绿抵消。

唯一标准 CLI：`/private/tmp/codrax-hmc311-cli-standard-v1`，SHA256 `fe960a78514c144bc399c37f08a131133a4ec1cad6ecfe57f747e76396efede3`。冻结7495仓库输入、87外部输入、14仓外快照；freeze SHA256 `1a79e519b63a3baf780a77da68e2aa9e8f658c3d921d66da015f857989a15505`。冻结后源码、测试、注释和 helper 停写，仅新增审计记录及五文档。

唯一全仓 `go test -json ./... -count=1 -timeout=30m`：**98包 PASS、12无测试；28518顶层/33690子例 PASS；43顶层/14子例 SKIP；0 FAIL、0未终止、0重复终态**。原JSONL261101行、无非JSON行，SHA256 `209c15b6c3888d407fdcf5126221a07c177c997a98b3c9d530211781abd21095`。

全仓记录进程在任务中断时丢失，原 Ruby 父进程退出码/信号未知，不能声称正常完成。原 `go test` PID11617仍运行且PPID=1，未重跑：通过 Darwin `EVFILT_PROC/NOTE_EXITSTATUS` 只读观察同一PID，09:23:18.654899注册，09:23:49.679080收到真实内核 wait status=0、exit=0、无signal；最后包终态09:23:49.610728。开始09:12:15来自该PID的ps秒精度，不伪造原spawn微秒时间。`full-exit-observer-v1.json` 与明确 `recovered=true` 的 `full-v1-receipt.json` 保留中断、恢复和状态来源；不是代签原runner成功。结束后原冻结校验7495/87/14通过。

## 两例真实执行

| 项目 | A：业务时序图 | B：IO 在途完整能力 |
| --- | --- | --- |
| 原case | `trace_business_io_sequence` | `trace_io_inflight_overview` |
| 真实run | `A/trace_business_io_sequence-20261029-021217` | `B/trace_io_inflight_overview-20261029-021217` |
| 原附件 | `hmosperf_business_io_chain/events.systrace`，2312B | `hmosperf_io_inflight/events.systrace`，1839B |
| 记录进程区间UTC | 09:12:17.197696–09:16:54.689456 | 09:12:17.200008–09:18:43.034725 |
| CLI实际共同发射时刻 | 09:12:19.907906 | 09:12:19.907906 |
| 实际进程退出/机器检查 | 0 / PASS | 0 / FAIL：`no_primary_regex_match:(歧义\|无法唯一\|不能唯一)` |
| 独立原生JSON / 实际INIT | 9 / 3 | 3 / 3 |
| 最终MD | `.codrax/output/20261029-021651.351-11711.md` | `.codrax/output/20261029-021840.190-11710.md` |

结果根为 `eval/results/hmc_business_io_identity_20261029_v1`。两个原QUESTION、EXPECT、附件、15步read预算不改，oracle仅供审计，不入模型。实际机器1/2，过程退出码不等于case verdict；早期进度曾误据exit0说两例机器通过，此处按原 `run-1.verdict` 更正，不改原件。A/B各17个唯一配对完成请求，2个歧义响应排除；已证实际请求最大重叠32.981秒，不拿进程并行冒称模型并行。原件、实际参数、原生结果、三次INIT、最终MD/HTML及sidecars均采集；HTML为源码/表格/图语义核对，**不是像素QA**。

完整人工0/2；剩余验收4原因组为同源状态表重复、单边优先级外推、IO集合/物理解释越权及原图上CPU时点覆盖，均留旧ID。逐面 verdict、失败来源、冻结后处置及下一ROI见[人工审计](parallel_selected_summary_hmc_business_io_identity_20261029_manual_audit.md)。完整能力与机器通过分开计数，未触发不作成功。

## 原件封存与分笔发布

旧12批3341件/5640313157B、288快照源身份（270外部＋18历史污染身份）经独立字节核对不变；18个历史污染路径仍隔离，快照Go源码在仓外且用 `.txt`，不污染全仓。provider内容未读、未哈希、未复制。

本批按冻结 `close-v1.rb` 封存 **278件** 实际收据、原始失败、审计、输入和运行产物；索引SHA256 `705e2a998b762ff7e374ca2312d4433bc2eece0414f62605b77e3cc04de0ed31`。封存只证明字节留存，不推导验收；紧凑进程摘要的三个exit0不替代原case verdict。恢复全仓收据及observer原件分别保留，不能只看紧凑摘要省略的recovered字段。

候选代码两文件提交 `1b098068f2b473c019f2705ba352ecdd53168a61`，五文档单独紧邻提交，普通推送，不合main；文档提交SHA、远端/clean/两个提交同冻结字节及main未变，以结果根 `publication-v1.json` 为准，不用封存代签发布完成。

封存后发布前只读核账命令曾因本机Ruby不支持 `filter_map` 退出1，尚未进入stage/commit；改同义 `map.compact`，不改任务或断言。原输出单独保存在结果根 `publication-operator-diagnostics-v1.json`，属于278件封存之后的追加发布诊断，不冒产品RED或测试重跑。
