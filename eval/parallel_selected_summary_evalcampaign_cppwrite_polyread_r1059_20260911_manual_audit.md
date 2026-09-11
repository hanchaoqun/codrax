# r1059 人工审计：C++ 原生写与 Python/Rust 跨语言读

- date: 2026-09-11T06:54:21Z
- sweep_start_ts: 20260910-235420
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

运行基线为 `f6e64a545d01`，构建时间 `2026-09-11T06:53:46Z`。两路各一次，实际运行 `06:54:21Z–07:14:23Z`；源码/测试在 live 全程冻结，无第三路、追跑、改 oracle 或中途调预算。下列机器字段保留 runner 原值，人工判定另列。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260910-235421 | answer_regex | none | 481s | 55 | read=7,repo_map=2,list=2,trace=0,source_lens=1 | midloop=7,inv=3/0,fin_reject=5,unavail=0,prune=0 | fail | 主链与 ImportError 回退大体正确，但模块身份错误、概念终点自相矛盾、引用偏移；最终无图，不能签图渲染通过 |
| 1 | github_issue_nlohmann_long_double_symptom | TIMEOUT | eval/results/github_issue_nlohmann_long_double_symptom-20260910-235421 | write_apply,write_patch_oracle | none | 1200s | 28 | read=43,repo_map=3,list=7,trace=0,source_lens=0 | midloop=4,inv=0/0,fin_reject=0,unavail=2,prune=0 | fail | 原验证三次 SDK 链接失败且 proof 保持 failed；最后双头 Lf 改变数值格式，独立原生后验失败，不回填原 TIMEOUT |

## 1. 跨语言读：机器通过不能代替关系与事实审计

工件：`.codrax/output/20260911-000220.757-57976.{md,html}`；以下日志行指 `eval/results/mr_poly_binding_chain-20260910-235421/run-1.logs.all.log`。人工逐条核 Python/Rust 源码、完整答案和成文/patch 过程；相关源码与 fixture 保持相同。

- 主体已说明 `FastTokenizer.tokenize → _fastlex.tokenize_bytes → PyO3 wrapper → Rust core → best_merge`，并正确把 `ImportError` 作为纯 Python 回退的触发。没有把无条件编译/native 安装成功作为已证事实；fixture 的 Cargo 配置并无本次 Python feature 构建证明。
- MD28 将模块标识符写成 `py::PyModule`，实际导出入口是 `py::_fastlex`，`PyModule` 是参数类型。MD17 的 `best_merge → MergeTable.rank` 引到 lib.rs:22，真实调用在23；精确引用修正建议已入模（日志3842/4197），不能认定系统没提供代码证据。
- MD50 的“概念目标核对”选 `best_merge → ids.len` 并称尚未确认到达 Rust 实现，与正文已解释 wrapper→core 不一致。该选择由模型最后 patch 提交（日志4256），不是系统代选终点；不得为修正这次误选扫描正文并替模型改结论。
- wrapper 的 `MergeTable::from_triples` 再调用 core 的先后关系已有上下文（3702/3781/3789），最终省略。最终另罗列多个低层 API 操作，不等于完整跨语言链；这些额外关系同样是模型选择（4191），不是系统自动加边。答案对“完全等价、只损性能”的主张也缺本次完整等价性验证。
- 最终 **没有 Mermaid 图**。模型在成文第5轮 patch 删除 `d-call-chain`（4107）；最终仅保留用户请求的 text fence，图语法/渲染验收为 N/A。`mermaid_source_repair_applied=1` 是中间草稿收据，不可当最终图验收。机器宽关键词 oracle 因此仍 PASS；本批不改 oracle 追绿。
- 5次成文拒绝、6次 patch、7次 finalizer 迭代；总上下文109710/200000（55%），未见预算耗尽。注册不是运行期调用的教学已在初始上下文（3646）；失败不能全归模型缺知识，也不能全归系统。

### B1663：真实系统缺口是错误定位，不是摘要被合同禁止

日志3968一次追加两块：首项是缺失的 `s-summary`，第二项是已有 `d-call-chain` 整图。原二进制已经允许补齐 required block，但整批资格失败后统一把 `AddBlocks[0]` 报为违规（3972），因而错误指向合法摘要。模型随后推断摘要无法追加（3999–4017）；后续单独摘要追加实际成功（4211），最终 patch 亦接受（4272）。

代码依据为 `localDiagramLeaseWholeBlockMutationViolations` 与 `requiredAnswerBlockAdditionsAuthorized`：第二项不减少 required deficit 才是真失败项。最小修复只让同一资格遍历返回真正失败位置，保持 bool 资格、JSON/schema、原子失败与模型已接受文档不变，见统一台账§123.1767。**不能由此断言该误报必然导致删图或解释全部5次拒绝；也不把这次日志裁为“摘要同时必带、必拒”的无解硬合同。**

## 2. C++ 写：环境失败、模型补丁错误与证明保护分开记

结果目录 `eval/results/github_issue_nlohmann_long_double_symptom-20260910-235421`。以下 L1/L2 分别指其中 `run-1.logs/codrax-20260910-235422-000-57952.log` / `run-1.logs/codrax-20260910-235737-000-58246.log`（两个 controller 进程日志属于同一次 selected case，不是重跑）。

1. 原 case 要求双发布头修正 long double 严格编译且不损数值格式。原测试仅检查 `1.25L` 输出非空；oracle 检查 `%.*Lg`，人工另核实际行为，未放宽原测试/告警/字段期待。
2. 首个 `%.*Lg` 候选还修改受保护测试而被拒；模型随后主动提交 `%.*g + static_cast<double>(x)`（L1:2533，2554接受），此前源码和系统指令没有建议 cast。后续 durable goal 回显来自该模型计划，不能倒认为系统先倡导窄化。最终补丁又改为 `%.*Lf`；因此不能以中间 cast 代替最终作品评判。
3. 共12个 planner阶段、31次 emit结果（26拒/5接受）；两份已接受候选仍因 protected-test/offscope 被阻止，实际 apply 3代。第二代只有空提交；最后持久 ref 为 `refs/codrax/applied/plan-1789110580851920000-58246`，commit `2295b34358a0ac8bce7a0d6a00a1854b231b6b2a`。与初始 `9b231241e12b44dc02b0b258146b04c5d2dbd0b0` 相比仅双头 `%.*lg → %.*Lf`，原测试/Makefile/README 未改；隔离仓 main HEAD 也未被合并改动。
4. 原生 `make check` 实际3次，L2:689/4410/8097，均 exit2：默认 SDK27 的 `.tbd` 无法被当前链接器读取。此次 live 未显式传入 Go/build 已在使用的 SDKROOT26.5，是跑批环境前提遗漏；不将该链接错误说成源码格式失败或 B1661 回归。原测试程序没有在这三次验证中执行成功。
5. 实际8次 planner probes，4失败、4 exit0。4次成功回包（L2:5294/6217/8992/10647）均是 `observation_only`，明确不能授权空 changes 或正式行为证明。模型称“已验证”是过称；持久 proof 仍 failed，系统未把观察值铸为 PTO/正式覆盖。这里0个 typed 硬合同不等于程序没运行；planning_only 计数不能当正式验证计数。
6. 最后快照 `plan-1789110580851920000-58246.final.json` 是 `in_progress / verify_failed / failed`，非最终成功报告。L2:10654收到 runner TERM；case raw exit124/1200s，machine为 `TIMEOUT selected_eval_worker_incomplete`。runner整体exit0不覆盖case失败，也没有模型最终 block/完成答。

### 原生独立后验：原测试绿，格式语义确实失败

在独有 `.codrax/tmp/r1059-nlohmann-durable.Inr26W/source` 展开最后持久 commit；5个源 blob与展开文件逐个hash一致，验收前后SHA一致。原结果目录/fixture未改，不发起新模型 eval。用显式 SDK26.5、原 `-std=c++17 -Wall -Wextra -Wformat -Werror` 跑原测试及先前有效编译RED的同一个独立审计器：

| 验收项 | 结果 |
|---|---|
| 原测试与独立审计器严格编译 | 均 exit0 |
| 原 `tests/long_double_format.cpp` | exit0；只检查非空，不能发现格式变化 |
| 双头×2类型×14值×6精度×7缓冲组合 | 2352项，644通过、1708失败，exit1 |
| precision=3, 1.25L | 应为 `1.25`/长度4，实际 `1.250`/长度5 |
| precision=6, 1e12L | 应为 `1e+12`/长度5，实际 `1000000000000.000000`/长度20 |
| precision=6, 1e-12L | 应为 `1e-12`/长度5，实际 `0.000000`/长度8 |

收据：`.codrax/tmp/r1059-nlohmann-audit.Zk7wDg/delivery-run.OvyiuX/{status.tsv,format-audit.log,source-before.sha256,source-after.sha256}`；字节例在 `.codrax/tmp/r1059-nlohmann-durable.Inr26W/format-samples.log`。原始基线 RED 在同审计根的 `baseline-run.zA66WU`：两个编译均因原 `%.*lg`/long double 的严格格式告警失败，非审计器自身错误。

本机 ARM64 的 double/long double 都为8字节、53位有效精度，不能外推更宽 long double ABI；最终 Lf 的格式回归已在本机直接证明，无须依赖中间 cast 或跨ABI猜测。**后验不回填原流程的 TIMEOUT、SDK27链接失败或 failed proof。**

### 恢复策略：记入 B1561，不扩大为无出口合同

泛化缺口是恢复建议没有清楚区分当前 adapter 与 probe 的能力：Make 只有 aggregate结果，没有本 fixture 当前可授逐断言 PTO 的 native通道；笼统“加native测试/PTO”和 noop 后“先跑typed probe再发空changes”的建议未说明这条路径的限制。模型因而反复尝试不支持的 cpp、外部编译包装或空计划；该循环是既有 B1561 的新见证，不新增按模型措辞强制 block 的门。

controller **确有 block 出口**且失败保护允许，当前教学主要列结构安全/预算情形，环境/能力阻断说明不足。模型的 finish在真失败后改为replan是正确保护，不能放宽 accept_unverified 掩盖失败。后续高ROI方案应基于本轮已有 runner/probe 能力事实给出软恢复选项；不承诺任意语言都有逐断言接口，不自动改模型计划/答案，不绕过受保护测试/审批/隔离。

## 3. 红线与后续顺序

- 本批机器1/2通过，人工两份均不签绿；不为单场景追跑或增加关键词答案门。上下文28%/55%，没有证据要求加大上下文或继续重试同类不可用探针。
- 先闭 B1663 精确失败定位；B1651b Gradle当前执行来源、B1662 Meson命令/原生报告语义仍P1，但本机缺所需工具链，未安装、未签native。B1561按能力事实改善软引导另案，不把观察性探针升级为正式证明。
- 本轮是源码读/写，未自然命中 Trace；不得倒签显式窗/投影生产回放。链上根因、IO/D/有证S等待、业务/语义线索及背景隔离均未改。
- 活跃流专项与分帧专项原参数count3通过（`.codrax/tmp/20260911-r1059-{active-stream,partial-frame}-count3.log`）；连接持续推理/工具参数/heartbeat/分帧字节时不按4ms或旧4分钟无可见答案降级。C++本轮1200秒是预先设置的外层case硬预算，不是活跃流误判；真实停滞、取消和明确调用方截止仍有效。受控专项不等于单条真实客户流持续4分钟的生产见证。
