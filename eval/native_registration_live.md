# 恢复后的只读测试登记：真实模型验收入口

此入口验证已交付源码之后的补证分支，避免普通 apply 问题偶然没有进入登记路径，却被当作该路径已通过。问题是“实现已修好，请运行保留的现有测试，确认返回值正确。不要再修改源码、测试、配置或依赖。”内部计划、断言身份和授权由当前系统上下文提供，不加进用户问句。

## 执行

在仓库根目录、代码和测试冻结后运行；需已配置 planner provider、Git 和 Python 3。输出父目录必须存在，输出目录必须是新的绝对路径。默认全仓测试跳过此付费模型调用，只有显式环境变量才启用；不要在普通全仓测试命令上设置它。

```sh
CODRAX_LIVE_REGISTRATION_EVAL_DIR=/absolute/new-result-directory \
  go test ./internal/orchestrator \
  -run '^TestNativeRegistrationLiveRestoredFollowup$' -count=1 -timeout=20m -v
```

provider 默认读取项目 `providers.yaml`，可通过 `CODRAX_LIVE_PROVIDERS` 指定，不在报告中复制凭据。本例占一例真实评测名额，与另一例冻结 CLI 评测并行，各运行一次；不能另追跑第三例求绿。

## 验收边界

- 前置：公开计划发射与应用、真实 Git 提交和旧测试执行构造已交付源码；旧执行没有新登记，不能证明新合同。
- 模型：真实默认 planner、默认 planning skill 和公开读取/两个发射工具；模型必须读取当前测试，自行选择断言与原合同的对应关系。
- 恢复：用默认 `WriteWorkflowRunStore` 保存登记计划和工作流，再启动独立测试进程；子进程只收到路径/验收期望，不注入 IR、私有授权或报告。生产 load/hydrate 恢复，脚本化 controller 决策驱动 verify-only→finish，公开测试工具再次真实执行。
- 通过：登记不修改任何文件；原合同获得新 invocation 的精确断言见证；执行归属原源码交付；授权撤销；源码、测试及 HEAD 不变。旧 PASS 不能代替新执行。
- 留档：evaluation、来源计划、旧报告、重启前计划/工作流、最终计划/报告、工作流、源码/测试及模型日志。只有本入口断言全部通过才写 `receipt.json` 的 `passed=true`；单列 `proof_batch_verified` 与 `workflow_completion`，`durable_run_complete` 不等于 verified。人工仍须审计实际工具顺序与身份。缺依赖 SKIP 不算 PASS。

这是**真实 planner 阶段＋新进程/默认持久化存储/生产调度器的恢复验收**，不是完整 CLI、真实 controller 模型决策或最终回答验收。fixture 准备强合同、真实源码交付及旧执行，但来源批次没有终态验证凭证；新补证批次必须 verified，整轮仍须诚实保留 `unverified / missing_terminal_verify_verdict`。这一负控防止把新测试 PASS 倒签成原批次已验证。完整 CLI 收尾、多来源及其他原生框架继续独立验收。

2026-10-07 冻结 `91aaa9be3` 的原评测已暴露新补证批次被错误附加源码定位义务，原始 workflow 保留 `verification_proof_incomplete`，不能按新断言重新签绿。后续 `db54efcd2` 公共新进程回归修复该接缝；未追加第三例真实模型重跑。
