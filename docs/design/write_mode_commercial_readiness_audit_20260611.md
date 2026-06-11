# 写模式商用化判定审计

方法:四路并行深审(用户旅程 / 稳定性与恢复 / 安全与审计链 / 能力边界)+ 端到端人工实操(plan→apply→verify→cherry-pick 落地全程 + 负向门控)+ 既有实测证据(GitHub issue 写模式套件 8/8 PASS)。**审计原始产出中 6 条 blocker/major 经人工核实为误报**(§4),判定只基于核实后的事实。

## 1. 判定

**有条件商用:核心引擎达到商用质量,可面向受控用户群(beta / 内部 / 有引导的早期客户)发布;面向自助式大众 GA 前需完成 §3 的打磨清单。**

支撑判定的硬证据:

- **端到端旅程实操通过**:plan(中文摘要/变更列表/目标路径/验收测试完整渲染)→ apply(W1 白名单内单文件 patch)→ verify(真实编译+测试通过,报告落盘)→ `git cherry-pick refs/codrax/applied/<id>` 一条命令落地用户分支 → fixture 测试在用户分支复验通过。全程零人工干预修正。
- **实测通过率**:跨 C/C++/Java/TS/Python/JS 的 8 个真实 GitHub issue 修复案 8/8 PASS(含 runner 缺失逃逸、零测试逃逸、replan 收敛、审批自动执行等困难形态)。
- **负向门控正确**:未开 `write_enabled` 时单行明确拒绝并指出 yaml 开关。

## 2. 各维度评估

### 2.1 安全(强,商用就绪)

- 四层纵深全部核实在位:`write_enabled: false` 默认主开关(L2);审批策略确定性(critical 必拒、high 必人工、低/中按策略;LLM 分类只到 advisory Medium,不驱动硬门);W1 TargetPaths 白名单 + worktree 隔离(主仓 HEAD 字节不变,L5 无条件清理);内容信号结构化解析(PEM/包生命周期/CI 提权/manifest 权限/下载执行管道),`.env`/`.env.*` 在 secrets 路径类强制 High(risk.go:552)。
- **指纹防陈旧核实在位**:apply 前钩子重算 plan 指纹,与审批记录不符即拒(stage_hooks.go:268-343)。
- verify 阶段资源墙在位(进程组隔离,内存/CPU/墙钟上限,§8.17)。
- 跨子仓写硬拒(fail-loud,提示按子仓拆分)。

### 2.2 稳定性与恢复(良,个别残余)

- 状态机:typed 动作枚举三层掩码(schema 投影/emit 拒绝/调度器守卫)、canonical attempt state、finish 硬门 + typed escape、resume 三件套水化(重试计数/活跃 plan/失败 handoff)均已交付并有测试与实测证据。
- 持久化:plan/workflow/report 写入均 tmp+rename 原子(残余:无 fsync,极端掉电窗口;见 §3)。
- worktree 生命周期:成功保留(可选)+ 失败证据先落盘再清理 + `refs/codrax/applied/<id>` 常驻主仓兜底——worktree 即使被丢弃,字节仍可恢复,且系统消息明确给出恢复命令(orchestrator.go:3097)。
- 残余:blocked 终态(verify 重试预算耗尽)的 CLI 退出文案只含失败原因,未指向已落盘的 report/attempt-diff/ref 恢复通道;blocked run 不被 `/workflow resume` 接续(设计如此),但用户侧缺一句"如何从 blocked 继续"。

### 2.3 用户体验(良,打磨项明确)

- 强项:REPL 全动词族(/plan /approve /reject /verify /merge /workflow /worktree)带双语文案与下一步提示;apply 成功输出直接给出两种落地方式(/merge 或 cherry-pick 命令原文);plan 渲染含验收测试,审批决策可见(risk/action/reason 三元组)。
- 打磨项:① blocked/预算耗尽文案缺"调大哪个 yaml 旋钮/如何继续"的指引;② 落地提交信息为机器风格 `codrax apply iter (plan=...)`,应采用 plan 摘要;③ W2 编辑格式压缩(`if (...) { return ...; }` 同行)实操复现——功能正确但 diff 观感损害专业度;④ 多仓写不支持、裸目录授权等硬边界在 plan 阶段甚至 apply 阶段才暴露,应在入口预检;⑤ user_guide 与 CLI help 对部分边界(多仓写、log+write 组合语义)未声明。

### 2.4 能力边界(清晰,降级体面)

- 12 测试 runner + runner 缺失/零测试 typed 逃逸 + env_recommend 诊断;弱模型路径有统一 JSON 修复层 + 有界重试 + typed escape;`--plan-file` 手写 plan 走同一校验与审批门。
- 明确不支持:多仓写(硬拒)、写+读混合 artifact 场景未声明语义(文档项)。

## 3. GA 前打磨清单(按优先级)

1. blocked/预算耗尽终态消息附恢复指引(已落盘工件路径、ref、可用动词、相关 yaml 旋钮)——typed 状态渲染,小改动。
2. 落地提交信息采用 plan 摘要(首行)+ plan id(尾行)。
3. W2 编辑格式质量:advisory-only 校验(行压缩检测仅提示重发,按红线永不硬门)。
4. 审计链企业化(可选,面向合规客户):审批记录加自身指纹/操作者身份字段;plan 工件保留策略(目前 worktree 有 7 天/20 个上限,plan JSON 无上限)。
5. 存储写入补 fsync(掉电一致性)。
6. 文档补齐:多仓写边界、log+write 组合语义、`pipeline_keep_worktree_on_success` 在主流程文档中前置。

## 4. 审计误报记录(已核实剔除)

- "apply 不校验 plan 指纹"(blocker)— 假:stage_hooks.go:268-343 重算并比对。
- "`.env`/云凭据未检测"(major)— 假:secrets 路径类覆盖 `.env`/`.env.*`,强制 High。
- "store 无原子写"(major)— 假:tmp+rename 在位;残余仅 fsync。
- "verify 无资源墙"(major)— 假:§8.17 进程组隔离 + 内存/CPU/墙钟上限。
- "成功后 worktree 即弃导致 /merge 三步惩罚"(major)— 大体假:ref 常驻主仓,丢弃路径有明确恢复文案;残余为文档前置问题。
- "G1/G3/G4/G9/G10 未修"(blocker/major)— 假:均已于统一 gap 审计批次交付并有测试(审计 agent 误读账本处置列)。
