# 恢复后的只读测试登记：真实模型验收入口

此入口验证已交付源码之后的补证分支，避免普通 apply 问题偶然没有进入登记路径，却被当作该路径已通过。问题是“实现已修好，请运行保留的现有测试，确认返回值正确。不要再修改源码、测试、配置或依赖。”内部计划、断言身份和授权由当前系统上下文提供，不加进用户问句。

## 自然控制器终态：native_registration_natural_controller_terminal

此独立入口复用下方正向终态的原始 fixture、自然问题、公开源码准备和来源状态；父进程与恢复后的子进程均使用真实 `write_controller` provider、默认 controller skill 和公开决策工具，登记 planner 也是真实模型。模型选择 verify/finish/block 等动作，系统只按已有 typed 状态派发；不改写模型动作、不预填 verified、不用 ordinary commandless 的成功代签登记。

```sh
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk \
CODRAX_LIVE_REGISTRATION_NATURAL_EVAL_DIR=/absolute/new-result-directory \
  go test ./internal/orchestrator \
  -run '^TestNativeRegistrationLiveNaturalControllerTerminal$' -count=1 -timeout=20m -v
```

仅该新环境变量启用此付费入口；不要同时设置两个旧 live 变量。需 Git、Python 3 及已配置的 planner/write_controller provider，可用 `CODRAX_LIVE_PROVIDERS` 指定配置路径，凭据不复制到原件。输出父目录须存在、结果目录须全新。默认测试仅 SKIP，不调用 provider；两个 `TestNativeRegistrationNaturalController*Wiring` 用确定性 adapter 检查真实 agent/skill/公开工具接线，不冒称真实模型验收。

源码初始修复、原合同、Git 交付及旧测试执行由 operator 通过原有公共 fixture 准备，不是模型自然生成；verify 阶段由公开 `RunTests` 真执行，不另调用 verifier 模型，受限源码读取复用原 fixture。登记计划持久化后在现有边界重启，子进程只接路径/期望与 provider 配置路径，仍由生产 load/hydrate 恢复 IR/权限。**这是实际模型控制器＋planner＋跨进程恢复的有界验收，不是完整 CLI、自然代码创作或所有阶段模型化。** 子进程的模型调用属于同一问题恢复，不是额外第三例。

只有登记确实发生，来源批、补证批、整轮均 verified，新执行不复用历史 invocation，原实现/测试/配置及 HEAD 不变，才可通过。父/子模型消息、公开发射、默认持久化与实际执行均需人工核对；模型 block、错误选择、权限拒绝、预算或终态缺口如实 FAIL，不用脚本纠偏追绿。保留 `evaluation.json` 的阶段边界、父日志 `logs/`、子日志 `child-logs/`、各原件以及原有三层 receipt；模型 reason 不代替 typed 完成事实。自然验收不得与下方旧 scripted-controller 收据混称。

## 正向终态：native_registration_restored_terminal

该入口先让实际 controller 完成来源批验证，再通过真实 planner 登记既存测试，持久化后启动独立进程恢复并重新执行。它与下方旧负控分开，不能用补证批 PASS 倒签来源批。

在仓库根目录、代码和测试冻结后运行；需已配置 planner provider、Git 和 Python 3。输出父目录必须存在，输出目录必须是新的绝对路径。默认全仓测试跳过此付费模型调用，只有显式环境变量才启用；不要在普通全仓测试命令上设置它。

```sh
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk \
CODRAX_LIVE_REGISTRATION_TERMINAL_EVAL_DIR=/absolute/new-result-directory \
  go test ./internal/orchestrator \
  -run '^TestNativeRegistrationLiveRestoredTerminal$' -count=1 -timeout=20m -v
```

provider 默认读取项目 `providers.yaml`，可通过 `CODRAX_LIVE_PROVIDERS` 指定，不在报告中复制凭据。本例占一例真实评测名额，与另一例冻结 CLI 评测同时启动，各运行一次；不能另追跑第三例求绿。不要同时设置旧负控环境变量，精确 `-run` 只选上方入口；子进程只执行恢复 helper，不发起另一次模型规划，因此不是第三例自然评测。

## 验收边界

- 前置：公开计划发射与应用、真实 Git 提交和旧测试执行构造已交付源码；旧执行没有新登记，不能证明新合同。正向入口还运行实际 controller 的 source verify/post-hook，来源批必须具有匹配 source plan/report 的 `verify_attempt` 终态；系统依据 typed 缺证状态进入只读登记，而非夹具填入 verified。
- 模型：真实默认 planner、默认 planning skill 和公开读取/两个发射工具；模型必须读取当前测试，自行选择断言与原合同的对应关系。
- 恢复：用默认 `WriteWorkflowRunStore` 保存登记计划和工作流，再启动独立测试进程；子进程只收到路径/验收期望，不注入 IR、私有授权或报告。生产 load/hydrate 恢复，脚本化 controller 决策驱动 verify-only→finish，公开测试工具再次真实执行。
- 通过：登记不修改任何文件；原合同获得新 invocation 的精确断言见证；执行归属原源码交付；授权撤销；源码、测试及 HEAD 不变。新执行不能复用旧调用或来源 controller 的调用。正向入口同时要求来源批、补证批及整轮 `workflow_completion.verdict=verified`，不是只检查进程成功。
- 留档：evaluation、来源计划、旧报告、重启前计划/工作流、最终计划/报告、工作流、源码/测试及模型日志。正向入口另存 `source-verify-report.json`、`source-verified-workflow.json`，供人工将来源终态与补证终态分别核验。只有本入口断言全部通过才写 `receipt.json` 的 `passed=true`；单列 `source_terminal_verification`、`proof_batch_verified` 与 `workflow_completion`，`durable_run_complete` 不等于 verified。人工仍须审计实际工具顺序与身份。缺依赖 SKIP 不算 PASS。

这是**真实 planner 阶段＋新进程/默认持久化存储/生产调度器的恢复验收**，不是完整 CLI、真实 controller 模型决策或最终回答验收。controller 决策仍脚本化为 verify/finish；源码准备与独立恢复不调用额外模型。完整 CLI 收尾、多来源及其他原生框架继续独立验收。

## 保留负控：来源批缺终态

原入口 `TestNativeRegistrationLiveRestoredFollowup` 与环境变量 `CODRAX_LIVE_REGISTRATION_EVAL_DIR` 保持不变。该 fixture 只有真实源码交付及旧执行，来源批没有终态验证凭证；新补证批次必须 verified，整轮仍须保留 `unverified / missing_terminal_verify_verdict`。不得将它改成期望 overall verified，也不要在同一固定双例批次额外运行这个旧 live。

无需模型的 `TestNativeRegistrationLiveTerminalPublic` 与正向 live 共用来源验证/规划/持久化准备路径，随后走相同的新进程恢复；`TestNativeRegistrationLiveTerminalMissingSourceRemainsUnverified` 及原 `TestNativeRegistrationFreshProcess` 保留缺来源终态负控。定向回归可以匹配 `^TestNativeRegistration`，但必须不设置任何 live opt-in 环境变量。

2026-10-07 冻结 `91aaa9be3` 的原评测已暴露新补证批次被错误附加源码定位义务，原始 workflow 保留 `verification_proof_incomplete`，不能按新断言重新签绿。后续 `db54efcd2` 公共新进程回归修复该接缝；未追加第三例真实模型重跑。

上述“未追加真实模型重跑”仅指 §203 当批。后续独立 §206 已运行一次 `TestNativeRegistrationLiveRestoredFollowup`，真实 planner＋独立恢复子进程限定人工 PASS：新补证批 `verified`，来源批仍无终态凭证，整体诚实保留 `unverified / missing_terminal_verify_verdict`。实际父/子 PID 为44867/45788，旧/新 invocation 不同；controller 决策仍脚本化，`full_cli_run=false`。见[§206完整人工审计](parallel_selected_summary_hmc_native_sql_cpu_20261007_manual_audit.md)、[原始收据](results/hmc_native_sql_cpu_20261007/native_registration_restored/receipt.json)及[原始工作流](results/hmc_native_sql_cpu_20261007/native_registration_restored/workflow.json)。这份后续收据不回签[§203原始FAIL](parallel_selected_summary_hmc_restart_graph_20261007_manual_audit.md)，也不覆盖真实自然controller的缺来源负向决策、完整CLI、多来源或其他框架。此后再次运行相同负向入口属于当前版本轮换回归，不算首次能力补齐。
