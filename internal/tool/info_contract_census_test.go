package tool

// info_contract_census_test.go — UXG-1 T1 + T2-armB 三向 tripwire census
// (ledger docs/design/real_trace_campaign_20260705.md §29.40; mechanization
// spec scratchpad/info_contract_report.md §④). Kills 病理形A (fields minted
// but never read) and half of 形B (wire fields with no Node mirror and no
// consumed note), and makes the §29.40 exemption adjudication a TYPED
// commitment surface (豁免表=承诺面).
//
// CONTRACT TABLES: every exported field of the projection wire types carries
// exactly one disposition. Statuses and their MECHANICAL arms:
//
//   displayed      — consumed by a display authority source; arm: the field
//                    token (".Field", word-bounded; Token overrides for
//                    method-mediated consumption) MUST appear in the display
//                    authority sources (runtime_tree/_rcr/runtime/_typelabels/
//                    _supplyfold/_rcm, comments stripped).
//   internal_gate  — consumed as a typed gate input only; same presence arm.
//   exempt         — adjudicated word-face exemption; Ref names a W-# row of
//                    the exemption registry (no scan arm — the ruling, not
//                    the code, is the authority).
//   known_gap      — "typed 有据但显示无踪" (the 13 OM findings); arm: the
//                    token must NOT appear in the display authority sources —
//                    when a fix batch wires the face, this census REDDENS
//                    until the row flips to displayed (悄悄修而不销账 and
//                    销账而未修 both bite red).
//
// RankItem (T2 armB) statuses:
//
//   node_mirror    — the wire field has a projection mirror; arm: the mapped
//                    field (same name, Ms→MS-normalized, or the explicit
//                    mapping below) exists on TraceCausalProjectionNode /
//                    TraceCausalProjection (reflection).
//   note_consumed  — the value travels via a registered rich note with a
//                    parsing consumer; arm: the named key is registered with
//                    carrier != display_only.
//   engine_gate    — consumed by tracequery engine logic pre-wire; arm: the
//                    token appears in ../tracequery non-test sources.
//   exempt / known_gap — as above; known_gap additionally arms the reverse
//                    direction: NO same/normalized-name projection field may
//                    exist (a fix batch adding the mirror must flip the row).
//
// Failure direction is conservative (报红必真,漏报可能): token collisions
// across types can only satisfy a presence arm (false green), never redden a
// correct row; collision-prone rows are marked NoScan with the collision
// documented.
//
// 账实一致纪律 (§29.40): every fix batch (v5 P2c / IC-A / IC-L / IC-E / IC-S)
// flips its contract rows known_gap→displayed IN THE SAME COMMIT.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
	"reflect"
)

type fieldDisposition struct {
	Status string // displayed | internal_gate | exempt | known_gap | node_mirror | note_consumed | engine_gate
	Ref    string // face/gate/W-#/OM-# pointer
	Token  string // optional scan-token override (method-mediated consumption)
	NoScan bool   // documented name collision — presence/absence arms skipped
}

// --- §29.40 exemption registry (豁免表 = 承诺面) -------------------------------
//
// 确凿 7 (W-1/6/8/11/15/17/20) + 维持 11 (W-2/3/4/5/7/9/10/13/18/19/21) +
// 收窄 3 已折 IC-A (W-12/14/16). Every row carries its reason and escalation
// condition; ghost rows (unreferenced W-#) redden.
type infoContractExemption struct {
	Reason     string
	Escalation string
}

var infoContractExemptions = map[string]infoContractExemption{
	"W-1":  {"S1 修根:排序合成分数禁以 ms 事实身份发布 + §29.22 乘子泄漏修根(RankSortBoostedEffectiveMs json:\"-\" 内部排序道)", "无(裁定终局)"},
	"W-2":  {"§29.36.2 裁定:背景通道无序数、#N chip 退役;提及义务=LLM 面 background_rank= note 软引导,Node 字段无显示消费合规(假注释已随 UXG-1 勘正)", "无"},
	"W-3":  {"§28.11-3(a):同值双归因披露只落证据索引 audit 面为设计决定", "无(维持现状)"},
	"W-4":  {"§29.30/.1+§29.37 EPUB:typed 内部门信号,唯一消费=lead 拒冕臂;词面无痕是设计", "effective-published 记号批若给词面,随批翻 displayed"},
	"W-5":  {"三态内部判定;false 经 ⚠实际/⚠跨窗 间接现形;nil-vs-true 不区分=absence never guesses 同法理", "无(true 正向词倾向否,防噪)"},
	"W-6":  {"§29.18 DISP-3 假⚠ donor carve-out:纯判官字段,值不落面是设计", "无(裁定终局)"},
	"W-7":  {"depth 挂靠硬门输入;被拒行有词面结果(链上─/父节点未确认/深度未解析);拒绝理由本身无词面", "回访/冷读再现 B2/B4 类拒因追问 → 升 IC-A(明细一行披露拒因)"},
	"W-8":  {"折叠强制展开选择器;效果结构性可见(行不折叠),无需词面", "无(裁定终局)"},
	"W-9":  {"值不印为 U10 裁定方向;语义经口径词代言,ideal+deficit 在拆解行可见", "回访出现频点覆盖比例数值需求 → 升 IC-A"},
	"W-10": {"Σ==窗硬门自算使显示冗余;字段作 wire 对账载体保留", "随 OM-5(IC-E 批)一并评估是否入 audit 面"},
	"W-11": {"数值语义全走 ImpactMS 族口径车道;三要素行文法取代源观测散文(系统不代写/proof-lane 同源);Summary 是 LLM 面载体", "无(裁定终局)"},
	"W-12": {"端点经 actual_window note 有显示消费(runtime.go 窗标)", "收窄已折 IC-A 顺手项(§29.40):⚠实际 行明细补实际段端点,IC-A 合入时翻 displayed"},
	"W-13": {"coverage 视图+ledger notes 消费;投影面以 MergedQueryWindows 代位", "非合并聚合行逐次发生窗出现独立需求再议"},
	"W-14": {"内部管线 token,入用户面有 no-internal-info 张力", "收窄已折 IC-A 顺手项(§29.40):audit 面补 source token,IC-A 合入时翻 displayed"},
	"W-15": {"合并行上清空,真实区分键经 FamilyMemberRoster 词面到达", "无(裁定终局)"},
	"W-16": {"吸收披露已有 链上并入+身份串+E# 名单;absorber 侧计数缺口小", "收窄已折 IC-A 顺手项(§29.40):链上并入行加共N计数,IC-A 合入时翻 displayed"},
	"W-17": {"经 selected_window note→Node.QueryWindow*,已有窗 chip/窗标词面;非缺口", "无"},
	"W-18": {"perf 车道行是否进入投影树未证实(投影编译零读);LLM 面承载", "先审 perf 行可达投影与否(可达性探针),再议词面"},
	"W-19": {"SupportRefs file:line 与 ArtifactLabel 疑似覆盖物理身份;防并族用途是引擎门", "多物理文件 bundle 内单树混行辨识一次实证后再议"},
	"W-20": {"priority_inversion_gated 理想源 note:显示不凭空合成(no_system_backfill);分量和回退已 pin", "接线留 P0-E,账本宿主 campaign:1272"},
	"W-21": {"rcr.go 形态表 SemanticsZH 空的 5 glyph spec:行2 类别词有词面,图例语义骑既有状态图标条目(两列单源表设计)", "P8 图例承诺面尺子下如需独立图例条,经用户裁定再开"},
}

// Non-field exemption attachment sites (surface-level rulings referenced by
// name so the ghost check spans them too).
var infoContractNonFieldExemptionSites = map[string]string{
	"W-20": "note ledger: priority_inversion_gated (internal/types info_contract_notekeys census)",
	"W-21": "legend surface: the five §24.3 form specs without GeneratedLegend semantics (T3 promise census)",
}

// --- §29.40 known-gap registry (13 OM,修复批翻状态用) -------------------------
type infoContractKnownGap struct {
	Summary   string
	HostBatch string
}

var infoContractKnownGaps = map[string]infoContractKnownGap{
	"OM-1":  {"周期源行迟到量分量不落任何面(承诺-数据脱节:图例/阅读参考/明细括注点名分解)", "①明细(a)格随 v5 P2c;②行3 完整修随 IC-A"},
	"OM-2":  {"锁行排队等待者数 BlockingWaiters 零消费(N 定修理方向)", "IC-L(可提前搭 v5 P2c)"},
	"OM-3":  {"下钻边 E#/来历语义零消费(行动指令回证据链断开)", "IC-E(边观测入册通道)"},
	"OM-4":  {"span 三层身份词(kind/category/subcategory)零消费", "v5 P2c"},
	"OM-5":  {"四态账 EvidenceID 铸而不读(发布值不能回指自身证据,违 §29.29 审计闭环)", "IC-E(v5 F10 条款增补)"},
	"OM-6":  {"fold peer TypeWord 写后不读(aabccb6f 删唯一读者;§29.40 裁决=A 臂保留字段)", "v5 P2c(「链上并入」行词面臂)"},
	"OM-7":  {"DrillStatus 投影头部强制披露半场七面全零(RCX① 裁定明文未兑现)", "IC-A"},
	"OM-8":  {"PriorityInversionLockDominated 并存披露注记缺失(§29.40.1:并存事实披露,非降级替换)", "IC-A"},
	"OM-9":  {"◇ 邻近行判据四元(overlap/edge_count/nearest_chain_*)发布即坠零解析", "IC-A(campaign:1397 立账族宿主)"},
	"OM-10": {"跨 ns 锁持有者身份两键(ns 统一/宿主进程)未显(假注释已随 UXG-1 勘正)", "IC-L"},
	"OM-11": {"锁对端状态族 peer_state_* 八键 + wait_object 未显(advisory 反超确定性面);peer_chain_* 伴生", "IC-L"},
	"OM-12": {"非焦点线程四态拆分不可见(dominant 单词掩盖第二可行动轴)", "IC-S(§29.23 成员级论证先行)"},
	"OM-13": {"InheritedTargetBlockedMs 承接注记未接线(注记面从未接线)", "IC-A"},
}

// --- T1 · TraceCausalProjectionNode contract (B 区) ---------------------------

var nodeFieldContract = map[string]fieldDisposition{
	"Role":                          {Status: "displayed", Ref: "行1 glyph/段位 + 明细层级"},
	"EvidenceID":                    {Status: "displayed", Ref: "[E#] 行尾 + 证据索引"},
	"Subject":                       {Status: "displayed", Ref: "行1 名字场 + 明细 + 证据索引"},
	"Predicate":                     {Status: "displayed", Ref: "证据索引 audit token"},
	"Object":                        {Status: "displayed", Ref: "行1 词位输入(DisplayCauseName) + 明细影响点"},
	"Value":                         {Status: "exempt", Ref: "W-11"},
	"Unit":                          {Status: "exempt", Ref: "W-11"},
	"Summary":                       {Status: "exempt", Ref: "W-11(LLM 面载体)"},
	"SupportRefs":                   {Status: "displayed", Ref: "证据索引 locator"},
	"LineStart":                     {Status: "displayed", Ref: "证据索引 locator 行区间"},
	"LineEnd":                       {Status: "displayed", Ref: "证据索引 locator 行区间"},
	"Rank":                          {Status: "displayed", Ref: "行1 ❶..❺ + 行2 chip(有效持席单门)"},
	"Tier":                          {Status: "displayed", Ref: "明细席位行 + audit token(三 typed 谓词门)"},
	"Causality":                     {Status: "displayed", Ref: "行1 段位 ⛓vs⧗ + audit"},
	"ChainRelevance":                {Status: "displayed", Ref: "行2 链上L# + 通道词权威"},
	"ChainDepth":                    {Status: "displayed", Ref: "行2 链上L# chip + 明细因果位置"},
	"TraceGapKind":                  {Status: "displayed", Ref: "行1 判据二分词(◇盲区行)"},
	"ChainBranch":                   {Status: "internal_gate", Ref: "W-7 depth 挂靠门(tree.go 树位判定)"},
	"ImpactMS":                      {Status: "displayed", Ref: "行1 值+bar+% + 明细(a)"},
	"CumulativeImpactMS":            {Status: "displayed", Ref: "行2 链上累计 + 明细(a)"},
	"SpanName":                      {Status: "displayed", Ref: "行1 SpanName 词位 + 明细 span 原文 + audit"},
	"SpanKind":                      {Status: "known_gap", Ref: "OM-4"},
	"SpanCategory":                  {Status: "known_gap", Ref: "OM-4"},
	"SpanSubcategory":               {Status: "known_gap", Ref: "OM-4"},
	"SemanticClass":                 {Status: "displayed", Ref: "行1 类词 + 行3 双口径(语义参赛门)"},
	"StartTs":                       {Status: "displayed", Ref: "行2 窗标 + 明细窗来源"},
	"EndTs":                         {Status: "displayed", Ref: "行2 窗标 + 明细窗来源"},
	"QueryWindowStartTs":            {Status: "displayed", Ref: "行2 来自查询窗 + 明细窗基"},
	"QueryWindowEndTs":              {Status: "displayed", Ref: "行2 来自查询窗 + 明细窗基"},
	"WithinRequestedWindow":         {Status: "internal_gate", Ref: "W-5 三处判定,false→⚠间接"},
	"Confidence":                    {Status: "displayed", Ref: "行2 置信 tier + 明细(a)"},
	"StateKind":                     {Status: "displayed", Ref: "行1 glyph+状态词 + 行2 [状态state]"},
	"DStateRefinedNonIO":            {Status: "displayed", Ref: "件③ arm a: 细化「D-state」词面门(名/形态格/对端措辞族)"},
	"DStateCauseUnprovenRemainder":  {Status: "displayed", Ref: "§29.50.5 件②: 「(原因未证)」余数词面门(cause 词族 qualifier)"},
	"BlockedReasonCaller":           {Status: "displayed", Ref: "件③ 行2 等待对象 披露"},
	"BlockedReasonWindowCount":      {Status: "displayed", Ref: "CR-3 件② 行2/headline 未核销 blocked_reason 残余披露"},
	"BlockedReasonWindowCaller":     {Status: "displayed", Ref: "CR-3 件② 行2/headline 未核销 caller 符号"},
	"ProcessTGID":                   {Status: "displayed", Ref: "CR-3 件③ 明细「进程 tgid=G」行"},
	"ProcessComm":                   {Status: "displayed", Ref: "CR-3 件③ 明细「comm=P」半场"},
	"ThermalCapWitnessed":           {Status: "displayed", Ref: "CR-3 件⑥ 受热限压/运行于(未见证) 词面门"},
	"UndrillableReason":             {Status: "displayed", Ref: "行1 ⊘链止(+wordless 补主语臂)"},
	"EffectiveImpactMS":             {Status: "displayed", Ref: "行1 席位输入 + 行2/行3 有效归因"},
	"EffectiveImpactPublished":      {Status: "internal_gate", Ref: "W-4 lead 拒冕臂(tree.go)"},
	"ActualImpactMS":                {Status: "displayed", Ref: "行1 ⚠实际 + 明细实际口径行"},
	"SemanticChainProjectedMS":      {Status: "displayed", Ref: "行3 链上计入 + 明细(a)镜像"},
	"ActualTotalMS":                 {Status: "displayed", Ref: "明细实际口径行线程总量半场"},
	"ActualCaliberNote":             {Status: "displayed", Ref: "明细实际口径行判词"},
	"TargetImpactMS":                {Status: "displayed", Ref: "覆盖句分子(链行)"},
	"DrilldownTarget":               {Status: "displayed", Ref: "lead next-step target 名"},
	"DrilldownEvidenceID":           {Status: "known_gap", Ref: "OM-3"},
	"DrilldownRelation":             {Status: "known_gap", Ref: "OM-3"},
	"MergedEvidenceIDs":             {Status: "displayed", Ref: "证据索引 E#(+N)"},
	"MergedCount":                   {Status: "displayed", Ref: "行1 ×N 五式"},
	"MergedMinMS":                   {Status: "displayed", Ref: "N次(a~b) 区间"},
	"MergedMaxMS":                   {Status: "displayed", Ref: "N次(a~b) 区间"},
	"MergedValuelessCount":          {Status: "displayed", Ref: "合并明细(值缺席成员)"},
	"MergedIntervalUnion":           {Status: "displayed", Ref: "N次union口径"},
	"MergedSumMS":                   {Status: "displayed", Ref: "×N SUM 口径"},
	"MergedQueryWindows":            {Status: "displayed", Ref: "明细窗来源(多窗合并行)"},
	"MergedCrossWindowMax":          {Status: "displayed", Ref: "N次跨窗取最大口径"},
	"MergedMaxWindowStartTs":        {Status: "displayed", Ref: "跨窗最大成员窗标"},
	"MergedMaxWindowEndTs":          {Status: "displayed", Ref: "跨窗最大成员窗标"},
	"RankQueryWindowStartTs":        {Status: "displayed", Ref: "多榜窗 chip(根因排序#N·窗X)"},
	"RankQueryWindowEndTs":          {Status: "displayed", Ref: "多榜窗 chip"},
	"MergedActualDonorCumulativeMS": {Status: "internal_gate", Ref: "W-6 纯判官字段(DISP-3 假⚠ carve-out)"},
	// CR-2 组③ P7 (2026-07-12): the actual channel's physical interval — the
	// ⚠/超出发生段/区间未发布 word-face containment judge (never printed raw).
	"ActualWindowStartTs":   {Status: "internal_gate", Ref: "CR-2 P7 ⚠ 词面区间包含判官(runtimeTraceProjActualWindowScope)"},
	"ActualWindowEndTs":     {Status: "internal_gate", Ref: "CR-2 P7 ⚠ 词面区间包含判官(runtimeTraceProjActualWindowScope)"},
	"DuplicatePublications": {Status: "displayed", Ref: "行1 N次同值 + 明细重复发布"},
	"MergedSubjects":        {Status: "displayed", Ref: "×N 成员清单"},
	"SecondaryObjects":      {Status: "displayed", Ref: "明细影响点清单"},
	// ENG-2 (复核冷读 CP1-③, 2026-07-12): the absorbed idle-cadence
	// annotation — 「其中 X.XXXms 帧间空闲(等待下一帧)/周期空闲(…)」 on the
	// surviving seat + the matching teaching legend entry.
	"IdleCadenceMS":                {Status: "displayed", Ref: "行内 其中X.XXXms 帧间/周期空闲 注记"},
	"IdleCadenceKind":              {Status: "displayed", Ref: "同上(词面分叉键)+图例条"},
	"PriorityInversionCandidate":   {Status: "displayed", Ref: "行1 ⇅ + 行2(典型经 canonical Object 词位)"},
	"RunnableBelowRTPreempted":     {Status: "displayed", Ref: "行2 (优先级低于RT) 尾缀"},
	"OnChainOverflowFold":          {Status: "displayed", Ref: "行1 折叠行(其余N项)"},
	"MergedAllDataGap":             {Status: "displayed", Ref: "全零注(禁席禁徽章门)"},
	"SameValueMembers":             {Status: "displayed", Ref: "证据索引 audit(W-3 设计:仅 audit 面)"},
	"FamilyMemberCount":            {Status: "displayed", Ref: "行1 合计词位 + 明细家族合并"},
	"FamilyMemberMaxMS":            {Status: "displayed", Ref: "明细成员范围"},
	"FamilyMemberMinMS":            {Status: "displayed", Ref: "明细成员范围"},
	"FamilyMemberSumMS":            {Status: "displayed", Ref: "行3 族形/明细"},
	"FamilyFoldCaliber":            {Status: "displayed", Ref: "族口径词"},
	"FamilyMemberRoster":           {Status: "displayed", Ref: "明细成员/区分键"},
	"RankFamilyKey":                {Status: "displayed", Ref: "明细链上并入(G1 对账键)"},
	"AbsorbedByRankFamily":         {Status: "displayed", Ref: "明细链上并入 + audit"},
	"AbsorbedInto":                 {Status: "displayed", Ref: "明细链上并入"},
	"BackgroundRank":               {Status: "exempt", Ref: "W-2 §29.36.2(词面缺席合规;semlead fold 转移为载体保真)"},
	"Inode":                        {Status: "displayed", Ref: "明细区分键"},
	"Dev":                          {Status: "displayed", Ref: "明细区分键"},
	"SubjectKind":                  {Status: "displayed", Ref: "行1 (跨线程累计,非墙钟) + ≈密度", Token: ".IsAggregateMetric("},
	"BlockingKind":                 {Status: "displayed", Ref: "行1 ⊗+词位 + 明细五键行族"},
	"BlockingPeer":                 {Status: "displayed", Ref: "行2 持有点对端"},
	"BlockingHolderSite":           {Status: "displayed", Ref: "明细持有点"},
	"BlockingFromSite":             {Status: "displayed", Ref: "明细等待点行"},
	"BlockingWaiters":              {Status: "known_gap", Ref: "OM-2"},
	"BlockingHolderSource":         {Status: "displayed", Ref: "明细持有者来历 + 推断 qualifier"},
	"BlockingOwnerTidRaw":          {Status: "displayed", Ref: "幻影 tid 半场词面"},
	"BlockingHolderHandoff":        {Status: "displayed", Ref: "换手披露三面"},
	"BlockingHolderContradiction":  {Status: "displayed", Ref: "自相矛盾撤回披露"},
	"BlockingSubjectIsHolder":      {Status: "displayed", Ref: "HOLD 朝向词面(twin-port)"},
	"TypeToken":                    {Status: "displayed", Ref: "行1 词位输入 + 行2 类别词 + 明细类型(raw)"},
	"GatedRunnableMS":              {Status: "displayed", Ref: "行1 词位构成 + 行3 恒等式(反转行)"},
	"GatedRunningDeficitMS":        {Status: "displayed", Ref: "行3 拆解(反转行)"},
	"PeriodicSource":               {Status: "displayed", Ref: "行2 周期性信号源 tag + 图例条"},
	"DetectedPeriodMS":             {Status: "displayed", Ref: "行2 周期 tag 值"},
	"PeriodicLatenessMS":           {Status: "known_gap", Ref: "OM-1"},
	"OccupierSummary":              {Status: "displayed", Ref: "行2 同窗占用者尾 tag + 明细"},
	"SupplyFoldComputed":           {Status: "displayed", Ref: "行2 折算 clause 在场信号"},
	"SupplyFoldDeficitMS":          {Status: "displayed", Ref: "行3 恒等式/拆解(供给折算)"},
	"SupplyFoldIdealMS":            {Status: "exempt", Ref: "W-9(值不印;拆解行经口径词代言)"},
	"SupplyFoldKnownMS":            {Status: "exempt", Ref: "W-9"},
	"SupplyFoldUnknownMS":          {Status: "exempt", Ref: "W-9"},
	"SupplyFoldCapabilitySource":   {Status: "displayed", Ref: "折算口径 capability 词面 fork"},
	"GatedCapabilitySource":        {Status: "displayed", Ref: "按下游消费核 capability 披露"},
	"SupplyFoldReferenceClass":     {Status: "displayed", Ref: "按X核满频 basis 词面"},
	"SupplyFoldTopologySource":     {Status: "displayed", Ref: "簇拓扑折算词面 fork"},
	"GatedTopologySource":          {Status: "displayed", Ref: "gated 簇拓扑词面 fork"},
	"ThermalCapKHz":                {Status: "displayed", Ref: "热限压披露句"},
	"RunnableMS":                   {Status: "displayed", Ref: "行2 机制句量级(RN-1 显著性门)"},
	"DStateSplitMS":                {Status: "displayed", Ref: "WO-A1 加法恒等式判定输入(不可相加指针词面)"},
	"IOWaitSplitMS":                {Status: "displayed", Ref: "WO-A1 加法恒等式判定输入(不可相加指针词面)"},
	"OverflowMirrorEvidenceIDs":    {Status: "displayed", Ref: "WO-D1③ 折叠池 headline 多引用镜像 tag"},
	"OverflowProjectionEvidenceID": {Status: "displayed", Ref: "P2-2 折叠池跨口径投影 tag"},
	"FullWindowStateMS":            {Status: "displayed", Ref: "行2 全窗合计 tag + 明细全窗合计行"},
	"FullWindowStateSource":        {Status: "displayed", Ref: "全窗合计来源词"},
	"FullWindowStateWindowStart":   {Status: "displayed", Ref: "全窗合计窗标"},
	"FullWindowStateWindowEnd":     {Status: "displayed", Ref: "全窗合计窗标"},
	"FullWindowStateSameWindow":    {Status: "displayed", Ref: "全窗==查询窗判词(F-2)"},
}

// --- T1 · TraceCausalProjection contract (A 区) --------------------------------

var projectionFieldContract = map[string]fieldDisposition{
	"PrimaryRootCause":             {Status: "displayed", Ref: "行族桶来源(F1–F7)"},
	"PrimaryRootCauses":            {Status: "displayed", Ref: "行族桶来源"},
	"OnChainCauses":                {Status: "displayed", Ref: "行族桶来源"},
	"AdjacentCauses":               {Status: "displayed", Ref: "◇ 区段来源"},
	"BackgroundCauses":             {Status: "displayed", Ref: "▒ 区段来源"},
	"SemanticSpans":                {Status: "displayed", Ref: "✦ 语义行来源"},
	"WakeupPath":                   {Status: "displayed", Ref: "树干 rails/edge"},
	"WakeupPathUserElected":        {Status: "internal_gate", Ref: "R2 比较短路"},
	"WakeupPathUserEntityHits":     {Status: "internal_gate", Ref: "W-8 折叠强拆选择器(效果结构性可见)"},
	"WakeupPathBranch":             {Status: "internal_gate", Ref: "W-7 depth 挂靠门"},
	"WakeupPathRootDepth":          {Status: "internal_gate", Ref: "W-7 depth 挂靠门"},
	"WakeupPathQueryWindowStartTs": {Status: "internal_gate", Ref: "W-7 窗对门"},
	"WakeupPathQueryWindowEndTs":   {Status: "internal_gate", Ref: "W-7 窗对门"},
	"SupportingHops":               {Status: "displayed", Ref: "并入树行池 + 证据索引"},
	"WakeupChainRecommendedNotRun": {Status: "displayed", Ref: "flat 三形树头(⊘ 未下钻)"},
	"RootCauseFamilyObserved":      {Status: "displayed", Ref: "树头分叉"},
	"WindowStartTs":                {Status: "displayed", Ref: "满格=窗口/回退尺度句 + 证据索引"},
	"WindowEndTs":                  {Status: "displayed", Ref: "尺度句"},
	"ArtifactPath":                 {Status: "displayed", Ref: "多工件树头"},
	"ArtifactLabel":                {Status: "displayed", Ref: "多工件树头/章节后缀"},
	"CapacityTruncated":            {Status: "displayed", Ref: "证据索引头披露"},
	"QueryWindows":                 {Status: "displayed", Ref: "树头含N个查询窗"},
	"QueryWindowsTruncated":        {Status: "displayed", Ref: "查询窗清单截断注"},
	"AbsorbedChainRows":            {Status: "displayed", Ref: "明细链上并入 + 证据索引"},
	"TargetStateAccount":           {Status: "displayed", Ref: "F10 两行(Σ==窗门) + 图例条"},
}

// --- T1 · TargetStateAccount / QueryWindow contracts (C 区) --------------------
//
// Field names of these small structs collide with Node/RankItem names in the
// display sources (e.g. account.EvidenceID vs node.EvidenceID), so scan arms
// are disabled row-by-row with the collision documented; the registration,
// ghost, and registry-reference arms still apply.

var targetStateAccountContract = map[string]fieldDisposition{
	"Subject":                {Status: "displayed", Ref: "F10 账主语", NoScan: true},
	"RunningMS":              {Status: "displayed", Ref: "F10 两行分量", NoScan: true},
	"RunnableMS":             {Status: "displayed", Ref: "F10 两行分量", NoScan: true},
	"SleepMS":                {Status: "displayed", Ref: "F10 两行分量", NoScan: true},
	"DStateMS":               {Status: "displayed", Ref: "F10 两行分量", NoScan: true},
	"IOWaitMS":               {Status: "displayed", Ref: "F10 两行分量", NoScan: true},
	"SleepIOWaitMS":          {Status: "displayed", Ref: "其中IO等待从句(Σ门忽略)", NoScan: true},
	"TotalMS":                {Status: "exempt", Ref: "W-10", NoScan: true},
	"DeterministicRunningMS": {Status: "displayed", Ref: "F10 确定性运行从句", NoScan: true},
	"WindowStartTs":          {Status: "internal_gate", Ref: "F-2 容差锚窗门", NoScan: true},
	"WindowEndTs":            {Status: "internal_gate", Ref: "F-2 容差锚窗门", NoScan: true},
	"EvidenceID":             {Status: "known_gap", Ref: "OM-5", NoScan: true},
}

var queryWindowContract = map[string]fieldDisposition{
	"StartTs": {Status: "displayed", Ref: "窗清单/audit 载体", NoScan: true},
	"EndTs":   {Status: "displayed", Ref: "窗清单/audit 载体", NoScan: true},
}

// --- T1 · display-layer fold-peer carrier (OM-6 host struct) -------------------

var rankFoldPeerContract = map[string]fieldDisposition{
	"TypeWord":           {Status: "known_gap", Ref: "OM-6"},
	"Rank":               {Status: "displayed", Ref: "行2 榜位(fold-adopted)", NoScan: true},
	"Confidence":         {Status: "displayed", Ref: "行2 置信(fold-adopted)", NoScan: true},
	"EvidenceTag":        {Status: "displayed", Ref: "行1 [E#+E#] bracket + 明细根因排序行"},
	"CumulativeImpactMS": {Status: "internal_gate", Ref: "W-A 累计相等 fold guard + 覆盖分子不变量", NoScan: true},
	"DisplayImpactMS":    {Status: "internal_gate", Ref: "bar scale/unadmitted-disclosure MAX 不变量"},
	"TargetImpactMS":     {Status: "internal_gate", Ref: "覆盖分子不变量(COV D-1)", NoScan: true},
}

// --- T2 armB · RootCauseRankItem contract (D 区) --------------------------------

// rankItemNodeMirror maps wire field → projection mirror where the names
// differ (same-name and Ms→MS-normalized mirrors resolve automatically).
var rankItemNodeMirror = map[string]string{
	"Type":                    "TypeToken",
	"Thread":                  "Subject",
	"DominantState":           "StateKind",
	"LatenessMs":              "PeriodicLatenessMS",
	"ProjectedImpactMs":       "ImpactMS",
	"GatedClusterTopology":    "GatedTopologySource",
	"SupplyFoldBasis":         "SupplyFoldComputed",
	"HolderSite":              "BlockingHolderSite",
	"SubjectIsLockHolder":     "BlockingSubjectIsHolder",
	"HolderSource":            "BlockingHolderSource",
	"OwnerTidRaw":             "BlockingOwnerTidRaw",
	"HolderHandoff":           "BlockingHolderHandoff",
	"HolderSelfContradiction": "BlockingHolderContradiction",
	"MemberCount":             "FamilyMemberCount",
	"MemberRoster":            "FamilyMemberRoster",
	"MemberMaxMs":             "FamilyMemberMaxMS",
	"MemberMinMs":             "FamilyMemberMinMS",
	"MemberSumMs":             "FamilyMemberSumMS",
	"MemberFoldCaliber":       "FamilyFoldCaliber",
	"AbsorbedIntoFamily":      "AbsorbedInto",
	"AbsorbedChainRows":       "AbsorbedChainRows", // projection-level mirror
}

var rankItemContract = map[string]fieldDisposition{
	"Rank":                           {Status: "node_mirror", Ref: "Node.Rank"},
	"Tier":                           {Status: "node_mirror", Ref: "Node.Tier"},
	"BackgroundRank":                 {Status: "node_mirror", Ref: "Node.BackgroundRank(W-2 词面豁免在 Node 侧)"},
	"Type":                           {Status: "node_mirror", Ref: "Node.TypeToken"},
	"SubjectKind":                    {Status: "node_mirror", Ref: "Node.SubjectKind"},
	"Thread":                         {Status: "node_mirror", Ref: "Node.Subject"},
	"PhysicalSourcePath":             {Status: "exempt", Ref: "W-19"},
	"PerfContext":                    {Status: "exempt", Ref: "W-18"},
	"PerfContexts":                   {Status: "exempt", Ref: "W-18"},
	"StartTs":                        {Status: "node_mirror", Ref: "Node.StartTs"},
	"EndTs":                          {Status: "node_mirror", Ref: "Node.EndTs"},
	"ActualStartTs":                  {Status: "exempt", Ref: "W-12(收窄已折 IC-A)"},
	"ActualEndTs":                    {Status: "exempt", Ref: "W-12(收窄已折 IC-A)"},
	"DominantState":                  {Status: "node_mirror", Ref: "Node.StateKind"},
	"RunningMs":                      {Status: "known_gap", Ref: "OM-12"},
	"RunnableMs":                     {Status: "node_mirror", Ref: "Node.RunnableMS"},
	"SleepMs":                        {Status: "known_gap", Ref: "OM-12"},
	"DStateMs":                       {Status: "known_gap", Ref: "OM-12"},
	"IOWaitMs":                       {Status: "known_gap", Ref: "OM-12"},
	"DStateAllNonIOProven":           {Status: "note_consumed", Ref: "dstate_all_noniowait → Node.DStateRefinedNonIO"},
	"DStateCauseUnprovenRemainder":   {Status: "note_consumed", Ref: "dstate_cause_unproven_remainder → Node.DStateCauseUnprovenRemainder"},
	"BlockedReasonCaller":            {Status: "node_mirror", Ref: "Node.BlockedReasonCaller"},
	"BlockedReasonWindowCount":       {Status: "node_mirror", Ref: "CR-3 件② Node.BlockedReasonWindowCount"},
	"BlockedReasonWindowCaller":      {Status: "node_mirror", Ref: "CR-3 件② Node.BlockedReasonWindowCaller"},
	"ProcessComm":                    {Status: "note_consumed", Ref: "process_comm → Node.ProcessComm(CR-3 件③,+板摘要席位)"},
	"ImpactMs":                       {Status: "node_mirror", Ref: "Node.ImpactMS"},
	"ProjectedImpactMs":              {Status: "node_mirror", Ref: "Node.ImpactMS(窗口投影值;projected_impact_ms note 为 display-only 回声)"},
	"CumulativeImpactMs":             {Status: "node_mirror", Ref: "Node.CumulativeImpactMS"},
	"EffectiveImpactMs":              {Status: "node_mirror", Ref: "Node.EffectiveImpactMS"},
	"RankSortBoostedEffectiveMs":     {Status: "exempt", Ref: "W-1(json:\"-\" 内部排序道)"},
	"GatedRunnableMs":                {Status: "node_mirror", Ref: "Node.GatedRunnableMS"},
	"GatedRunningDeficitMs":          {Status: "node_mirror", Ref: "Node.GatedRunningDeficitMS"},
	"GatedCapabilitySource":          {Status: "node_mirror", Ref: "Node.GatedCapabilitySource"},
	"GatedClusterTopology":           {Status: "node_mirror", Ref: "Node.GatedTopologySource"},
	"PeriodicSource":                 {Status: "node_mirror", Ref: "Node.PeriodicSource"},
	"DetectedPeriodMs":               {Status: "node_mirror", Ref: "Node.DetectedPeriodMS"},
	"LatenessMs":                     {Status: "node_mirror", Ref: "Node.PeriodicLatenessMS(镜像在;显示缺口挂 Node 侧 OM-1)"},
	"SupplyFoldDeficitMs":            {Status: "node_mirror", Ref: "Node.SupplyFoldDeficitMS"},
	"SupplyFoldIdealMs":              {Status: "node_mirror", Ref: "Node.SupplyFoldIdealMS(W-9 值不印在 Node 侧)"},
	"SupplyFoldBasis":                {Status: "node_mirror", Ref: "Node.SupplyFoldComputed(fold_basis 在场信号)"},
	"TargetImpactMs":                 {Status: "node_mirror", Ref: "Node.TargetImpactMS"},
	"ActualImpactMs":                 {Status: "node_mirror", Ref: "Node.ActualImpactMS"},
	"ActualTotalMs":                  {Status: "node_mirror", Ref: "Node.ActualTotalMS"},
	"Score":                          {Status: "exempt", Ref: "W-1 S1 修根"},
	"Confidence":                     {Status: "node_mirror", Ref: "Node.Confidence"},
	"LineStart":                      {Status: "node_mirror", Ref: "Node.LineStart"},
	"LineEnd":                        {Status: "node_mirror", Ref: "Node.LineEnd"},
	"Source":                         {Status: "exempt", Ref: "W-14(收窄已折 IC-A)"},
	"Causality":                      {Status: "node_mirror", Ref: "Node.Causality"},
	"ChainRelevance":                 {Status: "node_mirror", Ref: "Node.ChainRelevance"},
	"ChainDepth":                     {Status: "node_mirror", Ref: "Node.ChainDepth"},
	"ChainBranch":                    {Status: "node_mirror", Ref: "Node.ChainBranch(W-7 门在 Node 侧)"},
	"OverlapMs":                      {Status: "known_gap", Ref: "OM-9"},
	"EdgeCount":                      {Status: "known_gap", Ref: "OM-9"},
	"NearestChainThread":             {Status: "known_gap", Ref: "OM-9"},
	"NearestChainWindow":             {Status: "known_gap", Ref: "OM-9"},
	"OccurrenceWindows":              {Status: "exempt", Ref: "W-13"},
	"StatsWindowStartTs":             {Status: "note_consumed", Ref: "note:selected_window → Node.QueryWindow*(W-17 非缺口)"},
	"StatsWindowEndTs":               {Status: "note_consumed", Ref: "note:selected_window → Node.QueryWindow*(W-17 非缺口)"},
	"BlockingKind":                   {Status: "node_mirror", Ref: "Node.BlockingKind"},
	"BlockingPeer":                   {Status: "node_mirror", Ref: "Node.BlockingPeer"},
	"HolderSite":                     {Status: "node_mirror", Ref: "Node.BlockingHolderSite"},
	"BlockingFromSite":               {Status: "node_mirror", Ref: "Node.BlockingFromSite"},
	"SubjectIsAnalysisTarget":        {Status: "engine_gate", Ref: "SYM §24.13 裁定一 target_self_state 降级门(query.go/root_cause_rank_capacity.go);效果经 tier 词面现形"},
	"RunnableBelowRTPreempted":       {Status: "node_mirror", Ref: "Node.RunnableBelowRTPreempted"},
	"SubjectIsLockHolder":            {Status: "node_mirror", Ref: "Node.BlockingSubjectIsHolder"},
	"HolderSource":                   {Status: "node_mirror", Ref: "Node.BlockingHolderSource"},
	"OwnerTidRaw":                    {Status: "node_mirror", Ref: "Node.BlockingOwnerTidRaw"},
	"HolderNsUnification":            {Status: "known_gap", Ref: "OM-10"},
	"HolderHostProcess":              {Status: "known_gap", Ref: "OM-10"},
	"HolderHandoff":                  {Status: "node_mirror", Ref: "Node.BlockingHolderHandoff"},
	"HolderSelfContradiction":        {Status: "node_mirror", Ref: "Node.BlockingHolderContradiction"},
	"DrillStatus":                    {Status: "known_gap", Ref: "OM-7"},
	"InheritedTargetBlockedMs":       {Status: "known_gap", Ref: "OM-13"},
	"PriorityInversionLockDominated": {Status: "known_gap", Ref: "OM-8"},
	"SpanName":                       {Status: "node_mirror", Ref: "Node.SpanName"},
	"SpanKind":                       {Status: "node_mirror", Ref: "Node.SpanKind(镜像在;显示缺口挂 Node 侧 OM-4)"},
	"SpanCategory":                   {Status: "node_mirror", Ref: "Node.SpanCategory(OM-4 在 Node 侧)"},
	"SpanSubcategory":                {Status: "node_mirror", Ref: "Node.SpanSubcategory(OM-4 在 Node 侧)"},
	"SemanticClass":                  {Status: "node_mirror", Ref: "Node.SemanticClass"},
	"MemberCount":                    {Status: "node_mirror", Ref: "Node.FamilyMemberCount"},
	"MemberRoster":                   {Status: "node_mirror", Ref: "Node.FamilyMemberRoster"},
	"MemberMaxMs":                    {Status: "node_mirror", Ref: "Node.FamilyMemberMaxMS"},
	"MemberMinMs":                    {Status: "node_mirror", Ref: "Node.FamilyMemberMinMS"},
	"MemberSumMs":                    {Status: "node_mirror", Ref: "Node.FamilyMemberSumMS"},
	"MemberFoldCaliber":              {Status: "node_mirror", Ref: "Node.FamilyFoldCaliber"},
	"MemberKey":                      {Status: "exempt", Ref: "W-15"},
	"Inode":                          {Status: "node_mirror", Ref: "Node.Inode"},
	"Dev":                            {Status: "node_mirror", Ref: "Node.Dev"},
	"TraceGapKind":                   {Status: "node_mirror", Ref: "Node.TraceGapKind"},
	"RankFamilyKey":                  {Status: "node_mirror", Ref: "Node.RankFamilyKey"},
	"AbsorbedChainRows":              {Status: "node_mirror", Ref: "Projection.AbsorbedChainRows"},
	"AbsorbedRankRows":               {Status: "exempt", Ref: "W-16(收窄已折 IC-A)"},
	"AbsorbedByRankFamily":           {Status: "node_mirror", Ref: "Node.AbsorbedByRankFamily"},
	"AbsorbedIntoFamily":             {Status: "node_mirror", Ref: "Node.AbsorbedInto"},
	"Summary":                        {Status: "exempt", Ref: "W-11(LLM 面载体)"},
}

// --- census machinery -----------------------------------------------------------

var infoContractDisplayAuthorityFiles = []string{
	"answer_document_mutation_runtime_tree.go",
	"answer_document_mutation_runtime_rcr.go",
	"answer_document_mutation_runtime.go",
	"answer_document_mutation_runtime_typelabels.go",
	"answer_document_mutation_runtime_supplyfold.go",
	"answer_document_mutation_runtime_rcm.go",
	// SMR-1 批 (2026-07-12): the relation-arm passes (WO-A1/D2/D3/C1/B1).
	"answer_document_mutation_runtime_smr1.go",
}

func readDisplayAuthoritySources(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, name := range infoContractDisplayAuthorityFiles {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(uxg1StripComments(string(raw)))
		b.WriteString("\n")
	}
	return b.String()
}

func readTracequeryEngineSources(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir("../tracequery")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("../tracequery", name))
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(uxg1StripComments(string(raw)))
		b.WriteString("\n")
	}
	return b.String()
}

func identTokenAppears(src, field, override string) bool {
	if override != "" {
		return strings.Contains(src, override)
	}
	re := regexp.MustCompile(`\.` + regexp.QuoteMeta(field) + `\b`)
	return re.MatchString(src)
}

// infoContractCheckRegistration: registration + ghost + registry-reference
// arms shared by every contract table.
func infoContractCheckRegistration(t *testing.T, table string, typ reflect.Type, contract map[string]fieldDisposition, statuses map[string]bool, usedW, usedOM map[string]bool) {
	t.Helper()
	for _, f := range reflect.VisibleFields(typ) {
		if !f.IsExported() || f.Anonymous {
			continue
		}
		if _, ok := contract[f.Name]; !ok {
			t.Errorf("%s: 新字段 %s 未登记信息契约表(%v 中四选一)", table, f.Name, keysOf(statuses))
		}
	}
	for name, d := range contract {
		if _, ok := typ.FieldByName(name); !ok {
			t.Errorf("%s: 契约表幽灵行 %s(字段已不存在,同步删行)", table, name)
		}
		if !statuses[d.Status] {
			t.Errorf("%s: 字段 %s 状态 %q 不在本表合法集 %v", table, name, d.Status, keysOf(statuses))
		}
		for _, w := range regexp.MustCompile(`W-\d+`).FindAllString(d.Ref, -1) {
			usedW[w] = true
			if _, ok := infoContractExemptions[w]; !ok {
				t.Errorf("%s: 字段 %s 引用未登记豁免 %s", table, name, w)
			}
		}
		for _, om := range regexp.MustCompile(`OM-\d+`).FindAllString(d.Ref, -1) {
			usedOM[om] = true
			if _, ok := infoContractKnownGaps[om]; !ok {
				t.Errorf("%s: 字段 %s 引用未登记 known_gap %s", table, name, om)
			}
		}
		if d.Status == "exempt" && !regexp.MustCompile(`W-\d+`).MatchString(d.Ref) {
			t.Errorf("%s: exempt 行 %s 必须引用 W-# 豁免号", table, name)
		}
		if d.Status == "known_gap" && !regexp.MustCompile(`OM-\d+`).MatchString(d.Ref) {
			t.Errorf("%s: known_gap 行 %s 必须引用 OM-# 立案号", table, name)
		}
	}
}

func keysOf(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

var infoContractT1Statuses = map[string]bool{
	"displayed": true, "internal_gate": true, "exempt": true, "known_gap": true,
}

// infoContractTypesLedgerOMRefs reads the OM-# references out of the
// types-side note-key ledger block (internal/types
// info_contract_notekeys_census_test.go :: infoContractNoteKeyLedger) — the
// surface that carries the note-family half of the known-gap filings. Only
// the ledger var block is scanned, never the whole file (error-message prose
// must not mint references).
func infoContractTypesLedgerOMRefs(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../types/info_contract_notekeys_census_test.go")
	if err != nil {
		t.Fatalf("types note-key 账本表面缺失(F3 引用臂断):%v", err)
	}
	src := string(raw)
	start := strings.Index(src, "var infoContractNoteKeyLedger = map[string]string{")
	if start < 0 {
		t.Fatal("types note-key 账本块 infoContractNoteKeyLedger 缺失")
	}
	src = src[start:]
	if end := strings.Index(src, "\n}"); end >= 0 {
		src = src[:end]
	}
	seen := map[string]bool{}
	var out []string
	for _, om := range regexp.MustCompile(`OM-\d+`).FindAllString(src, -1) {
		if !seen[om] {
			seen[om] = true
			out = append(out, om)
		}
	}
	return out
}

var infoContractRankStatuses = map[string]bool{
	"node_mirror": true, "note_consumed": true, "engine_gate": true, "exempt": true, "known_gap": true,
}

// TestInfoContractFieldCensus — T1 registration/ghost/reference arms over all
// contract tables, plus the exemption/known-gap registry completeness checks
// (known_gap 引用集恰=13 OM; 豁免表零幽灵).
func TestInfoContractFieldCensus(t *testing.T) {
	usedW, usedOM := map[string]bool{}, map[string]bool{}
	infoContractCheckRegistration(t, "Node", reflect.TypeOf(types.TraceCausalProjectionNode{}), nodeFieldContract, infoContractT1Statuses, usedW, usedOM)
	infoContractCheckRegistration(t, "Projection", reflect.TypeOf(types.TraceCausalProjection{}), projectionFieldContract, infoContractT1Statuses, usedW, usedOM)
	infoContractCheckRegistration(t, "TargetStateAccount", reflect.TypeOf(types.TraceCausalProjectionTargetStateAccount{}), targetStateAccountContract, infoContractT1Statuses, usedW, usedOM)
	infoContractCheckRegistration(t, "QueryWindow", reflect.TypeOf(types.TraceCausalProjectionQueryWindow{}), queryWindowContract, infoContractT1Statuses, usedW, usedOM)
	infoContractCheckRegistration(t, "RankFoldPeer", reflect.TypeOf(runtimeTraceProjRankFoldPeer{}), rankFoldPeerContract, infoContractT1Statuses, usedW, usedOM)
	infoContractCheckRegistration(t, "RankItem", reflect.TypeOf(tracequery.RootCauseRankItem{}), rankItemContract, infoContractRankStatuses, usedW, usedOM)

	// The known-gap reference set must be EXACTLY the 13 OM findings (§29.40
	// acceptance: T1 known_gap 列表恰=13 OM). OM-11 has no wire FIELD (its
	// carriers are the peer_state_*/wait_object/peer_chain_* notes), so its
	// reference comes from the REAL types-side ledger surface — the
	// infoContractNoteKeyLedger block of the note-key census — read
	// mechanically here instead of being decreed (修复轮 F3: "恰13" 不得钦定).
	for _, om := range infoContractTypesLedgerOMRefs(t) {
		usedOM[om] = true
		if _, ok := infoContractKnownGaps[om]; !ok {
			t.Errorf("types 侧 note-key 账本引用了未登记的 %s", om)
		}
	}
	if !usedOM["OM-11"] {
		t.Errorf("OM-11 无任何表面引用(types note-key 账本应承载其 note 族半场)")
	}
	for om := range infoContractKnownGaps {
		if !usedOM[om] {
			t.Errorf("known_gap 登记 %s 无任何契约行引用(幽灵立案)", om)
		}
	}
	for om := range usedOM {
		if _, ok := infoContractKnownGaps[om]; !ok {
			t.Errorf("契约行引用了未登记的 %s", om)
		}
	}
	if len(infoContractKnownGaps) != 13 {
		t.Errorf("known_gap 登记数 %d ≠ 13(§29.40 恰 13 OM)", len(infoContractKnownGaps))
	}
	// Exemption ghost check (field rows + non-field attachment sites).
	for w := range infoContractNonFieldExemptionSites {
		usedW[w] = true
	}
	for w := range infoContractExemptions {
		if !usedW[w] {
			t.Errorf("豁免登记 %s 无任何契约行/表面引用(幽灵豁免)", w)
		}
	}
	if len(infoContractExemptions) != 21 {
		t.Errorf("豁免登记数 %d ≠ 21(§29.40 全裁决)", len(infoContractExemptions))
	}
}

// TestInfoContractDisplayedClaimsHaveRealConsumers — T1 第二/第三臂 (杀
// OM-6/OM-10/W-2 假指针病 + 悄悄修而不销账): displayed/internal_gate rows
// must have a real token in the display authority; known_gap rows must NOT.
func TestInfoContractDisplayedClaimsHaveRealConsumers(t *testing.T) {
	src := readDisplayAuthoritySources(t)
	check := func(table string, contract map[string]fieldDisposition) {
		for name, d := range contract {
			if d.NoScan {
				continue
			}
			switch d.Status {
			case "displayed", "internal_gate":
				if !identTokenAppears(src, name, d.Token) {
					t.Errorf("%s.%s 声明 %s 但显示权威零引用(假指针;Ref=%s)", table, name, d.Status, d.Ref)
				}
			case "known_gap":
				if identTokenAppears(src, name, d.Token) {
					t.Errorf("%s.%s 已获显示权威引用,契约表须翻 displayed 并销 %s(账实一致纪律)", table, name, d.Ref)
				}
			}
		}
	}
	check("Node", nodeFieldContract)
	check("Projection", projectionFieldContract)
	check("TargetStateAccount", targetStateAccountContract)
	check("QueryWindow", queryWindowContract)
	check("RankFoldPeer", rankFoldPeerContract)
}

// TestInfoContractRankItemWireDisposition — T2 armB: node-mirror existence,
// known-gap reverse tripwire, note registration, engine-gate presence.
func TestInfoContractRankItemWireDisposition(t *testing.T) {
	nodeType := reflect.TypeOf(types.TraceCausalProjectionNode{})
	projType := reflect.TypeOf(types.TraceCausalProjection{})
	mirrorExists := func(field string) (string, bool) {
		if mapped, ok := rankItemNodeMirror[field]; ok {
			if _, ok := nodeType.FieldByName(mapped); ok {
				return mapped, true
			}
			if _, ok := projType.FieldByName(mapped); ok {
				return mapped, true
			}
			return mapped, false
		}
		for _, candidate := range []string{field, strings.TrimSuffix(field, "Ms") + "MS"} {
			if _, ok := nodeType.FieldByName(candidate); ok {
				return candidate, true
			}
		}
		return field, false
	}
	engineSrc := readTracequeryEngineSources(t)
	for name, d := range rankItemContract {
		switch d.Status {
		case "node_mirror":
			if mapped, ok := mirrorExists(name); !ok {
				t.Errorf("RankItem.%s 声明 node_mirror 但投影无 %s 字段(断链)", name, mapped)
			}
		case "known_gap":
			if mapped, ok := mirrorExists(name); ok {
				t.Errorf("RankItem.%s 已获投影镜像 %s,契约表须翻 node_mirror 并销 %s", name, mapped, d.Ref)
			}
		case "note_consumed":
			key := strings.TrimPrefix(d.Ref, "note:")
			key = strings.FieldsFunc(key, func(r rune) bool { return r == ' ' || r == '(' })[0]
			row, ok := types.TraceNoteKeyLookup(key)
			if !ok {
				t.Errorf("RankItem.%s 声明 note_consumed 但键 %q 未注册", name, key)
			} else if row.Carrier == types.TraceNoteCarrierDisplayOnly {
				t.Errorf("RankItem.%s 的载体键 %q 是 display_only(无解析消费者)——note_consumed 声明为假", name, key)
			}
		case "engine_gate":
			if !identTokenAppears(engineSrc, name, d.Token) {
				t.Errorf("RankItem.%s 声明 engine_gate 但 tracequery 非测试源零引用", name)
			}
		}
	}
}
