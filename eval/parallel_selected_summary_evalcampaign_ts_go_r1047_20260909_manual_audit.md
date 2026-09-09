# r1047：TS 跨包调用链与 Go 精准补丁人工审计

- 基线：已推送 `b12e3385b`，clean binary revision `b12e3385b3e9`，built `2026-09-09T09:05:38Z`。
- 运行：`2026-09-09T09:06:17Z`，严格两路并行、各一次、timeout1200s；未修改 case、oracle 或步数预算求绿。
- 选例：TS workspace 跨包调用/别名（r1030 后未复放）与 Go 单行真实 apply（r1037 后未复放），覆盖读写及老化风险；不是反复刷同一 Trace 题。
- 机器 1/2 PASS；**人工 TS 整份答案 fail，Go 源码交付与原生行为 pass、正式证明仍未闭合**。保留原机器 FAIL，不反写旧答案或正式报告。

| case | 机器/耗时 | 人工结论 | 过程与上下文 |
|---|---|---|---|
| sr_ts_workspace_chain | PASS / 246s | fail：别名映射值错误，时序省略关键条件 | read14/repo_map3/source_lens1，ctx36%；成文拒绝1、patch1，图保留 |
| patch_go_typo | FAIL / 198s | 补丁/原生测试正确；proof 未闭合 | read4/repo_map1，ctx28%；最终如实“未完全验证”，非运行时崩溃 |

runner 汇总与 case summary 计时边界不同：Go case wall 为195s，以上198s为并行 runner 记录，不混用。

## 1. TS：区分模型遗漏与系统供给/显示问题

产物 `.codrax/output/20260909-021022.035-40722.md`（5930B）；日志 `eval/results/sr_ts_workspace_chain-20260909-020617/run-1.logs/codrax-20260909-020618-000-40722.log`。日志含 NUL，文本检索用 `rg -a`。

1. 核心 `run → ApiClient.fetchUser → HttpTransport.send → dispatchOnce → fetch` 保留，实际 `FixedDelay(200,3)` 与 `nextDelay → sleep → setTimeout` 也在。唯一成文拒绝（4012）为 summary 缺失、5条独立关系身份不全、7条图边无锚；当前教学和已有 recipe 足够，4138由模型补身份/7条图锚/summary，4159接受。没有发现同一声明必带且必拒、JSON修补合同自冲突或系统删图边。
2. 最终表53–54行丢失别名路径的 `packages/`。首稿3998已经写错，patch4138原样保留；系统只修 citation.quote，因此77–78行引用正确而表错。不能把它称为 renderer 截断路径。**但也不能称正确值已充分交给 finalizer**：探索1399/2489、旧证据1521/1526曾有完整映射，最终3610–3613仅保别名字符串锚与坐标。后续源码定位确认为 **B1634d/P1**（同一候选升级）：真实源行沿Snippet→RawExcerpt保留，但主投影默认隐藏current-source excerpt，其它support通道又按call-chain位置过滤而排掉配置。没有运行对象dump和新RED，不能冒称公开入口已复现；保模型Summary隔离，后续仅有界保字节传真实source excerpt，不能直接打开会合并字符串内空白的旧选项。详见统一台账§123.1716。
3. 图把等待画成每次请求后的无条件动作，缺少 `<500` 立即返回和 `attempt < maxAttempts` 才等待的分支。两条条件已在 finalizer 上下文3904/3918，首稿即遗漏，patch只添加metadata而未删除原分支。真实语义最多3次发送、最多2次200ms等待；网络/读body异常不在 send 中重试。现答案“最大重试次数”有歧义，但没有明确声称“三次额外重试”，不扩大错误。按模型表达遗漏留账，不增加关键词硬门或系统画分支。
4. **B1634b/P2 确认显示缺口**：第17–21行追加关系列表泄漏 `ApiClient_fetchUser` 等内部节点ID。`internal/render/answerdoc.go:368/440`只按兄弟图相同local ID查可见标签，而本次列表和图ID不同。模型已给完整身份及图的可见标签；后续可审计“同关系、完整身份对、同证据域、唯一图边 → 其原可见标签”的精确映射。验证身份明确不是可见文案，不直接复制为正文、不猜下划线、不跨图首胜；本轮未实施。
5. 缺省“列2/列3/列4”以及列表教学仍称仅输出编号列表，分别为模型未给columns/旧软教学显示债；不是本次硬拒原因。无强制图要求，不能因可选图缺某个细节就造额外图合同。

## 2. Go：代码成功不等于完整验证证明成功

结果目录 `eval/results/patch_go_typo-20260909-020617`；实际apply日志 `run-1.logs/codrax-20260909-020725-000-40830.log`；原补丁 `run-1.plan-1.json`，最终proof-only计划/报告 `plan-1788944943112579000-40830{,.report,.final}.json`。

1. 实际diff唯一 `main.go:25 retrun → return`，1删1增，其他源文件/测试不改；原运行repo HEAD仍 `cdcf660cde20079656f6da6110afb4fa9924d82e`，保留交付source commit `7f6bf2d`。未合并交付到fixture。
2. 前两轮是probe-primary策略跳过原生套件，不是fixture没有测试。最终系统真实执行 `go test -json ./...`（2524），`TestGreet`通过；独立审计在保留树补跑 `go test ./... -count=1` exit0/0.436s，前后git status空。该人工执行输出在本轮工具记录中，没有另存独立日志，也没有写回正式report。
3. 探针先因无失败信号、再因复制实现不调用真实模块被拒，后由模型改成合法同包测试；现有JSON/验证教学可操作。模型把源文件拼写修复描述成 `repr` 放置合同是其结构化选择，不能由系统偷偷换operator或自动补placement_refs。
4. **B1634a/P1 确认控制器上下文缺口**：终态2652的 `verification_completion_scope=1/1`只数behavior witness；正式proof仍缺 `probe_placement_refs/line25-correct`。该精确原因被16项pack上限挤掉：2683首条satisfied、2684省略17项，2641只剩泛化 `verification_proof_incomplete`。这是两个验证层的计数域/提示缺失，不是两个门数学矛盾。下批独立有界显示现有typed ledger未闭合kind/category/contract_ref/detail，明确1/1不等于全proof；不放松验证、不系统填写模型引用。
5. 两份报告从未有placement绑定通过，不能用本例证明B1616b“已通过证明跨计划丢失”。最终UI诚实“未完全验证”，机器该FAIL仍正确。
6. 机器另一条 `no_plan_regex(kind=patch)`来自 `eval/run.sh:1660` 对最终mirror检查，最后空changes的proof-only计划不代表最初补丁。初始计划确实patch且有ApplyCheckpoint。**B1634c/P2**：后续显式增加“已应用变更计划”oracle域，以durable checkpoint/ref关联，单计划独立满足整组条件、不跨计划拼接；末代正式report校验不改，旧默认不静默改，本轮原FAIL不改写。
7. 保留路径回写loader拒proof-only空计划（2757），实际JSON已经带正确worktree_path、树与报告未丢。归并 **B676-PROBEONLYDURABILITY1生命周期续修/P1**：现有严格shape仅认 `no_change_required`，verify后转`applied`导致同计划重载被拒。需分离合法proof-only身份与可变状态，复用严格probe/target检查；不简单放行所有状态的空plan。真实sentinel→verify成功/失败/不可用→保存/重载及普通非法空计划为必测面。

## 3. 保护边界与后续排序

- B1624b已通过公开入口先红后绿、最终race与86包全绿并推送；本轮两例无Trace附件、`trace_query=0`，**只算异构隔离回归，不算导航生产正证**。
- 第一优先B1634a控制器精确未闭合上下文；随后B676生命周期续修、配置值交接、B1634b显示、B1634c机评域。B1629b范围迁移、B1626多请求窗、B1622次数口径、B1616b/B1561证明债仍开放。模型漏条件不追加拟合硬门。
- 没有修改模型正文/图/结论或Trace根因算法、显式窗、投影/补采；没有把背景升级链上根因。两路活跃流正常、无年龄降级；本轮不是持续超过4分钟的流见证，不冒称长流已实测。已通过的4ms/旧4m工程回归保留真实停滞、取消与显式deadline。
