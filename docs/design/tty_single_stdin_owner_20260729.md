# TTY 单 stdin owner 专项设计（T-0）

- 日期：2026-07-29；前置账本：`docs/design/pi_borrow_analysis_20260729.md` §7.6（PIB-2 v1 非 TTY 队列已落地；TTY 全形推迟至本专项）
- 目标：TTY 交互下 pipeline 运行期不锁输入——输入排队回放（follow-up）、esc 中止还原、后续阶段边界注入（steering）；同时根治输入层三处现状真缺陷。
- 本文档兼作专项账本：§5 批次表随落地更新。

## 1. 普查定谳（全部消费点与关键事实）

**TTY 交互车道（r.in==nil 且 stdin 是终端）**的 stdin 消费点：
1. `native_input.go` raw 编辑器（**默认**，非 fallback）：per-prompt `MakeRaw`+新建 `bufio.NewReader(os.Stdin)`+返回即弃；ESC 超时用 fd 级 `unix.Select`（旁路 bufio 缓冲）；bracketed paste 仅 prompt 窗口开启。
2. `readInputBubble`（bubbletea v1.3.6）：**仅当** `!IsTerminal(0)` 或 `MakeRaw` 失败时的 fallback；无 `WithInput`，bubbletea 自管 raw（对非 tty 输入不 raw）。
3. 交互态 captureScanner（`/log` 粘贴、`/paste`）：cooked 态每次新建 scanner 即弃。
4. huh `NewConfirm` 七站点（/approve、/merge、/clear 等）：自起 tea program 直读 os.Stdin 自管 raw；输出写 os.Stderr。
5. Run 期间：**无任何读取者**（cancelListener 在 interactive 时被显式挡掉）。

**脚本车道（r.in!=nil）**：长寿 `r.scanner`（有意复用防 read-ahead 丢失）+ 非 TTY cancelListener 队列（PIB-2 v1）。**本专项不动脚本车道**。

**关键版本事实**：bubbletea v1.3.6 `initInput` 只在 input 是真 tty File 时 MakeRaw——`WithInput(pipe)` 时 bubbletea 完全不碰终端模式（模式管理权可完整归 owner）；huh 有 `Form.WithInput` 可注入。

**现状真缺陷（本专项顺修）**：
- D1 type-ahead 静默丢字节：per-prompt bufio/scanner 弃置时缓冲内已读入的预输入丢失（普查风险#1）。
- D2 强杀路径遗留 raw 终端：`installCancelSignalHandler` 双击路径 `os.Exit(130)` 不跑 defer Restore（风险#3）。
- D3 idle prompt 单次 Ctrl+C 直接退出，与 handler 注释宣称的"首次只警告"矛盾（raw 下 Ctrl+C 是 0x03 字节非 SIGINT，普查 §5）——本专项落地时显式定其语义。

## 2. 架构决策

**核心**：`ttyStdinOwner` 单例——REPL 生命周期内 os.Stdin 的**唯一** `*bufio.Reader` + 终端模式（raw/cooked）的**唯一**管理者。消费点不再自建 reader/自管 MakeRaw，改为向 owner **借窗口**：

- `borrowRaw()`：raw 字节窗口（native 编辑器、T-2 的 Run 期队列读循环）。owner 提供 buffered-aware 的 `readByteWithTimeout`（**先查 `Buffered()>0` 再 fd select**——共享缓冲后 ESC 超时探测若仍直接 select fd 会在缓冲残留时误判裸 Esc；这是共享化的必修细节）。
- `borrowCookedLines()`：cooked 行窗口（交互态 capture 粘贴）。同一 bufio 缓冲跨窗口连续——type-ahead 字节保留到下一窗口（D1 的修复本体）。
- 模式借用带恢复保障：Restore 挂进强杀路径（D2 修复：exit 前显式 Restore）。

**收窄面（均落账，不做）**：
- bubbletea fallback 车道保持现状：它只在非 TTY 交互或 MakeRaw 失败时可达——前者不属于 TTY 车道、后者 owner 自己也无法 raw，让位旧路径是正确降级；且 cancelreader 对非 File reader 不可取消（Quit 挂 500ms），接管性价比为负。
- huh 七站点暂不接管：全部运行在 prompt 间隙（cooked、无并发读者），现状无竞态；type-ahead 残留字节在 confirm 窗口不可见（现状=弃置丢失，新=滞留 owner 缓冲、confirm 后的下一 prompt 重现）——两者都"huh 读不到预输入"，行为等价，滞留重现还是修复方向。T-3 后视需要经 `Form.WithInput` 收编。
- `!cmd` 的 Stdin=/dev/null（交互命令拿不到终端）：已知限制，非本专项。

**Run 期回显纪律**：owner 的任何用户可见输出（入队回显、取消提示）必须经 Renderer 加锁提交（新增最小公开面 `Renderer.CommitUserInputLine`），禁止直写 r.out——dock 的 `\x1b[4A` 重绘计数不容第三方写者（普查风险#11）。

**EOF 语义**：owner 读到真 stdin EOF → 设 sticky eof 标志，所有后续 borrow 立即 EOF（与现状"Goodbye!"路径一致）；不引入 pipe 中间层，无 pipe-EOF 与真 EOF 混淆问题（风险#6 经架构选择消解）。

## 3. 批次计划

| 批 | 范围 | Pin 要点 |
|----|------|---------|
| **T-1 纯重构** | `ttyStdinOwner`（共享 bufio + 模式借用 + buffered-aware ESC 探测 + 强杀 Restore）；native 编辑器接 borrowRaw；交互 capture 接 borrowCookedLines；其余车道不动 | ① type-ahead 保留（prompt A 尾部预输入在 prompt B 可读——现状丢失，新行为 pin 并在账本明示这是 D1 修复而非回归）② 既有 native/input/capture 测试全绿 ③ 强杀路径 Restore 被调（注入 restore hook 断言）④ ESC 序列跨缓冲边界不误判（buffered-aware pin） |
| **T-2 运行期队列** | `runInFlightWrap` TTY 分支向 owner 借 Run 窗口：raw 读循环 + 最小行组装 + 入队（**drain 汇入 PIB-2 v1 的 `r.pendingFollowUps` 回放通路**，TTY/非 TTY 汇合）；0x03 走既有单击/双击取消语义（字节路与 SIGINT 路同一实现）；esc=中止 Run 且队列还原为下一 prompt 预填；入队回显经 Renderer | ① 运行期输入入队+回放顺序 ② 0x03 单击=cancel 双击=退出（与 cooked SIGINT 语义一致）③ esc 还原队列 ④ dock 无第三方写者（回显走 Renderer 锁） |
| **T-3 注入与捆绑件** | steering 阶段边界注入（队列行中 @path 经线程安全通道 → 下一 explore 窗口 `AddPendingRead`+`runForcedReads`；自由文本 → `RetryHint` 车道）；@ 补全与粘贴折叠在单一输入层上重估 | 按 T-2 落地后现状定，先设计后动手 |

**D3 裁定（T-1 内落地）**：idle prompt 的 Ctrl+C 保持"清行/退出"编辑器语义不变（native 现状），但 Loop 侧把编辑器 aborted 与真 EOF 区分，Ctrl+C 首次给"再按一次退出"警告——使实现与 handler 注释的承诺一致。若实现中发现该语义牵连过宽，允许收窄为"维持现状+改注释"，落账。

## 4. 风险映射（普查 13 条 → 处置）

#1 type-ahead→T-1 修复本体；#2 idle Ctrl+C→D3 裁定；#3 raw 归属/强杀遗留→owner 唯一管理+强杀 Restore；#4 脚本态双读→不动（v1 已单独治理，direct-LLM 不起 listener 的现状维持）；#5 bracketed paste 双开关→owner 统一（prompt 窗口开、Run 窗口关，T-2 粘贴多行逐行入队；T-3 重估折叠）；#6 EOF 三义→sticky eof 架构消解；#7 /exit→owner shutdown 挂 Loop 退出路径；#8 三处上限不一致→owner 行组装用 cancelListener 同款 1 MiB 帽+超帽披露；#9 Windows→T-1 保持 per-平台 readByteWithTimeout 原语不变（owner 包装层缓冲优先，平台层只在缓冲空时触达）；#10 huh stderr→收窄不动；#11 dock 并发→Renderer 锁提交纪律；#12 GetSize(0) 硬编码→顺手改显式 tty fd；#13 账本交叉引用已更新。

## 5. 落地记录（随批更新）

**T-1 已落地（2026-07-29，main=ce30508bb）**：`internal/repl/stdin_owner.go` — `ttyStdinOwner`（REPL 生命周期唯一 bufio.Reader + 唯一模式管理者；`borrowRaw`/`borrowCookedLines` 互斥窗口 + release 幂等；`makeRaw` 可注入供测试）。接线三处：native 编辑器 prompt 窗口借共享 reader（不再 per-prompt 自建+自管 MakeRaw）；交互 capture（/log 粘贴、/paste）借 cooked 行窗口（`captureScanner` 接口化 + release，borrow 失败降级旧直读并告警）；强杀路径 `os.Exit(130)` 前 `restoreTTYForExit()`（D2 修复）。ESC 超时 Buffered() 快路径经核实现状已存在（native_input_unix.go），共享化天然安全。D3 收窄：编辑器 0x03 语义保持现状，矛盾点是 handler 注释的过度承诺——T-2 引入 Run 窗口字节路时一并统一表述。pin 四组：type-ahead 跨窗口零丢失（D1 判决性证明）、窗口互斥+release 幂等、强杀恢复 exactly-once + nil 安全、行扫描器 scanner-parity。全仓 83 包零 FAIL。

**T-2 已落地（2026-07-29）**：owner 新增 Run 输入窗口 `borrowRunInput`——tick 轮询（100ms，复用平台 `readByteFDWithTimeout`，goroutine 永不无限期 park 在 Read，drain 一个 tick 内归还，普查"跨界悬挂 Read"隐患结构性消除）+ 最小行组装（1MiB 行帽/32 条队列帽=与既有两车道同族，超帽一次性披露）。三键语义：**Ctrl+C 字节 → `raiseSelfSIGINT` 自举回唯一 SIGINT handler**（单击=取消/双击=退出与 cooked 信号路完全同一实现；Windows 无自举回退直接单击取消，账本披露）；**裸 ESC 才中止**（ESC 后一个 tick 内有后继字节即判转义序列——CSI/SS3 吞到终结符，方向键误触不可能取消 Run，判决性 pin）+ 队列**还原不回放**（单行→prompt 预填，多行→既有粘贴占位符车道折叠还原）；普通行入队并经 `Renderer.CommitUserInputLine` 加锁回显（dock 重绘算术零第三方写者）。排干汇合：正常结束队列并入 PIB-2 v1 的 `pendingFollowUps` 回放通路（TTY/非 TTY 单一回放面），半行成为下一 prompt 预填（运行期击键零丢失）。平台原语抽取为包级 `readByteFDWithTimeout`（unix/windows 双实现，编辑器方法委托，行为不变）。pin 六组：排队+回显+空行不回显、CSI 吞掉/裸 ESC 中止判别、Ctrl+C 路由+半行移交、排干双车道（回放/还原×单多行）、坏 fd 退避不 wedge、既有全套存活。全仓 83 包零 FAIL。

**T-3 已落地（2026-07-29，T-3a 全形 + T-3b 重估裁定）**：
**T-3a 真 steering**：`internal/orchestrator/steering_notes.go` — `steeringIntake`（Run 生命周期开/关、线程安全 push/drain）；REPL 运行窗口每行**先 offer 给运行中管线**（`steeringSink.PushSteeringNote`，接受即回显「已注入本轮探索」不入队），explore 每个 window 边界消费：@path 钉选（复用共享 `internal/userhint.ExtractPinnedFiles`——从 repl 移出成单一实现）→ `AddPendingRead(Origin=explore.user_steering_pin)` 汇入紧随其后的 `runForcedReads`（explorer LLM 看到任何东西之前真正读完），自由文本 %q 引用为 data 追加进既有 window-hint 车道（软引导，精确/嘈声原则合规）。**接受未消费的 notes 在 drain 时经 `TakeUnconsumedSteeringNotes` 一次性交还**并排在回放队列最前——steered 或 replayed，不丢失、不双跑（写模式 Run 无 explore 边界时自动全量回退为回放，零特判）。orchestrator god-file ratchet 再次触发，按其本意瘦身调用点（说明集中在 concern 文件）。pin 五组：intake 生命周期（关闭拒收/空白拒收/交还 exactly-once）、边界消费（真实文件钉选入 PendingReads 带 origin + hint 逐字引用 + 消费后不再交还 + 空 drain 零改动）、窗口 steer 旁路队列（sink 拒绝后回落排队）、未消费交还排序在前。
**T-3b 重估裁定**：粘贴折叠——普查证实 native 编辑器**已有**占位符折叠（bracketed paste + injectPlaceholder + pasteFoldMinChars），PIB-5b 当初按 bubbletea 假设的收窄系过度收窄，现状已覆盖，无需开发（pi 借鉴账本 §7.5 该条勘误指向本记录）；@ 自动补全——纯 polish 件，锚定设计（native 编辑器 refreshSuggest 机制 + repomap FileIndex 候选源 + 前缀/子串两档打分），效益不再被竞态问题绑定，留待常规迭代，不占本专项。
**专项收官**：T-0/T-1/T-2/T-3 全落地；pi 借鉴账本的「PIB-2 TTY 全形」候办就此关闭。

**尾件消化（2026-07-29 续）**：① **@ 补全已落地**（internal/repl/at_file_suggest.go）：native 编辑器 filterSuggestions 漏斗新增 @ 分支（slash 建议恒胜，仅在其空时查询）；会话级懒建文件索引（一次有界 walk，cap 5000，跳过 .git/node_modules/vendor/.codrax/隐藏目录，每击键只过滤内存切片零 fs 访问）；pi 式四档打分（basename 精确 100/前缀 80/含 50/全路径含 30）+ 同档 basename 短者优先平局；整值替换只改尾部 @token。pin 两组（打分序+边界负臂含 email/裸@/cap、索引跳过规则精确集合断言）。bubbletea fallback 车道不加（既有收窄裁定维持）。② **按 turn-id 导出已落地**（memory.Store.Turn(id) 导出访问器（in-memory recent 优先）+ /export turn-id 分支：仅问答不含附件——sticky 附件属于当前会话不属于历史轮，确认消息明示；未知 id fail-loud 点名；/help 子命令注册）。pin 一组（by-id bundle 形+无附件负臂+未知 id）。


## 6. 客户事故复盘（2026-07-29/30，两案并行）

**案一：REPL 显示/挂死回归（我方 T-2 引入，已止血 58ea9f91c）**。三症状一根因族：Windows 运行窗口 raw 读被非按键控制台记录诱导 park → drain 死等 → REPL 冻结（"整理上下文中"卡死）与输出积压（敲键才批量倾泻）；unix 侧 MakeRaw 清 OPOST 的楼梯化隐患同批预防性修复（cbreak 保 OPOST/ISIG）。止血=Windows 平台整体禁用运行窗口+drain 3s 保险丝+`CODRAX_DISABLE_RUN_INPUT` 总闸。**流程教训（教训条款）：平台不可本地验证（CGO 交叉编译不可行）的输入层改动，必须平台门控默认关闭再逐平台放开；"unix 推理+Windows 放行"不可复现。**

**案二：零探索捏造答案（触发源待客户证据，捏造授权面已封死 FABGATE-1）**。调查定谳（agent 全文另存）：`› 2/4 正在生成最终答案` 的 2/4 来自 soft-notice 的"下一个未开始 slot"投影——**铁证=finalize 通知时渲染器从未见过任何 explore 行**；auto-resume 本案**排除**（复活必打的披露行在 transcript 缺席）但其设计风险实存→已按用户裁定默认关闭+/clear 连清（08a93455b）；确证的捏造授权面=`applyRuntimeTraceSourceOptionalSurfacePlan` 允许"裸路径引用+零附加+零观察"铸 source-optional 豁免，四道门（tier1 零证放行/ta==nil 旁路/引用下限被剥/观察无下限）真空放行。**FABGATE-1 已落地**：裸引用+零附加+零 ledger 工件+零确定性 trace 观察 → 不铸豁免（四判据全 typed 精确信号；判决性 pin=客户形状负臂+附加车道不回归臂）。**fix ① 撤回记录**：ta==nil 臂两次尝试均撞真墙（stub 工件形/既有 pin `IgnoresUnknownState`="未观测状态≠结构空"是有意裁定）——正确落点=forced-finalize 逃生舱（orchestrator.go:5424-5449）加"零投资节点不得强制成文"门，**待客户日志确证 S2 触发后再动**（需 grep `blocked on entry conditions; forcing finalize` / `DAG scheduler stalled`）。**候办 FABGATE-2**：runtime-observation 出厂下限（disposition 激活+external_observation 块 ⇒ ≥1 条确定性 runtime 观察，精确信号 `HasDeterministicRuntimeQueryObservation` 现成，emit 层 typed reject+escape lane）。**向客户取证清单**：① `codrax --version` 的 buildRevision 对表；② `.codrax/logs` grep 上述两行+`stop condition fired`+`read run snapshot seed applied`。规避=显式 `/htrace <路径>` 附加（正规车道，涉事形状不存在）。


## 7. 回归 sweep 与 REGFIX 四批（2026-07-30）

客户两起事故后对当日全部批次做了 25-agent 五维对抗复核（并发挂死 / 平台兼容 / 管线语义 / 显示 / REPL 动词），**19 条确认缺陷（3 high / 16 medium）、1 条证伪**，全部落在我方批次内。分四批根修，全推 main。

**REGFIX-A（运行窗口生命周期，6 条）**：#0(high) 保险丝的放弃路径把 run-N 回调留在会话级对象上——迟到解冻的 reader 能取消未来 Run、把陈旧半行当 steering note 注入其 explorer prompt、并吞掉按键（修：原子 dead 标志，drain 前置位、每次读后与每次回调前复查）；#1 保险丝永不 release 使 owner 永久 brick 且 cbreak 泄漏到正常退出（修：release-on-done watcher + `/exit`/EOF 路径恢复终端）；#2 `cancelToken` 裸指针 -race（改 atomic.Pointer；取消面移入 cancel.go 化解 LOC ratchet）；#3(high) `unix.Select` 的 EINTR（拖 tmux 分屏的 SIGWINCH 风暴即可）被 ESC 探测当错误上抛 → Loop 判会话结束、打印 Goodbye 并丢失已输入行（修：剩余预算内重试）；#4 任何探测错误被当裸 ESC 取消 Run；#5 DEL 作为裸字节进入队列文本（改按 rune 编辑）。

**REGFIX-B（steering 语义，4 条）**：#9(high) 并行 explore 车道先组装 worker hint 再 drain steering，worker 又用 pre-drain 值覆盖 clone hint → 文本回显“已注入”却到不了任何 prompt，且因已 drain 也无法回放（修：drain 返回文本，调度器合并进每个 worker hint）；#10/#15 mid-run 斜杠命令与 `!cmd` 被当 steering 散文吞进 prompt（intake 拒收命令行，交由回放漏斗执行）；#11 mid-run @pin 绕过 PIB-5c 的显式 source-exclusion 边界（同款钳制：文本仍进 prompt，仅扣留强制读）；#13 回显在 intake 接受时就宣称“已注入本轮探索”（改为“已交给本轮处理，未采纳则回放”）。

**REGFIX-C（交付面与平台/显示，5 条）**：#8(high) `MergeFromRef` 新分支车道枚举了全链却只 cherry-pick tip——早于 tip 的提交永不落地而 `/merge` 报成功（其文档契约本就写 commits 复数；W-3 的 partial checkpoint 让这个潜伏遗漏可达）；#7 Windows `term.GetSize(stdin)` 失败 → 宽度恒 80 → 帧算术错乱（改 fd→stdout→stderr 递降探测）；#12 usage 只在首个 objective 归零（analyze 后才发、写模式永不发）→ 显示陈旧账（改 pipeline start 归零）；#14 `dockWidthCond` 由窄翻**宽**——仍钉死、仍 locale 无关（BARGRID-1 红线是“禁包级默认”而非“必须窄”），方向改为 fail-safe（欠量会溢出 → 终端换行 → 光标回卷少算 → 行漏进永久滚动区，正是客户症状类；过量只是行短），并修掉省略号自身宽度未预留导致的一列溢出；固定保险列方案先试后否（全歧义行可近两倍窄测量值）。#6 Windows 控制台非按键记录导致的阻塞读**记档不盲修**——正确修法（PeekConsoleInputW 丢弃非按键记录）无法在本机（unix + CGO）编译验证，按复盘的平台裁定不放行未验证的 Windows syscall 代码；仅做有界切片等待，并就地记录限制与既有围堵（运行窗口在 Windows 永不 arm）。

**REGFIX-D（REPL 动词面，3 条）**：#16 回放绕过输入漏斗（`!cmd` 与模板命令被当 LLM 提问送出 → 改为走同一漏斗：shell-bang → 别名 → 模板 → dispatch）；#17 `/import` 违反自身“篡改零挂载”契约（log 先挂载才校验 trace；剥掉 hash 的 manifest 全放行 → 改为全部校验通过后才动 sticky 车道，缺 hash 视为不可验证并拒绝）；#18 native 编辑器 Enter 劫持尾随 @token（可见文件建议时自动接受最高分项并提交 → Enter 只接受斜杠建议，文件建议仍可 Tab 显式接受）。

**教训固化**：① 我自己写的止血保险丝成了新缺陷源（#0/#1）——任何“放弃/降级”路径必须**先失效化再放手**，且必须保留归还资源的回路；② 平台不可验证的改动一律记档 + 围堵，不盲修（#6 按此裁定处理）；③ 对抗复核必须覆盖“当日自己的批次”——本轮 19 条无一由同事引入。
