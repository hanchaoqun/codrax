# P5: Finalizer Skill Prompt Restructure — Detailed Design

**Status**: Active Design (2026-05-10)
**Baseline**: `0ba6b4d` HEAD at design time. `internal/skill/defaults.go::answer-document-skill` (23 Workflow items + 10 Prohibitions + ~10 OutputFormat sections + ~30 Per-block-kind tables, total ~5000 tokens).

**Scope of work**: per-rule classification analysis + tier-aware rendering surface in `internal/skill/`. **Implementation deferred to P5-B after this doc is approved.**

---

## §1 Why P5 (forensic context)

Per `docs/design/finalizer_pretrip_prevention.md` §3 root-cause analysis, defect #5:

> **Skill prompt size + soft-constraint density** — answer-document-skill prompt is ~5000 tokens with 23 workflow items + 10 prohibitions + 10 OutputFormat sections + 30 per-block-kind table cells. Each emit, the LLM picks a stochastic subset of rules to honor. Many rules are **soft prose conventions** (e.g., "use inline backticks for identifiers", "open with the conclusion") — these don't have hard schema enforcement, so LLM compliance is voluntary.

Forensic data (May-9 sweep, 26 run): **trigger axis distribution shows different runs fail on different soft-rule axes** — block_items_label (7 cases), inline_identifier (1), answer_prose_density (1), answer_summary_body_consistency (1) — suggesting LLM is randomly missing different rules per emit.

**P5 hypothesis**: If we (a) reduce the *first-emit* prompt density to load-bearing rules only, AND (b) inject specialized rules conditionally based on the question shape + retry path, the LLM's compliance probability per rule rises. The rules don't disappear — they're tier-gated by *applicability*.

**Estimated jitter削减**: 8% → 5% (per design doc §5; 3 percentage points absolute, on top of P1+P2+P3+P4+P6+P7's削减).

---

## §2 Complete inventory of answer-document-skill

### 2.1 Workflow items (23 total)

```
W1.  Write directly into emit_answer_document tool call
W2.  CRITICAL: Required Answer Blocks list is mandatory
W3.  blocks[] carrier (id required, kind required, optional fields enumerated)
W4.  claim_uses required on principal blocks (3-field shape)
W5.  edge_anchors optional (diagram-edge typed anchors)
W6.  diagram.kind semantic family (NOT Mermaid keyword)
W7.  enumeration ordered_list — items[] discipline
W8.  Abstraction-level matching for "what does each X do" questions
W9.  hop-chain ordered_list — items[] discipline
W10. Bucket alignment — user-named bucket labels MUST appear
W11. scalar block — text + items=[{citation_ref}] anchor
W12. decision block — verdict at start of text
W13. ordered_list grounded file:line per item
W14. summary-only / explanation answer prose
W15. Citation channel discipline (citations[], citation_ref, claim_uses[].facet_id)
W16. log-triage prefer hop-chain ordered_list
W17. Diagram-grounding contract
W18. Sealed-seed rule for diagram anchors (log-derived)
W19. Log-triage Errors tree coverage contract
W20. Subject discipline (relevance to question subject)
W21. Authority discipline (drift-bounded answers)
W22. Code-vs-narrative divergence (enumeration over code+comment)
W23. Absence-citation discipline (status='absent' contracts)
```

### 2.2 Prohibitions (10 total)

```
P1.  Don't write prose outside emit_answer_document tool call
P2.  Don't cite a file/line not in evidence
P3.  Don't invent line numbers
P4.  Don't put prose in citation.quote field
P5.  Don't pre-shrink prose for any block
P6.  Don't set citation_ref to zero-value sentinel; use -1 or pool index
P7.  Don't silently truncate user-bounded set
P8.  Don't invent codename labels (S1, F2, Stage-N) that aren't cited
P9.  Don't omit claim_uses on principal blocks
P10. Don't write Mermaid keywords in diagram.kind
```

### 2.3 OutputFormat sections (~10)

```
O1.  Tool choice (full vs patch)
O2.  Block contract (id, kind, payload)
O3.  Block-kind payloads (table)
O4.  Block-level optional fields
O5.  Claim annotations (REQUIRED on principal blocks) — full claim form list
O6.  Diagram contract (semantic family, language, body)
O7.  Diagram edge label vocabulary (rendered from BuildDiagramRelationContractDoc)
O8.  Citation pool (citations[], citation_ref, file/line/quote)
O9.  Enumeration completeness and bounded sets
O10. Length / Exact resolution / Per-block prose guidance / Visual structure
```

---

## §3 Tier classification matrix

Each rule is classified along three independent axes:

| Axis | Values | Meaning |
|---|---|---|
| **Criticality** | A / B | A = load-bearing for emit acceptance; B = style polish |
| **Applicability** | always / log / diagram / enumeration / scalar / decision / absence / write-mode-only | What kinds of questions the rule applies to |
| **Phase** | first / retry / both | When the LLM should see the rule |

A rule's effective rendering = `criticality && applicability-matches && phase-matches`.

### 3.1 Workflow items classification

| ID | Critical | Applies To | Phase | Notes |
|---|---|---|---|---|
| W1  | A | always | both | meta — emit_answer_document is the only delivery surface |
| W2  | A | always | both | Required Blocks list is hard-rejected; cannot be "polish" |
| W3  | A | always | both | blocks[] schema basics — every emit touches this |
| W4  | A | always | both | claim_uses required on principal — load-bearing |
| W5  | B | diagram | both | edge_anchors only matters with a diagram block |
| W6  | A | diagram | both | diagram.kind enum — hard reject if Mermaid keyword |
| W7  | A | enumeration (ordered_list principal) | both | enumeration item discipline; load-bearing for that family |
| W8  | B | enumeration + role/responsibility shape | both | abstraction-level prose polish |
| W9  | A | call-chain / mechanism (ordered_list principal) | both | hop-chain item discipline |
| W10 | A | bucket-partition (analyzer.buckets[] non-empty) | both | bucket label survival; hard reject |
| W11 | A | scalar (principal) | both | scalar block + cite anchor pattern |
| W12 | A | decision (principal) | both | decision block + verdict at start |
| W13 | A | enumeration (ordered_list) | both | grounded file:line per item — anti-fabrication |
| W14 | A | summary-only (principal) | both | summary block prose discipline |
| W15 | A | always | both | citation channel discipline (P1's concern) — load-bearing |
| W16 | B | log-triage | both | log → prefer hop-chain — applicability filter only |
| W17 | A | diagram | both | diagram-grounding gate — hard reject |
| W18 | B | log-triage + diagram present | both | sealed-seed rule, only when log + mermaid seed |
| W19 | A | log-triage | both | Errors tree coverage — hard reject if naming missing |
| W20 | B | always | both | subject discipline — soft style |
| W21 | B | log-triage with drift | both | authority discipline — log-only |
| W22 | A | enumeration over implementers | both | code-vs-narrative divergence — hard rejected |
| W23 | A | exact_resolution.status='absent' | both | absence citation discipline — hard rejected |

**Tier A count**: 16/23 (W1, W2, W3, W4, W6, W7, W9-W15, W17, W19, W22, W23)
**Tier B count**: 7/23 (W5, W8, W16, W18, W20, W21)

### 3.2 Prohibitions classification

| ID | Critical | Applies To | Phase | Notes |
|---|---|---|---|---|
| P1  | A | always | both | meta — text outside tool doesn't ship |
| P2  | A | always | both | citation grounding — hard reject |
| P3  | A | always | both | line invention — hard reject |
| P4  | A | always | both | quote field discipline — auto-strip |
| P5  | B | always | both | length policy soft — schema emits cap if hit |
| P6  | A | always | both | citation_ref sentinel — hard reject |
| P7  | A | bounded enumeration | both | hard reject for partial-set silent emit |
| P8  | B | always | both | codename invention — soft polish |
| P9  | A | principal blocks with claim_form list | both | claim_uses presence — same as W4 |
| P10 | A | diagram present | both | Mermaid-keyword-in-kind — hard reject |

**Tier A count**: 8/10
**Tier B count**: 2/10 (P5, P8)

### 3.3 OutputFormat sections classification

| ID | Critical | Applies To | Phase | Notes |
|---|---|---|---|---|
| O1  | A | always | both | tool choice routing |
| O2-O4 | A | always | both | block contract reference |
| O5  | A | principal-claim-required | both | claim annotation table |
| O6  | A | diagram | both | diagram contract |
| O7  | B | diagram present | both | edge label vocabulary — only if diagram |
| O8  | A | always | both | citation pool — every answer cites |
| O9  | A | bounded enumeration | both | completeness contract |
| O10 | B | always | both | visual structure / length / per-block prose — style |

**Tier A count**: 6 (O1-O6, O8, O9)
**Tier B count**: 2 (O7, O10)

---

## §4 Per-rule classification rationale (audit table)

For each Tier B item, rationale why it's NOT load-bearing:

| ID | Rule | Why Tier B |
|---|---|---|
| W5  | edge_anchors | Optional field; absence has zero rejection consequence |
| W8  | abstraction-level matching | Soft prose convention; reviewer flags but doesn't gate emit |
| W16 | log-triage hop-chain preference | "Prefer" not "must" — choice within a family |
| W18 | sealed-seed rule | Specialized; only log-attached questions with mermaid seed |
| W20 | subject discipline | Style polish; reviewer reviews but doesn't hard-reject |
| W21 | authority discipline | Log-drift specific; only triggers when ApplyAuthorityHedging fired |
| O7  | edge label vocabulary | Reference table; only useful when emitting diagram edges |
| O10 | per-block prose / visual / length | Style polish; reviewer + length cap handle the gates |
| P5  | don't pre-shrink prose | Length policy soft — tool emits cap when hit |
| P8  | don't invent codename labels | Style polish; downstream codename-grounding gate catches |

For Tier B rules, applicability gating means: **first-emit prompt does NOT include them when the question doesn't match**. Retry prompt MAY include them when previous emit triggered a corresponding violation (e.g. ViolDiagramEdgeLabelMismatch → W5+O7 surface on retry).

---

## §5 Tier rendering strategy

### 5.1 Default first-emit prompt (Tier A only)

Render order:
1. Goal + meta-rules (W1, P1)
2. Block-shape contract (W2, W3, W4, P9, P10) — block coverage / claim_uses
3. Per-kind discipline only for kinds the family actually uses:
   - `principal_kind=ordered_list` + `intent=enumerate` → W7, W13
   - `principal_kind=ordered_list` + `intent=trace/explain` → W9, W13
   - `principal_kind=summary` → W14
   - `principal_kind=scalar` → W11
   - `principal_kind=decision` → W12
4. Bucket alignment (W10) only when `buckets[]` non-empty
5. Diagram contract (W6, W17, P10, O6) only when `view.DiagramPlan.Required`
6. Citation discipline (W15, O8, P2, P3, P4, P6) — always
7. Special-case: log-triage (W19) only when `LogTriage` non-nil
8. Special-case: absence (W23) only when contract may set `exact_resolution.status='absent'`

**Estimated first-emit prompt size**: ~3000 tokens (was ~5000) — 40% reduction.

### 5.2 Retry-emit prompt (Tier A + relevant Tier B)

Tier B rules become visible IF they match a violation kind in the prior emit's RetryState:

| Violation | Show Tier B rule |
|---|---|
| ViolDiagramEdgeLabelMismatch / ViolDiagramRelationLabelOnly | W5, O7 (edge anchors) |
| ViolPrincipalProseUnderfilled / answer_semantic_quality concern | W8, W20, O10 |
| ViolEnumerationLabelHallucinated | W22 (code-vs-narrative) |
| ViolAuthorityOverreach | W21 (authority drift) |
| ViolBlockCoverageMissing | always already in Tier A |

This way the LLM's retry prompt grows to address the specific gap, not a generic "all rules dump".

### 5.3 Always-show "What this dispatch needs (priority)" snippet

Following the prompt's existing pattern (the user-section's "Required Answer Blocks" list), add a short top-of-prompt snippet:

```
## Priority for this dispatch

1. Required blocks: 1× summary, 2× ordered_list, 1× caveat (read user-section for full contract)
2. Citations: every claim cited via items[i].citation_ref pointing to citations[]
3. Diagram: required (kind=architecture)
4. Length: no hard cap unless tool rejects with one
```

This is ~80 tokens, dynamic per-dispatch, and tells the LLM what's load-bearing for THIS question.

---

## §6 Implementation approach

### 6.1 Skill struct extension

Current shape (`internal/skill/types.go`):

```go
type Config struct {
    Name         string
    Goal         string
    Workflow     []string
    Prohibitions []string
    OutputFormat string
    ToolSuggestions []string
}
```

P5-B will extend to:

```go
type Config struct {
    Name string
    Goal string
    
    // Workflow tier split. WorkflowTierA + WorkflowTierB combined
    // is the logical equivalent of the old Workflow []string (no
    // rule deletions). Rendering decides which tier to show based
    // on dispatch context.
    WorkflowTierA []WorkflowItem // load-bearing — always renders
    WorkflowTierB []WorkflowItem // style/applicability-gated
    
    // ProhibitionsTierA / ProhibitionsTierB — same split.
    ProhibitionsTierA []string
    ProhibitionsTierB []ProhibitionItem
    
    OutputFormat string
    ToolSuggestions []string
}

type WorkflowItem struct {
    Body         string
    AppliesTo    AppliesToFilter // log / diagram / enumeration / etc.
    OnViolation  []ViolationKind // retry-only Tier B trigger
}

type ProhibitionItem struct {
    Body        string
    AppliesTo   AppliesToFilter
    OnViolation []ViolationKind
}

type AppliesToFilter struct {
    Always           bool
    PrincipalKinds   []AnswerBlockKind
    Intents          []Intent
    RequiresDiagram  bool
    RequiresLog      bool
    RequiresBuckets  bool
    AbsenceContract  bool
}
```

### 6.2 Renderer change site

`internal/context/builder.go` (or wherever skill prompt renders into the system message):

```go
// renderSkillBody picks the visible workflow/prohibition items
// based on the active dispatch context.
func renderSkillBody(cfg *skill.Config, ctx *types.AgentContext) string {
    var b strings.Builder
    b.WriteString("## Goal\n\n" + cfg.Goal + "\n\n")
    
    // Tier A always.
    for _, item := range cfg.WorkflowTierA {
        b.WriteString("- " + item.Body + "\n")
    }
    
    // Tier B filtered by applicability.
    for _, item := range cfg.WorkflowTierB {
        if shouldRenderItem(item.AppliesTo, ctx) {
            b.WriteString("- " + item.Body + "\n")
        }
    }
    
    // Prohibitions same dual-tier.
    // ...
    
    return b.String()
}

func shouldRenderItem(filter AppliesToFilter, ctx *types.AgentContext) bool {
    if filter.Always { return true }
    if filter.RequiresLog && hasLogTriage(ctx) { return true }
    if filter.RequiresDiagram && diagramRequired(ctx) { return true }
    if filter.RequiresBuckets && hasBuckets(ctx) { return true }
    // ... etc
    return false
}
```

### 6.3 Migration shape

P5-B rolls out as **6 incremental commits** to keep risk-bounded:

| Commit | Scope | LOC | Risk |
|---|---|---|---|
| 1 | skill.Config struct extension + types | ~80 | Low (additive types, no behavior change yet) |
| 2 | Migrate Workflow into WorkflowTierA/TierB (mechanical re-classification per §3) | ~200 | Med (every test that asserts on Workflow body needs re-inspection) |
| 3 | Add applicability filter renderer | ~100 | Med (new code path; old path still default) |
| 4 | Migrate Prohibitions same way | ~80 | Low (smaller surface) |
| 5 | Switch finalizer prompt to filtered rendering (gated yaml `pipeline_skill_tier_filter_enabled`) | ~50 | Med (fixture diff expected on every existing case) |
| 6 | Default-on after eval pass + delete legacy Workflow path | ~30 | Low (cleanup) |

### 6.4 Risk + rollback

| Risk | Mitigation |
|---|---|
| Tier classification wrong → load-bearing rule hidden → first-emit fails more | Extensive eval before default-on. Per-Tier-A item pinned in fixtures. |
| Renderer-side regression on existing single-shot tests | Yaml-gate the new path; legacy renderer keeps working. |
| Per-question shape detection (AppliesToFilter) miscalibrated | Conservative defaults: when in doubt, render Tier B (false-positive rendering is recoverable; false-negative hidden rule is not). |
| LLM behavior change on full sweep | 30+ case eval comparing pre/post prompt; gate flip only after 0% regression. |

---

## §7 Acceptance criteria

P5-B ships when:

- [ ] All 23 Workflow items + 10 Prohibitions classified in code per §3 tables
- [ ] First-emit prompt size ≤ 3500 tokens (was ~5000) on a representative dispatch
- [ ] Tier B rules render on retry when matching ViolKind appears
- [ ] go test ./... green across full suite
- [ ] Real-LLM eval on 30+ case sweep shows ≥ same PASS rate as pre-P5-B
- [ ] Each Tier A item has a unit test pinning it in the rendered prompt
- [ ] All LLM-facing strings re-audited (R3/R4/R5/R6/R7/SST + ATOMIC checklist)
- [ ] Yaml gate `pipeline_skill_tier_filter_enabled` defaults true after eval pass

---

## §8 Out of scope (P5-B explicitly does NOT do)

- ❌ Reword any existing Workflow body — the bodies are kept verbatim, only categorization changes
- ❌ Delete any rule — every rule moves into Tier A or Tier B; nothing is removed
- ❌ Touch other skills (analysis-skill, explore-skill, extract-skill) — those have their own separate audits
- ❌ Move OutputFormat sections into tier — OutputFormat is a single string today; tier split there is a follow-up if needed
- ❌ Build a yaml-driven tier override — operators can flip the gate only, not re-classify rules per deploy

---

## §9 References

- `docs/design/finalizer_pretrip_prevention.md` §3 defect #5, §4.5 P5 placeholder
- `internal/skill/defaults.go::answer-document-skill` (line 119+)
- `feedback_prompt_redline_checklist.md` ATOMIC 7-step audit
- May-9 sweep forensic data (project_session* memory entries)

---

**End of P5-A design.** P5-B implementation plan committed to a separate branch after this design is reviewed and approved.
