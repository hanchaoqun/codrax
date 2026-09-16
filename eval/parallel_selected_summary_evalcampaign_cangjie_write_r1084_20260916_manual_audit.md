# r1084：仓颉声明清单与 C 错误码传播写修复人工审计

- 时间：2026-09-16T10:01:34Z–10:05:50Z；清洁二进制 `8cb0cc7402e5`，构建时间 `10:01:09Z`。
- 严格 `CAP=5 / PARALLEL=2 / TIMEOUT=1200`，两例各一次，无第三例或追绿重跑。
- 原始机评：[本批汇总](parallel_selected_summary_evalcampaign_cangjie_write_r1084_20260916.md)。保留其原始内容，人工判断不倒填机评。

| 场景 | 机器 | 人工 | 外层耗时／case 耗时 | 核心结论 |
|---|---|---|---|---|
| `cangjie_repomap`，read15 | PASS | PASS | 255s／252s | 2 extend、2 foreign func、8 public class 及全部路径、包声明、引用正确 |
| `github_issue_libgit2_foreach_worktree_symptom`，apply24 | FAIL | FAIL | 232s／230s | 未接受计划、未应用、未正式验证；原缺陷仍在，不是业务修复已交付但证明不足 |

## 1. 选择理由与验收边界

按用户影响、模式/语言稀疏性、最近覆盖和本机原生验收能力排序：上一批 TS 与 Trace 双读，本批补全仓仓颉声明读题及真实 C 编译可验的写题。仓颉全仓版区别于近期隔离 fixture；写题可能触发 replan/source owner/累计验证，但不得预签其自然命中。本对无图、无 Trace、无成功写应用；图表修复、显式窗因果投影、IO 修复与累计写证明的生产验证均为 **N/A**。

## 2. 仓颉读题：答案和上下文正确，保留调度冗余观察

结果目录：`eval/results/cangjie_repomap-20260916-030134`。原答案：`.codrax/output/20260916-030546.535-5489.md`；同名 HTML 保持不动。以下日志行号均指该结果目录的 `run-1.logs/codrax-20260916-030136-000-5489.log`。

人工核对 11 个受管 `.cj` 文件，所问声明分布于 8 个文件：

- `extend String`、`extend Cart`，共 2 项；Cart 的 class 与 extend 是不同声明，不混并。
- 两处 `foreign func native_add` 位于不同文件和 `demo.ffi`／`demo.bridge` 包，分别保留。普通 public func、注解及回调未误当 foreign 声明。
- Bridge、Greeter、Cart、Version、App、Animal、Dog、Service，共 8 个 public class；其中 sealed 1、abstract 1，未误收 struct、interface、enum 或 internal class。
- 最终 12 行的完整文件路径、行号、符号与 `package` 声明逐项一致。12 个引用片段与当前源码对应行逐字一致；包路径不是目录猜测。

模型在原始成文中自写摘要和三张表，首次成文即接受，零 finalizer 拒绝/patch。系统补齐只涉及已有明确源码位置的空 quote（日志 L2587、L2677），不代写成员、包名或正文。`source_inventory`／`repo_map` 已足够供给清单，不能因为 read_file 为 0 推断缺证。

主日志只有 **1 次独立 investigation completion 拒绝**（L1271–1272）：模型提交 public class `value=9`，同一提交的成员只有 8；L1304 改为 8，L1309 接受。此前 L1193–1205 已完整供给 12 项，属于模型填值错误，不是系统漏声明。人工脚手架聚合 `inv=4/2` 不能当作两个独立缺陷计数；不修改原自动指标。

三次 explorer dispatch（L677／998／1384）分别覆盖 lens、breadth 与五个 evidence objectives，后两轮未增加源码、重复提交已有清单。本轮作为既有 **B1586c P2** 机械枚举记账/窗口调度冗余的见证；不因此无条件跳过所有后续探索。摘要中的“fixture 和 thirdparty 源类”为模型自写、事实范围正确但业务可读性可改善，不加原文关键词门。

原 oracle 对预期成员有判别力，但 basename 包含匹配及从预期成员回填计数不能独立证明完整路径、逐字引用和额外成员边界；本次 PASS 依赖上述源码逐项人审，不是仅凭机评。

## 3. C 写题：正确拒绝不等于成功交付，失败原因不是超时

结果目录：`eval/results/github_issue_libgit2_foreach_worktree_symptom-20260916-030134`。以下日志行号指 `run-1.logs/codrax-20260916-030136-000-5506.log`。

原缺陷将比较结果赋给 error，而不是先取得返回码：回调的任意非零原始返回值必须保留并优先返回；回调为 0 时，lookup 的负返回值保留，非负值正常返回 0。旧实现把这些失败压成 1。

三次 fresh planner dispatch 各提交三个计划，共九次拒绝；没有 accepted plan、coder、apply 或 verifier。第一次空失败信号探针无效；后续是不存在路径与 `old_text` 不匹配。第六个草稿的两条源码修复已正确，但测试仍基于虚构的参数/API，整计划原子拒绝，这两条源码从未应用。一次 changes 字符串到数组的结构兼容恢复（L1325）未篡改可见业务事实，不是 JSON 无法解析导致答案消失。

`run-1.write-apply.json` 的 `plan_written=false / apply_attempted=false` 与实际目录一致。交付 repo 仍为 seed `bd1fc27550ed7b5a57242c8ba3bd81afdebd36c2`，源码、原测试、Makefile 哈希均与 baseline 相同；只有 harness 的未跟踪 `.gitignore`。正式 proof、owner、累计验证均 **未运行／N/A**，不是验证失败，也不是“修复正确但证明弱”。CLI 正常退出不抹掉 case FAIL。

审计者后验直接严格编译实际交付目录：原 4 断言为 1 过 3 败；另以 `INT_MIN,-42,-1,0,1,17,INT_MAX` 的 7×7 输入矩阵核对，49 格为 **11 过 38 败**。矩阵只覆盖该约简 API 的 callback/lookup 返回码，不声称覆盖不可注入的 open 失败。独立探针放在 `.codrax/tmp/20260916-r1084-libgit2/`，没有提供给模型、修改原 fixture 或补签正式 proof。完整命令与退出码见同目录 `baseline_receipt.txt`、`post_delivery_audit.txt`。

### 3.1 上下文与读取预算：模型误用为主，恢复携带另留观察

1. `write_analyzer` 在 L698／L726／L763 读过完整原测试、33 行源码与 Makefile。这不是 planner 的成功读取；不能声称“planner 看过全部源码仍忽略”。planner 获得正确 scope/expected paths，同时又收到 controller 在 L1041 自填的陈旧 `src/repository.c`、`tests-legacy/worktree/worktree.c` 候选。
2. 第二次 planner 的 `repo_map`（L1713）明确给出根目录路径及真实 `(int,int)` 签名，模型却在 L1749 重读错误路径。这是已有精确定位仍被误用，不是系统从未供给路径。
3. L1755 达到既定独立失败预算：success=1/3、failure=2/2；随后 L1772 正确读取 `repository.c` 的请求确被当前工具范围拒绝。不能忽略这一政策后果，也不能误报旧 B1431“失败与成功共用计数”回归。
4. L1831 的结构化 emit 拒绝后，L1836 工具由 4 个恢复到 8 个，当前代码及 `TestPlannerFilterToolSchemas_StructuredEmitRepairKeepsReadTools` 已有明确回读出口；模型未使用，而继续猜测试内容。因此本例**不是已证的合同无出口、自相矛盾或新 P1**。
5. 第三次 fresh dispatch 再供陈旧 candidate_paths（L2080），仅携最新 test L14 修补（L2095–2119），不累计先前 source L24/L28 与定位事实（L2121 为 fresh messages=4）。记录为 **B1431 后续有界上下文保留观察**：需公共复现确认跨重派发的精确定位/当前字节如何安全保留，暂不升 P1、不写成已修。

下一步候选设计是按文件身份、新鲜度和批次保留有限 typed 定位/修补事实，区分已验证实际路径与低信任候选；必须保护改动后失效、多候选歧义、跨批隔离及已有读取预算/emit-repair 出口。不提高全局预算、不从正文猜路径、不放宽 exact old_text 或证明门，也不针对该 C 文件特判。

本轮实际等待配置为首响应 600s、静默 300s、非流式 600s；终止是结构化计划拒绝耗尽，不是流式超时。活跃流不会仅因 4ms 或旧 4min 没有可见正文而降级；取消及独立总预算仍是不同条件。最终通用“请把目标说具体”不能反证原请求不清楚，留作低优先可读性观察。

## 4. 原件封存与后续优先级

- 20 项 case/fixture/源码语料/runner：`.codrax/tmp/20260916-r1084-cases-fixtures-runner-before.sha`。
- 4764 个受测 Go/build 输入：`.codrax/tmp/20260916-b1712-final-build-inputs.sha`。
- 原结果、答案和机器汇总：`.codrax/tmp/20260916-r1084-results-original.sha`、`20260916-r1084-answers-summary-original.sha`。
- 未改生产源码、原答案、原机评或原 oracle；审计阶段只写统一账本与本文，校验结果另在统一账本收账。

优先将 B1431 跨重派发有界定位保留作小范围公共复现，再决定是否值得实现；B1586c 调度冗余及模型用词低于错误修改/证据丢失风险。不得因本对无图/Trace 就销清完整关系表达、显式窗、链上根因、IO 及跨语言原生证明的开放债。
