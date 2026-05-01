package repl

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

// handlePhaseCmd services the /phase subcommand family. Operator
// surface for stage II's multi-phase PlanGroups (commit 18+).
//
// Recognised forms:
//
//	/phase                      — alias of /phase show (most-recent active group)
//	/phase show                 — display the active group + each phase status
//	/phase show <group-id>      — explicit target
//	/phase next                 — advance ActiveIdx past the current phase
//	                              when the orchestrator paused at a phase boundary
//	                              (e.g. user invoked Ctrl-C between phases)
//	/phase rollback             — git reset --hard to the previous phase's
//	                              AppliedSHA on the active group; mark
//	                              current phase rolled_back; group → failed
//	/phase skip <phase-index>   — mark the named phase as skipped so the
//	                              scheduler steps over it on next dispatch
//
// Single-phase work is unaffected: when no group exists the
// /phase commands surface a "no active group" info message and
// return.
func (r *REPL) handlePhaseCmd(line string) {
	// /phase manipulates write-mode multi-phase state; refuse
	// when write is gated off by codrax.yaml — same posture as
	// /approve, /reject, /merge.
	if !r.writeEnabled {
		for _, line := range writeModeDisabled(r.language, "/phase", r.settingsPath) {
			r.warn("%s\n", line)
		}
		return
	}
	if r.planGroupStore == nil {
		r.warn("/phase disabled: no plan group store configured\n")
		return
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/phase"))
	if rest == "" {
		rest = "show"
	}
	// /phase show <group-id> — explicit target.
	if showID := strings.TrimSpace(strings.TrimPrefix(rest, "show ")); showID != rest && showID != "" {
		// Cross-command nudge: a "plan-" prefix is a ChangePlan
		// id, not a group id. Point the operator at /plan show
		// before the resolve fails with a generic "not found".
		if strings.HasPrefix(showID, "plan-") {
			r.info(fmt.Sprintf("/phase show: %q is a plan id; use `/plan show %s` instead\n", showID, showID))
			return
		}
		r.phaseShow(showID)
		return
	}
	// /phase skip <idx>
	if skipArg := strings.TrimSpace(strings.TrimPrefix(rest, "skip ")); skipArg != rest && skipArg != "" {
		r.phaseSkip(skipArg)
		return
	}
	switch rest {
	case "show":
		r.phaseShow("")
	case "next":
		r.phaseNext()
	case "rollback":
		r.phaseRollback()
	case "resume":
		r.phaseResume()
	default:
		r.warn("unknown /phase subcommand %q. expected: show / show <id> / next / rollback / resume / skip <idx>\n", rest)
	}
}

// phaseShow renders the named group (or the most recent active
// group when groupID is empty). Lists each phase with status,
// goal, plan-id cross-link, and acceptance verdict when present.
func (r *REPL) phaseShow(groupID string) {
	g, err := r.resolvePhaseGroup(groupID)
	if err != nil {
		r.errorf("/phase show: %v\n", err)
		return
	}
	if g == nil {
		r.info("/phase show: no active plan group\n")
		return
	}
	fmt.Fprintf(r.out, "  group: %s\n", g.ID)
	fmt.Fprintf(r.out, "  status: %s\n", g.Status)
	if g.Goal != "" {
		fmt.Fprintf(r.out, "  goal: %s\n", oneLine(g.Goal))
	}
	// Worktree path resolved from any phase plan with a
	// non-empty WorktreePath. Useful when the operator wants to
	// inspect the cumulative state on disk between phases or
	// before /phase rollback.
	if wt := r.findGroupWorktree(g); wt != "" {
		fmt.Fprintf(r.out, "  worktree: %s\n", wt)
	}
	// 1-indexed for the operator-facing "active phase" line.
	// Beyond-last (group complete) renders as "all done".
	if g.ActiveIdx >= len(g.Phases) {
		fmt.Fprintf(r.out, "  phases: %d total, all done\n\n", len(g.Phases))
	} else {
		fmt.Fprintf(r.out, "  phases: %d total, active phase %d\n\n",
			len(g.Phases), g.ActiveIdx+1)
	}
	for _, p := range g.Phases {
		marker := "  "
		if p.Index == g.ActiveIdx && !types.IsTerminalGroupStatus(g.Status) {
			marker = "→ "
		}
		fmt.Fprintf(r.out, "    %s[%d] %-13s %s\n",
			marker, p.Index+1, "("+string(p.Status)+")", oneLine(p.Goal))
		if p.PlanID != "" {
			fmt.Fprintf(r.out, "         plan: %s", p.PlanID)
			if p.AppliedSHA != "" {
				fmt.Fprintf(r.out, "  sha: %s", shortSHA(p.AppliedSHA))
			}
			fmt.Fprintln(r.out)
		}
		if p.AcceptanceCheck != nil {
			verdict := "passed"
			if !p.AcceptanceCheck.Passed {
				verdict = "REJECTED"
			}
			fmt.Fprintf(r.out, "         acceptance: %s — %s\n",
				verdict, oneLine(p.AcceptanceCheck.Reasoning))
			if p.AcceptanceCheck.NextHint != "" {
				fmt.Fprintf(r.out, "         next-hint: %s\n", oneLine(p.AcceptanceCheck.NextHint))
			}
		}
	}
}

// phaseNext is a manual advancement primitive: mark the active
// phase as Accepted and bump ActiveIdx. Useful when the
// orchestrator paused mid-group (Ctrl-C, /cancel between
// phases) and the operator wants to resume from the next phase
// without retroactively re-running the previous one. Refuses
// when the group is already terminal.
func (r *REPL) phaseNext() {
	g, err := r.resolvePhaseGroup("")
	if err != nil {
		r.errorf("/phase next: %v\n", err)
		return
	}
	if g == nil {
		r.info("/phase next: no active plan group\n")
		return
	}
	if types.IsTerminalGroupStatus(g.Status) {
		r.warn("/phase next: group %s is already terminal (%s); cannot advance\n", g.ID, g.Status)
		return
	}
	if g.ActiveIdx >= len(g.Phases) {
		r.info(fmt.Sprintf("/phase next: group %s already past last phase (%d/%d)\n",
			g.ID, g.ActiveIdx, len(g.Phases)))
		return
	}
	phase := &g.Phases[g.ActiveIdx]
	if types.IsTerminalPhaseStatus(phase.Status) {
		r.info(fmt.Sprintf("/phase next: phase %d already terminal (%s); advancing\n",
			phase.Index+1, phase.Status))
	} else {
		now := time.Now()
		phase.Status = types.PhaseAccepted
		phase.FinishedAt = &now
	}
	g.ActiveIdx++
	if g.ActiveIdx >= len(g.Phases) {
		g.Status = types.PlanGroupCompleted
	} else {
		g.Status = types.PlanGroupInFlight
	}
	if _, err := r.planGroupStore.Save(g); err != nil {
		r.errorf("/phase next: persist failed: %v\n", err)
		return
	}
	r.success(fmt.Sprintf("/phase next: advanced past phase %d; group status now %s\n",
		phase.Index+1, g.Status))
}

// phaseRollback walks the worktree back to the previous phase's
// AppliedSHA, then resets the active phase to Pending and the
// group to InFlight so a subsequent /mode apply re-enters that
// phase fresh. Resumability — not termination — is the contract
// the operator expects from /phase rollback; marking the phase
// or group terminal would deadlock the scheduler against its
// own non-terminal entry condition.
//
// Worktree resolution: read the active phase's PlanID, load
// that plan from PlanStore for WorktreePath. Single-phase
// design assumed one worktree per Run; multi-phase reuses the
// same worktree across phases, but the WorktreePath persists
// only on the LAST phase's plan when the orchestrator preserved
// the worktree on success. For mid-group rollback we walk the
// plans newest-first to find any plan with a non-empty
// WorktreePath.
func (r *REPL) phaseRollback() {
	g, err := r.resolvePhaseGroup("")
	if err != nil {
		r.errorf("/phase rollback: %v\n", err)
		return
	}
	if g == nil {
		r.info("/phase rollback: no active plan group\n")
		return
	}
	if types.IsTerminalGroupStatus(g.Status) {
		r.warn("/phase rollback: group %s already terminal (%s); nothing to roll back\n",
			g.ID, g.Status)
		return
	}
	if g.ActiveIdx >= len(g.Phases) {
		r.info(fmt.Sprintf("/phase rollback: group %s already past last phase (%d); nothing in flight\n",
			g.ID, g.ActiveIdx))
		return
	}

	// Find the worktree path. Walk active phase's plan first;
	// if missing, walk earlier phase plans.
	wtPath := r.findGroupWorktree(g)
	if wtPath == "" {
		r.warn("/phase rollback: no worktree path persisted on any phase plan; group state cannot be rewound. Use /reject if you want to discard this group.\n")
		return
	}

	// Determine target SHA. If the active phase is index 0,
	// there's no prior phase to rewind to — that's a "discard
	// everything" action; refuse and point at /reject.
	if g.ActiveIdx == 0 {
		r.warn("/phase rollback: cannot roll back phase 1 (no prior phase to rewind to); use /reject to discard the entire group\n")
		return
	}
	prev := g.Phases[g.ActiveIdx-1]
	if prev.AppliedSHA == "" {
		r.warn("/phase rollback: previous phase %d has no AppliedSHA; cannot rewind\n", prev.Index+1)
		return
	}

	// Issue the reset.
	if err := worktree.ResetHard(wtPath, prev.AppliedSHA); err != nil {
		r.errorf("/phase rollback: git reset --hard %s failed: %v\n", shortSHA(prev.AppliedSHA), err)
		return
	}

	// Reset the active phase + group so the scheduler can replay
	// this phase. PhaseRolledBack / PlanGroupFailed are terminal
	// statuses — using them here would lock the scheduler out of
	// its own non-terminal entry condition and the operator would
	// have to /reject the entire group to recover.
	phase := &g.Phases[g.ActiveIdx]
	phase.Status = types.PhasePending
	phase.StartedAt = nil
	phase.FinishedAt = nil
	phase.PlanID = ""
	phase.AppliedSHA = ""
	phase.AcceptanceCheck = nil
	g.Status = types.PlanGroupInFlight
	if _, err := r.planGroupStore.Save(g); err != nil {
		r.errorf("/phase rollback: persist failed: %v\n", err)
		return
	}
	r.success(fmt.Sprintf("/phase rollback: rewound to phase %d's SHA %s; phase %d reset to pending, group %s — replay with /mode apply\n",
		prev.Index+1, shortSHA(prev.AppliedSHA), phase.Index+1, g.Status))
}

// phaseResume is an info-only navigator: after `/phase rollback`
// reset the active phase to Pending the user needs to know the
// next step is `/mode apply <new request>` to drive a fresh Run
// against the same group. Surfacing this explicitly avoids the
// "I rolled back, now what?" pause.
//
// No state mutation — pure orientation. Refuses when no group
// is in flight or when the active phase is not Pending (e.g. a
// successful in_progress phase doesn't need resuming).
func (r *REPL) phaseResume() {
	g, err := r.resolvePhaseGroup("")
	if err != nil {
		r.errorf("/phase resume: %v\n", err)
		return
	}
	if g == nil {
		r.info("/phase resume: no active plan group\n")
		return
	}
	if types.IsTerminalGroupStatus(g.Status) {
		r.warn("/phase resume: group %s is %s; nothing to resume\n", g.ID, g.Status)
		return
	}
	if g.ActiveIdx >= len(g.Phases) {
		r.info(fmt.Sprintf("/phase resume: group %s already past last phase (%d/%d); use /merge to land it\n",
			g.ID, g.ActiveIdx, len(g.Phases)))
		return
	}
	phase := g.Phases[g.ActiveIdx]
	switch phase.Status {
	case types.PhasePending:
		r.info(fmt.Sprintf("/phase resume: group %s phase %d (%q) is pending. Type `/mode apply` then describe your goal again to drive plan→apply→verify on this phase.\n",
			g.ID, phase.Index+1, oneLine(phase.Goal)))
	case types.PhaseInProgress:
		r.info(fmt.Sprintf("/phase resume: group %s phase %d is already in_progress; another Run is driving it (or a previous Run was interrupted). Wait, or use /cancel + /phase rollback to reset.\n",
			g.ID, phase.Index+1))
	default:
		r.info(fmt.Sprintf("/phase resume: group %s phase %d status is %s; resume is only meaningful for pending phases. Use /phase next to advance, /phase rollback to reset, or /phase skip %d to step over.\n",
			g.ID, phase.Index+1, phase.Status, phase.Index+1))
	}
}

// phaseSkip marks a named phase as skipped so the scheduler
// steps over it without running it. Argument is the 1-indexed
// phase number from /phase show output. Refuses to skip
// already-terminal phases (operator clarity — silent re-skip
// would confuse).
func (r *REPL) phaseSkip(arg string) {
	g, err := r.resolvePhaseGroup("")
	if err != nil {
		r.errorf("/phase skip: %v\n", err)
		return
	}
	if g == nil {
		r.info("/phase skip: no active plan group\n")
		return
	}
	idx, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil {
		r.errorf("/phase skip: phase index %q not a number\n", arg)
		return
	}
	// Operators see 1-indexed phase numbers in /phase show; PhaseRecord.Index is 0-based.
	zeroIdx := idx - 1
	if zeroIdx < 0 || zeroIdx >= len(g.Phases) {
		r.errorf("/phase skip: phase %d out of range (group has %d phases)\n", idx, len(g.Phases))
		return
	}
	phase := &g.Phases[zeroIdx]
	if types.IsTerminalPhaseStatus(phase.Status) {
		r.warn("/phase skip: phase %d already terminal (%s); cannot skip\n", idx, phase.Status)
		return
	}
	now := time.Now()
	phase.Status = types.PhaseSkipped
	phase.FinishedAt = &now
	if _, err := r.planGroupStore.Save(g); err != nil {
		r.errorf("/phase skip: persist failed: %v\n", err)
		return
	}
	r.success(fmt.Sprintf("/phase skip: phase %d marked skipped; scheduler will step over on next dispatch\n", idx))
}

// resolvePhaseGroup loads the named group, or — when groupID is
// empty — finds the most recent non-terminal group in the store.
// Returns (nil, nil) when groupID is empty AND no active group
// exists.
func (r *REPL) resolvePhaseGroup(groupID string) (*types.PlanGroup, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID != "" {
		return r.planGroupStore.Load(groupID)
	}
	return r.planGroupStore.FindActiveGroup()
}

// phaseTotalForGroup returns len(group.Phases) when the named
// group exists in the store, or 0 when it doesn't (or the
// store isn't wired). Used by /plan show + /plan list to
// render "phase X of Y in group <id>" without forcing every
// caller to load and unmarshal the full PlanGroup. Failure
// modes degrade silently — render falls back to "phase X in
// group <id>" when total is unknown.
func (r *REPL) phaseTotalForGroup(groupID string) int {
	if r == nil || r.planGroupStore == nil || strings.TrimSpace(groupID) == "" {
		return 0
	}
	g, err := r.planGroupStore.Load(groupID)
	if err != nil || g == nil {
		return 0
	}
	return len(g.Phases)
}

// findGroupWorktree walks the group's phase plans newest-first,
// returning the first non-empty WorktreePath. The orchestrator
// only persists WorktreePath when keep_on_success is enabled OR
// the operator deliberately preserved it; mid-group rollback
// fires before the final preserve decision, so callers may need
// to walk all phases to find a usable path.
func (r *REPL) findGroupWorktree(g *types.PlanGroup) string {
	if r.planStore == nil || g == nil {
		return ""
	}
	for i := len(g.Phases) - 1; i >= 0; i-- {
		p := g.Phases[i]
		if p.PlanID == "" {
			continue
		}
		full, err := r.planStore.Load(p.PlanID)
		if err != nil || full == nil {
			continue
		}
		if strings.TrimSpace(full.WorktreePath) != "" {
			return full.WorktreePath
		}
	}
	return ""
}

// shortSHA returns the first 8 chars of a git SHA for compact
// rendering. Leaves shorter inputs unchanged.
func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
