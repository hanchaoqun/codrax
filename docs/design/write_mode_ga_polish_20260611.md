# 写模式 GA 打磨方案

来源:`write_mode_commercial_readiness_audit_20260611.md` §3 清单(6 项)。每项给出系统级泛化机制,精确信号驱动,零关键字路由;实施前各落点已逐一核实(见各项"落点")。

## 1. 终态恢复指引(P1)

**问题类**:控制器一切 blocked/pending 终态(scheduler 八个 `return fmt.Errorf` 出口)只携带原因散文;已落盘的恢复资产(report JSON、attempt diff、`refs/codrax/applied/<id>`、可用动词、相关 yaml 旋钮)用户不可见。

**机制**:单一 `composeBlockedRunGuidance(run, reasonCode)` 出口合成器——从 typed 状态(batch attempt 记录的 ReportID/ArtifactRef/PlanID、plan 状态、reason code 枚举)合成双语恢复段;reason code → yaml 旋钮映射为静态表(枚举→旋钮名,非散文解析)。所有终态出口统一经一个 `failWorkflow(run, reasonCode, err)` 收口函数返回,恢复段写入 `Mutable.Result`(单发 CLI 打印 Result;REPL 同源)。

**落点**:write_controller_scheduler.go 八个出口;render 双语文案。

## 2. 落地提交信息(P2)

**问题类**:checkpoint 提交信息为机器风格 `codrax apply iter (plan=<id>)`,cherry-pick 后污染用户历史。

**机制**:提交信息 = plan.Summary 首行(有界 72 字符,UTF-8 安全截断)+ 空行 + `plan: <id>` trailer。Summary 为空时回退现格式。单点改 stage_hooks.go:799 的消息构造,提取具名函数 + 测试钉格式。

## 3. W2 编辑格式 advisory(P2,按红线只软不硬)

**问题类**:patch 把原文多行结构压缩进单行(实操复现 `if (...) {<tab>return error;`),功能对但 diff 不专业。风格=噪声信号,**永不硬门**。

**机制**:emit_change_plan 校验链内新增纯结构 advisory:对每个 kind=patch 的统一 diff,比较 removed 块与 added 块的"行结构"——当 removed 侧存在 `{` 独立断行而 added 侧出现 `{` 后同行跟随非空语句(或多个 `;` 语句合并到一行且原文为多行)时,在 emit **成功** 结果 Summary 追加 compat 风格注记(与 salvage 注记同通道):指出 edits[i]/文件与行,建议保持原文行结构重发。模型可自愿重发;不计失败、不进重试预算。结构比较只看括号/分号/换行的排布,与语言无关、与关键词无关。

## 4. 审批记录身份与自指纹(P2,合规向)

**问题类**:审批记录无操作者身份、无自身防篡改指纹;事后编辑 UserDecision/DecidedAt 不可检测。

**机制**:`WriteApprovalRecord` 增 `Operator`(git config user.name <email>,取主仓配置;失败回退 OS 用户名;纯确定性,无新 flag)与 `RecordFingerprint`(SHA256 over 规范化字段串:policy|risk|action|user_decision|reason_code|source|plan_fingerprint|decided_at_unix|operator)。`NewApprovalRecord` 统一计算;apply 前钩子在既有 plan 指纹校验旁增记录自校验:重算不符 → 与陈旧审批同 lane 拒绝(typed)。/approve 路径同样经 NewApprovalRecord 落 operator。

## 5. 持久化 fsync(P2)

**问题类**:全部 tmp+rename 写入(types 三个 persist 文件 + repomap cache)缺 `f.Sync()`,掉电窗口可致 JSON 损坏。

**机制**:`types` 包单一 `AtomicWriteFileSync(path, data, perm)`(O_WRONLY 写 tmp → Sync → Close → Rename);五处写入点全部迁移。repomap cache 的 writeFileAtomic 同样补 Sync(独立包,同模式)。

## 6. 文档边界三处(P2)

- user_guide 写模式主流程前置 `pipeline_keep_worktree_on_success` 说明 + ref 恢复通道(两种落地方式并列)。
- 多仓写硬边界(按子仓拆分)写入写模式章节与 CLI 参考。
- log/trace + write 组合语义据实声明:pre-stage 按附件存在运行于任何 mode,triage 产物供 write_analyzer 消费,plan/apply 不受 artifact 锚点污染(W3 红线不变)。

## 7. 任务列表

- [x] 批 1:`publishBlockedRunGuidance` 合成器(typed attempt 记录 + 静态 reason→旋钮表)接线全部 10 个终态出口;双语;追加式不覆盖既有 Result;测试钉工件/动词/旋钮齐全。**活体确认**:3 步预算逼出 blocked,输出含"恢复指引"段(旋钮 + plan id + /plan show + /workflow show);6 步预算时完成 lane 直接全程成功(顺带再证完成 lane 价值)。
- [x] 批 2:`applyCommitMessage`(plan 摘要首行 72 字符有界 subject + `plan: <id>` trailer,空摘要回退旧格式,钉测试);`analyzePatchLineCompression` 行结构 advisory(只比较 removed/added 的括号·分号·换行排布,相对原文才触发——原文本就单行风格不报;成功 Summary 追加 compat 注记"accepted as-is",永不拒绝;四类负样本钉死不误报)。
- [ ] 批 3:第 4+5 项(记录身份+自指纹+apply 校验+测试;AtomicWriteFileSync 统一迁移+测试)。
- [ ] 批 4:第 6 项文档 + 全量回归 + 实测复跑(github_issue 一案全绿确认无回归)。

## 8. 进度

- 方案落盘,待分批交付。
