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

（T-1/T-2/T-3 落地后在此追加：提交哈希/验证结果/偏离裁定。）
